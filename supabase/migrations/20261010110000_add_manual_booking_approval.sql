-- Manual approval requests supersede the earlier email-verified immediate
-- booking path. This migration only adds request state and narrow functions.

CREATE ROLE mmsu_booking_admin_api
    WITH NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
GRANT USAGE ON SCHEMA public TO mmsu_booking_admin_api;

GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET TRUE;
SET ROLE mmsu_booking_function_owner;
REVOKE EXECUTE ON FUNCTION public.request_booking_email_verification(text, bytea) FROM mmsu_booking_api;
REVOKE EXECUTE ON FUNCTION public.confirm_booking_email_verification(text, bytea, bytea) FROM mmsu_booking_api;
REVOKE EXECUTE ON FUNCTION public.book_public_training_session(uuid, text, text, bytea) FROM mmsu_booking_api;
RESET ROLE;

CREATE TABLE public.booking_requests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id uuid NOT NULL REFERENCES public.training_sessions(id) ON DELETE RESTRICT,
    full_name text NOT NULL CHECK (btrim(full_name) <> '' AND char_length(btrim(full_name)) <= 200),
    email text,
    phone text,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    resolved_player_id uuid REFERENCES public.players(id) ON DELETE RESTRICT,
    decided_by uuid,
    decided_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT booking_requests_contact_required CHECK (
        (email IS NOT NULL AND btrim(email) <> '') OR (phone IS NOT NULL AND btrim(phone) <> '')
    ),
    CONSTRAINT booking_requests_decision_consistent CHECK (
        (status = 'pending' AND decided_by IS NULL AND decided_at IS NULL AND resolved_player_id IS NULL)
        OR (status IN ('approved', 'rejected') AND decided_by IS NOT NULL AND decided_at IS NOT NULL)
    )
);
CREATE INDEX booking_requests_pending_created_idx ON public.booking_requests(created_at)
    WHERE status = 'pending';
CREATE UNIQUE INDEX booking_requests_pending_session_email_idx
    ON public.booking_requests(session_id, pg_catalog.lower(email))
    WHERE status = 'pending' AND email IS NOT NULL;
CREATE UNIQUE INDEX booking_requests_pending_session_phone_idx
    ON public.booking_requests(session_id, phone)
    WHERE status = 'pending' AND phone IS NOT NULL;
ALTER TABLE public.booking_requests ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE public.booking_requests FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_api;
GRANT SELECT, INSERT, UPDATE ON TABLE public.booking_requests TO mmsu_booking_function_owner;
CREATE POLICY booking_requests_function_owner_all ON public.booking_requests
    FOR ALL TO mmsu_booking_function_owner USING (true) WITH CHECK (true);
CREATE POLICY training_sessions_booking_function_owner_review
    ON public.training_sessions FOR SELECT TO mmsu_booking_function_owner USING (true);

GRANT SELECT (id, full_name, email, phone), INSERT (full_name, email, phone)
    ON TABLE public.players TO mmsu_booking_function_owner;
GRANT SELECT (id, program_id, starts_at, status, is_public, capacity)
    ON TABLE public.training_sessions TO mmsu_booking_function_owner;
GRANT SELECT (location) ON TABLE public.training_sessions TO mmsu_booking_function_owner;

CREATE OR REPLACE FUNCTION public.create_public_booking_request(
    p_session_id uuid, p_full_name text, p_email text, p_phone text
)
RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = ''
AS $function$
DECLARE
    v_program_id uuid;
    v_locked_program_id uuid;
    v_request_id uuid;
    v_email text := NULLIF(pg_catalog.lower(pg_catalog.btrim(p_email)), '');
    v_phone text := NULLIF(pg_catalog.btrim(p_phone), '');
BEGIN
    IF p_full_name IS NULL OR pg_catalog.btrim(p_full_name) = ''
       OR pg_catalog.char_length(pg_catalog.btrim(p_full_name)) > 200
       OR (v_email IS NULL AND v_phone IS NULL)
       OR (v_email IS NOT NULL AND (pg_catalog.char_length(v_email) > 254 OR pg_catalog.strpos(v_email, '@') < 2))
       OR (v_phone IS NOT NULL AND pg_catalog.char_length(v_phone) > 40) THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid booking request';
    END IF;
    SELECT s.program_id INTO v_program_id FROM public.training_sessions AS s WHERE s.id = p_session_id;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable'; END IF;
    PERFORM 1 FROM public.training_programs AS p WHERE p.id = v_program_id AND p.is_active IS TRUE FOR SHARE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable'; END IF;
    SELECT s.program_id INTO v_locked_program_id
      FROM public.training_sessions AS s
     WHERE s.id = p_session_id AND s.status = 'open' AND s.is_public IS TRUE
       AND s.starts_at > pg_catalog.clock_timestamp()
     FOR SHARE OF s;
    IF NOT FOUND OR v_locked_program_id <> v_program_id THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable';
    END IF;
    IF v_email IS NOT NULL THEN
        PERFORM pg_catalog.pg_advisory_xact_lock(pg_catalog.hashtextextended('booking-request-email:' || p_session_id::text || ':' || v_email, 0));
        IF EXISTS (SELECT 1 FROM public.booking_requests AS br WHERE br.session_id = p_session_id AND br.status = 'pending' AND pg_catalog.lower(br.email) = v_email) THEN
            RAISE EXCEPTION USING ERRCODE = 'PZ004', MESSAGE = 'Duplicate pending request';
        END IF;
    END IF;
    IF v_phone IS NOT NULL THEN
        PERFORM pg_catalog.pg_advisory_xact_lock(pg_catalog.hashtextextended('booking-request-phone:' || p_session_id::text || ':' || v_phone, 0));
        IF EXISTS (SELECT 1 FROM public.booking_requests AS br WHERE br.session_id = p_session_id AND br.status = 'pending' AND br.phone = v_phone) THEN
            RAISE EXCEPTION USING ERRCODE = 'PZ004', MESSAGE = 'Duplicate pending request';
        END IF;
    END IF;
    INSERT INTO public.booking_requests(session_id, full_name, email, phone)
    VALUES (p_session_id, pg_catalog.btrim(p_full_name), v_email, v_phone)
    RETURNING id INTO v_request_id;
    RETURN v_request_id;
END;
$function$;

CREATE OR REPLACE FUNCTION public.list_pending_booking_requests()
RETURNS TABLE(id uuid, session_id uuid, full_name text, email text, phone text, status text,
              created_at timestamptz, starts_at timestamptz, location text, session_status text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = ''
AS $function$
    SELECT br.id, br.session_id, br.full_name, br.email, br.phone, br.status,
           br.created_at, s.starts_at, s.location, s.status
      FROM public.booking_requests AS br
      JOIN public.training_sessions AS s ON s.id = br.session_id
     WHERE br.status = 'pending'
     ORDER BY br.created_at, br.id
$function$;

CREATE OR REPLACE FUNCTION public.search_booking_players(p_query text)
RETURNS TABLE(id uuid, full_name text, email text, phone text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = ''
AS $function$
    SELECT pl.id, pl.full_name, pl.email, pl.phone
      FROM public.players AS pl
     WHERE pg_catalog.char_length(pg_catalog.btrim(p_query)) BETWEEN 2 AND 100
       AND (pl.full_name ILIKE '%' || pg_catalog.btrim(p_query) || '%'
         OR pl.email ILIKE '%' || pg_catalog.btrim(p_query) || '%'
         OR pl.phone ILIKE '%' || pg_catalog.btrim(p_query) || '%')
     ORDER BY pl.full_name, pl.id
     LIMIT 50
$function$;

CREATE OR REPLACE FUNCTION public.approve_booking_request(
    p_request_id uuid, p_admin_id uuid, p_player_id uuid, p_create_new boolean
)
RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = ''
AS $function$
DECLARE
    v_request public.booking_requests;
    v_program_id uuid;
    v_locked_program_id uuid;
    v_player_id uuid;
    v_capacity integer;
    v_confirmed_count bigint;
    v_booking_id uuid;
BEGIN
    SELECT br.* INTO v_request FROM public.booking_requests AS br WHERE br.id = p_request_id FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'PZ006', MESSAGE = 'Booking request not found'; END IF;
    IF v_request.status <> 'pending' THEN RAISE EXCEPTION USING ERRCODE = 'PZ003', MESSAGE = 'Booking request already decided'; END IF;
    IF p_admin_id IS NULL OR ((p_player_id IS NULL) = (p_create_new IS FALSE)) THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Choose an existing player or create a new one';
    END IF;
    SELECT s.program_id INTO v_program_id FROM public.training_sessions AS s WHERE s.id = v_request.session_id;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable'; END IF;
    PERFORM 1 FROM public.training_programs AS p WHERE p.id = v_program_id AND p.is_active IS TRUE FOR SHARE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable'; END IF;
    SELECT s.program_id, s.capacity INTO v_locked_program_id, v_capacity
      FROM public.training_sessions AS s
     WHERE s.id = v_request.session_id AND s.status = 'open' AND s.is_public IS TRUE
       AND s.starts_at > pg_catalog.clock_timestamp()
     FOR UPDATE OF s;
    IF NOT FOUND OR v_locked_program_id <> v_program_id THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable';
    END IF;
    IF p_create_new THEN
        INSERT INTO public.players(full_name, email, phone)
        VALUES (v_request.full_name, v_request.email, v_request.phone)
        RETURNING id INTO v_player_id;
    ELSE
        SELECT pl.id INTO v_player_id FROM public.players AS pl WHERE pl.id = p_player_id;
        IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'PZ005', MESSAGE = 'Selected player unavailable'; END IF;
    END IF;
    IF EXISTS (SELECT 1 FROM public.bookings AS b WHERE b.player_id = v_player_id AND b.session_id = v_request.session_id AND b.status = 'confirmed') THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ004', MESSAGE = 'Player is already booked for this session';
    END IF;
    IF v_capacity IS NOT NULL THEN
        SELECT pg_catalog.count(*) INTO v_confirmed_count FROM public.bookings AS b
         WHERE b.session_id = v_request.session_id AND b.status = 'confirmed';
        IF v_confirmed_count >= v_capacity THEN
            RAISE EXCEPTION USING ERRCODE = 'PZ002', MESSAGE = 'Training session is full';
        END IF;
    END IF;
    INSERT INTO public.bookings(player_id, session_id, status, external_booking_reference)
    VALUES (v_player_id, v_request.session_id, 'confirmed', 'request:' || p_request_id::text)
    RETURNING id INTO v_booking_id;
    UPDATE public.booking_requests
       SET status = 'approved', resolved_player_id = v_player_id,
           decided_by = p_admin_id, decided_at = pg_catalog.clock_timestamp()
     WHERE id = p_request_id;
    RETURN v_booking_id;
END;
$function$;

CREATE OR REPLACE FUNCTION public.reject_booking_request(p_request_id uuid, p_admin_id uuid)
RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = ''
AS $function$
BEGIN
    IF p_admin_id IS NULL THEN RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Admin identity required'; END IF;
    UPDATE public.booking_requests
       SET status = 'rejected', decided_by = p_admin_id, decided_at = pg_catalog.clock_timestamp()
     WHERE id = p_request_id AND status = 'pending';
    IF NOT FOUND THEN
        IF EXISTS (SELECT 1 FROM public.booking_requests WHERE id = p_request_id) THEN
            RAISE EXCEPTION USING ERRCODE = 'PZ003', MESSAGE = 'Booking request already decided';
        END IF;
        RAISE EXCEPTION USING ERRCODE = 'PZ006', MESSAGE = 'Booking request not found';
    END IF;
    RETURN true;
END;
$function$;

REVOKE ALL ON FUNCTION public.create_public_booking_request(uuid, text, text, text) FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader;
REVOKE ALL ON FUNCTION public.list_pending_booking_requests() FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader;
REVOKE ALL ON FUNCTION public.search_booking_players(text) FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader;
REVOKE ALL ON FUNCTION public.approve_booking_request(uuid, uuid, uuid, boolean) FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader;
REVOKE ALL ON FUNCTION public.reject_booking_request(uuid, uuid) FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader;
GRANT EXECUTE ON FUNCTION public.create_public_booking_request(uuid, text, text, text) TO mmsu_booking_api;
REVOKE EXECUTE ON FUNCTION public.list_pending_booking_requests() FROM mmsu_booking_api;
REVOKE EXECUTE ON FUNCTION public.search_booking_players(text) FROM mmsu_booking_api;
REVOKE EXECUTE ON FUNCTION public.approve_booking_request(uuid, uuid, uuid, boolean) FROM mmsu_booking_api;
REVOKE EXECUTE ON FUNCTION public.reject_booking_request(uuid, uuid) FROM mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.list_pending_booking_requests() TO mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.search_booking_players(text) TO mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.approve_booking_request(uuid, uuid, uuid, boolean) TO mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.reject_booking_request(uuid, uuid) TO mmsu_booking_admin_api;

GRANT CREATE ON SCHEMA public TO mmsu_booking_function_owner;
ALTER FUNCTION public.create_public_booking_request(uuid, text, text, text) OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.list_pending_booking_requests() OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.search_booking_players(text) OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.approve_booking_request(uuid, uuid, uuid, boolean) OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.reject_booking_request(uuid, uuid) OWNER TO mmsu_booking_function_owner;
REVOKE CREATE ON SCHEMA public FROM mmsu_booking_function_owner;
GRANT mmsu_booking_function_owner TO CURRENT_USER
WITH SET FALSE;
