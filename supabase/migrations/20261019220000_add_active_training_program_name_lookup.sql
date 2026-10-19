-- Allow public booking status to resolve a selected program name without
-- granting the public booking API direct access to training_programs.
GRANT mmsu_booking_function_owner TO CURRENT_USER
    WITH ADMIN FALSE, INHERIT FALSE, SET TRUE;
GRANT CREATE ON SCHEMA public TO mmsu_booking_function_owner;
SET ROLE mmsu_booking_function_owner;

CREATE OR REPLACE FUNCTION public.get_active_training_program_name(p_program_id text)
RETURNS text
LANGUAGE sql
STABLE
STRICT
SECURITY DEFINER
SET search_path = ''
AS $function$
    SELECT tp.name
      FROM public.training_programs AS tp
     WHERE tp.id::text = p_program_id
       AND tp.is_active IS TRUE
     LIMIT 1
$function$;

REVOKE ALL ON FUNCTION public.get_active_training_program_name(text)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_admin_api, mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.get_active_training_program_name(text)
    TO mmsu_booking_api;

RESET ROLE;
REVOKE CREATE ON SCHEMA public FROM mmsu_booking_function_owner;
REVOKE mmsu_booking_function_owner FROM CURRENT_USER;
