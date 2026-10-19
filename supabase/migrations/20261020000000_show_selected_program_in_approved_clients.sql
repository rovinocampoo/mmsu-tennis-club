-- Preserve the scheduled session's program while also exposing the separate
-- optional program selected on the original booking request.
GRANT mmsu_schedule_admin_function_owner TO CURRENT_USER
    WITH ADMIN FALSE, INHERIT FALSE, SET TRUE;
GRANT CREATE ON SCHEMA public TO mmsu_schedule_admin_function_owner;
SET ROLE mmsu_schedule_admin_function_owner;

DROP FUNCTION public.admin_list_approved_clients();

CREATE FUNCTION public.admin_list_approved_clients()
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
    booking_created_at timestamptz,
    participant_names jsonb,
    selected_training_program_name text
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
           session_program.name,
           s.starts_at,
           s.ends_at,
           s.location,
           s.capacity,
           a.coach_id,
           a.coach_name,
           b.created_at,
           COALESCE(roster.participant_names, '[]'::jsonb),
           COALESCE(selected_program.name, '')
      FROM public.bookings AS b
      JOIN public.players AS pl ON pl.id = b.player_id
      JOIN public.training_sessions AS s ON s.id = b.session_id
      JOIN public.training_programs AS session_program ON session_program.id = s.program_id
      LEFT JOIN public.coach_session_assignments AS a
        ON a.session_id = s.id
      LEFT JOIN LATERAL (
          SELECT pg_catalog.jsonb_agg(bp.full_name ORDER BY bp.position, bp.full_name) AS participant_names
            FROM public.booking_participants AS bp
           WHERE bp.booking_id = b.id
      ) AS roster ON true
      LEFT JOIN public.booking_requests AS br
        ON b.external_booking_reference = 'request:' || br.id::text
        OR b.external_booking_reference = 'request:' || br.id::text || ':' || b.session_id::text
      LEFT JOIN public.training_programs AS selected_program
        ON selected_program.id::text = br.training_focus
     WHERE b.status = 'confirmed'
     ORDER BY s.starts_at ASC, pl.full_name ASC, b.id ASC;
$function$;

REVOKE ALL ON FUNCTION public.admin_list_approved_clients()
    FROM PUBLIC, anon, authenticated, service_role,
         mmsu_schedule_reader, mmsu_booking_api, mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.admin_list_approved_clients()
    TO mmsu_booking_admin_api;

RESET ROLE;
REVOKE CREATE ON SCHEMA public FROM mmsu_schedule_admin_function_owner;
REVOKE mmsu_schedule_admin_function_owner FROM CURRENT_USER;
