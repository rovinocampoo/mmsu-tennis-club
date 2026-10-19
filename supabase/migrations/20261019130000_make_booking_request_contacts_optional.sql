-- Make email and phone independently optional for public training requests.
--
-- A visitor may submit a valid request with only a full name. Both contact
-- fields are optional, so the previous "at least one contact method" checks
-- must be removed from the database constraints and the request entry point.
-- Player records created during approval may therefore also have no contact
-- details, so the matching players constraint is dropped as well.
--
-- This migration does not create new roles or logins. It reuses the existing
-- mmsu_booking_function_owner role for the SECURITY DEFINER entry point.

-- 1. Remove the "email or phone required" table constraints.
ALTER TABLE public.booking_requests
    DROP CONSTRAINT IF EXISTS booking_requests_contact_required;

ALTER TABLE public.players
    DROP CONSTRAINT IF EXISTS players_contact_required;

-- 2. Replace the public request entry point without the contact requirement.
--    Every other validation, duplicate guard, capacity check, and advisory lock
--    is preserved exactly, so optional contacts do not weaken duplicate or
--    capacity protection.
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

RESET ROLE;
REVOKE CREATE ON SCHEMA public FROM mmsu_booking_function_owner;
GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET FALSE;
