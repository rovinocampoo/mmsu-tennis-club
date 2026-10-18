-- Expose approved booking data only to the admin API role for calendar event creation.
GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET TRUE;
GRANT CREATE ON SCHEMA public TO mmsu_booking_function_owner;
SET ROLE mmsu_booking_function_owner;

CREATE OR REPLACE FUNCTION public.get_booking_calendar_event(p_request_id uuid)
RETURNS TABLE(request_id uuid, request_reference text, participant_name text, program_name text,
              starts_at timestamptz, ends_at timestamptz, location text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = ''
AS $function$
 SELECT br.id, COALESCE(br.public_reference, 'MMSU-' || pg_catalog.upper(br.id::text)),
        br.full_name, n.program_name, n.starts_at, n.ends_at, n.location
 FROM public.booking_requests br
 JOIN public.booking_approval_notifications n ON n.request_id=br.id
 WHERE br.id=p_request_id AND br.status='approved'
$function$;

REVOKE ALL ON FUNCTION public.get_booking_calendar_event(uuid)
 FROM PUBLIC, anon, authenticated, service_role, mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.get_booking_calendar_event(uuid) TO mmsu_booking_admin_api;
ALTER FUNCTION public.get_booking_calendar_event(uuid) OWNER TO mmsu_booking_function_owner;
RESET ROLE;
REVOKE CREATE ON SCHEMA public FROM mmsu_booking_function_owner;
GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET FALSE;
