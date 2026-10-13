-- Coach availability submission and admin session management workflow.
-- Allows coaches to submit weekly/monthly availability and admins to schedule
-- training sessions and manage schedule exceptions.

CREATE TABLE public.coach_availability (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    coach_id uuid NOT NULL,
    coach_name text NOT NULL DEFAULT '',
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    notes text,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT coach_availability_time_order CHECK (starts_at < ends_at)
);

CREATE INDEX coach_availability_coach_id_idx ON public.coach_availability (coach_id);
CREATE INDEX coach_availability_starts_at_idx ON public.coach_availability (starts_at);

ALTER TABLE public.coach_availability ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE public.coach_availability FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_api, mmsu_booking_admin_api;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.coach_availability TO mmsu_booking_function_owner;
CREATE POLICY coach_availability_function_owner_all ON public.coach_availability
    FOR ALL TO mmsu_booking_function_owner USING (true) WITH CHECK (true);

GRANT SELECT (name, description, is_active) ON TABLE public.training_programs TO mmsu_booking_function_owner;
GRANT INSERT, UPDATE (status, notes, is_public, location, starts_at, ends_at, capacity) ON TABLE public.training_sessions TO mmsu_booking_function_owner;

CREATE POLICY training_sessions_function_owner_manage ON public.training_sessions
    FOR ALL TO mmsu_booking_function_owner USING (true) WITH CHECK (true);
CREATE POLICY training_programs_function_owner_select ON public.training_programs
    FOR SELECT TO mmsu_booking_function_owner USING (true);

CREATE OR REPLACE FUNCTION public.submit_coach_availability(
    p_coach_id uuid,
    p_coach_name text,
    p_starts_at timestamptz,
    p_ends_at timestamptz,
    p_notes text
)
RETURNS uuid
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
AS $function$
DECLARE
    v_id uuid;
BEGIN
    IF p_coach_id IS NULL OR p_starts_at IS NULL OR p_ends_at IS NULL OR p_starts_at >= p_ends_at THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid availability times';
    END IF;

    INSERT INTO public.coach_availability (
        coach_id, coach_name, starts_at, ends_at, notes
    )
    VALUES (
        p_coach_id,
        COALESCE(pg_catalog.btrim(p_coach_name), ''),
        p_starts_at,
        p_ends_at,
        NULLIF(pg_catalog.btrim(p_notes), '')
    )
    RETURNING id INTO v_id;

    RETURN v_id;
END;
$function$;

CREATE OR REPLACE FUNCTION public.list_coach_availability(
    p_coach_id uuid
)
RETURNS TABLE (
    id uuid,
    coach_id uuid,
    coach_name text,
    starts_at timestamptz,
    ends_at timestamptz,
    notes text,
    created_at timestamptz
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = ''
AS $function$
    SELECT ca.id, ca.coach_id, ca.coach_name, ca.starts_at, ca.ends_at, ca.notes, ca.created_at
      FROM public.coach_availability AS ca
     WHERE ca.coach_id = p_coach_id
     ORDER BY ca.starts_at ASC;
$function$;

CREATE OR REPLACE FUNCTION public.delete_coach_availability(
    p_availability_id uuid,
    p_coach_id uuid
)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
AS $function$
BEGIN
    DELETE FROM public.coach_availability
     WHERE id = p_availability_id
       AND coach_id = p_coach_id;
    RETURN FOUND;
END;
$function$;

CREATE OR REPLACE FUNCTION public.admin_list_coach_availability()
RETURNS TABLE (
    id uuid,
    coach_id uuid,
    coach_name text,
    starts_at timestamptz,
    ends_at timestamptz,
    notes text,
    created_at timestamptz
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = ''
AS $function$
    SELECT ca.id, ca.coach_id, ca.coach_name, ca.starts_at, ca.ends_at, ca.notes, ca.created_at
      FROM public.coach_availability AS ca
     ORDER BY ca.starts_at ASC;
$function$;

CREATE OR REPLACE FUNCTION public.admin_list_training_programs()
RETURNS TABLE (
    id uuid,
    name text,
    description text,
    is_active boolean
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = ''
AS $function$
    SELECT tp.id, tp.name, tp.description, tp.is_active
      FROM public.training_programs AS tp
     WHERE tp.is_active IS TRUE
     ORDER BY tp.name ASC;
$function$;

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
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
AS $function$
DECLARE
    v_session_id uuid;
BEGIN
    IF p_program_id IS NULL OR p_starts_at IS NULL OR p_ends_at IS NULL
       OR p_starts_at >= p_ends_at
       OR p_location IS NULL OR pg_catalog.btrim(p_location) = ''
       OR (p_capacity IS NOT NULL AND p_capacity <= 0) THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid session parameters';
    END IF;

    IF NOT EXISTS (SELECT 1 FROM public.training_programs WHERE id = p_program_id AND is_active IS TRUE) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training program unavailable';
    END IF;

    INSERT INTO public.training_sessions (
        program_id, starts_at, ends_at, location, capacity, notes, is_public, status
    )
    VALUES (
        p_program_id,
        p_starts_at,
        p_ends_at,
        pg_catalog.btrim(p_location),
        p_capacity,
        NULLIF(pg_catalog.btrim(p_notes), ''),
        COALESCE(p_is_public, true),
        'open'
    )
    RETURNING id INTO v_session_id;

    RETURN v_session_id;
END;
$function$;

CREATE OR REPLACE FUNCTION public.admin_update_training_session(
    p_session_id uuid,
    p_status text,
    p_notes text,
    p_is_public boolean
)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
AS $function$
BEGIN
    IF p_session_id IS NULL
       OR (p_status IS NOT NULL AND p_status NOT IN ('open', 'closed', 'cancelled')) THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid update parameters';
    END IF;

    UPDATE public.training_sessions
       SET status = COALESCE(p_status, status),
           notes = CASE WHEN p_notes IS NOT NULL THEN NULLIF(pg_catalog.btrim(p_notes), '') ELSE notes END,
           is_public = COALESCE(p_is_public, is_public),
           updated_at = pg_catalog.clock_timestamp()
     WHERE id = p_session_id;

    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'P0002', MESSAGE = 'Training session not found';
    END IF;

    RETURN true;
END;
$function$;

CREATE OR REPLACE FUNCTION public.admin_list_training_sessions()
RETURNS TABLE (
    id uuid,
    program_id uuid,
    program_name text,
    starts_at timestamptz,
    ends_at timestamptz,
    location text,
    capacity integer,
    confirmed_booking_count bigint,
    status text,
    notes text,
    is_public boolean
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = ''
AS $function$
    SELECT s.id,
           s.program_id,
           p.name,
           s.starts_at,
           s.ends_at,
           s.location,
           s.capacity,
           COALESCE(cb.confirmed_booking_count, 0),
           s.status,
           s.notes,
           s.is_public
      FROM public.training_sessions AS s
      JOIN public.training_programs AS p ON p.id = s.program_id
      LEFT JOIN LATERAL (
          SELECT pg_catalog.count(*)::bigint AS confirmed_booking_count
            FROM public.bookings AS b
           WHERE b.session_id = s.id
             AND b.status = 'confirmed'
      ) AS cb ON true
     ORDER BY s.starts_at DESC;
$function$;

REVOKE ALL ON FUNCTION public.submit_coach_availability(uuid, text, timestamptz, timestamptz, text) FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader;
REVOKE ALL ON FUNCTION public.list_coach_availability(uuid) FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader;
REVOKE ALL ON FUNCTION public.delete_coach_availability(uuid, uuid) FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader;
REVOKE ALL ON FUNCTION public.admin_list_coach_availability() FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader;
REVOKE ALL ON FUNCTION public.admin_list_training_programs() FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader;
REVOKE ALL ON FUNCTION public.admin_create_training_session(uuid, timestamptz, timestamptz, text, integer, text, boolean) FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader;
REVOKE ALL ON FUNCTION public.admin_update_training_session(uuid, text, text, boolean) FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader;
REVOKE ALL ON FUNCTION public.admin_list_training_sessions() FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader;

GRANT EXECUTE ON FUNCTION public.submit_coach_availability(uuid, text, timestamptz, timestamptz, text) TO mmsu_booking_api, mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.list_coach_availability(uuid) TO mmsu_booking_api, mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.delete_coach_availability(uuid, uuid) TO mmsu_booking_api, mmsu_booking_admin_api;

GRANT EXECUTE ON FUNCTION public.admin_list_coach_availability() TO mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.admin_list_training_programs() TO mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.admin_create_training_session(uuid, timestamptz, timestamptz, text, integer, text, boolean) TO mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.admin_update_training_session(uuid, text, text, boolean) TO mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.admin_list_training_sessions() TO mmsu_booking_admin_api;

GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET TRUE;
GRANT CREATE ON SCHEMA public TO mmsu_booking_function_owner;
ALTER TABLE public.coach_availability OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.submit_coach_availability(uuid, text, timestamptz, timestamptz, text) OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.list_coach_availability(uuid) OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.delete_coach_availability(uuid, uuid) OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.admin_list_coach_availability() OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.admin_list_training_programs() OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.admin_create_training_session(uuid, timestamptz, timestamptz, text, integer, text, boolean) OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.admin_update_training_session(uuid, text, text, boolean) OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.admin_list_training_sessions() OWNER TO mmsu_booking_function_owner;
REVOKE CREATE ON SCHEMA public FROM mmsu_booking_function_owner;
GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET FALSE;
