-- Keep direct public request creation consistent with session capacity even
-- when this function's original migration has already been applied.
GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET TRUE;
GRANT CREATE ON SCHEMA public TO mmsu_booking_function_owner;
SET ROLE mmsu_booking_function_owner;

CREATE OR REPLACE FUNCTION public.create_public_booking_request_with_status_token(
    p_session_id uuid,
    p_full_name text,
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
    v_program_id uuid;
    v_locked_program_id uuid;
    v_capacity integer;
    v_confirmed_count bigint;
    v_email text := NULLIF(pg_catalog.lower(pg_catalog.btrim(p_email)), '');
    v_phone text := NULLIF(pg_catalog.btrim(p_phone), '');
    v_public_reference text;
BEGIN
    IF p_tracking_token_hash IS NULL OR pg_catalog.octet_length(p_tracking_token_hash) <> 32
       OR p_full_name IS NULL OR pg_catalog.btrim(p_full_name) = ''
       OR pg_catalog.char_length(pg_catalog.btrim(p_full_name)) > 200
       OR (v_email IS NULL AND v_phone IS NULL)
       OR (v_email IS NOT NULL AND (pg_catalog.char_length(v_email) > 254 OR pg_catalog.strpos(v_email, '@') < 2))
       OR (v_phone IS NOT NULL AND pg_catalog.char_length(v_phone) > 40) THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid booking request';
    END IF;

    SELECT s.program_id INTO v_program_id
      FROM public.training_sessions AS s
     WHERE s.id = p_session_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable';
    END IF;

    PERFORM 1
      FROM public.training_programs AS p
     WHERE p.id = v_program_id AND p.is_active IS TRUE
     FOR SHARE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable';
    END IF;

    SELECT s.program_id, s.capacity
      INTO v_locked_program_id, v_capacity
      FROM public.training_sessions AS s
     WHERE s.id = p_session_id
       AND s.status = 'open'
       AND s.is_public IS TRUE
       AND s.starts_at > pg_catalog.clock_timestamp()
     FOR SHARE OF s;
    IF NOT FOUND OR v_locked_program_id <> v_program_id THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable';
    END IF;

    IF v_capacity IS NOT NULL THEN
        SELECT pg_catalog.count(*)
          INTO v_confirmed_count
          FROM public.bookings AS b
         WHERE b.session_id = p_session_id
           AND b.status = 'confirmed';

        IF v_confirmed_count >= v_capacity THEN
            RAISE EXCEPTION USING ERRCODE = 'PZ002', MESSAGE = 'Training session is full';
        END IF;
    END IF;

    IF v_email IS NOT NULL THEN
        PERFORM pg_catalog.pg_advisory_xact_lock(pg_catalog.hashtextextended('booking-request-email:' || p_session_id::text || ':' || v_email, 0));
        IF EXISTS (
            SELECT 1 FROM public.booking_requests AS br
             WHERE br.session_id = p_session_id
               AND br.status = 'pending'
               AND pg_catalog.lower(br.email) = v_email
        ) THEN
            RAISE EXCEPTION USING ERRCODE = 'PZ004', MESSAGE = 'Duplicate pending request';
        END IF;
    END IF;

    IF v_phone IS NOT NULL THEN
        PERFORM pg_catalog.pg_advisory_xact_lock(pg_catalog.hashtextextended('booking-request-phone:' || p_session_id::text || ':' || v_phone, 0));
        IF EXISTS (
            SELECT 1 FROM public.booking_requests AS br
             WHERE br.session_id = p_session_id
               AND br.status = 'pending'
               AND br.phone = v_phone
        ) THEN
            RAISE EXCEPTION USING ERRCODE = 'PZ004', MESSAGE = 'Duplicate pending request';
        END IF;
    END IF;

    v_public_reference := 'MMSU-' || pg_catalog.upper(
        pg_catalog.substr(pg_catalog.replace(pg_catalog.gen_random_uuid()::text, '-', ''), 1, 16)
    );
    INSERT INTO public.booking_requests (
        session_id, full_name, email, phone, public_reference,
        tracking_token_hash, tracking_token_expires_at
    ) VALUES (
        p_session_id, pg_catalog.btrim(p_full_name), v_email, v_phone,
        v_public_reference, p_tracking_token_hash,
        pg_catalog.clock_timestamp() + interval '30 days'
    );
    RETURN v_public_reference;
END
$function$;

REVOKE ALL ON FUNCTION public.create_public_booking_request_with_status_token(uuid, text, text, text, bytea)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.create_public_booking_request_with_status_token(uuid, text, text, text, bytea)
    TO mmsu_booking_api;

ALTER FUNCTION public.create_public_booking_request_with_status_token(uuid, text, text, text, bytea)
    OWNER TO mmsu_booking_function_owner;
RESET ROLE;
REVOKE CREATE ON SCHEMA public FROM mmsu_booking_function_owner;
GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET FALSE;
