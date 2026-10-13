-- Admins can assign one coach to each training session. Availability remains
-- an independent planning input and never creates or publishes sessions.

GRANT mmsu_schedule_admin_function_owner TO CURRENT_USER
    WITH ADMIN FALSE, INHERIT FALSE, SET TRUE;
SET ROLE mmsu_schedule_admin_function_owner;
DROP FUNCTION public.admin_list_training_sessions();
RESET ROLE;

CREATE TABLE public.coach_session_assignments (
    session_id uuid PRIMARY KEY REFERENCES public.training_sessions(id) ON DELETE CASCADE,
    coach_id uuid NOT NULL,
    coach_name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT pg_catalog.clock_timestamp()
);

CREATE INDEX coach_session_assignments_coach_id_idx
    ON public.coach_session_assignments (coach_id);

ALTER TABLE public.coach_session_assignments ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE public.coach_session_assignments
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_api, mmsu_booking_admin_api;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.coach_session_assignments
    TO mmsu_schedule_admin_function_owner;
CREATE POLICY coach_session_assignments_admin_manage
    ON public.coach_session_assignments
    FOR ALL TO mmsu_schedule_admin_function_owner
    USING (true) WITH CHECK (true);

GRANT SELECT (coach_id, coach_name, created_at)
    ON TABLE public.coach_availability TO mmsu_schedule_admin_function_owner;
CREATE POLICY coach_availability_schedule_admin_select
    ON public.coach_availability
    FOR SELECT TO mmsu_schedule_admin_function_owner USING (true);
GRANT SELECT (id, program_id, starts_at, ends_at, location, capacity, status, is_public)
    ON TABLE public.training_sessions TO mmsu_schedule_admin_function_owner;
GRANT SELECT (id, name, description, is_active)
    ON TABLE public.training_programs TO mmsu_schedule_admin_function_owner;

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
    is_public boolean,
    assigned_coach_id uuid,
    assigned_coach_name text
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
           s.is_public,
           a.coach_id,
           a.coach_name
      FROM public.training_sessions AS s
      JOIN public.training_programs AS p ON p.id = s.program_id
      LEFT JOIN public.coach_session_assignments AS a ON a.session_id = s.id
      LEFT JOIN LATERAL (
          SELECT pg_catalog.count(*)::bigint AS confirmed_booking_count
            FROM public.bookings AS b
           WHERE b.session_id = s.id
             AND b.status = 'confirmed'
      ) AS cb ON true
     ORDER BY s.starts_at DESC;
$function$;

CREATE OR REPLACE FUNCTION public.admin_assign_training_session_coach(
    p_session_id uuid,
    p_coach_id uuid
)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
AS $function$
DECLARE
    v_coach_name text;
BEGIN
    IF p_session_id IS NULL THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Session is required';
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM public.training_sessions AS s WHERE s.id = p_session_id
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'P0002', MESSAGE = 'Training session not found';
    END IF;

    IF p_coach_id IS NULL THEN
        DELETE FROM public.coach_session_assignments
         WHERE session_id = p_session_id;
        RETURN true;
    END IF;

    SELECT ca.coach_name
      INTO v_coach_name
      FROM public.coach_availability AS ca
     WHERE ca.coach_id = p_coach_id
     ORDER BY ca.created_at DESC
     LIMIT 1;

    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ004', MESSAGE = 'Coach has not submitted availability';
    END IF;

    INSERT INTO public.coach_session_assignments (session_id, coach_id, coach_name)
    VALUES (p_session_id, p_coach_id, v_coach_name)
    ON CONFLICT (session_id) DO UPDATE
        SET coach_id = EXCLUDED.coach_id,
            coach_name = EXCLUDED.coach_name;

    RETURN true;
END;
$function$;

CREATE OR REPLACE FUNCTION public.coach_list_assigned_training_sessions(
    p_coach_id uuid
)
RETURNS TABLE (
    id uuid,
    program_name text,
    program_description text,
    starts_at timestamptz,
    ends_at timestamptz,
    location text,
    status text,
    is_public boolean
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = ''
AS $function$
    SELECT s.id,
           p.name,
           p.description,
           s.starts_at,
           s.ends_at,
           s.location,
           s.status,
           s.is_public
      FROM public.coach_session_assignments AS a
      JOIN public.training_sessions AS s ON s.id = a.session_id
      JOIN public.training_programs AS p ON p.id = s.program_id
     WHERE p_coach_id IS NOT NULL
       AND a.coach_id = p_coach_id
     ORDER BY s.starts_at ASC;
$function$;

REVOKE ALL ON FUNCTION public.admin_list_training_sessions()
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader;
REVOKE ALL ON FUNCTION public.admin_assign_training_session_coach(uuid, uuid)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_api;
REVOKE ALL ON FUNCTION public.coach_list_assigned_training_sessions(uuid)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader;

GRANT EXECUTE ON FUNCTION public.admin_list_training_sessions()
    TO mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.admin_assign_training_session_coach(uuid, uuid)
    TO mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.coach_list_assigned_training_sessions(uuid)
    TO mmsu_booking_api;

GRANT CREATE ON SCHEMA public TO mmsu_schedule_admin_function_owner;
ALTER TABLE public.coach_session_assignments OWNER TO mmsu_schedule_admin_function_owner;
ALTER FUNCTION public.admin_list_training_sessions()
    OWNER TO mmsu_schedule_admin_function_owner;
ALTER FUNCTION public.admin_assign_training_session_coach(uuid, uuid)
    OWNER TO mmsu_schedule_admin_function_owner;
ALTER FUNCTION public.coach_list_assigned_training_sessions(uuid)
    OWNER TO mmsu_schedule_admin_function_owner;
REVOKE CREATE ON SCHEMA public FROM mmsu_schedule_admin_function_owner;
REVOKE mmsu_schedule_admin_function_owner FROM CURRENT_USER;
