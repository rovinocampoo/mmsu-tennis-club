-- Keep optional program selections tied to the active database catalogue.
-- The trigger runs inside create_public_booking_request_with_participants_token,
-- whose dedicated SECURITY DEFINER owner has only the required read privileges.
CREATE OR REPLACE FUNCTION public.validate_booking_training_program()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
AS $function$
DECLARE
    v_program_id uuid;
BEGIN
    IF NEW.training_focus IS NULL OR pg_catalog.btrim(NEW.training_focus) = '' THEN
        NEW.training_focus := NULL;
        RETURN NEW;
    END IF;

    BEGIN
        v_program_id := NEW.training_focus::uuid;
    EXCEPTION WHEN invalid_text_representation THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ011', MESSAGE = 'Training program unavailable';
    END;

    PERFORM 1
      FROM public.training_programs AS p
     WHERE p.id = v_program_id AND p.is_active IS TRUE
     FOR SHARE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ011', MESSAGE = 'Training program unavailable';
    END IF;
    NEW.training_focus := v_program_id::text;
    RETURN NEW;
END
$function$;

GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET TRUE;
GRANT CREATE ON SCHEMA public TO mmsu_booking_function_owner;
ALTER FUNCTION public.validate_booking_training_program() OWNER TO mmsu_booking_function_owner;
REVOKE ALL ON FUNCTION public.validate_booking_training_program()
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_api;
REVOKE CREATE ON SCHEMA public FROM mmsu_booking_function_owner;
GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET FALSE;

CREATE TRIGGER booking_requests_validate_training_program
    BEFORE INSERT OR UPDATE OF training_focus ON public.booking_requests
    FOR EACH ROW EXECUTE FUNCTION public.validate_booking_training_program();
