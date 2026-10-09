-- Least-privilege access for the server-side public training schedule API.
-- The login role and password are provisioned separately after this migration
-- is reviewed and applied. This group role deliberately has no memberships.
CREATE ROLE mmsu_schedule_reader
    WITH NOLOGIN
         NOSUPERUSER
         NOCREATEDB
         NOCREATEROLE
         NOINHERIT
         NOREPLICATION
         NOBYPASSRLS;

-- Supabase projects can have default privileges for PUBLIC, anon, and
-- authenticated. Remove broad access to these application tables before adding
-- the narrow reader grants below. PUBLIC's schema CREATE privilege, if present,
-- would be inherited by every role, so remove it as well.
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
REVOKE ALL PRIVILEGES ON TABLE
    public.training_programs,
    public.training_sessions,
    public.players,
    public.bookings
FROM PUBLIC, anon, authenticated;

-- The read-only schedule role must not be able to invoke the booking writer,
-- including through the default PUBLIC function EXECUTE grant.
REVOKE EXECUTE ON FUNCTION public.book_training_session(uuid, uuid, text)
    FROM PUBLIC, anon, authenticated;

-- The reader needs schema lookup plus SELECT on only the columns used by the
-- repository query and the row policies. It receives no player access and no
-- write, truncate, trigger, or grant-option privileges.
GRANT USAGE ON SCHEMA public TO mmsu_schedule_reader;

GRANT SELECT (id, name, description, is_active)
    ON TABLE public.training_programs TO mmsu_schedule_reader;

GRANT SELECT (
    id,
    program_id,
    starts_at,
    ends_at,
    location,
    capacity,
    status,
    is_public
)
    ON TABLE public.training_sessions TO mmsu_schedule_reader;

GRANT SELECT (session_id, status)
    ON TABLE public.bookings TO mmsu_schedule_reader;

-- Programs must be active for the public schedule.
CREATE POLICY training_programs_schedule_reader_select
    ON public.training_programs
    FOR SELECT
    TO mmsu_schedule_reader
    USING (is_active IS TRUE);

-- Closed sessions remain visible, matching the API contract. Cancelled,
-- private, and already-started sessions are hidden at the database boundary.
CREATE POLICY training_sessions_schedule_reader_select
    ON public.training_sessions
    FOR SELECT
    TO mmsu_schedule_reader
    USING (
        starts_at >= statement_timestamp()
        AND is_public IS TRUE
        AND status IN ('open', 'closed')
    );

-- A reader can count confirmed bookings only when their session and program
-- are visible under the same predicates. These policies do not refer back to
-- bookings, so the policy graph has no recursive dependency.
CREATE POLICY bookings_schedule_reader_select
    ON public.bookings
    FOR SELECT
    TO mmsu_schedule_reader
    USING (
        status = 'confirmed'
        AND EXISTS (
            SELECT 1
            FROM public.training_sessions AS s
            JOIN public.training_programs AS p ON p.id = s.program_id
            WHERE s.id = bookings.session_id
              AND s.starts_at >= statement_timestamp()
              AND s.is_public IS TRUE
              AND s.status IN ('open', 'closed')
              AND p.is_active IS TRUE
        )
    );

-- training_sessions and training_programs already have RLS enabled. Players
-- remains RLS-enabled and has no policy or privilege for this reader.
-- Do not grant this group role membership in postgres, service_role, or other
-- privileged roles. Supabase's existing service_role remains independently
-- privileged; it is not a member of this role and is not the API credential.
-- NOINHERIT prevents this group role from inheriting privileges if it is ever
-- made a member elsewhere. The eventual LOGIN role must be fresh/non-privileged
-- and inherit only membership in mmsu_schedule_reader.
