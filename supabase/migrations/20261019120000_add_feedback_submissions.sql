-- Public suggestions and feedback submissions for the MMSU Tennis Club.
-- Submissions are write-only for the public booking API role and readable only
-- by the admin API role. This migration reuses the existing restricted roles
-- (mmsu_booking_api, mmsu_booking_admin_api, mmsu_booking_function_owner);
-- it does not create new roles or login accounts.

CREATE TABLE public.feedback_submissions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    category text NOT NULL CHECK (
        category IN ('suggestion', 'feedback', 'facility', 'training', 'scheduling', 'other')
    ),
    message text NOT NULL CHECK (
        pg_catalog.btrim(message) <> '' AND pg_catalog.char_length(pg_catalog.btrim(message)) <= 2000
    ),
    name text,
    email text,
    status text NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'reviewed', 'archived')),
    reviewed_by uuid,
    reviewed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT pg_catalog.clock_timestamp(),
    CONSTRAINT feedback_submissions_name_length CHECK (
        name IS NULL OR (pg_catalog.btrim(name) <> '' AND pg_catalog.char_length(pg_catalog.btrim(name)) <= 200)
    ),
    CONSTRAINT feedback_submissions_email_length CHECK (
        email IS NULL OR (
            pg_catalog.btrim(email) <> ''
            AND pg_catalog.char_length(pg_catalog.btrim(email)) <= 254
            AND pg_catalog.strpos(pg_catalog.lower(pg_catalog.btrim(email)), '@') >= 2
        )
    ),
    CONSTRAINT feedback_submissions_review_consistent CHECK (
        (status = 'new' AND reviewed_by IS NULL AND reviewed_at IS NULL)
        OR (status IN ('reviewed', 'archived') AND reviewed_by IS NOT NULL AND reviewed_at IS NOT NULL)
    )
);

CREATE INDEX feedback_submissions_created_at_idx
    ON public.feedback_submissions (created_at DESC, id);
CREATE INDEX feedback_submissions_status_idx
    ON public.feedback_submissions (status, created_at DESC);

ALTER TABLE public.feedback_submissions ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE public.feedback_submissions
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_api, mmsu_booking_admin_api;
GRANT SELECT, INSERT, UPDATE ON TABLE public.feedback_submissions
    TO mmsu_booking_function_owner;
CREATE POLICY feedback_submissions_function_owner_all
    ON public.feedback_submissions
    FOR ALL TO mmsu_booking_function_owner USING (true) WITH CHECK (true);

-- Create the entry-point functions as the dedicated non-login owner so that
-- SECURITY DEFINER execution never runs as the API login or the migrator.
GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET TRUE;
GRANT CREATE ON SCHEMA public TO mmsu_booking_function_owner;
SET ROLE mmsu_booking_function_owner;

-- Public write path. Contact fields are optional; anonymous submissions are
-- accepted. Validation mirrors the Go handler so the database stays authoritative.
CREATE OR REPLACE FUNCTION public.submit_feedback(
    p_category text,
    p_message text,
    p_name text,
    p_email text
)
RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = ''
AS $function$
DECLARE
    v_category text := pg_catalog.lower(pg_catalog.btrim(COALESCE(p_category, '')));
    v_message text := pg_catalog.btrim(COALESCE(p_message, ''));
    v_name text := NULLIF(pg_catalog.btrim(COALESCE(p_name, '')), '');
    v_email text := NULLIF(pg_catalog.lower(pg_catalog.btrim(COALESCE(p_email, ''))), '');
    v_id uuid;
BEGIN
    IF v_category NOT IN ('suggestion', 'feedback', 'facility', 'training', 'scheduling', 'other')
       OR v_message = '' OR pg_catalog.char_length(v_message) > 2000
       OR (v_name IS NOT NULL AND pg_catalog.char_length(v_name) > 200)
       OR (v_email IS NOT NULL AND (pg_catalog.char_length(v_email) > 254 OR pg_catalog.strpos(v_email, '@') < 2)) THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid feedback submission';
    END IF;

    INSERT INTO public.feedback_submissions (category, message, name, email)
    VALUES (v_category, v_message, v_name, v_email)
    RETURNING id INTO v_id;

    RETURN v_id;
END;
$function$;

-- Admin read path. Returns submitted records only to the admin API role.
CREATE OR REPLACE FUNCTION public.admin_list_feedback_submissions()
RETURNS TABLE (
    id uuid,
    category text,
    message text,
    name text,
    email text,
    status text,
    created_at timestamptz,
    reviewed_at timestamptz,
    reviewed_by uuid
)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = ''
AS $function$
    SELECT f.id, f.category, f.message, f.name, f.email, f.status,
           f.created_at, f.reviewed_at, f.reviewed_by
      FROM public.feedback_submissions AS f
     ORDER BY f.created_at DESC, f.id
$function$;

-- Admin status update path. Marking a record 'new' clears its review metadata.
CREATE OR REPLACE FUNCTION public.admin_update_feedback_status(
    p_id uuid,
    p_status text,
    p_admin_id uuid
)
RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = ''
AS $function$
DECLARE
    v_status text := pg_catalog.lower(pg_catalog.btrim(COALESCE(p_status, '')));
BEGIN
    IF p_id IS NULL OR p_admin_id IS NULL
       OR v_status NOT IN ('new', 'reviewed', 'archived') THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid feedback status update';
    END IF;

    UPDATE public.feedback_submissions
       SET status = v_status,
           reviewed_by = CASE WHEN v_status = 'new' THEN NULL ELSE p_admin_id END,
           reviewed_at = CASE WHEN v_status = 'new' THEN NULL ELSE pg_catalog.clock_timestamp() END
     WHERE id = p_id;

    RETURN FOUND;
END;
$function$;

REVOKE ALL ON FUNCTION public.submit_feedback(text, text, text, text)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.submit_feedback(text, text, text, text) TO mmsu_booking_api;

REVOKE ALL ON FUNCTION public.admin_list_feedback_submissions()
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.admin_list_feedback_submissions() TO mmsu_booking_admin_api;

REVOKE ALL ON FUNCTION public.admin_update_feedback_status(uuid, text, uuid)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.admin_update_feedback_status(uuid, text, uuid) TO mmsu_booking_admin_api;

RESET ROLE;
REVOKE CREATE ON SCHEMA public FROM mmsu_booking_function_owner;
GRANT mmsu_booking_function_owner TO CURRENT_USER WITH SET FALSE;
