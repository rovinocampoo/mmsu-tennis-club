-- Extend public booking requests to recurring sessions on multiple Manila
-- dates, and expose each requested slot to the admin review queue.
GRANT mmsu_booking_function_owner TO CURRENT_USER
    WITH ADMIN FALSE, INHERIT FALSE, SET TRUE;
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
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
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
    IF v_session_count < 1 OR v_session_count > 16 THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ012', MESSAGE = 'Booking request exceeds 16 session slots';
    END IF;
    IF (SELECT pg_catalog.count(DISTINCT id) FROM pg_catalog.unnest(v_session_ids) AS ids(id)) <> v_session_count THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Duplicate session IDs';
    END IF;

    -- Lock requested rows in a stable order before checking availability.
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
       OR (SELECT pg_catalog.count(DISTINCT pg_catalog.lower(pg_catalog.btrim(s.location))) FROM public.training_sessions AS s WHERE s.id = ANY(v_session_ids)) <> 1 THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ010', MESSAGE = 'Slots must share a program and location';
    END IF;
    IF (SELECT pg_catalog.count(DISTINCT (s.starts_at AT TIME ZONE 'Asia/Manila')::date)
          FROM public.training_sessions AS s WHERE s.id = ANY(v_session_ids)) > 8 THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ012', MESSAGE = 'Booking request exceeds 8 dates';
    END IF;
    IF EXISTS (
        SELECT 1
          FROM public.training_sessions AS first_slot
          JOIN public.training_sessions AS second_slot
            ON first_slot.id < second_slot.id
           AND first_slot.starts_at < second_slot.ends_at
           AND second_slot.starts_at < first_slot.ends_at
         WHERE first_slot.id = ANY(v_session_ids)
           AND second_slot.id = ANY(v_session_ids)
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ010', MESSAGE = 'Selected session times overlap';
    END IF;
    IF EXISTS (
        SELECT 1 FROM (
            SELECT s.ends_at,
                   pg_catalog.lead(s.starts_at) OVER (
                       PARTITION BY (s.starts_at AT TIME ZONE 'Asia/Manila')::date
                       ORDER BY s.starts_at, s.id
                   ) AS next_start
              FROM public.training_sessions AS s WHERE s.id = ANY(v_session_ids)
        ) AS ordered_slots
        WHERE ordered_slots.next_start IS NOT NULL AND ordered_slots.ends_at <> ordered_slots.next_start
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ010', MESSAGE = 'Slots on each date must be adjacent';
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
END
$function$;

CREATE FUNCTION public.admin_list_pending_booking_requests_with_slots()
RETURNS TABLE (
    id uuid,
    session_id uuid,
    full_name text,
    email text,
    phone text,
    status text,
    created_at timestamptz,
    starts_at timestamptz,
    ends_at timestamptz,
    location text,
    session_status text,
    training_focus text,
    training_focus_notes text,
    requested_slots jsonb
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = ''
AS $function$
    SELECT br.id, br.session_id, br.full_name, br.email, br.phone, br.status,
           br.created_at, pg_catalog.min(s.starts_at), pg_catalog.max(s.ends_at),
           pg_catalog.max(s.location),
           CASE WHEN pg_catalog.bool_and(s.status = 'open') THEN 'open' ELSE 'closed' END,
           br.training_focus, br.training_focus_notes,
           pg_catalog.jsonb_agg(
               pg_catalog.jsonb_build_object(
                   'session_id', s.id,
                   'program_name', p.name,
                   'starts_at', s.starts_at,
                   'ends_at', s.ends_at,
                   'location', s.location
               ) ORDER BY s.starts_at, s.id
           )
      FROM public.booking_requests AS br
      JOIN public.booking_request_sessions AS requested ON requested.request_id = br.id
      JOIN public.training_sessions AS s ON s.id = requested.session_id
      JOIN public.training_programs AS p ON p.id = s.program_id
     WHERE br.status = 'pending'
     GROUP BY br.id
     ORDER BY br.created_at ASC, br.id ASC
$function$;

REVOKE ALL ON FUNCTION public.admin_list_pending_booking_requests_with_slots()
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_api, mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.admin_list_pending_booking_requests_with_slots()
    TO mmsu_booking_admin_api;

RESET ROLE;
REVOKE CREATE ON SCHEMA public FROM mmsu_booking_function_owner;
REVOKE mmsu_booking_function_owner FROM CURRENT_USER;
