-- Add training focus and notes to booking requests, optional for backward compatibility.
ALTER TABLE public.booking_requests
    ADD COLUMN IF NOT EXISTS training_focus text,
    ADD COLUMN IF NOT EXISTS training_focus_notes text;

-- Update create_public_booking_request_with_participants_token to accept and store training focus.
CREATE OR REPLACE FUNCTION public.create_public_booking_request_with_participants_token(
    p_session_ids jsonb,
    p_participants jsonb,
    p_email text,
    p_phone text,
    p_tracking_token_hash bytea,
    p_training_focus text,
    p_training_focus_notes text
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
       OR p_participants IS NULL OR pg_catalog.cardinality(p_participants) = 0
       OR p_session_ids IS NULL OR pg_catalog.cardinality(p_session_ids) = 0 THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid booking request';
    END IF;

    SELECT s.program_id INTO v_program_id
      FROM public.training_sessions AS s WHERE s.id = (SELECT jsonb_array_elements_text(p_session_ids)::uuid LIMIT 1);
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable';
    END IF;
    PERFORM 1 FROM public.training_programs AS p
     WHERE p.id = v_program_id AND p.is_active IS TRUE FOR SHARE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable';
    END IF;
    SELECT s.program_id, s.capacity INTO v_locked_program_id, v_capacity
      FROM public.training_sessions AS s
     WHERE s.id = (SELECT jsonb_array_elements_text(p_session_ids)::uuid LIMIT 1)
       AND s.status = 'open' AND s.is_public IS TRUE
      AND s.starts_at > pg_catalog.clock_timestamp()
     FOR SHARE OF s;
    IF NOT FOUND OR v_locked_program_id <> v_program_id THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable';
    END IF;

    IF v_capacity IS NOT NULL THEN
        SELECT pg_catalog.count(*) INTO v_confirmed_count
          FROM public.bookings AS b
         WHERE b.session_id = (SELECT jsonb_array_elements_text(p_session_ids)::uuid LIMIT 1) AND b.status = 'confirmed';
        IF v_confirmed_count >= v_capacity THEN
            RAISE EXCEPTION USING ERRCODE = 'PZ002', MESSAGE = 'Training session is full';
        END IF;
    END IF;

    IF v_email IS NOT NULL THEN
        PERFORM pg_catalog.pg_advisory_xact_lock(pg_catalog.hashtextextended('booking-request-email:' || (SELECT jsonb_array_elements_text(p_session_ids)::uuid LIMIT 1)::text || ':' || v_email, 0));
        IF EXISTS (SELECT 1 FROM public.booking_requests AS br WHERE br.session_id = (SELECT jsonb_array_elements_text(p_session_ids)::uuid LIMIT 1) AND br.status = 'pending' AND pg_catalog.lower(br.email) = v_email) THEN
            RAISE EXCEPTION USING ERRCODE = 'PZ004', MESSAGE = 'Duplicate pending request';
        END IF;
    END IF;
    IF v_phone IS NOT NULL THEN
        PERFORM pg_catalog.pg_advisory_xact_lock(pg_catalog.hashtextextended('booking-request-phone:' || (SELECT jsonb_array_elements_text(p_session_ids)::uuid LIMIT 1)::text || ':' || v_phone, 0));
        IF EXISTS (SELECT 1 FROM public.booking_requests AS br WHERE br.session_id = (SELECT jsonb_array_elements_text(p_session_ids)::uuid LIMIT 1) AND br.status = 'pending' AND br.phone = v_phone) THEN
            RAISE EXCEPTION USING ERRCODE = 'PZ004', MESSAGE = 'Duplicate pending request';
        END IF;
    END IF;

    v_public_reference := 'MMSU-' || pg_catalog.upper(
        pg_catalog.substr(pg_catalog.replace(gen_random_uuid()::text, '-', ''), 1, 16)
    );
    INSERT INTO public.booking_requests (
        session_id, full_name, email, phone, public_reference,
        tracking_token_hash, tracking_token_expires_at, training_focus, training_focus_notes
    ) VALUES (
        (SELECT jsonb_array_elements_text(p_session_ids)::uuid LIMIT 1),
        pg_catalog.btrim((SELECT jsonb_array_elements_text(p_participants) LIMIT 1)::text),
        v_email, v_phone,
        v_public_reference, p_tracking_token_hash,
        pg_catalog.clock_timestamp() + interval '30 days',
        p_training_focus, p_training_focus_notes
    );
    RETURN v_public_reference;
END
$function$;

-- Update get_public_booking_status to return training focus and notes.
-- PostgreSQL cannot CREATE OR REPLACE a function when its OUT/RETURNS TABLE
-- row type changes. Drop the existing signatures; grants and owner are
-- restored below after the replacement functions are created.
DROP FUNCTION IF EXISTS public.get_public_booking_status(bytea);
CREATE OR REPLACE FUNCTION public.get_public_booking_status(p_tracking_token_hash bytea)
RETURNS TABLE (
    request_reference text,
    request_status text,
    program_name text,
    starts_at timestamptz,
    ends_at timestamptz,
    location text,
    is_expired boolean,
    training_focus text,
    training_focus_notes text
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = ''
AS $function$
    SELECT br.public_reference, br.status, pg_catalog.max(p.name), pg_catalog.min(s.starts_at), pg_catalog.max(s.ends_at),
           pg_catalog.max(s.location), br.tracking_token_expires_at <= pg_catalog.statement_timestamp(),
           br.training_focus, br.training_focus_notes
      FROM public.booking_requests AS br
      JOIN public.booking_request_sessions AS requested ON requested.request_id = br.id
      JOIN public.training_sessions AS s ON s.id = requested.session_id
      JOIN public.training_programs AS p ON p.id = s.program_id
     WHERE pg_catalog.octet_length(p_tracking_token_hash) = 32
       AND br.tracking_token_hash = p_tracking_token_hash
     GROUP BY br.id
$function$;

-- Update list_pending_booking_requests_with_slots to include training focus and notes.
DROP FUNCTION IF EXISTS public.list_pending_booking_requests_with_slots();
CREATE OR REPLACE FUNCTION public.list_pending_booking_requests_with_slots()
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
    training_focus_notes text
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = ''
AS $function$
    SELECT br.id, br.session_id, br.full_name, br.email, br.phone, br.status, br.created_at,
           pg_catalog.min(s.starts_at), pg_catalog.max(s.ends_at), pg_catalog.max(s.location),
           CASE WHEN pg_catalog.bool_and(s.status = 'open') THEN 'open' ELSE 'closed' END,
           br.training_focus, br.training_focus_notes
      FROM public.booking_requests AS br
      JOIN public.booking_request_sessions AS requested ON requested.request_id = br.id
      JOIN public.training_sessions AS s ON s.id = requested.session_id
     WHERE br.status = 'pending'
     GROUP BY br.id
     ORDER BY br.created_at, br.id
$function$;

-- Re-grant permissions so the API role can read the new columns.
GRANT SELECT (training_focus, training_focus_notes) ON public.booking_requests TO mmsu_booking_api;
GRANT SELECT (training_focus, training_focus_notes) ON public.booking_requests TO mmsu_schedule_admin_function_owner;
GRANT SELECT (training_focus, training_focus_notes) ON public.booking_requests TO mmsu_schedule_reader;

-- Temporarily let the migration role transfer these SECURITY DEFINER
-- functions to their restricted owner. SET FALSE is restored after transfer.
GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET TRUE;
GRANT CREATE ON SCHEMA public TO mmsu_booking_function_owner;

REVOKE ALL ON FUNCTION public.create_public_booking_request_with_participants_token(jsonb, jsonb, text, text, bytea, text, text)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.create_public_booking_request_with_participants_token(jsonb, jsonb, text, text, bytea, text, text)
    TO mmsu_booking_api;
ALTER FUNCTION public.create_public_booking_request_with_participants_token(jsonb, jsonb, text, text, bytea, text, text)
    OWNER TO mmsu_booking_function_owner;

REVOKE ALL ON FUNCTION public.get_public_booking_status(bytea)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.get_public_booking_status(bytea)
    TO mmsu_booking_api;
ALTER FUNCTION public.get_public_booking_status(bytea)
    OWNER TO mmsu_booking_function_owner;

REVOKE ALL ON FUNCTION public.list_pending_booking_requests_with_slots()
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.list_pending_booking_requests_with_slots()
    TO mmsu_booking_api;
ALTER FUNCTION public.list_pending_booking_requests_with_slots()
    OWNER TO mmsu_booking_function_owner;
REVOKE CREATE ON SCHEMA public FROM mmsu_booking_function_owner;
GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET FALSE;
