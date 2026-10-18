-- Multi-slot scheduling, guarded session editing, and atomic multi-slot requests.
-- A request remains one reviewable record and may link to several adjacent slots.

CREATE TABLE public.booking_request_sessions (
    request_id uuid NOT NULL REFERENCES public.booking_requests(id) ON DELETE RESTRICT,
    session_id uuid NOT NULL REFERENCES public.training_sessions(id) ON DELETE RESTRICT,
    position smallint NOT NULL CHECK (position > 0),
    PRIMARY KEY (request_id, session_id),
    UNIQUE (request_id, position)
);
CREATE INDEX booking_request_sessions_session_idx
    ON public.booking_request_sessions(session_id, request_id);

ALTER TABLE public.booking_request_sessions ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE public.booking_request_sessions
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_api, mmsu_booking_admin_api;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.booking_request_sessions
    TO mmsu_booking_function_owner;
CREATE POLICY booking_request_sessions_function_owner_all
    ON public.booking_request_sessions
    FOR ALL TO mmsu_booking_function_owner USING (true) WITH CHECK (true);
GRANT SELECT ON TABLE public.booking_request_sessions TO mmsu_schedule_admin_function_owner;
CREATE POLICY booking_request_sessions_schedule_admin_select
    ON public.booking_request_sessions
    FOR SELECT TO mmsu_schedule_admin_function_owner USING (true);
GRANT SELECT (id, status) ON TABLE public.booking_requests TO mmsu_schedule_admin_function_owner;
CREATE POLICY booking_requests_schedule_admin_status_select
    ON public.booking_requests
    FOR SELECT TO mmsu_schedule_admin_function_owner USING (true);

-- Preserve all pre-existing requests as one-slot requests.
INSERT INTO public.booking_request_sessions(request_id, session_id, position)
SELECT br.id, br.session_id, 1
  FROM public.booking_requests AS br
ON CONFLICT DO NOTHING;

-- Session writes, batch writes, and coach assignment share one transaction lock.
-- This serializes conflict checks so two admins cannot create overlapping slots
-- at the same venue/coach concurrently.
GRANT UPDATE (program_id) ON TABLE public.training_sessions
    TO mmsu_schedule_admin_function_owner;
GRANT mmsu_schedule_admin_function_owner TO CURRENT_USER
    WITH ADMIN FALSE, INHERIT FALSE, SET TRUE;
GRANT CREATE ON SCHEMA public TO mmsu_schedule_admin_function_owner;
SET ROLE mmsu_schedule_admin_function_owner;

CREATE OR REPLACE FUNCTION public.admin_create_training_session(
    p_program_id uuid,
    p_starts_at timestamptz,
    p_ends_at timestamptz,
    p_location text,
    p_capacity integer,
    p_notes text,
    p_is_public boolean
)
RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = ''
AS $function$
DECLARE
    v_session_id uuid;
    v_location text := pg_catalog.lower(pg_catalog.btrim(p_location));
BEGIN
    PERFORM pg_catalog.pg_advisory_xact_lock(
        pg_catalog.hashtextextended('mmsu-training-schedule-write', 0)
    );
    IF p_program_id IS NULL OR p_starts_at IS NULL OR p_ends_at IS NULL
       OR p_starts_at >= p_ends_at OR p_location IS NULL
       OR pg_catalog.btrim(p_location) = ''
       OR pg_catalog.char_length(pg_catalog.btrim(p_location)) > 200
       OR (p_capacity IS NOT NULL AND p_capacity <= 0)
       OR pg_catalog.char_length(COALESCE(p_notes, '')) > 2000 THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid session parameters';
    END IF;
    PERFORM 1 FROM public.training_programs AS p
     WHERE p.id = p_program_id AND p.is_active IS TRUE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training program unavailable';
    END IF;
    IF EXISTS (
        SELECT 1 FROM public.training_sessions AS s
         WHERE s.status <> 'cancelled'
           AND pg_catalog.lower(pg_catalog.btrim(s.location)) = v_location
           AND s.starts_at < p_ends_at AND s.ends_at > p_starts_at
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ007', MESSAGE = 'Training location conflict';
    END IF;
    INSERT INTO public.training_sessions(
        program_id, starts_at, ends_at, location, capacity, notes, is_public, status
    ) VALUES (
        p_program_id, p_starts_at, p_ends_at, pg_catalog.btrim(p_location),
        p_capacity, NULLIF(pg_catalog.btrim(p_notes), ''), COALESCE(p_is_public, true), 'open'
    ) RETURNING id INTO v_session_id;
    RETURN v_session_id;
END;
$function$;

CREATE OR REPLACE FUNCTION public.admin_create_training_sessions_batch(p_sessions jsonb)
RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path = ''
AS $function$
DECLARE
    v_item jsonb;
    v_program_id uuid;
    v_starts_at timestamptz;
    v_ends_at timestamptz;
    v_location text;
    v_capacity integer;
    v_notes text;
    v_is_public boolean;
    v_coach_id uuid;
    v_coach_name text;
    v_session_id uuid;
    v_created_ids jsonb := '[]'::jsonb;
    v_count integer;
BEGIN
    IF pg_catalog.jsonb_typeof(p_sessions) IS DISTINCT FROM 'array' THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Sessions must be an array';
    END IF;
    SELECT pg_catalog.count(*) INTO v_count FROM pg_catalog.jsonb_array_elements(p_sessions);
    IF v_count < 1 OR v_count > 500 THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Batch size is invalid';
    END IF;

    PERFORM pg_catalog.pg_advisory_xact_lock(
        pg_catalog.hashtextextended('mmsu-training-schedule-write', 0)
    );

    FOR v_item IN
        SELECT value FROM pg_catalog.jsonb_array_elements(p_sessions) WITH ORDINALITY AS entries(value, ordinal)
        ORDER BY ordinal
    LOOP
        BEGIN
            v_program_id := NULLIF(v_item->>'program_id', '')::uuid;
            v_starts_at := NULLIF(v_item->>'starts_at', '')::timestamptz;
            v_ends_at := NULLIF(v_item->>'ends_at', '')::timestamptz;
            v_location := pg_catalog.btrim(COALESCE(v_item->>'location', ''));
            v_capacity := NULLIF(v_item->>'capacity', '')::integer;
            v_notes := pg_catalog.btrim(COALESCE(v_item->>'notes', ''));
            v_is_public := COALESCE((v_item->>'is_public')::boolean, false);
            v_coach_id := NULLIF(v_item->>'coach_id', '')::uuid;
        EXCEPTION WHEN OTHERS THEN
            RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'A session in the batch has invalid fields';
        END;

        IF v_program_id IS NULL OR v_starts_at IS NULL OR v_ends_at IS NULL
           OR v_starts_at >= v_ends_at OR v_location = ''
           OR pg_catalog.char_length(v_location) > 200
           OR pg_catalog.char_length(v_notes) > 2000
           OR (v_capacity IS NOT NULL AND v_capacity <= 0) THEN
            RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'A session in the batch is invalid';
        END IF;

        PERFORM 1 FROM public.training_programs AS p
         WHERE p.id = v_program_id AND p.is_active IS TRUE;
        IF NOT FOUND THEN
            RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training program unavailable';
        END IF;

        IF EXISTS (
            SELECT 1 FROM public.training_sessions AS s
             WHERE s.status <> 'cancelled'
               AND pg_catalog.lower(pg_catalog.btrim(s.location)) = pg_catalog.lower(v_location)
               AND s.starts_at < v_ends_at AND s.ends_at > v_starts_at
        ) THEN
            RAISE EXCEPTION USING ERRCODE = 'PZ007', MESSAGE = 'Training location conflict';
        END IF;

        v_coach_name := NULL;
        IF v_coach_id IS NOT NULL THEN
            SELECT ca.coach_name INTO v_coach_name
              FROM public.coach_availability AS ca
             WHERE ca.coach_id = v_coach_id
             ORDER BY ca.created_at DESC LIMIT 1;
            IF NOT FOUND THEN
                RAISE EXCEPTION USING ERRCODE = 'PZ004', MESSAGE = 'Coach has not submitted availability';
            END IF;
            IF EXISTS (
                SELECT 1
                  FROM public.coach_session_assignments AS a
                  JOIN public.training_sessions AS s ON s.id = a.session_id
                 WHERE a.coach_id = v_coach_id AND s.status <> 'cancelled'
                   AND s.starts_at < v_ends_at AND s.ends_at > v_starts_at
            ) THEN
                RAISE EXCEPTION USING ERRCODE = 'PZ007', MESSAGE = 'Coach schedule conflict';
            END IF;
        END IF;

        INSERT INTO public.training_sessions(
            program_id, starts_at, ends_at, location, capacity, notes, is_public, status
        ) VALUES (
            v_program_id, v_starts_at, v_ends_at, v_location, v_capacity,
            NULLIF(v_notes, ''), v_is_public, 'open'
        ) RETURNING id INTO v_session_id;

        IF v_coach_id IS NOT NULL THEN
            INSERT INTO public.coach_session_assignments(session_id, coach_id, coach_name)
            VALUES (v_session_id, v_coach_id, v_coach_name);
        END IF;
        v_created_ids := v_created_ids || pg_catalog.jsonb_build_array(v_session_id::text);
    END LOOP;

    RETURN v_created_ids;
END;
$function$;

CREATE OR REPLACE FUNCTION public.admin_update_training_session(
    p_session_id uuid,
    p_status text,
    p_notes text,
    p_is_public boolean
)
RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = ''
AS $function$
DECLARE
    v_old_status text;
    v_starts_at timestamptz;
    v_ends_at timestamptz;
    v_location text;
    v_coach_id uuid;
BEGIN
    PERFORM pg_catalog.pg_advisory_xact_lock(
        pg_catalog.hashtextextended('mmsu-training-schedule-write', 0)
    );
    IF p_session_id IS NULL
       OR (p_status IS NOT NULL AND p_status NOT IN ('open', 'closed', 'cancelled'))
       OR (p_status IS NULL AND p_notes IS NULL AND p_is_public IS NULL) THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid session update parameters';
    END IF;

    SELECT s.status, s.starts_at, s.ends_at, s.location
      INTO v_old_status, v_starts_at, v_ends_at, v_location
      FROM public.training_sessions AS s
     WHERE s.id = p_session_id
     FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'P0002', MESSAGE = 'Training session not found';
    END IF;

    -- Cancelled sessions stop blocking the schedule. If an admin reopens one
    -- through the legacy status endpoint, validate venue/coach conflicts too.
    IF v_old_status = 'cancelled' AND p_status = 'open' THEN
        IF EXISTS (
            SELECT 1 FROM public.training_sessions AS s
             WHERE s.id <> p_session_id AND s.status <> 'cancelled'
               AND pg_catalog.lower(pg_catalog.btrim(s.location)) = pg_catalog.lower(pg_catalog.btrim(v_location))
               AND s.starts_at < v_ends_at AND s.ends_at > v_starts_at
        ) THEN
            RAISE EXCEPTION USING ERRCODE = 'PZ007', MESSAGE = 'Training location conflict';
        END IF;
        SELECT a.coach_id INTO v_coach_id
          FROM public.coach_session_assignments AS a WHERE a.session_id = p_session_id;
        IF v_coach_id IS NOT NULL AND EXISTS (
            SELECT 1 FROM public.coach_session_assignments AS a
            JOIN public.training_sessions AS s ON s.id = a.session_id
            WHERE a.coach_id = v_coach_id AND s.id <> p_session_id
              AND s.status <> 'cancelled' AND s.starts_at < v_ends_at AND s.ends_at > v_starts_at
        ) THEN
            RAISE EXCEPTION USING ERRCODE = 'PZ007', MESSAGE = 'Coach schedule conflict';
        END IF;
    END IF;

    UPDATE public.training_sessions
       SET status = COALESCE(p_status, status),
           notes = CASE WHEN p_notes IS NOT NULL THEN NULLIF(pg_catalog.btrim(p_notes), '') ELSE notes END,
           is_public = COALESCE(p_is_public, is_public),
           updated_at = pg_catalog.clock_timestamp()
     WHERE id = p_session_id;
    RETURN true;
END;
$function$;

CREATE OR REPLACE FUNCTION public.admin_edit_training_session(
    p_session_id uuid,
    p_program_id uuid,
    p_starts_at timestamptz,
    p_ends_at timestamptz,
    p_location text,
    p_capacity integer,
    p_notes text,
    p_is_public boolean,
    p_acknowledge_booked_changes boolean
)
RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = ''
AS $function$
DECLARE
    v_old_program_id uuid;
    v_old_starts_at timestamptz;
    v_old_ends_at timestamptz;
    v_old_location text;
    v_confirmed_count bigint;
    v_schedule_changed boolean;
    v_time_changed boolean;
    v_location_changed boolean;
    v_coach_id uuid;
    v_location text;
BEGIN
    PERFORM pg_catalog.pg_advisory_xact_lock(
        pg_catalog.hashtextextended('mmsu-training-schedule-write', 0)
    );
    IF p_session_id IS NULL OR p_program_id IS NULL OR p_starts_at IS NULL
       OR p_ends_at IS NULL OR p_starts_at >= p_ends_at OR p_location IS NULL
       OR pg_catalog.btrim(p_location) = '' OR pg_catalog.char_length(pg_catalog.btrim(p_location)) > 200
       OR pg_catalog.char_length(COALESCE(p_notes, '')) > 2000
       OR (p_capacity IS NOT NULL AND p_capacity <= 0) THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid session edit parameters';
    END IF;

    SELECT s.program_id, s.starts_at, s.ends_at, s.location
      INTO v_old_program_id, v_old_starts_at, v_old_ends_at, v_old_location
      FROM public.training_sessions AS s WHERE s.id = p_session_id FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'P0002', MESSAGE = 'Training session not found';
    END IF;
    PERFORM 1 FROM public.training_programs AS p
     WHERE p.id = p_program_id AND p.is_active IS TRUE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training program unavailable';
    END IF;

    SELECT pg_catalog.count(*) INTO v_confirmed_count
      FROM public.bookings AS b WHERE b.session_id = p_session_id AND b.status = 'confirmed';
    IF p_capacity IS NOT NULL AND p_capacity < v_confirmed_count THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ009', MESSAGE = 'Capacity below confirmed bookings';
    END IF;

    v_time_changed := v_old_starts_at IS DISTINCT FROM p_starts_at
        OR v_old_ends_at IS DISTINCT FROM p_ends_at;
    v_location_changed := pg_catalog.lower(pg_catalog.btrim(v_old_location))
        IS DISTINCT FROM pg_catalog.lower(pg_catalog.btrim(p_location));
    v_schedule_changed := v_old_program_id IS DISTINCT FROM p_program_id
        OR v_time_changed OR v_location_changed;
    IF v_schedule_changed AND EXISTS (
        SELECT 1
          FROM public.booking_request_sessions AS requested
          JOIN public.booking_requests AS br ON br.id = requested.request_id
         WHERE requested.session_id = p_session_id AND br.status = 'pending'
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ011', MESSAGE = 'Resolve pending booking requests before rescheduling';
    END IF;
    IF v_confirmed_count > 0 AND v_schedule_changed
       AND COALESCE(p_acknowledge_booked_changes, false) IS NOT TRUE THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ008', MESSAGE = 'Explicit acknowledgement required for booked session changes';
    END IF;

    v_location := pg_catalog.lower(pg_catalog.btrim(p_location));
    IF (v_time_changed OR v_location_changed) AND EXISTS (
        SELECT 1 FROM public.training_sessions AS s
         WHERE s.id <> p_session_id AND s.status <> 'cancelled'
           AND pg_catalog.lower(pg_catalog.btrim(s.location)) = v_location
           AND s.starts_at < p_ends_at AND s.ends_at > p_starts_at
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ007', MESSAGE = 'Training location conflict';
    END IF;

    SELECT a.coach_id INTO v_coach_id
      FROM public.coach_session_assignments AS a WHERE a.session_id = p_session_id;
    IF v_time_changed AND v_coach_id IS NOT NULL AND EXISTS (
        SELECT 1 FROM public.coach_session_assignments AS a
        JOIN public.training_sessions AS s ON s.id = a.session_id
        WHERE a.coach_id = v_coach_id AND s.id <> p_session_id
          AND s.status <> 'cancelled'
          AND s.starts_at < p_ends_at AND s.ends_at > p_starts_at
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ007', MESSAGE = 'Coach schedule conflict';
    END IF;

    UPDATE public.training_sessions
       SET program_id = p_program_id,
           starts_at = p_starts_at,
           ends_at = p_ends_at,
           location = pg_catalog.btrim(p_location),
           capacity = p_capacity,
           notes = NULLIF(pg_catalog.btrim(COALESCE(p_notes, '')), ''),
           is_public = COALESCE(p_is_public, false),
           updated_at = pg_catalog.clock_timestamp()
     WHERE id = p_session_id;
    RETURN true;
END;
$function$;

CREATE OR REPLACE FUNCTION public.admin_assign_training_session_coach(
    p_session_id uuid,
    p_coach_id uuid
)
RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = ''
AS $function$
DECLARE
    v_coach_name text;
    v_starts_at timestamptz;
    v_ends_at timestamptz;
BEGIN
    PERFORM pg_catalog.pg_advisory_xact_lock(
        pg_catalog.hashtextextended('mmsu-training-schedule-write', 0)
    );
    IF p_session_id IS NULL THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Session is required';
    END IF;
    SELECT s.starts_at, s.ends_at INTO v_starts_at, v_ends_at
      FROM public.training_sessions AS s WHERE s.id = p_session_id FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'P0002', MESSAGE = 'Training session not found';
    END IF;
    IF p_coach_id IS NULL THEN
        DELETE FROM public.coach_session_assignments WHERE session_id = p_session_id;
        RETURN true;
    END IF;
    SELECT ca.coach_name INTO v_coach_name
      FROM public.coach_availability AS ca
     WHERE ca.coach_id = p_coach_id
     ORDER BY ca.created_at DESC LIMIT 1;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ004', MESSAGE = 'Coach has not submitted availability';
    END IF;
    IF EXISTS (
        SELECT 1 FROM public.coach_session_assignments AS a
        JOIN public.training_sessions AS s ON s.id = a.session_id
        WHERE a.coach_id = p_coach_id AND s.id <> p_session_id
          AND s.status <> 'cancelled' AND s.starts_at < v_ends_at AND s.ends_at > v_starts_at
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ007', MESSAGE = 'Coach schedule conflict';
    END IF;
    INSERT INTO public.coach_session_assignments(session_id, coach_id, coach_name)
    VALUES (p_session_id, p_coach_id, v_coach_name)
    ON CONFLICT (session_id) DO UPDATE
       SET coach_id = EXCLUDED.coach_id, coach_name = EXCLUDED.coach_name;
    RETURN true;
END;
$function$;

REVOKE ALL ON FUNCTION public.admin_create_training_sessions_batch(jsonb)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_api;
REVOKE ALL ON FUNCTION public.admin_edit_training_session(uuid, uuid, timestamptz, timestamptz, text, integer, text, boolean, boolean)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.admin_create_training_sessions_batch(jsonb) TO mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.admin_edit_training_session(uuid, uuid, timestamptz, timestamptz, text, integer, text, boolean, boolean) TO mmsu_booking_admin_api;
ALTER FUNCTION public.admin_create_training_session(uuid, timestamptz, timestamptz, text, integer, text, boolean)
    OWNER TO mmsu_schedule_admin_function_owner;
ALTER FUNCTION public.admin_create_training_sessions_batch(jsonb)
    OWNER TO mmsu_schedule_admin_function_owner;
ALTER FUNCTION public.admin_edit_training_session(uuid, uuid, timestamptz, timestamptz, text, integer, text, boolean, boolean)
    OWNER TO mmsu_schedule_admin_function_owner;
ALTER FUNCTION public.admin_assign_training_session_coach(uuid, uuid)
    OWNER TO mmsu_schedule_admin_function_owner;
RESET ROLE;
REVOKE CREATE ON SCHEMA public FROM mmsu_schedule_admin_function_owner;
REVOKE mmsu_schedule_admin_function_owner FROM CURRENT_USER;

-- Store the requested session set. All requested sessions must be one
-- contiguous, same-day block at the same venue and for the same program.
-- Existing booking requests are backfilled before the new request functions are installed.
GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET TRUE;
GRANT CREATE ON SCHEMA public TO mmsu_booking_function_owner;
SET ROLE mmsu_booking_function_owner;

CREATE OR REPLACE FUNCTION public.create_public_booking_request_with_slots_token(
    p_session_ids jsonb,
    p_full_name text,
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
    v_first_session_id uuid;
    v_request_id uuid;
    v_full_name text := pg_catalog.btrim(COALESCE(p_full_name, ''));
    v_email text := NULLIF(pg_catalog.lower(pg_catalog.btrim(p_email)), '');
    v_phone text := NULLIF(pg_catalog.btrim(p_phone), '');
    v_public_reference text;
    v_session_id uuid;
BEGIN
    IF pg_catalog.jsonb_typeof(p_session_ids) IS DISTINCT FROM 'array'
       OR p_tracking_token_hash IS NULL OR pg_catalog.octet_length(p_tracking_token_hash) <> 32
       OR v_full_name = '' OR pg_catalog.char_length(v_full_name) > 200
       OR (v_email IS NULL AND v_phone IS NULL)
       OR (v_email IS NOT NULL AND (pg_catalog.char_length(v_email) > 254 OR pg_catalog.strpos(v_email, '@') < 2))
       OR (v_phone IS NOT NULL AND pg_catalog.char_length(v_phone) > 40) THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid booking request';
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

    INSERT INTO public.booking_request_sessions(request_id, session_id, position)
    SELECT v_request_id, s.id,
           pg_catalog.row_number() OVER (ORDER BY s.starts_at, s.id)::smallint
      FROM public.training_sessions AS s WHERE s.id = ANY(v_session_ids);
    RETURN v_public_reference;
END;
$function$;

-- Keep older web/API instances compatible during a rolling deploy.
CREATE OR REPLACE FUNCTION public.create_public_booking_request_with_status_token(
    p_session_id uuid, p_full_name text, p_email text, p_phone text, p_tracking_token_hash bytea
)
RETURNS text
LANGUAGE sql SECURITY DEFINER SET search_path = ''
AS $function$
    SELECT public.create_public_booking_request_with_slots_token(
        pg_catalog.jsonb_build_array(p_session_id::text), p_full_name, p_email, p_phone, p_tracking_token_hash)
$function$;

CREATE OR REPLACE FUNCTION public.get_public_booking_status(p_tracking_token_hash bytea)
RETURNS TABLE (
    request_reference text, request_status text, program_name text,
    starts_at timestamptz, ends_at timestamptz, location text, is_expired boolean
)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = ''
AS $function$
    SELECT br.public_reference, br.status, max(p.name), min(s.starts_at), max(s.ends_at),
           max(s.location), br.tracking_token_expires_at <= pg_catalog.statement_timestamp()
      FROM public.booking_requests AS br
      JOIN public.booking_request_sessions AS requested ON requested.request_id = br.id
      JOIN public.training_sessions AS s ON s.id = requested.session_id
      JOIN public.training_programs AS p ON p.id = s.program_id
     WHERE pg_catalog.octet_length(p_tracking_token_hash) = 32
       AND br.tracking_token_hash = p_tracking_token_hash
     GROUP BY br.id
$function$;

CREATE OR REPLACE FUNCTION public.list_pending_booking_requests_with_slots()
RETURNS TABLE (
    id uuid, session_id uuid, full_name text, email text, phone text, status text,
    created_at timestamptz, starts_at timestamptz, ends_at timestamptz,
    location text, session_status text
)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = ''
AS $function$
    SELECT br.id, br.session_id, br.full_name, br.email, br.phone, br.status,
           br.created_at, min(s.starts_at), max(s.ends_at), max(s.location),
           CASE WHEN pg_catalog.bool_and(s.status = 'open') THEN 'open' ELSE 'closed' END
      FROM public.booking_requests AS br
      JOIN public.booking_request_sessions AS requested ON requested.request_id = br.id
      JOIN public.training_sessions AS s ON s.id = requested.session_id
     WHERE br.status = 'pending'
     GROUP BY br.id
     ORDER BY br.created_at, br.id
$function$;

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

    -- Rows are locked in stable order above; every capacity check and insert
    -- happens before the request is marked approved, in the same transaction.
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
    END LOOP;

    UPDATE public.booking_requests
       SET status = 'approved', resolved_player_id = v_player_id,
           decided_by = p_admin_id, decided_at = pg_catalog.clock_timestamp()
     WHERE id = p_request_id;
    RETURN v_first_booking_id;
END;
$function$;

CREATE OR REPLACE FUNCTION public.record_booking_approval_notification()
RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = ''
AS $function$
BEGIN
    IF NEW.status = 'approved' AND OLD.status IS DISTINCT FROM 'approved' THEN
        INSERT INTO public.booking_approval_notifications (
            request_id, support_reference, program_name, starts_at, ends_at, location, status, detail_code
        )
        SELECT NEW.id,
               COALESCE(NEW.public_reference, 'MMSU-' || pg_catalog.upper(NEW.id::text)),
               max(p.name), min(s.starts_at), max(s.ends_at), max(s.location),
               CASE WHEN NULLIF(pg_catalog.btrim(NEW.email), '') IS NULL THEN 'not_configured' ELSE 'pending' END,
               CASE WHEN NULLIF(pg_catalog.btrim(NEW.email), '') IS NULL THEN 'missing_email' ELSE NULL END
          FROM public.booking_request_sessions AS requested
          JOIN public.training_sessions AS s ON s.id = requested.session_id
          JOIN public.training_programs AS p ON p.id = s.program_id
         WHERE requested.request_id = NEW.id
         GROUP BY NEW.id;
        IF NOT FOUND THEN
            RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable';
        END IF;
    END IF;
    RETURN NEW;
END;
$function$;

REVOKE ALL ON FUNCTION public.create_public_booking_request_with_slots_token(jsonb, text, text, text, bytea)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_admin_api;
REVOKE ALL ON FUNCTION public.list_pending_booking_requests_with_slots()
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.create_public_booking_request_with_slots_token(jsonb, text, text, text, bytea)
    TO mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.list_pending_booking_requests_with_slots()
    TO mmsu_booking_admin_api;
ALTER FUNCTION public.create_public_booking_request_with_slots_token(jsonb, text, text, text, bytea)
    OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.create_public_booking_request_with_status_token(uuid, text, text, text, bytea)
    OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.get_public_booking_status(bytea)
    OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.list_pending_booking_requests_with_slots()
    OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.approve_booking_request(uuid, uuid, uuid, boolean)
    OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.record_booking_approval_notification()
    OWNER TO mmsu_booking_function_owner;
RESET ROLE;
REVOKE CREATE ON SCHEMA public FROM mmsu_booking_function_owner;
GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET FALSE;
