-- Restore the write-concurrency and coach-overlap guards on session coach
-- assignment.
--
-- 20261019150000 repointed coach assignment at the new coach directory but, in
-- doing so, dropped the club-wide schedule advisory lock, the row lock on the
-- target session, and the coach overlap-conflict check that previously lived in
-- 20261016093515_multi_slot_training_bookings.sql. Without them two concurrent
-- requests could assign the same coach to overlapping sessions, or assign a
-- coach to a session whose times were being edited at the same time.
--
-- This migration recreates the function against the directory (unchanged
-- signature and semantics) with those guards restored, and additionally refuses
-- to assign a coach to a cancelled session. Unassigning a coach remains allowed
-- regardless of session status.
--
-- 20261019150000 is already applied in environments that ran it, so the fix is
-- delivered as a new forward migration rather than by editing that file.

GRANT mmsu_schedule_admin_function_owner TO CURRENT_USER
    WITH ADMIN FALSE, INHERIT FALSE, SET TRUE;
GRANT CREATE ON SCHEMA public TO mmsu_schedule_admin_function_owner;
SET ROLE mmsu_schedule_admin_function_owner;

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
    v_status text;
    v_starts_at timestamptz;
    v_ends_at timestamptz;
BEGIN
    PERFORM pg_catalog.pg_advisory_xact_lock(
        pg_catalog.hashtextextended('mmsu-training-schedule-write', 0)
    );

    IF p_session_id IS NULL THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Session is required';
    END IF;

    SELECT s.status, s.starts_at, s.ends_at
      INTO v_status, v_starts_at, v_ends_at
      FROM public.training_sessions AS s
     WHERE s.id = p_session_id
     FOR UPDATE;

    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'P0002', MESSAGE = 'Training session not found';
    END IF;

    IF p_coach_id IS NULL THEN
        DELETE FROM public.coach_session_assignments
         WHERE session_id = p_session_id;
        RETURN true;
    END IF;

    IF v_status = 'cancelled' THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Cancelled sessions cannot be assigned a coach';
    END IF;

    SELECT c.full_name
      INTO v_coach_name
      FROM public.coaches AS c
     WHERE c.id = p_coach_id
       AND c.is_active IS TRUE;

    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ004', MESSAGE = 'Coach is unavailable';
    END IF;

    IF EXISTS (
        SELECT 1
          FROM public.coach_session_assignments AS a
          JOIN public.training_sessions AS s ON s.id = a.session_id
         WHERE a.coach_id = p_coach_id
           AND s.id <> p_session_id
           AND s.status <> 'cancelled'
           AND s.starts_at < v_ends_at
           AND s.ends_at > v_starts_at
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ007', MESSAGE = 'Coach schedule conflict';
    END IF;

    INSERT INTO public.coach_session_assignments (session_id, coach_id, coach_name)
    VALUES (p_session_id, p_coach_id, v_coach_name)
    ON CONFLICT (session_id) DO UPDATE
        SET coach_id = EXCLUDED.coach_id,
            coach_name = EXCLUDED.coach_name;

    RETURN true;
END;
$function$;

REVOKE ALL ON FUNCTION public.admin_assign_training_session_coach(uuid, uuid)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.admin_assign_training_session_coach(uuid, uuid)
    TO mmsu_booking_admin_api;

RESET ROLE;

REVOKE CREATE ON SCHEMA public FROM mmsu_schedule_admin_function_owner;
REVOKE mmsu_schedule_admin_function_owner FROM CURRENT_USER;
