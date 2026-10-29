GRANT mmsu_booking_function_owner TO CURRENT_USER
    WITH ADMIN FALSE, INHERIT FALSE, SET TRUE;
GRANT CREATE ON SCHEMA public TO mmsu_booking_function_owner;
SET ROLE mmsu_booking_function_owner;

CREATE FUNCTION public.list_booking_calendar_events(p_request_id uuid)
RETURNS TABLE(event_id text, request_id uuid, request_reference text, participant_name text,
              program_name text, starts_at timestamptz, ends_at timestamptz, location text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = ''
AS $function$
    SELECT 'mmsu' || pg_catalog.replace(br.id::text, '-', '') ||
               CASE WHEN pg_catalog.row_number() OVER (ORDER BY rs.decided_at, rs.session_id) = 1
                    THEN '' ELSE pg_catalog.replace(rs.session_id::text, '-', '') END,
           br.id, COALESCE(br.public_reference, 'MMSU-' || pg_catalog.upper(br.id::text)),
           br.full_name, p.name, s.starts_at, s.ends_at, s.location
      FROM public.booking_requests br
      JOIN public.booking_request_sessions rs ON rs.request_id = br.id AND rs.status = 'approved'
      JOIN public.training_sessions s ON s.id = rs.session_id
      JOIN public.training_programs p ON p.id = s.program_id
     WHERE br.id = p_request_id AND br.status IN ('approved', 'partially_decided')
     ORDER BY rs.decided_at, rs.session_id
$function$;

REVOKE ALL ON FUNCTION public.list_booking_calendar_events(uuid)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.list_booking_calendar_events(uuid) TO mmsu_booking_admin_api;
ALTER FUNCTION public.list_booking_calendar_events(uuid) OWNER TO mmsu_booking_function_owner;

RESET ROLE;
REVOKE CREATE ON SCHEMA public FROM mmsu_booking_function_owner;
REVOKE mmsu_booking_function_owner FROM CURRENT_USER;
