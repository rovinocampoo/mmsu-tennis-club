-- Keep program management behind narrowly scoped SECURITY DEFINER functions.
-- The existing schedule-admin owner receives only the columns required here.
GRANT SELECT (id, name, description, is_active),
      INSERT (name, description),
      UPDATE (name, description, is_active)
    ON TABLE public.training_programs TO mmsu_schedule_admin_function_owner;

CREATE POLICY training_programs_schedule_admin_insert
    ON public.training_programs
    FOR INSERT TO mmsu_schedule_admin_function_owner
    WITH CHECK (true);
CREATE POLICY training_programs_schedule_admin_update
    ON public.training_programs
    FOR UPDATE TO mmsu_schedule_admin_function_owner
    USING (true) WITH CHECK (true);

GRANT mmsu_schedule_admin_function_owner TO CURRENT_USER
    WITH ADMIN FALSE, INHERIT FALSE, SET TRUE;
GRANT CREATE ON SCHEMA public TO mmsu_schedule_admin_function_owner;
SET ROLE mmsu_schedule_admin_function_owner;

CREATE OR REPLACE FUNCTION public.get_training_program_name(p_program_id text)
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
     LIMIT 1
$function$;

CREATE OR REPLACE FUNCTION public.admin_list_training_program_catalog()
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
     ORDER BY tp.is_active DESC, tp.name ASC;
$function$;

CREATE OR REPLACE FUNCTION public.admin_create_training_program(
    p_name text,
    p_description text
)
RETURNS uuid
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
AS $function$
DECLARE
    v_id uuid;
BEGIN
    IF p_name IS NULL OR pg_catalog.btrim(p_name) = '' THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Training program name is required';
    END IF;

    INSERT INTO public.training_programs (name, description)
    VALUES (pg_catalog.btrim(p_name), NULLIF(pg_catalog.btrim(p_description), ''))
    RETURNING id INTO v_id;

    RETURN v_id;
END;
$function$;

CREATE OR REPLACE FUNCTION public.admin_update_training_program(
    p_id uuid,
    p_name text,
    p_description text,
    p_is_active boolean
)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
AS $function$
BEGIN
    IF p_id IS NULL OR p_name IS NULL OR pg_catalog.btrim(p_name) = '' OR p_is_active IS NULL THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Training program update is invalid';
    END IF;

    UPDATE public.training_programs
       SET name = pg_catalog.btrim(p_name),
           description = NULLIF(pg_catalog.btrim(p_description), ''),
           is_active = p_is_active
     WHERE id = p_id;

    RETURN FOUND;
END;
$function$;

REVOKE ALL ON FUNCTION public.admin_list_training_program_catalog()
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_api, mmsu_booking_admin_api;
REVOKE ALL ON FUNCTION public.admin_create_training_program(text, text)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_api, mmsu_booking_admin_api;
REVOKE ALL ON FUNCTION public.admin_update_training_program(uuid, text, text, boolean)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_api, mmsu_booking_admin_api;
REVOKE ALL ON FUNCTION public.get_training_program_name(text)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_admin_api, mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.admin_list_training_program_catalog()
    TO mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.admin_create_training_program(text, text)
    TO mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.admin_update_training_program(uuid, text, text, boolean)
    TO mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.get_training_program_name(text)
    TO mmsu_booking_api;

RESET ROLE;
REVOKE CREATE ON SCHEMA public FROM mmsu_schedule_admin_function_owner;
REVOKE mmsu_schedule_admin_function_owner FROM CURRENT_USER;
