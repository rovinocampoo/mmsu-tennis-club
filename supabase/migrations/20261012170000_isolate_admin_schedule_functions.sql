-- Keep broad schedule-management reads out of the function owner used by public booking APIs.
CREATE ROLE mmsu_schedule_admin_function_owner
    WITH NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;

GRANT USAGE ON SCHEMA public TO mmsu_schedule_admin_function_owner;
GRANT SELECT (id, name, is_active) ON TABLE public.training_programs TO mmsu_schedule_admin_function_owner;
GRANT SELECT (id, program_id, starts_at, ends_at, location, capacity, status, notes, is_public),
      INSERT,
      UPDATE (status, notes, is_public, location, starts_at, ends_at, capacity)
    ON TABLE public.training_sessions TO mmsu_schedule_admin_function_owner;
GRANT SELECT ON TABLE public.bookings TO mmsu_schedule_admin_function_owner;

CREATE POLICY training_programs_schedule_admin_select ON public.training_programs
    FOR SELECT TO mmsu_schedule_admin_function_owner USING (true);
CREATE POLICY training_sessions_schedule_admin_manage ON public.training_sessions
    FOR ALL TO mmsu_schedule_admin_function_owner USING (true) WITH CHECK (true);
CREATE POLICY bookings_schedule_admin_select ON public.bookings
    FOR SELECT TO mmsu_schedule_admin_function_owner USING (true);

GRANT mmsu_schedule_admin_function_owner TO CURRENT_USER
    WITH ADMIN FALSE, INHERIT FALSE, SET TRUE;
GRANT CREATE ON SCHEMA public TO mmsu_schedule_admin_function_owner;
GRANT mmsu_schedule_admin_function_owner TO mmsu_booking_function_owner
    WITH ADMIN FALSE, INHERIT FALSE, SET TRUE;
GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET TRUE;
SET ROLE mmsu_booking_function_owner;
ALTER FUNCTION public.admin_create_training_session(uuid, timestamptz, timestamptz, text, integer, text, boolean)
    OWNER TO mmsu_schedule_admin_function_owner;
ALTER FUNCTION public.admin_update_training_session(uuid, text, text, boolean)
    OWNER TO mmsu_schedule_admin_function_owner;
ALTER FUNCTION public.admin_list_training_sessions()
    OWNER TO mmsu_schedule_admin_function_owner;
RESET ROLE;
REVOKE CREATE ON SCHEMA public FROM mmsu_schedule_admin_function_owner;
REVOKE mmsu_schedule_admin_function_owner FROM mmsu_booking_function_owner;
GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET FALSE;
REVOKE mmsu_schedule_admin_function_owner FROM CURRENT_USER;
