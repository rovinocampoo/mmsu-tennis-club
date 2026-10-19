-- Coach directory, coach-facing schedule, and participant rosters.
--
-- Coaches previously existed only implicitly through coach_availability rows,
-- and admins could only assign a coach who had already submitted availability.
-- This migration introduces an admin-managed coach directory that is
-- independent of any login, repoints session assignment at that directory, and
-- exposes read-only schedule + participant rosters to the shared coach portal.
--
-- Historical coach_availability rows are preserved untouched; they are simply
-- no longer required to assign a coach and are no longer submitted by coaches.
--
-- Existing assignments and availability are seeded into the directory so no
-- current booking or assignment becomes invalid.

-- Participant rosters for the schedule-admin function owner. RLS is enabled on
-- booking_participants with only the booking owner policy, so an explicit read
-- grant + policy is required. This runs as the migration owner because the
-- schedule-admin function owner cannot grant on a table it does not own.
GRANT SELECT ON TABLE public.booking_participants
    TO mmsu_schedule_admin_function_owner;
CREATE POLICY booking_participants_schedule_admin_select
    ON public.booking_participants
    FOR SELECT TO mmsu_schedule_admin_function_owner USING (true);

-- 1. Admin-managed coach directory. Records are independent of logins: a coach
--    can exist in the directory without holding a Supabase account.
GRANT mmsu_schedule_admin_function_owner TO CURRENT_USER
    WITH ADMIN FALSE, INHERIT FALSE, SET TRUE;
GRANT CREATE ON SCHEMA public TO mmsu_schedule_admin_function_owner;
SET ROLE mmsu_schedule_admin_function_owner;

CREATE TABLE public.coaches (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    full_name text NOT NULL
        CHECK (btrim(full_name) <> '' AND char_length(btrim(full_name)) <= 120),
    email text CHECK (email IS NULL OR char_length(email) <= 254),
    phone text CHECK (phone IS NULL OR char_length(phone) <= 40),
    notes text CHECK (notes IS NULL OR char_length(notes) <= 2000),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT pg_catalog.clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT pg_catalog.clock_timestamp()
);

ALTER TABLE public.coaches ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE public.coaches
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_api, mmsu_booking_admin_api, mmsu_booking_function_owner;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.coaches
    TO mmsu_schedule_admin_function_owner;
CREATE POLICY coaches_schedule_admin_manage
    ON public.coaches
    FOR ALL TO mmsu_schedule_admin_function_owner USING (true) WITH CHECK (true);

-- Seed every coach previously seen in availability, preserving the login id as
-- the directory id so existing coach accounts and assignments keep working.
INSERT INTO public.coaches (id, full_name, created_at, updated_at)
SELECT ca.coach_id,
       COALESCE(MIN(NULLIF(pg_catalog.btrim(ca.coach_name), '')), 'Coach'),
       MIN(ca.created_at),
       pg_catalog.clock_timestamp()
  FROM public.coach_availability AS ca
 GROUP BY ca.coach_id
ON CONFLICT (id) DO NOTHING;

-- Seed any assigned coach that never submitted availability.
INSERT INTO public.coaches (id, full_name)
SELECT a.coach_id, COALESCE(NULLIF(pg_catalog.btrim(a.coach_name), ''), 'Coach')
  FROM public.coach_session_assignments AS a
  LEFT JOIN public.coaches AS c ON c.id = a.coach_id
 WHERE c.id IS NULL
ON CONFLICT (id) DO NOTHING;

ALTER TABLE public.coach_session_assignments
    ADD CONSTRAINT coach_session_assignments_coach_fk
    FOREIGN KEY (coach_id) REFERENCES public.coaches (id) ON DELETE RESTRICT;

-- 2. Repoint coach assignment at the directory instead of availability. The
--    signature is unchanged so existing admin UI and handlers keep working.
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
BEGIN
    IF p_session_id IS NULL THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Session is required';
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM public.training_sessions AS s WHERE s.id = p_session_id
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'P0002', MESSAGE = 'Training session not found';
    END IF;

    IF p_coach_id IS NULL THEN
        DELETE FROM public.coach_session_assignments
         WHERE session_id = p_session_id;
        RETURN true;
    END IF;

    SELECT c.full_name
      INTO v_coach_name
      FROM public.coaches AS c
     WHERE c.id = p_coach_id
       AND c.is_active IS TRUE;

    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ004', MESSAGE = 'Coach is unavailable';
    END IF;

    INSERT INTO public.coach_session_assignments (session_id, coach_id, coach_name)
    VALUES (p_session_id, p_coach_id, v_coach_name)
    ON CONFLICT (session_id) DO UPDATE
        SET coach_id = EXCLUDED.coach_id,
            coach_name = EXCLUDED.coach_name;

    RETURN true;
END;
$function$;

-- 3. Admin coach directory management.
CREATE OR REPLACE FUNCTION public.admin_list_coaches()
RETURNS TABLE (
    id uuid,
    full_name text,
    email text,
    phone text,
    notes text,
    is_active boolean,
    created_at timestamptz,
    updated_at timestamptz,
    assigned_session_count bigint
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = ''
AS $function$
    SELECT c.id,
           c.full_name,
           c.email,
           c.phone,
           c.notes,
           c.is_active,
           c.created_at,
           c.updated_at,
           COALESCE(assigned.assigned_session_count, 0)
      FROM public.coaches AS c
      LEFT JOIN LATERAL (
          SELECT pg_catalog.count(*)::bigint AS assigned_session_count
            FROM public.coach_session_assignments AS a
           WHERE a.coach_id = c.id
      ) AS assigned ON true
     ORDER BY c.is_active DESC, c.full_name ASC;
$function$;

CREATE OR REPLACE FUNCTION public.admin_create_coach(
    p_full_name text,
    p_email text,
    p_phone text,
    p_notes text
)
RETURNS uuid
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
AS $function$
DECLARE
    v_name text := pg_catalog.btrim(p_full_name);
    v_id uuid;
BEGIN
    IF v_name IS NULL OR v_name = '' OR pg_catalog.char_length(v_name) > 120
       OR (p_email IS NOT NULL AND pg_catalog.char_length(pg_catalog.btrim(p_email)) > 254)
       OR (p_phone IS NOT NULL AND pg_catalog.char_length(pg_catalog.btrim(p_phone)) > 40)
       OR (p_notes IS NOT NULL AND pg_catalog.char_length(pg_catalog.btrim(p_notes)) > 2000) THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid coach details';
    END IF;

    INSERT INTO public.coaches (full_name, email, phone, notes)
    VALUES (
        v_name,
        NULLIF(pg_catalog.btrim(p_email), ''),
        NULLIF(pg_catalog.btrim(p_phone), ''),
        NULLIF(pg_catalog.btrim(p_notes), '')
    )
    RETURNING id INTO v_id;

    RETURN v_id;
END;
$function$;

CREATE OR REPLACE FUNCTION public.admin_update_coach(
    p_coach_id uuid,
    p_full_name text,
    p_email text,
    p_phone text,
    p_notes text,
    p_is_active boolean
)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
AS $function$
DECLARE
    v_name text := pg_catalog.btrim(p_full_name);
BEGIN
    IF p_coach_id IS NULL OR v_name IS NULL OR v_name = '' OR pg_catalog.char_length(v_name) > 120
       OR (p_email IS NOT NULL AND pg_catalog.char_length(pg_catalog.btrim(p_email)) > 254)
       OR (p_phone IS NOT NULL AND pg_catalog.char_length(pg_catalog.btrim(p_phone)) > 40)
       OR (p_notes IS NOT NULL AND pg_catalog.char_length(pg_catalog.btrim(p_notes)) > 2000) THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid coach details';
    END IF;

    UPDATE public.coaches
       SET full_name = v_name,
           email = NULLIF(pg_catalog.btrim(p_email), ''),
           phone = NULLIF(pg_catalog.btrim(p_phone), ''),
           notes = NULLIF(pg_catalog.btrim(p_notes), ''),
           is_active = COALESCE(p_is_active, is_active),
           updated_at = pg_catalog.clock_timestamp()
     WHERE id = p_coach_id;

    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'P0002', MESSAGE = 'Coach not found';
    END IF;

    -- Keep the denormalized name on existing assignments in sync.
    UPDATE public.coach_session_assignments
       SET coach_name = v_name
     WHERE coach_id = p_coach_id;

    RETURN true;
END;
$function$;

-- 4. Coach portal reads: the shared coach login sees the whole club schedule
--    with assigned coach and participant roster, plus the active directory.
CREATE OR REPLACE FUNCTION public.coach_list_coaches()
RETURNS TABLE (
    id uuid,
    full_name text,
    email text,
    phone text,
    notes text,
    is_active boolean
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = ''
AS $function$
    SELECT c.id, c.full_name, c.email, c.phone, c.notes, c.is_active
      FROM public.coaches AS c
     WHERE c.is_active IS TRUE
     ORDER BY c.full_name ASC;
$function$;

CREATE OR REPLACE FUNCTION public.coach_list_training_schedule()
RETURNS TABLE (
    id uuid,
    program_name text,
    program_description text,
    starts_at timestamptz,
    ends_at timestamptz,
    location text,
    status text,
    is_public boolean,
    assigned_coach_id uuid,
    assigned_coach_name text,
    participant_names jsonb,
    confirmed_booking_count bigint
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = ''
AS $function$
    SELECT s.id,
           p.name,
           p.description,
           s.starts_at,
           s.ends_at,
           s.location,
           s.status,
           s.is_public,
           a.coach_id,
           a.coach_name,
           COALESCE(roster.participant_names, '[]'::jsonb),
           COALESCE(cb.confirmed_booking_count, 0)
      FROM public.training_sessions AS s
      JOIN public.training_programs AS p ON p.id = s.program_id
      LEFT JOIN public.coach_session_assignments AS a ON a.session_id = s.id
      LEFT JOIN LATERAL (
          SELECT pg_catalog.jsonb_agg(names.full_name ORDER BY names.position, names.full_name) AS participant_names
            FROM (
                SELECT bp.position, bp.full_name
                  FROM public.bookings AS b
                  JOIN public.booking_participants AS bp ON bp.booking_id = b.id
                 WHERE b.session_id = s.id AND b.status = 'confirmed'
                UNION ALL
                SELECT 0 AS position, pl.full_name
                  FROM public.bookings AS b
                  JOIN public.players AS pl ON pl.id = b.player_id
                 WHERE b.session_id = s.id AND b.status = 'confirmed'
                   AND NOT EXISTS (
                       SELECT 1 FROM public.booking_participants AS bp2
                        WHERE bp2.booking_id = b.id
                   )
            ) AS names
      ) AS roster ON true
      LEFT JOIN LATERAL (
          SELECT pg_catalog.count(*)::bigint AS confirmed_booking_count
            FROM public.bookings AS b
           WHERE b.session_id = s.id AND b.status = 'confirmed'
      ) AS cb ON true
     ORDER BY s.starts_at DESC;
$function$;

-- 5. Approved-clients read now includes the booking participant roster. The
--    return shape changes, so the function is recreated.
DROP FUNCTION public.admin_list_approved_clients();

CREATE FUNCTION public.admin_list_approved_clients()
RETURNS TABLE (
    booking_id uuid,
    player_id uuid,
    player_name text,
    player_email text,
    player_phone text,
    session_id uuid,
    program_name text,
    starts_at timestamptz,
    ends_at timestamptz,
    location text,
    capacity integer,
    assigned_coach_id uuid,
    assigned_coach_name text,
    booking_created_at timestamptz,
    participant_names jsonb
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = ''
AS $function$
    SELECT b.id,
           pl.id,
           pl.full_name,
           pl.email,
           pl.phone,
           s.id,
           p.name,
           s.starts_at,
           s.ends_at,
           s.location,
           s.capacity,
           a.coach_id,
           a.coach_name,
           b.created_at,
           COALESCE(roster.participant_names, '[]'::jsonb)
      FROM public.bookings AS b
      JOIN public.players AS pl ON pl.id = b.player_id
      JOIN public.training_sessions AS s ON s.id = b.session_id
      JOIN public.training_programs AS p ON p.id = s.program_id
      LEFT JOIN public.coach_session_assignments AS a
        ON a.session_id = s.id
      LEFT JOIN LATERAL (
          SELECT pg_catalog.jsonb_agg(bp.full_name ORDER BY bp.position, bp.full_name) AS participant_names
            FROM public.booking_participants AS bp
           WHERE bp.booking_id = b.id
      ) AS roster ON true
     WHERE b.status = 'confirmed'
     ORDER BY s.starts_at ASC, pl.full_name ASC, b.id ASC;
$function$;

-- 6. Execute grants and ownership while still acting as
--    mmsu_schedule_admin_function_owner: the Supabase migration role does not
--    own these SECURITY DEFINER functions, so it cannot REVOKE or GRANT on them.
REVOKE ALL ON FUNCTION public.admin_assign_training_session_coach(uuid, uuid)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_api;
REVOKE ALL ON FUNCTION public.admin_list_coaches()
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_api;
REVOKE ALL ON FUNCTION public.admin_create_coach(text, text, text, text)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_api;
REVOKE ALL ON FUNCTION public.admin_update_coach(uuid, text, text, text, text, boolean)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_api;
REVOKE ALL ON FUNCTION public.coach_list_coaches()
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_admin_api;
REVOKE ALL ON FUNCTION public.coach_list_training_schedule()
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_admin_api;
REVOKE ALL ON FUNCTION public.admin_list_approved_clients()
    FROM PUBLIC, anon, authenticated, service_role,
         mmsu_schedule_reader, mmsu_booking_api;

GRANT EXECUTE ON FUNCTION public.admin_assign_training_session_coach(uuid, uuid)
    TO mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.admin_list_coaches()
    TO mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.admin_create_coach(text, text, text, text)
    TO mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.admin_update_coach(uuid, text, text, text, text, boolean)
    TO mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.coach_list_coaches()
    TO mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.coach_list_training_schedule()
    TO mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.admin_list_approved_clients()
    TO mmsu_booking_admin_api;

RESET ROLE;

REVOKE CREATE ON SCHEMA public FROM mmsu_schedule_admin_function_owner;
REVOKE mmsu_schedule_admin_function_owner FROM CURRENT_USER;
