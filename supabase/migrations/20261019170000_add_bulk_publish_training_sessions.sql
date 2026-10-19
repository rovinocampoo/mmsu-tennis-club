-- Atomic bulk publication of hidden training sessions for a Manila day or month.
--
-- Admins can already publish sessions one at a time through
-- admin_update_training_session. Publishing a whole day or month by hand is slow
-- and error-prone, so this adds a single set-based function the admin schedule
-- can call once per confirmed preview.
--
-- Eligibility is intentionally conservative and enforced in SQL so the frontend
-- can never publish something the database would refuse:
--   * only rows that are not already published (is_public IS NOT TRUE),
--   * only rows in the still-open registration state (status = 'open').
-- Closed sessions are never reopened and cancelled sessions are never
-- reactivated. Rows outside the half-open [p_starts_at, p_ends_at) range are
-- untouched.
--
-- The update runs as one statement under the club-wide schedule advisory lock,
-- so the whole run either commits or rolls back: there is no partial-publish
-- state and therefore no per-row failures to reconcile. The returned JSON
-- reports how many were published and how many hidden sessions in range were
-- skipped because they were closed or cancelled.

GRANT mmsu_schedule_admin_function_owner TO CURRENT_USER
    WITH ADMIN FALSE, INHERIT FALSE, SET TRUE;
GRANT CREATE ON SCHEMA public TO mmsu_schedule_admin_function_owner;
SET ROLE mmsu_schedule_admin_function_owner;

CREATE OR REPLACE FUNCTION public.admin_publish_training_sessions(
    p_starts_at timestamptz,
    p_ends_at timestamptz
)
RETURNS jsonb
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
AS $function$
DECLARE
    v_published integer := 0;
    v_skipped integer := 0;
BEGIN
    PERFORM pg_catalog.pg_advisory_xact_lock(
        pg_catalog.hashtextextended('mmsu-training-schedule-write', 0)
    );

    IF p_starts_at IS NULL OR p_ends_at IS NULL OR p_starts_at >= p_ends_at THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid publication range';
    END IF;

    WITH eligible AS (
        UPDATE public.training_sessions AS s
           SET is_public = true,
               updated_at = pg_catalog.clock_timestamp()
         WHERE s.is_public IS NOT TRUE
           AND s.status = 'open'
           AND s.starts_at >= p_starts_at
           AND s.starts_at < p_ends_at
        RETURNING 1
    )
    SELECT pg_catalog.count(*) INTO v_published FROM eligible;

    SELECT pg_catalog.count(*) INTO v_skipped
      FROM public.training_sessions AS s
     WHERE s.is_public IS NOT TRUE
       AND s.status <> 'open'
       AND s.starts_at >= p_starts_at
       AND s.starts_at < p_ends_at;

    RETURN pg_catalog.jsonb_build_object(
        'published', v_published,
        'skipped', v_skipped,
        'failed', 0
    );
END;
$function$;

REVOKE ALL ON FUNCTION public.admin_publish_training_sessions(timestamptz, timestamptz)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.admin_publish_training_sessions(timestamptz, timestamptz)
    TO mmsu_booking_admin_api;

RESET ROLE;

REVOKE CREATE ON SCHEMA public FROM mmsu_schedule_admin_function_owner;
REVOKE mmsu_schedule_admin_function_owner FROM CURRENT_USER;
