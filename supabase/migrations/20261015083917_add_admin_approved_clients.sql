
-- Expose confirmed bookings to the authenticated admin API through a
-- dedicated SECURITY DEFINER function. No direct table access is granted
-- to the API role.

CREATE OR REPLACE FUNCTION public.admin_list_approved_clients()
RETURNS TABLE (
    booking_id uuid,
    player_id uuid,
    player_name text,
    player_email text,
    player_phone text,
    session_id uuid,
    program_name text,
    starts_at timestamptz,
    ends_at timestamptz,
    location text,
    capacity integer,
    assigned_coach_id uuid,
    assigned_coach_name text,
    booking_created_at timestamptz
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = ''
AS $function$
    SELECT b.id,
           pl.id,
           pl.full_name,
           pl.email,
           pl.phone,
           s.id,
           p.name,
           s.starts_at,
           s.ends_at,
           s.location,
           s.capacity,
           a.coach_id,
           a.coach_name,
           b.created_at
      FROM public.bookings AS b
      JOIN public.players AS pl ON pl.id = b.player_id
      JOIN public.training_sessions AS s ON s.id = b.session_id
      JOIN public.training_programs AS p ON p.id = s.program_id
      LEFT JOIN public.coach_session_assignments AS a
        ON a.session_id = s.id
     WHERE b.status = 'confirmed'
     ORDER BY s.starts_at ASC, pl.full_name ASC, b.id ASC;
$function$;

REVOKE ALL ON FUNCTION public.admin_list_approved_clients()
    FROM PUBLIC, anon, authenticated, service_role,
         mmsu_schedule_reader, mmsu_booking_api;

GRANT EXECUTE ON FUNCTION public.admin_list_approved_clients()
    TO mmsu_booking_admin_api;


-- Allow the dedicated function owner to read only the player fields
-- returned by the admin approved-clients function.
GRANT SELECT (id, full_name, email, phone)
    ON TABLE public.players
    TO mmsu_schedule_admin_function_owner;

CREATE POLICY players_schedule_admin_select
    ON public.players
    FOR SELECT
    TO mmsu_schedule_admin_function_owner
    USING (true);


-- Temporarily grant schema CREATE so ownership can be transferred.
GRANT CREATE ON SCHEMA public TO mmsu_schedule_admin_function_owner;

-- Allow this migration role to transfer function ownership.
GRANT mmsu_schedule_admin_function_owner TO CURRENT_USER
    WITH ADMIN FALSE, INHERIT FALSE, SET TRUE;

ALTER FUNCTION public.admin_list_approved_clients()
    OWNER TO mmsu_schedule_admin_function_owner;

-- Remove the temporary privileges.
REVOKE CREATE ON SCHEMA public FROM mmsu_schedule_admin_function_owner;
REVOKE mmsu_schedule_admin_function_owner FROM CURRENT_USER;
