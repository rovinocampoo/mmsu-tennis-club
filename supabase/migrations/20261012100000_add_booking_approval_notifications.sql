-- Persist an immutable approval-email snapshot and its delivery status.
-- The notification is prepared by a trigger inside the approval transaction;
-- the API sends it only after that transaction commits.

CREATE TABLE public.booking_approval_notifications (
    request_id uuid PRIMARY KEY
        REFERENCES public.booking_requests (id) ON DELETE RESTRICT,
    support_reference text NOT NULL,
    program_name text NOT NULL,
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    location text NOT NULL,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'sending', 'sent', 'failed', 'not_configured')),
    detail_code text
        CHECK (detail_code IS NULL OR detail_code IN (
            'missing_email', 'resend_not_configured', 'provider_error', 'status_save_error'
        )),
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    send_started_at timestamptz,
    accepted_at timestamptz,
    resend_email_id text,
    created_at timestamptz NOT NULL DEFAULT pg_catalog.clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT pg_catalog.clock_timestamp(),
    CONSTRAINT booking_approval_notifications_sending_started
        CHECK ((status = 'sending') = (send_started_at IS NOT NULL)),
    CONSTRAINT booking_approval_notifications_time_order CHECK (starts_at < ends_at)
);

ALTER TABLE public.booking_approval_notifications ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE public.booking_approval_notifications
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_api, mmsu_booking_admin_api;
GRANT SELECT, INSERT, UPDATE ON TABLE public.booking_approval_notifications
    TO mmsu_booking_function_owner;
CREATE POLICY booking_approval_notifications_function_owner_all
    ON public.booking_approval_notifications
    FOR ALL TO mmsu_booking_function_owner
    USING (true) WITH CHECK (true);

GRANT SELECT (name) ON TABLE public.training_programs TO mmsu_booking_function_owner;
GRANT SELECT (ends_at) ON TABLE public.training_sessions TO mmsu_booking_function_owner;

CREATE OR REPLACE FUNCTION public.record_booking_approval_notification()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
AS $function$
BEGIN
    IF NEW.status = 'approved' AND OLD.status IS DISTINCT FROM 'approved' THEN
        INSERT INTO public.booking_approval_notifications (
            request_id, support_reference, program_name, starts_at, ends_at,
            location, status, detail_code
        )
        SELECT NEW.id,
               'MMSU-' || pg_catalog.upper(NEW.id::text),
               p.name,
               s.starts_at,
               s.ends_at,
               s.location,
               CASE WHEN NULLIF(pg_catalog.btrim(NEW.email), '') IS NULL
                    THEN 'not_configured' ELSE 'pending' END,
               CASE WHEN NULLIF(pg_catalog.btrim(NEW.email), '') IS NULL
                    THEN 'missing_email' ELSE NULL END
          FROM public.training_sessions AS s
          JOIN public.training_programs AS p ON p.id = s.program_id
         WHERE s.id = NEW.session_id;

        IF NOT FOUND THEN
            RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable';
        END IF;
    END IF;
    RETURN NEW;
END;
$function$;

CREATE TRIGGER booking_requests_record_approval_notification
AFTER UPDATE OF status ON public.booking_requests
FOR EACH ROW
EXECUTE FUNCTION public.record_booking_approval_notification();

-- Existing approvals predate notification tracking. Record their state but do
-- not send retroactive mail automatically.
INSERT INTO public.booking_approval_notifications (
    request_id, support_reference, program_name, starts_at, ends_at,
    location, status, detail_code
)
SELECT br.id,
       'MMSU-' || pg_catalog.upper(br.id::text),
       p.name,
       s.starts_at,
       s.ends_at,
       s.location,
       'not_configured',
       CASE WHEN NULLIF(pg_catalog.btrim(br.email), '') IS NULL
            THEN 'missing_email' ELSE 'resend_not_configured' END
  FROM public.booking_requests AS br
  JOIN public.training_sessions AS s ON s.id = br.session_id
  JOIN public.training_programs AS p ON p.id = s.program_id
 WHERE br.status = 'approved'
ON CONFLICT (request_id) DO NOTHING;

CREATE OR REPLACE FUNCTION public.claim_booking_approval_notification(
    p_request_id uuid,
    p_resend_configured boolean
)
RETURNS TABLE (
    notification_status text,
    notification_detail text,
    recipient_email text,
    program_name text,
    starts_at timestamptz,
    ends_at timestamptz,
    location text,
    support_reference text,
    attempt_count integer,
    claimed boolean
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
AS $function$
DECLARE
    v_request_status text;
    v_email text;
    v_notification public.booking_approval_notifications;
BEGIN
    SELECT br.status, NULLIF(pg_catalog.btrim(br.email), '')
      INTO v_request_status, v_email
      FROM public.booking_requests AS br
     WHERE br.id = p_request_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ006', MESSAGE = 'Booking request not found';
    END IF;
    IF v_request_status <> 'approved' THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ003', MESSAGE = 'Booking request is not approved';
    END IF;

    SELECT n.* INTO v_notification
      FROM public.booking_approval_notifications AS n
     WHERE n.request_id = p_request_id
     FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ006', MESSAGE = 'Approval notification record not found';
    END IF;

    IF v_notification.status = 'sent' THEN
        RETURN QUERY SELECT v_notification.status, v_notification.detail_code, v_email,
            v_notification.program_name, v_notification.starts_at, v_notification.ends_at,
            v_notification.location, v_notification.support_reference,
            v_notification.attempt_count, false;
        RETURN;
    END IF;

    IF v_email IS NULL THEN
        UPDATE public.booking_approval_notifications AS n
           SET status = 'not_configured', detail_code = 'missing_email',
               send_started_at = NULL, updated_at = pg_catalog.clock_timestamp()
         WHERE n.request_id = p_request_id
        RETURNING n.* INTO v_notification;
        RETURN QUERY SELECT v_notification.status, v_notification.detail_code, NULL::text,
            v_notification.program_name, v_notification.starts_at, v_notification.ends_at,
            v_notification.location, v_notification.support_reference,
            v_notification.attempt_count, false;
        RETURN;
    END IF;

    IF NOT COALESCE(p_resend_configured, false) THEN
        UPDATE public.booking_approval_notifications AS n
           SET status = 'not_configured', detail_code = 'resend_not_configured',
               send_started_at = NULL, updated_at = pg_catalog.clock_timestamp()
         WHERE n.request_id = p_request_id
        RETURNING n.* INTO v_notification;
        RETURN QUERY SELECT v_notification.status, v_notification.detail_code, v_email,
            v_notification.program_name, v_notification.starts_at, v_notification.ends_at,
            v_notification.location, v_notification.support_reference,
            v_notification.attempt_count, false;
        RETURN;
    END IF;

    IF v_notification.status = 'sending'
       AND v_notification.send_started_at > pg_catalog.clock_timestamp() - INTERVAL '2 minutes' THEN
        RETURN QUERY SELECT v_notification.status, v_notification.detail_code, v_email,
            v_notification.program_name, v_notification.starts_at, v_notification.ends_at,
            v_notification.location, v_notification.support_reference,
            v_notification.attempt_count, false;
        RETURN;
    END IF;

    UPDATE public.booking_approval_notifications AS n
       SET status = 'sending', detail_code = NULL,
           attempt_count = n.attempt_count + 1,
           send_started_at = pg_catalog.clock_timestamp(),
           updated_at = pg_catalog.clock_timestamp()
     WHERE n.request_id = p_request_id
    RETURNING n.* INTO v_notification;

    RETURN QUERY SELECT v_notification.status, v_notification.detail_code, v_email,
        v_notification.program_name, v_notification.starts_at, v_notification.ends_at,
        v_notification.location, v_notification.support_reference,
        v_notification.attempt_count, true;
END;
$function$;

CREATE OR REPLACE FUNCTION public.complete_booking_approval_notification(
    p_request_id uuid,
    p_attempt_count integer,
    p_succeeded boolean,
    p_resend_email_id text
)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
AS $function$
DECLARE
    v_updated boolean := false;
BEGIN
    IF p_attempt_count < 1 OR (p_succeeded AND NULLIF(pg_catalog.btrim(p_resend_email_id), '') IS NULL) THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid notification result';
    END IF;

    UPDATE public.booking_approval_notifications AS n
       SET status = CASE WHEN p_succeeded THEN 'sent' ELSE 'failed' END,
           detail_code = CASE WHEN p_succeeded THEN NULL ELSE 'provider_error' END,
           send_started_at = NULL,
           accepted_at = CASE WHEN p_succeeded THEN pg_catalog.clock_timestamp() ELSE NULL END,
           resend_email_id = CASE WHEN p_succeeded THEN p_resend_email_id ELSE NULL END,
           updated_at = pg_catalog.clock_timestamp()
     WHERE n.request_id = p_request_id
       AND n.status = 'sending'
       AND n.attempt_count = p_attempt_count
    RETURNING true INTO v_updated;

    RETURN COALESCE(v_updated, false);
END;
$function$;

REVOKE ALL ON FUNCTION public.record_booking_approval_notification() FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_api, mmsu_booking_admin_api;
REVOKE ALL ON FUNCTION public.claim_booking_approval_notification(uuid, boolean) FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_api;
REVOKE ALL ON FUNCTION public.complete_booking_approval_notification(uuid, integer, boolean, text) FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.record_booking_approval_notification() TO mmsu_booking_function_owner;
GRANT EXECUTE ON FUNCTION public.claim_booking_approval_notification(uuid, boolean) TO mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.complete_booking_approval_notification(uuid, integer, boolean, text) TO mmsu_booking_admin_api;

GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET TRUE;
GRANT CREATE ON SCHEMA public TO mmsu_booking_function_owner;
ALTER TABLE public.booking_approval_notifications OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.record_booking_approval_notification() OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.claim_booking_approval_notification(uuid, boolean) OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.complete_booking_approval_notification(uuid, integer, boolean, text) OWNER TO mmsu_booking_function_owner;
REVOKE CREATE ON SCHEMA public FROM mmsu_booking_function_owner;
GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET FALSE;
