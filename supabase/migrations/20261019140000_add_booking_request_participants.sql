-- Persist multiple participant names per booking request.
--
-- The public request form collects one name per participant. Previously the
-- form only joined those names into booking_requests.full_name, so the club
-- could not reliably tell participants apart (for example, commas inside a
-- name) and no per-participant row existed. This migration stores each
-- participant as its own normalized row.
--
-- Design notes:
--   * booking_request_participants is the request-time source of truth.
--   * booking_requests.full_name is kept as a denormalized display/report
--     fallback (the comma-joined names) so existing records and reports keep
--     working. Existing single-name requests are backfilled as one row.
--   * booking_participants records the participants against each confirmed
--     booking created at approval time, without changing how many players or
--     bookings a request produces (capacity and session rules are unchanged).
--   * Contacts stay on booking_requests/players; participants never carry
--     contact details.

-- 1. Normalized participants for a pending/decided request.
CREATE TABLE public.booking_request_participants (
    request_id uuid NOT NULL REFERENCES public.booking_requests(id) ON DELETE CASCADE,
    position smallint NOT NULL CHECK (position > 0),
    full_name text NOT NULL CHECK (btrim(full_name) <> '' AND char_length(btrim(full_name)) <= 200),
    PRIMARY KEY (request_id, position)
);

-- Preserve every pre-existing request as a single participant.
INSERT INTO public.booking_request_participants(request_id, position, full_name)
SELECT br.id, 1, pg_catalog.btrim(br.full_name)
  FROM public.booking_requests AS br
ON CONFLICT DO NOTHING;

-- 2. Participants attached to each confirmed booking created from a request.
CREATE TABLE public.booking_participants (
    booking_id uuid NOT NULL REFERENCES public.bookings(id) ON DELETE CASCADE,
    position smallint NOT NULL CHECK (position > 0),
    full_name text NOT NULL CHECK (btrim(full_name) <> '' AND char_length(btrim(full_name)) <= 200),
    PRIMARY KEY (booking_id, position)
);

-- Backfill confirmed bookings that already came from an approved request. Both
-- the legacy single-slot reference and the current per-slot reference are
-- handled.
INSERT INTO public.booking_participants(booking_id, position, full_name)
SELECT b.id, rp.position, rp.full_name
  FROM public.bookings AS b
  JOIN public.booking_request_participants AS rp
    ON b.external_booking_reference = 'request:' || rp.request_id::text
    OR b.external_booking_reference = 'request:' || rp.request_id::text || ':' || b.session_id::text
ON CONFLICT DO NOTHING;

ALTER TABLE public.booking_request_participants ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE public.booking_request_participants
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_api, mmsu_booking_admin_api;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.booking_request_participants
    TO mmsu_booking_function_owner;
CREATE POLICY booking_request_participants_function_owner_all
    ON public.booking_request_participants
    FOR ALL TO mmsu_booking_function_owner USING (true) WITH CHECK (true);

ALTER TABLE public.booking_participants ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE public.booking_participants
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_api, mmsu_booking_admin_api;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.booking_participants
    TO mmsu_booking_function_owner;
CREATE POLICY booking_participants_function_owner_all
    ON public.booking_participants
    FOR ALL TO mmsu_booking_function_owner USING (true) WITH CHECK (true);

-- 3. Public request entry point that stores normalized participants. Contact
--    fields remain independently optional and every existing slot, duplicate,
--    adjacency, and capacity rule is preserved. A name-based duplicate guard is
--    added because duplicate protection previously relied on optional contacts.
GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET TRUE;
GRANT CREATE ON SCHEMA public TO mmsu_booking_function_owner;
SET ROLE mmsu_booking_function_owner;

CREATE OR REPLACE FUNCTION public.create_public_booking_request_with_participants_token(
    p_session_ids jsonb,
    p_participants jsonb,
    p_email text,
    p_phone text,
    p_tracking_token_hash bytea
)
RETURNS text
LANGUAGE plpgsql SECURITY DEFINER SET search_path = ''
AS $function$
DECLARE
    v_session_ids uuid[];
    v_session_count integer;
    v_found_count integer;
    v_participants text[];
    v_participant_count integer;
    v_first_session_id uuid;
    v_request_id uuid;
    v_full_name text;
    v_email text := NULLIF(pg_catalog.lower(pg_catalog.btrim(p_email)), '');
    v_phone text := NULLIF(pg_catalog.btrim(p_phone), '');
    v_public_reference text;
    v_session_id uuid;
BEGIN
    IF pg_catalog.jsonb_typeof(p_session_ids) IS DISTINCT FROM 'array'
       OR pg_catalog.jsonb_typeof(p_participants) IS DISTINCT FROM 'array'
       OR p_tracking_token_hash IS NULL OR pg_catalog.octet_length(p_tracking_token_hash) <> 32
       OR (v_email IS NOT NULL AND (pg_catalog.char_length(v_email) > 254 OR pg_catalog.strpos(v_email, '@') < 2))
       OR (v_phone IS NOT NULL AND pg_catalog.char_length(v_phone) > 40) THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid booking request';
    END IF;

    BEGIN
        SELECT pg_catalog.array_agg(pg_catalog.btrim(item.value) ORDER BY item.ordinality)
          INTO v_participants
          FROM pg_catalog.jsonb_array_elements_text(p_participants) WITH ORDINALITY AS item(value, ordinality);
    EXCEPTION WHEN OTHERS THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid participant list';
    END;
    v_participant_count := COALESCE(pg_catalog.cardinality(v_participants), 0);
    IF v_participant_count < 1 OR v_participant_count > 50 THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Provide 1 to 50 participant names';
    END IF;
    IF EXISTS (
        SELECT 1 FROM pg_catalog.unnest(v_participants) AS names(value)
         WHERE names.value = '' OR pg_catalog.char_length(names.value) > 200
    ) THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid participant name';
    END IF;
    v_full_name := pg_catalog.array_to_string(v_participants, ', ');
    IF pg_catalog.char_length(v_full_name) > 200 THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Participant names are too long';
    END IF;

    BEGIN
        SELECT pg_catalog.array_agg(item.value::uuid ORDER BY item.ordinality)
          INTO v_session_ids
          FROM pg_catalog.jsonb_array_elements_text(p_session_ids) WITH ORDINALITY AS item(value, ordinality);
    EXCEPTION WHEN OTHERS THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid session ID list';
    END;
    v_session_count := COALESCE(pg_catalog.cardinality(v_session_ids), 0);
    IF v_session_count < 1 OR v_session_count > 8 THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Select 1 to 8 session slots';
    END IF;
    IF (SELECT pg_catalog.count(DISTINCT id) FROM pg_catalog.unnest(v_session_ids) AS ids(id)) <> v_session_count THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Duplicate session IDs';
    END IF;

    -- Lock requested session rows in a stable order and serialize identity/slot
    -- duplicate checks across concurrent submissions.
    PERFORM s.id FROM public.training_sessions AS s
     WHERE s.id = ANY(v_session_ids) ORDER BY s.id FOR SHARE OF s;
    SELECT pg_catalog.count(*) INTO v_found_count
      FROM public.training_sessions AS s WHERE s.id = ANY(v_session_ids);
    IF v_found_count <> v_session_count THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable';
    END IF;

    IF EXISTS (
        SELECT 1 FROM public.training_sessions AS s
        JOIN public.training_programs AS p ON p.id = s.program_id
        WHERE s.id = ANY(v_session_ids)
          AND (s.status <> 'open' OR s.is_public IS NOT TRUE
               OR s.starts_at <= pg_catalog.clock_timestamp() OR p.is_active IS NOT TRUE)
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable';
    END IF;
    IF (SELECT pg_catalog.count(DISTINCT s.program_id) FROM public.training_sessions AS s WHERE s.id = ANY(v_session_ids)) <> 1
       OR (SELECT pg_catalog.count(DISTINCT pg_catalog.lower(pg_catalog.btrim(s.location))) FROM public.training_sessions AS s WHERE s.id = ANY(v_session_ids)) <> 1
       OR (SELECT pg_catalog.count(DISTINCT (s.starts_at AT TIME ZONE 'Asia/Manila')::date) FROM public.training_sessions AS s WHERE s.id = ANY(v_session_ids)) <> 1 THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ010', MESSAGE = 'Slots must share a program, location, and local date';
    END IF;
    IF EXISTS (
        SELECT 1 FROM (
            SELECT s.ends_at,
                   pg_catalog.lead(s.starts_at) OVER (ORDER BY s.starts_at, s.id) AS next_start
              FROM public.training_sessions AS s WHERE s.id = ANY(v_session_ids)
        ) AS ordered_slots
        WHERE ordered_slots.next_start IS NOT NULL AND ordered_slots.ends_at <> ordered_slots.next_start
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ010', MESSAGE = 'Slots must be adjacent';
    END IF;

    FOR v_session_id IN
        SELECT id FROM pg_catalog.unnest(v_session_ids) AS ids(id) ORDER BY id
    LOOP
        IF v_email IS NOT NULL THEN
            PERFORM pg_catalog.pg_advisory_xact_lock(pg_catalog.hashtextextended(
                'booking-request-email:' || v_session_id::text || ':' || v_email, 0));
        END IF;
        IF v_phone IS NOT NULL THEN
            PERFORM pg_catalog.pg_advisory_xact_lock(pg_catalog.hashtextextended(
                'booking-request-phone:' || v_session_id::text || ':' || v_phone, 0));
        END IF;
        -- A request may have no contact details, so the participant name set is
        -- also used to serialize identical submissions.
        PERFORM pg_catalog.pg_advisory_xact_lock(pg_catalog.hashtextextended(
            'booking-request-name:' || v_session_id::text || ':' || pg_catalog.lower(v_full_name), 0));
    END LOOP;

    IF v_email IS NOT NULL AND EXISTS (
        SELECT 1 FROM public.booking_request_sessions AS requested
        JOIN public.booking_requests AS br ON br.id = requested.request_id
        WHERE requested.session_id = ANY(v_session_ids) AND br.status = 'pending'
          AND pg_catalog.lower(br.email) = v_email
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ004', MESSAGE = 'Duplicate pending request';
    END IF;
    IF v_phone IS NOT NULL AND EXISTS (
        SELECT 1 FROM public.booking_request_sessions AS requested
        JOIN public.booking_requests AS br ON br.id = requested.request_id
        WHERE requested.session_id = ANY(v_session_ids) AND br.status = 'pending'
          AND br.phone = v_phone
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ004', MESSAGE = 'Duplicate pending request';
    END IF;
    IF EXISTS (
        SELECT 1 FROM public.booking_request_sessions AS requested
        JOIN public.booking_requests AS br ON br.id = requested.request_id
        WHERE requested.session_id = ANY(v_session_ids) AND br.status = 'pending'
          AND pg_catalog.lower(br.full_name) = pg_catalog.lower(v_full_name)
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ004', MESSAGE = 'Duplicate pending request';
    END IF;
    IF EXISTS (
        SELECT 1 FROM public.training_sessions AS s
        WHERE s.id = ANY(v_session_ids) AND s.capacity IS NOT NULL
          AND (SELECT pg_catalog.count(*) FROM public.bookings AS b
               WHERE b.session_id = s.id AND b.status = 'confirmed') >= s.capacity
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ002', MESSAGE = 'Training session is full';
    END IF;

    SELECT s.id INTO v_first_session_id
      FROM public.training_sessions AS s WHERE s.id = ANY(v_session_ids)
     ORDER BY s.starts_at, s.id LIMIT 1;
    v_public_reference := 'MMSU-' || pg_catalog.upper(
        pg_catalog.substr(pg_catalog.replace(gen_random_uuid()::text, '-', ''), 1, 16));
    INSERT INTO public.booking_requests(
        session_id, full_name, email, phone, public_reference,
        tracking_token_hash, tracking_token_expires_at
    ) VALUES (
        v_first_session_id, v_full_name, v_email, v_phone, v_public_reference,
        p_tracking_token_hash, pg_catalog.clock_timestamp() + interval '30 days'
    ) RETURNING id INTO v_request_id;

    INSERT INTO public.booking_request_participants(request_id, position, full_name)
    SELECT v_request_id, names.ordinality::smallint, names.value
      FROM pg_catalog.unnest(v_participants) WITH ORDINALITY AS names(value, ordinality);

    INSERT INTO public.booking_request_sessions(request_id, session_id, position)
    SELECT v_request_id, s.id,
           pg_catalog.row_number() OVER (ORDER BY s.starts_at, s.id)::smallint
      FROM public.training_sessions AS s WHERE s.id = ANY(v_session_ids);
    RETURN v_public_reference;
END;
$function$;

-- Keep the previous JSON/name signatures working during a rolling deploy. A
-- single name becomes a one-participant request.
CREATE OR REPLACE FUNCTION public.create_public_booking_request_with_slots_token(
    p_session_ids jsonb,
    p_full_name text,
    p_email text,
    p_phone text,
    p_tracking_token_hash bytea
)
RETURNS text
LANGUAGE sql SECURITY DEFINER SET search_path = ''
AS $function$
    SELECT public.create_public_booking_request_with_participants_token(
        p_session_ids,
        pg_catalog.jsonb_build_array(COALESCE(p_full_name, '')),
        p_email, p_phone, p_tracking_token_hash)
$function$;

-- 4. Approval copies the request participants onto each created booking and
--    otherwise keeps every existing lock, capacity, and consistency rule.
CREATE OR REPLACE FUNCTION public.approve_booking_request(
    p_request_id uuid, p_admin_id uuid, p_player_id uuid, p_create_new boolean
)
RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = ''
AS $function$
DECLARE
    v_request public.booking_requests;
    v_player_id uuid;
    v_booking_id uuid;
    v_first_booking_id uuid;
    v_session_id uuid;
    v_session_ids uuid[];
    v_session_count integer;
    v_confirmed_count bigint;
    v_capacity integer;
    v_program_count integer;
    v_location_count integer;
    v_date_count integer;
BEGIN
    SELECT br.* INTO v_request FROM public.booking_requests AS br
     WHERE br.id = p_request_id FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ006', MESSAGE = 'Booking request not found';
    END IF;
    IF v_request.status <> 'pending' THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ003', MESSAGE = 'Booking request already decided';
    END IF;
    IF p_admin_id IS NULL OR ((p_player_id IS NULL) = (p_create_new IS FALSE)) THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Choose an existing player or create a new one';
    END IF;

    SELECT pg_catalog.array_agg(requested.session_id ORDER BY requested.session_id)
      INTO v_session_ids
      FROM public.booking_request_sessions AS requested
     WHERE requested.request_id = p_request_id;
    v_session_count := COALESCE(pg_catalog.cardinality(v_session_ids), 0);
    IF v_session_count < 1 OR v_session_count > 8 THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Requested sessions are unavailable';
    END IF;
    PERFORM s.id FROM public.training_sessions AS s
     WHERE s.id = ANY(v_session_ids) ORDER BY s.id FOR UPDATE OF s;
    IF (SELECT pg_catalog.count(*) FROM public.training_sessions AS s WHERE s.id = ANY(v_session_ids)) <> v_session_count THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable';
    END IF;
    IF EXISTS (
        SELECT 1 FROM public.training_sessions AS s
        JOIN public.training_programs AS p ON p.id = s.program_id
        WHERE s.id = ANY(v_session_ids)
          AND (s.status <> 'open' OR s.is_public IS NOT TRUE
               OR s.starts_at <= pg_catalog.clock_timestamp() OR p.is_active IS NOT TRUE)
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable';
    END IF;
    SELECT pg_catalog.count(DISTINCT s.program_id),
           pg_catalog.count(DISTINCT pg_catalog.lower(pg_catalog.btrim(s.location))),
           pg_catalog.count(DISTINCT (s.starts_at AT TIME ZONE 'Asia/Manila')::date)
      INTO v_program_count, v_location_count, v_date_count
      FROM public.training_sessions AS s WHERE s.id = ANY(v_session_ids);
    IF v_program_count <> 1 OR v_location_count <> 1 OR v_date_count <> 1 OR EXISTS (
        SELECT 1 FROM (
            SELECT s.ends_at,
                   pg_catalog.lead(s.starts_at) OVER (ORDER BY s.starts_at, s.id) AS next_start
              FROM public.training_sessions AS s WHERE s.id = ANY(v_session_ids)
        ) AS block_slots
        WHERE block_slots.next_start IS NOT NULL AND block_slots.ends_at <> block_slots.next_start
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ010', MESSAGE = 'Requested slots are no longer adjacent';
    END IF;

    IF p_create_new THEN
        INSERT INTO public.players(full_name, email, phone)
        VALUES (v_request.full_name, v_request.email, v_request.phone)
        RETURNING id INTO v_player_id;
    ELSE
        SELECT pl.id INTO v_player_id FROM public.players AS pl WHERE pl.id = p_player_id;
        IF NOT FOUND THEN
            RAISE EXCEPTION USING ERRCODE = 'PZ005', MESSAGE = 'Selected player unavailable';
        END IF;
    END IF;
    IF EXISTS (
        SELECT 1 FROM public.bookings AS b
        WHERE b.player_id = v_player_id AND b.session_id = ANY(v_session_ids) AND b.status = 'confirmed'
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ004', MESSAGE = 'Player is already booked for a requested slot';
    END IF;

    -- Rows are locked in stable order above; every capacity check, booking
    -- insert, and participant copy happens before the request is marked
    -- approved, in the same transaction.
    FOR v_session_id IN
        SELECT s.id FROM public.training_sessions AS s
         WHERE s.id = ANY(v_session_ids) ORDER BY s.starts_at, s.id
    LOOP
        SELECT s.capacity INTO v_capacity
          FROM public.training_sessions AS s WHERE s.id = v_session_id;
        IF v_capacity IS NOT NULL THEN
            SELECT pg_catalog.count(*) INTO v_confirmed_count
              FROM public.bookings AS b WHERE b.session_id = v_session_id AND b.status = 'confirmed';
            IF v_confirmed_count >= v_capacity THEN
                RAISE EXCEPTION USING ERRCODE = 'PZ002', MESSAGE = 'Training session is full';
            END IF;
        END IF;
        INSERT INTO public.bookings(player_id, session_id, status, external_booking_reference)
        VALUES (v_player_id, v_session_id, 'confirmed', 'request:' || p_request_id::text || ':' || v_session_id::text)
        RETURNING id INTO v_booking_id;
        IF v_first_booking_id IS NULL THEN v_first_booking_id := v_booking_id; END IF;

        INSERT INTO public.booking_participants(booking_id, position, full_name)
        SELECT v_booking_id, rp.position, rp.full_name
          FROM public.booking_request_participants AS rp
         WHERE rp.request_id = p_request_id;
        IF NOT FOUND THEN
            INSERT INTO public.booking_participants(booking_id, position, full_name)
            VALUES (v_booking_id, 1, v_request.full_name);
        END IF;
    END LOOP;

    UPDATE public.booking_requests
       SET status = 'approved', resolved_player_id = v_player_id,
           decided_by = p_admin_id, decided_at = pg_catalog.clock_timestamp()
     WHERE id = p_request_id;
    RETURN v_first_booking_id;
END;
$function$;

REVOKE ALL ON FUNCTION public.create_public_booking_request_with_participants_token(jsonb, jsonb, text, text, bytea)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.create_public_booking_request_with_participants_token(jsonb, jsonb, text, text, bytea)
    TO mmsu_booking_api;
REVOKE ALL ON FUNCTION public.create_public_booking_request_with_slots_token(jsonb, text, text, text, bytea)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.create_public_booking_request_with_slots_token(jsonb, text, text, text, bytea)
    TO mmsu_booking_api;
REVOKE ALL ON FUNCTION public.approve_booking_request(uuid, uuid, uuid, boolean)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.approve_booking_request(uuid, uuid, uuid, boolean)
    TO mmsu_booking_admin_api;

ALTER FUNCTION public.create_public_booking_request_with_participants_token(jsonb, jsonb, text, text, bytea)
    OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.create_public_booking_request_with_slots_token(jsonb, text, text, text, bytea)
    OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.approve_booking_request(uuid, uuid, uuid, boolean)
    OWNER TO mmsu_booking_function_owner;
RESET ROLE;
REVOKE CREATE ON SCHEMA public FROM mmsu_booking_function_owner;
GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET FALSE;
