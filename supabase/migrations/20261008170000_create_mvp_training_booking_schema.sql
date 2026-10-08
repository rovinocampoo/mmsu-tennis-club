-- Initial MVP schema for reusable training programs, scheduled sessions, players,
-- and booking history. Times are stored as timestamptz; applications display them
-- in the club timezone (Asia/Manila).

CREATE TABLE public.training_programs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (btrim(name) <> ''),
    description text,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE public.training_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    program_id uuid NOT NULL
        REFERENCES public.training_programs (id) ON DELETE RESTRICT,
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    location text NOT NULL,
    capacity integer,
    status text NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'closed', 'cancelled')),
    notes text,
    is_public boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT training_sessions_time_order CHECK (starts_at < ends_at),
    CONSTRAINT training_sessions_positive_capacity CHECK (capacity IS NULL OR capacity > 0),
    CONSTRAINT training_sessions_location_nonempty CHECK (btrim(location) <> '')
);

CREATE TABLE public.players (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    full_name text NOT NULL CHECK (btrim(full_name) <> ''),
    email text,
    phone text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT players_contact_required CHECK (
        (email IS NOT NULL AND btrim(email) <> '')
        OR (phone IS NOT NULL AND btrim(phone) <> '')
    ),
    CONSTRAINT players_email_nonempty CHECK (email IS NULL OR btrim(email) <> ''),
    CONSTRAINT players_phone_nonempty CHECK (phone IS NULL OR btrim(phone) <> '')
);

COMMENT ON TABLE public.players IS
    'Player identity matching is application/service logic for now. Do not require globally unique email or phone values; family members may share contact details. A future Supabase Auth integration may add a nullable stable auth.users relationship once accounts are introduced.';

CREATE TABLE public.bookings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    player_id uuid NOT NULL
        REFERENCES public.players (id) ON DELETE RESTRICT,
    session_id uuid NOT NULL
        REFERENCES public.training_sessions (id) ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'confirmed'
        CHECK (status IN ('confirmed', 'cancelled')),
    external_booking_reference text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT bookings_external_reference_nonempty CHECK (
        external_booking_reference IS NULL OR btrim(external_booking_reference) <> ''
    )
);

-- If multiple booking providers are supported in the future, a provider/source
-- field may be added alongside this external reference.

-- A program with scheduled sessions cannot be accidentally deleted.
-- RESTRICT behavior is enforced by the training_sessions foreign key above.

-- updated_at is refreshed on row updates, while preserving caller-supplied
-- values during INSERT through the column defaults.
CREATE OR REPLACE FUNCTION public.set_updated_at()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = ''
AS $function$
BEGIN
    NEW.updated_at := pg_catalog.now();
    RETURN NEW;
END;
$function$;

CREATE TRIGGER training_sessions_set_updated_at
BEFORE UPDATE ON public.training_sessions
FOR EACH ROW EXECUTE FUNCTION public.set_updated_at();

CREATE TRIGGER players_set_updated_at
BEFORE UPDATE ON public.players
FOR EACH ROW EXECUTE FUNCTION public.set_updated_at();

CREATE TRIGGER bookings_set_updated_at
BEFORE UPDATE ON public.bookings
FOR EACH ROW EXECUTE FUNCTION public.set_updated_at();

-- Operational lookup and foreign-key support indexes.
CREATE INDEX training_sessions_starts_at_idx
    ON public.training_sessions (starts_at);
CREATE INDEX training_sessions_program_id_idx
    ON public.training_sessions (program_id);
CREATE INDEX bookings_session_id_status_idx
    ON public.bookings (session_id, status);
CREATE INDEX bookings_player_id_created_at_idx
    ON public.bookings (player_id, created_at DESC);

-- Keep cancelled rows as history while allowing a player to book again.
CREATE UNIQUE INDEX bookings_one_confirmed_per_player_session_idx
    ON public.bookings (player_id, session_id)
    WHERE status = 'confirmed';

CREATE UNIQUE INDEX bookings_external_reference_unique_idx
    ON public.bookings (external_booking_reference)
    WHERE external_booking_reference IS NOT NULL;

-- RLS is enabled as a safe default before any future client-facing Supabase
-- Data API access. The current Go API connects server-side to PostgreSQL.
-- No policies are defined until authentication and client-facing access exist.
ALTER TABLE public.training_programs ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.training_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.players ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.bookings ENABLE ROW LEVEL SECURITY;

COMMENT ON TABLE public.training_programs IS
    'RLS enabled; policies will be introduced when authentication and client-facing Supabase access are implemented.';
COMMENT ON TABLE public.training_sessions IS
    'RLS enabled; policies will be introduced when authentication and client-facing Supabase access are implemented.';
COMMENT ON TABLE public.bookings IS
    'RLS enabled; policies will be introduced when authentication and client-facing Supabase access are implemented.';
COMMENT ON TABLE public.players IS
    'RLS enabled; policies will be introduced when authentication and client-facing Supabase access are implemented. Player identity matching is application/service logic for now. Do not require globally unique email or phone values; family members may share contact details. A future Supabase Auth integration may add a nullable stable auth.users relationship once accounts are introduced.';

-- This is the sole supported booking-creation operation for the Go API. The
-- session row lock serializes callers for the same session until the surrounding
-- transaction commits, making the capacity check and insert atomic.
-- This function is the intended application entry point for confirmed bookings;
-- the Go API must not perform COUNT bookings then INSERT as separate operations.
-- Booking operations are expected to run at PostgreSQL READ COMMITTED isolation.
-- The Go booking service must use the default READ COMMITTED isolation or set it
-- explicitly. Callers should not use REPEATABLE READ here without retry handling
-- for serialization or concurrency failures.
-- SQLSTATEs: P0002 = session not found; PZ001 = unavailable; PZ002 = full;
-- PZ003 = duplicate confirmed booking. The external-reference unique constraint
-- remains a normal unique_violation (23505) and is deliberately not swallowed.
CREATE OR REPLACE FUNCTION public.book_training_session(
    p_player_id uuid,
    p_session_id uuid,
    p_external_booking_reference text DEFAULT NULL
)
RETURNS public.bookings
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = ''
AS $function$
DECLARE
    v_session_status text;
    v_capacity integer;
    v_booking public.bookings;
    v_confirmed_count bigint;
BEGIN
    SELECT s.status, s.capacity
      INTO v_session_status, v_capacity
      FROM public.training_sessions AS s
     WHERE s.id = p_session_id
     FOR UPDATE;

    IF NOT FOUND THEN
        RAISE EXCEPTION USING
            ERRCODE = 'P0002', MESSAGE = 'Training session not found';
    END IF;

    IF v_session_status <> 'open' THEN
        RAISE EXCEPTION USING
            ERRCODE = 'PZ001', MESSAGE = 'Training session is unavailable';
    END IF;

    IF EXISTS (
        SELECT 1
          FROM public.bookings AS b
         WHERE b.player_id = p_player_id
           AND b.session_id = p_session_id
           AND b.status = 'confirmed'
    ) THEN
        RAISE EXCEPTION USING
            ERRCODE = 'PZ003', MESSAGE = 'Player already has a confirmed booking for this session';
    END IF;

    IF v_capacity IS NOT NULL THEN
        SELECT pg_catalog.count(*)
          INTO v_confirmed_count
          FROM public.bookings AS b
         WHERE b.session_id = p_session_id
           AND b.status = 'confirmed';

        IF v_confirmed_count >= v_capacity THEN
            RAISE EXCEPTION USING
                ERRCODE = 'PZ002', MESSAGE = 'Training session is full';
        END IF;
    END IF;

    INSERT INTO public.bookings (
        player_id,
        session_id,
        status,
        external_booking_reference
    )
    VALUES (
        p_player_id,
        p_session_id,
        'confirmed',
        p_external_booking_reference
    )
    RETURNING * INTO v_booking;

    RETURN v_booking;
END;
$function$;
