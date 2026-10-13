-- Narrow server-side entry point for public bookings. The owner of this
-- SECURITY DEFINER function is a dedicated non-login role, not the API role.
CREATE ROLE mmsu_booking_function_owner
    WITH NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;

CREATE ROLE mmsu_booking_api
    WITH NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;

GRANT USAGE ON SCHEMA public TO mmsu_booking_function_owner;
GRANT USAGE ON SCHEMA public TO mmsu_booking_api;

GRANT SELECT (id, is_active),
      UPDATE (id)
    ON TABLE public.training_programs TO mmsu_booking_function_owner;
GRANT SELECT (id, program_id, starts_at, status, is_public, capacity),
      UPDATE (id)
    ON TABLE public.training_sessions TO mmsu_booking_function_owner;
GRANT SELECT (id, email),
      INSERT (full_name, email)
    ON TABLE public.players TO mmsu_booking_function_owner;
GRANT SELECT, INSERT (player_id, session_id, status, external_booking_reference)
    ON TABLE public.bookings TO mmsu_booking_function_owner;

CREATE TABLE public.booking_email_verifications (
    email text PRIMARY KEY CHECK (email = pg_catalog.lower(pg_catalog.btrim(email))),
    code_hash bytea,
    code_expires_at timestamptz,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 5),
    last_sent_at timestamptz NOT NULL,
    window_started_at timestamptz NOT NULL,
    sends_in_window integer NOT NULL CHECK (sends_in_window BETWEEN 1 AND 5),
    verified_token_hash bytea,
    verified_until timestamptz,
    CONSTRAINT booking_email_verifications_code_hash_length
        CHECK (code_hash IS NULL OR pg_catalog.octet_length(code_hash) = 32),
    CONSTRAINT booking_email_verifications_token_hash_length
        CHECK (verified_token_hash IS NULL OR pg_catalog.octet_length(verified_token_hash) = 32),
    CONSTRAINT booking_email_verifications_code_pair
        CHECK ((code_hash IS NULL) = (code_expires_at IS NULL)),
    CONSTRAINT booking_email_verifications_token_pair
        CHECK ((verified_token_hash IS NULL) = (verified_until IS NULL))
);
ALTER TABLE public.booking_email_verifications ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE public.booking_email_verifications
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_api;
GRANT SELECT, INSERT, UPDATE ON TABLE public.booking_email_verifications
    TO mmsu_booking_function_owner;

CREATE POLICY booking_email_verifications_function_owner_all
    ON public.booking_email_verifications
    FOR ALL
    TO mmsu_booking_function_owner
    USING (true)
    WITH CHECK (true);

CREATE POLICY training_programs_booking_function_select
    ON public.training_programs
    FOR SELECT
    TO mmsu_booking_function_owner
    USING (is_active IS TRUE);

CREATE POLICY training_programs_booking_function_update
    ON public.training_programs
    FOR UPDATE
    TO mmsu_booking_function_owner
    USING (is_active IS TRUE);

CREATE POLICY training_sessions_booking_function_select
    ON public.training_sessions
    FOR SELECT
    TO mmsu_booking_function_owner
    USING (
        is_public IS TRUE
        AND status = 'open'
        AND starts_at > pg_catalog.statement_timestamp()
        AND EXISTS (
            SELECT 1
            FROM public.training_programs AS p
            WHERE p.id = training_sessions.program_id
              AND p.is_active IS TRUE
        )
    );

CREATE POLICY training_sessions_booking_function_update
    ON public.training_sessions
    FOR UPDATE
    TO mmsu_booking_function_owner
    USING (
        is_public IS TRUE
        AND status = 'open'
        AND starts_at > pg_catalog.statement_timestamp()
        AND EXISTS (
            SELECT 1
            FROM public.training_programs AS p
            WHERE p.id = training_sessions.program_id
              AND p.is_active IS TRUE
        )
    );

CREATE POLICY players_booking_function_select
    ON public.players
    FOR SELECT
    TO mmsu_booking_function_owner
    USING (true);

CREATE POLICY players_booking_function_insert
    ON public.players
    FOR INSERT
    TO mmsu_booking_function_owner
    WITH CHECK (true);

CREATE POLICY bookings_booking_function_select
    ON public.bookings
    FOR SELECT
    TO mmsu_booking_function_owner
    USING (
        status = 'confirmed'
        AND EXISTS (
            SELECT 1
            FROM public.training_sessions AS s
            JOIN public.training_programs AS p ON p.id = s.program_id
            WHERE s.id = bookings.session_id
              AND s.is_public IS TRUE
              AND s.status = 'open'
              AND s.starts_at > pg_catalog.statement_timestamp()
              AND p.is_active IS TRUE
        )
    );

CREATE POLICY bookings_booking_function_insert
    ON public.bookings
    FOR INSERT
    TO mmsu_booking_function_owner
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM public.training_sessions AS s
            JOIN public.training_programs AS p ON p.id = s.program_id
            WHERE s.id = bookings.session_id
              AND s.is_public IS TRUE
              AND s.status = 'open'
              AND s.starts_at > pg_catalog.statement_timestamp()
              AND p.is_active IS TRUE
        )
    );

CREATE OR REPLACE FUNCTION public.request_booking_email_verification(
    p_email text,
    p_code_hash bytea
)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
AS $function$
DECLARE
    v_email text;
    v_now timestamptz;
    v_last_sent_at timestamptz;
    v_window_started_at timestamptz;
    v_sends_in_window integer;
BEGIN
    v_email := NULLIF(pg_catalog.lower(pg_catalog.btrim(p_email)), '');
    IF v_email IS NULL OR pg_catalog.strpos(v_email, '@') < 2
       OR p_code_hash IS NULL OR pg_catalog.octet_length(p_code_hash) <> 32 THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid verification request';
    END IF;

    PERFORM pg_catalog.pg_advisory_xact_lock(
        pg_catalog.hashtextextended('booking-email:' || v_email, 0)
    );
    v_now := pg_catalog.clock_timestamp();
    SELECT verification.last_sent_at, verification.window_started_at,
           verification.sends_in_window
      INTO v_last_sent_at, v_window_started_at, v_sends_in_window
      FROM public.booking_email_verifications AS verification
     WHERE verification.email = v_email
     FOR UPDATE;

    IF NOT FOUND THEN
        INSERT INTO public.booking_email_verifications (
            email, code_hash, code_expires_at, attempts, last_sent_at,
            window_started_at, sends_in_window
        )
        VALUES (v_email, p_code_hash, v_now + INTERVAL '10 minutes', 0, v_now, v_now, 1);
        RETURN true;
    END IF;

    IF v_last_sent_at > v_now - INTERVAL '1 minute' THEN
        RETURN false;
    END IF;
    IF v_window_started_at <= v_now - INTERVAL '1 hour' THEN
        v_window_started_at := v_now;
        v_sends_in_window := 0;
    ELSIF v_sends_in_window >= 5 THEN
        RETURN false;
    END IF;

    UPDATE public.booking_email_verifications
       SET code_hash = p_code_hash,
           code_expires_at = v_now + INTERVAL '10 minutes',
           attempts = 0,
           last_sent_at = v_now,
           window_started_at = v_window_started_at,
           sends_in_window = v_sends_in_window + 1,
           verified_token_hash = NULL,
           verified_until = NULL
     WHERE email = v_email;
    RETURN true;
END;
$function$;

CREATE OR REPLACE FUNCTION public.confirm_booking_email_verification(
    p_email text,
    p_code_hash bytea,
    p_token_hash bytea
)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
AS $function$
DECLARE
    v_email text;
    v_code_hash bytea;
    v_expires_at timestamptz;
    v_attempts integer;
    v_now timestamptz;
BEGIN
    v_email := NULLIF(pg_catalog.lower(pg_catalog.btrim(p_email)), '');
    IF v_email IS NULL OR pg_catalog.strpos(v_email, '@') < 2
       OR p_code_hash IS NULL OR pg_catalog.octet_length(p_code_hash) <> 32
       OR p_token_hash IS NULL OR pg_catalog.octet_length(p_token_hash) <> 32 THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid verification request';
    END IF;

    SELECT verification.code_hash, verification.code_expires_at, verification.attempts
      INTO v_code_hash, v_expires_at, v_attempts
      FROM public.booking_email_verifications AS verification
     WHERE verification.email = v_email
     FOR UPDATE;
    v_now := pg_catalog.clock_timestamp();
    IF NOT FOUND OR v_code_hash IS NULL
       OR v_expires_at <= v_now
       OR v_attempts >= 5 THEN
        RETURN false;
    END IF;
    IF v_code_hash <> p_code_hash THEN
        UPDATE public.booking_email_verifications
           SET attempts = attempts + 1,
               code_hash = CASE WHEN attempts + 1 >= 5 THEN NULL ELSE code_hash END,
               code_expires_at = CASE WHEN attempts + 1 >= 5 THEN NULL ELSE code_expires_at END
         WHERE email = v_email;
        RETURN false;
    END IF;

    UPDATE public.booking_email_verifications
       SET code_hash = NULL,
           code_expires_at = NULL,
           attempts = 0,
           verified_token_hash = p_token_hash,
           verified_until = v_now + INTERVAL '10 minutes'
     WHERE email = v_email;
    RETURN true;
END;
$function$;

CREATE OR REPLACE FUNCTION public.book_public_training_session(
    p_session_id uuid,
    p_full_name text,
    p_email text,
    p_verification_token_hash bytea
)
RETURNS uuid
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
AS $function$
DECLARE
    v_player_id uuid;
    v_player_ids uuid[];
    v_booking public.bookings;
    v_program_id uuid;
    v_locked_program_id uuid;
    v_email text;
BEGIN
    v_email := NULLIF(pg_catalog.lower(pg_catalog.btrim(p_email)), '');
    IF p_full_name IS NULL OR pg_catalog.btrim(p_full_name) = ''
       OR pg_catalog.char_length(pg_catalog.btrim(p_full_name)) > 200
       OR v_email IS NULL
       OR p_verification_token_hash IS NULL
       OR pg_catalog.octet_length(p_verification_token_hash) <> 32 THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid booking details';
    END IF;

    -- Lock program before session, matching program deactivation workflows that
    -- may subsequently inspect/update sessions. Revalidate session eligibility
    -- while holding the program lock so deactivation cannot race this booking.
    SELECT s.program_id INTO v_program_id
      FROM public.training_sessions AS s
     WHERE s.id = p_session_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable';
    END IF;
    PERFORM 1
      FROM public.training_programs AS p
     WHERE p.id = v_program_id
       AND p.is_active IS TRUE
     FOR SHARE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable';
    END IF;
    SELECT s.program_id INTO v_locked_program_id
      FROM public.training_sessions AS s
     WHERE s.id = p_session_id
       AND s.status = 'open'
       AND s.is_public IS TRUE
       AND s.starts_at > pg_catalog.clock_timestamp()
     FOR UPDATE OF s;
    IF NOT FOUND OR v_locked_program_id <> v_program_id THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable';
    END IF;

    -- Email is the only verified identity in this release. Phone numbers are
    -- not accepted or used for player matching until SMS verification exists.
    PERFORM pg_catalog.pg_advisory_xact_lock(
        pg_catalog.hashtextextended('email:' || v_email, 0)
    );
    UPDATE public.booking_email_verifications
       SET verified_token_hash = NULL,
           verified_until = NULL
     WHERE email = v_email
       AND verified_token_hash = p_verification_token_hash
       AND verified_until > pg_catalog.clock_timestamp();
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ005', MESSAGE = 'Email verification is missing or expired';
    END IF;

    SELECT pg_catalog.array_agg(DISTINCT pl.id) INTO v_player_ids
      FROM public.players AS pl
     WHERE pg_catalog.lower(pg_catalog.btrim(pl.email)) = v_email;

    IF pg_catalog.cardinality(v_player_ids) > 1 THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ004', MESSAGE = 'Contact details identify different players';
    ELSIF pg_catalog.cardinality(v_player_ids) = 1 THEN
        v_player_id := v_player_ids[1];
    END IF;

    IF v_player_id IS NULL THEN
        INSERT INTO public.players (full_name, email)
        VALUES (pg_catalog.btrim(p_full_name), v_email)
        RETURNING id INTO v_player_id;
    END IF;

    v_booking := public.book_training_session(v_player_id, p_session_id, NULL);
    RETURN v_booking.id;
END;
$function$;

REVOKE ALL ON FUNCTION public.request_booking_email_verification(text, bytea)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader;
REVOKE ALL ON FUNCTION public.confirm_booking_email_verification(text, bytea, bytea)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader;
REVOKE ALL ON FUNCTION public.book_public_training_session(uuid, text, text, bytea)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader;
GRANT EXECUTE ON FUNCTION public.request_booking_email_verification(text, bytea) TO mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.confirm_booking_email_verification(text, bytea, bytea) TO mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.book_public_training_session(uuid, text, text, bytea) TO mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.book_training_session(uuid, uuid, text) TO mmsu_booking_function_owner;

-- Transfer ownership while the migrator can SET ROLE to the dedicated owner.
-- The temporary membership and schema CREATE privilege are removed immediately.
GRANT mmsu_booking_function_owner TO CURRENT_USER;
GRANT CREATE ON SCHEMA public TO mmsu_booking_function_owner;
ALTER FUNCTION public.request_booking_email_verification(text, bytea)
    OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.confirm_booking_email_verification(text, bytea, bytea)
    OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.book_public_training_session(uuid, text, text, bytea)
    OWNER TO mmsu_booking_function_owner;
REVOKE CREATE ON SCHEMA public FROM mmsu_booking_function_owner;
REVOKE mmsu_booking_function_owner FROM CURRENT_USER;
