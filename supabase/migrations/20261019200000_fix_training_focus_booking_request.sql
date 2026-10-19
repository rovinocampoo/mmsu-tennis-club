-- Keep the existing booking validation and multi-slot/participant handling.
-- The seven-argument entry point delegates to the established five-argument
-- function, then atomically stores the optional training program selection.
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
    v_public_reference text;
BEGIN
    v_public_reference := public.create_public_booking_request_with_participants_token(
        p_session_ids,
        p_participants,
        p_email,
        p_phone,
        p_tracking_token_hash
    );

    UPDATE public.booking_requests
       SET training_focus = p_training_focus,
           training_focus_notes = p_training_focus_notes
     WHERE public_reference = v_public_reference;

    RETURN v_public_reference;
END
$function$;

GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET TRUE;
GRANT CREATE ON SCHEMA public TO mmsu_booking_function_owner;
ALTER FUNCTION public.create_public_booking_request_with_participants_token(jsonb, jsonb, text, text, bytea, text, text)
    OWNER TO mmsu_booking_function_owner;
REVOKE CREATE ON SCHEMA public FROM mmsu_booking_function_owner;
GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET FALSE;

REVOKE ALL ON FUNCTION public.create_public_booking_request_with_participants_token(jsonb, jsonb, text, text, bytea, text, text)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.create_public_booking_request_with_participants_token(jsonb, jsonb, text, text, bytea, text, text)
    TO mmsu_booking_api;
