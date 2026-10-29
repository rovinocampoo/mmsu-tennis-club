-- Let a booking request retain several independently decided session slots.
-- Existing request-level decisions are copied onto every linked slot.
ALTER TABLE public.booking_request_sessions
    ADD COLUMN status text NOT NULL DEFAULT 'pending',
    ADD COLUMN decided_by uuid REFERENCES auth.users(id) ON DELETE SET NULL,
    ADD COLUMN decided_at timestamptz,
    ADD CONSTRAINT booking_request_sessions_status_check
        CHECK (status IN ('pending', 'approved', 'rejected')),
    ADD CONSTRAINT booking_request_sessions_decision_check
        CHECK ((status = 'pending') = (decided_at IS NULL));

UPDATE public.booking_request_sessions AS requested
   SET status = br.status,
       decided_by = br.decided_by,
       decided_at = br.decided_at
  FROM public.booking_requests AS br
 WHERE br.id = requested.request_id
   AND br.status IN ('approved', 'rejected');

ALTER TABLE public.booking_requests
    DROP CONSTRAINT booking_requests_status_check,
    ADD CONSTRAINT booking_requests_status_check
        CHECK (status IN ('pending', 'partially_decided', 'approved', 'rejected')),
    DROP CONSTRAINT booking_requests_decision_consistent,
    ADD CONSTRAINT booking_requests_decision_consistent CHECK (
        (status = 'pending' AND decided_by IS NULL AND decided_at IS NULL AND resolved_player_id IS NULL)
        OR (status = 'pending' AND decided_by IS NOT NULL AND decided_at IS NOT NULL AND resolved_player_id IS NOT NULL)
        OR (status = 'partially_decided' AND decided_by IS NOT NULL AND decided_at IS NOT NULL AND resolved_player_id IS NOT NULL)
        OR (status IN ('approved', 'rejected') AND decided_by IS NOT NULL AND decided_at IS NOT NULL)
    );

GRANT mmsu_booking_function_owner TO CURRENT_USER
    WITH ADMIN FALSE, INHERIT FALSE, SET TRUE;
GRANT CREATE ON SCHEMA public TO mmsu_booking_function_owner;
SET ROLE mmsu_booking_function_owner;

-- Do not require unrelated slots on the same date to be adjacent. A participant
-- may request any non-overlapping public slots; each remains individually reviewable.
CREATE OR REPLACE FUNCTION public.create_public_booking_request_with_participants_token(
    p_session_ids jsonb,
    p_participants jsonb,
    p_email text,
    p_phone text,
    p_tracking_token_hash bytea
)
RETURNS text
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
AS $function$
DECLARE
    v_session_ids uuid[];
    v_session_count integer;
    v_found_count integer;
    v_participants text[];
    v_participant_count integer;
    v_first_session_id uuid;
    v_request_id uuid;
    v_full_name text;
    v_email text := NULLIF(pg_catalog.lower(pg_catalog.btrim(p_email)), '');
    v_phone text := NULLIF(pg_catalog.btrim(p_phone), '');
    v_public_reference text;
    v_session_id uuid;
BEGIN
    IF pg_catalog.jsonb_typeof(p_session_ids) IS DISTINCT FROM 'array'
       OR pg_catalog.jsonb_typeof(p_participants) IS DISTINCT FROM 'array'
       OR p_tracking_token_hash IS NULL OR pg_catalog.octet_length(p_tracking_token_hash) <> 32
       OR (v_email IS NOT NULL AND (pg_catalog.char_length(v_email) > 254 OR pg_catalog.strpos(v_email, '@') < 2))
       OR (v_phone IS NOT NULL AND pg_catalog.char_length(v_phone) > 40) THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid booking request';
    END IF;

    BEGIN
        SELECT pg_catalog.array_agg(pg_catalog.btrim(item.value) ORDER BY item.ordinality)
          INTO v_participants
          FROM pg_catalog.jsonb_array_elements_text(p_participants) WITH ORDINALITY AS item(value, ordinality);
    EXCEPTION WHEN OTHERS THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid participant list';
    END;
    v_participant_count := COALESCE(pg_catalog.cardinality(v_participants), 0);
    IF v_participant_count < 1 OR v_participant_count > 50
       OR EXISTS (SELECT 1 FROM pg_catalog.unnest(v_participants) AS n(value)
                   WHERE n.value = '' OR pg_catalog.char_length(n.value) > 200) THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid participant list';
    END IF;
    v_full_name := pg_catalog.array_to_string(v_participants, ', ');
    IF pg_catalog.char_length(v_full_name) > 200 THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Participant names are too long';
    END IF;

    BEGIN
        SELECT pg_catalog.array_agg(item.value::uuid ORDER BY item.ordinality)
          INTO v_session_ids
          FROM pg_catalog.jsonb_array_elements_text(p_session_ids) WITH ORDINALITY AS item(value, ordinality);
    EXCEPTION WHEN OTHERS THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Invalid session ID list';
    END;
    v_session_count := COALESCE(pg_catalog.cardinality(v_session_ids), 0);
    IF v_session_count < 1 OR v_session_count > 16 THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ012', MESSAGE = 'Booking request exceeds 16 session slots';
    END IF;
    IF (SELECT pg_catalog.count(DISTINCT id) FROM pg_catalog.unnest(v_session_ids) AS ids(id)) <> v_session_count THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Duplicate session IDs';
    END IF;

    PERFORM s.id FROM public.training_sessions AS s
     WHERE s.id = ANY(v_session_ids) ORDER BY s.id FOR SHARE OF s;
    SELECT pg_catalog.count(*) INTO v_found_count
      FROM public.training_sessions AS s WHERE s.id = ANY(v_session_ids);
    IF v_found_count <> v_session_count THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable';
    END IF;
    IF EXISTS (
        SELECT 1 FROM public.training_sessions AS s
        JOIN public.training_programs AS p ON p.id = s.program_id
        WHERE s.id = ANY(v_session_ids)
          AND (s.status <> 'open' OR s.is_public IS NOT TRUE
               OR s.starts_at <= pg_catalog.clock_timestamp() OR p.is_active IS NOT TRUE)
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'Training session unavailable';
    END IF;
    IF (SELECT pg_catalog.count(DISTINCT (s.starts_at AT TIME ZONE 'Asia/Manila')::date)
          FROM public.training_sessions AS s WHERE s.id = ANY(v_session_ids)) > 8 THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ012', MESSAGE = 'Booking request exceeds 8 dates';
    END IF;
    IF (SELECT pg_catalog.count(DISTINCT s.program_id) FROM public.training_sessions AS s WHERE s.id = ANY(v_session_ids)) <> 1
       OR (SELECT pg_catalog.count(DISTINCT pg_catalog.lower(pg_catalog.btrim(s.location)))
             FROM public.training_sessions AS s WHERE s.id = ANY(v_session_ids)) <> 1 THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ015', MESSAGE = 'Sessions must share one program and location';
    END IF;
    IF EXISTS (
        SELECT 1 FROM public.training_sessions AS a
        JOIN public.training_sessions AS b ON a.id < b.id
         AND a.starts_at < b.ends_at AND b.starts_at < a.ends_at
        WHERE a.id = ANY(v_session_ids) AND b.id = ANY(v_session_ids)
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ013', MESSAGE = 'Selected session times overlap';
    END IF;

    FOR v_session_id IN SELECT id FROM pg_catalog.unnest(v_session_ids) AS ids(id) ORDER BY id LOOP
        IF v_email IS NOT NULL THEN
            PERFORM pg_catalog.pg_advisory_xact_lock(pg_catalog.hashtextextended('booking-request-email:' || v_session_id::text || ':' || v_email, 0));
        END IF;
        IF v_phone IS NOT NULL THEN
            PERFORM pg_catalog.pg_advisory_xact_lock(pg_catalog.hashtextextended('booking-request-phone:' || v_session_id::text || ':' || v_phone, 0));
        END IF;
        PERFORM pg_catalog.pg_advisory_xact_lock(pg_catalog.hashtextextended('booking-request-name:' || v_session_id::text || ':' || pg_catalog.lower(v_full_name), 0));
    END LOOP;
    IF v_email IS NOT NULL AND EXISTS (
        SELECT 1 FROM public.booking_request_sessions AS rs JOIN public.booking_requests AS br ON br.id = rs.request_id
         WHERE rs.session_id = ANY(v_session_ids) AND br.status IN ('pending', 'partially_decided')
           AND pg_catalog.lower(br.email) = v_email
    ) OR v_phone IS NOT NULL AND EXISTS (
        SELECT 1 FROM public.booking_request_sessions AS rs JOIN public.booking_requests AS br ON br.id = rs.request_id
         WHERE rs.session_id = ANY(v_session_ids) AND br.status IN ('pending', 'partially_decided') AND br.phone = v_phone
    ) OR EXISTS (
        SELECT 1 FROM public.booking_request_sessions AS rs JOIN public.booking_requests AS br ON br.id = rs.request_id
         WHERE rs.session_id = ANY(v_session_ids) AND br.status IN ('pending', 'partially_decided')
           AND pg_catalog.lower(br.full_name) = pg_catalog.lower(v_full_name)
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ004', MESSAGE = 'Duplicate pending request';
    END IF;
    IF EXISTS (
        SELECT 1 FROM public.training_sessions AS s
         WHERE s.id = ANY(v_session_ids) AND s.capacity IS NOT NULL
           AND (SELECT pg_catalog.count(*) FROM public.bookings AS b
                 WHERE b.session_id = s.id AND b.status = 'confirmed') >= s.capacity
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ002', MESSAGE = 'Training session is full';
    END IF;

    SELECT s.id INTO v_first_session_id FROM public.training_sessions AS s
     WHERE s.id = ANY(v_session_ids) ORDER BY s.starts_at, s.id LIMIT 1;
    v_public_reference := 'MMSU-' || pg_catalog.upper(pg_catalog.substr(pg_catalog.replace(gen_random_uuid()::text, '-', ''), 1, 16));
    INSERT INTO public.booking_requests(session_id, full_name, email, phone, public_reference, tracking_token_hash, tracking_token_expires_at)
    VALUES (v_first_session_id, v_full_name, v_email, v_phone, v_public_reference, p_tracking_token_hash, pg_catalog.clock_timestamp() + interval '30 days')
    RETURNING id INTO v_request_id;
    INSERT INTO public.booking_request_participants(request_id, position, full_name)
    SELECT v_request_id, n.ordinality::smallint, n.value FROM pg_catalog.unnest(v_participants) WITH ORDINALITY AS n(value, ordinality);
    INSERT INTO public.booking_request_sessions(request_id, session_id, position)
    SELECT v_request_id, s.id, pg_catalog.row_number() OVER (ORDER BY s.starts_at, s.id)::smallint
      FROM public.training_sessions AS s WHERE s.id = ANY(v_session_ids);
    RETURN v_public_reference;
END
$function$;

CREATE OR REPLACE FUNCTION public.approve_booking_request_sessions(
    p_request_id uuid, p_admin_id uuid, p_player_id uuid, p_create_new boolean, p_session_ids jsonb
)
RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path = ''
AS $function$
DECLARE
    v_request public.booking_requests;
    v_player_id uuid;
    v_session_id uuid;
    v_booking_id uuid;
    v_booking_ids uuid[] := ARRAY[]::uuid[];
    v_session_ids uuid[];
    v_count integer;
    v_confirmed_count bigint;
    v_capacity integer;
BEGIN
    SELECT br.* INTO v_request FROM public.booking_requests AS br WHERE br.id = p_request_id FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'PZ006', MESSAGE = 'Booking request not found'; END IF;
    IF v_request.status NOT IN ('pending', 'partially_decided') THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ003', MESSAGE = 'Booking request is already complete';
    END IF;
    IF p_admin_id IS NULL
       OR (v_request.resolved_player_id IS NULL AND ((p_player_id IS NULL) = (p_create_new IS FALSE)))
       OR (v_request.resolved_player_id IS NOT NULL AND (p_create_new OR (p_player_id IS NOT NULL AND p_player_id <> v_request.resolved_player_id)))
       OR pg_catalog.jsonb_typeof(p_session_ids) IS DISTINCT FROM 'array' THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Select sessions and choose a player';
    END IF;
    BEGIN
        SELECT pg_catalog.array_agg(value::uuid ORDER BY ordinality) INTO v_session_ids
          FROM pg_catalog.jsonb_array_elements_text(p_session_ids) WITH ORDINALITY AS ids(value, ordinality);
    EXCEPTION WHEN OTHERS THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Selected session IDs are invalid';
    END;
    v_count := COALESCE(pg_catalog.cardinality(v_session_ids), 0);
    IF v_count < 1 OR v_count > 16 THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Select 1 to 16 sessions';
    END IF;
    IF (SELECT pg_catalog.count(DISTINCT id) FROM pg_catalog.unnest(v_session_ids) AS ids(id)) <> v_count
       OR (SELECT pg_catalog.count(*) FROM public.booking_request_sessions AS rs
            WHERE rs.request_id = p_request_id AND rs.session_id = ANY(v_session_ids) AND rs.status = 'pending') <> v_count THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ014', MESSAGE = 'One or more selected sessions are no longer pending';
    END IF;
    PERFORM s.id FROM public.training_sessions AS s WHERE s.id = ANY(v_session_ids) ORDER BY s.id FOR UPDATE OF s;
    IF (SELECT pg_catalog.count(*) FROM public.training_sessions AS s WHERE s.id = ANY(v_session_ids)) <> v_count
       OR EXISTS (
          SELECT 1 FROM public.training_sessions AS s JOIN public.training_programs AS p ON p.id = s.program_id
           WHERE s.id = ANY(v_session_ids)
             AND (s.status <> 'open' OR s.is_public IS NOT TRUE OR s.starts_at <= pg_catalog.clock_timestamp() OR p.is_active IS NOT TRUE)
       ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ001', MESSAGE = 'One or more selected sessions are unavailable';
    END IF;
    IF v_request.resolved_player_id IS NOT NULL THEN
        v_player_id := v_request.resolved_player_id;
    ELSIF p_create_new THEN
        INSERT INTO public.players(full_name, email, phone) VALUES (v_request.full_name, v_request.email, v_request.phone)
        RETURNING id INTO v_player_id;
    ELSE
        SELECT pl.id INTO v_player_id FROM public.players AS pl WHERE pl.id = p_player_id;
        IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'PZ005', MESSAGE = 'Selected player unavailable'; END IF;
    END IF;
    IF EXISTS (SELECT 1 FROM public.bookings AS b WHERE b.player_id = v_player_id AND b.session_id = ANY(v_session_ids) AND b.status = 'confirmed') THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ004', MESSAGE = 'Player is already booked for a selected session';
    END IF;
    FOR v_session_id IN SELECT s.id FROM public.training_sessions AS s WHERE s.id = ANY(v_session_ids) ORDER BY s.starts_at, s.id LOOP
        SELECT s.capacity INTO v_capacity FROM public.training_sessions AS s WHERE s.id = v_session_id;
        IF v_capacity IS NOT NULL THEN
            SELECT pg_catalog.count(*) INTO v_confirmed_count FROM public.bookings AS b WHERE b.session_id = v_session_id AND b.status = 'confirmed';
            IF v_confirmed_count >= v_capacity THEN RAISE EXCEPTION USING ERRCODE = 'PZ002', MESSAGE = 'Training session is full'; END IF;
        END IF;
        INSERT INTO public.bookings(player_id, session_id, status, external_booking_reference)
        VALUES (v_player_id, v_session_id, 'confirmed', 'request:' || p_request_id::text || ':' || v_session_id::text)
        RETURNING id INTO v_booking_id;
        v_booking_ids := pg_catalog.array_append(v_booking_ids, v_booking_id);
        INSERT INTO public.booking_participants(booking_id, position, full_name)
        SELECT v_booking_id, rp.position, rp.full_name FROM public.booking_request_participants AS rp WHERE rp.request_id = p_request_id;
        IF NOT FOUND THEN INSERT INTO public.booking_participants(booking_id, position, full_name) VALUES (v_booking_id, 1, v_request.full_name); END IF;
    END LOOP;
    UPDATE public.booking_request_sessions SET status = 'approved', decided_by = p_admin_id, decided_at = pg_catalog.clock_timestamp()
     WHERE request_id = p_request_id AND session_id = ANY(v_session_ids) AND status = 'pending';
    UPDATE public.booking_requests AS br
       SET resolved_player_id = v_player_id, decided_by = p_admin_id, decided_at = pg_catalog.clock_timestamp(),
           status = CASE
             WHEN EXISTS (SELECT 1 FROM public.booking_request_sessions rs WHERE rs.request_id = p_request_id AND rs.status = 'pending') THEN 'pending'
             WHEN EXISTS (SELECT 1 FROM public.booking_request_sessions rs WHERE rs.request_id = p_request_id AND rs.status = 'approved')
              AND EXISTS (SELECT 1 FROM public.booking_request_sessions rs WHERE rs.request_id = p_request_id AND rs.status = 'rejected') THEN 'partially_decided'
             WHEN EXISTS (SELECT 1 FROM public.booking_request_sessions rs WHERE rs.request_id = p_request_id AND rs.status = 'approved') THEN 'approved'
             ELSE 'rejected' END
     WHERE br.id = p_request_id;
    RETURN pg_catalog.jsonb_build_object('booking_ids', pg_catalog.to_jsonb(v_booking_ids), 'request_status',
        (SELECT br.status FROM public.booking_requests br WHERE br.id = p_request_id));
END
$function$;

CREATE OR REPLACE FUNCTION public.approve_booking_request(p_request_id uuid, p_admin_id uuid, p_player_id uuid, p_create_new boolean)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path = '' AS $function$
DECLARE v_result jsonb; v_id uuid;
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM public.booking_request_sessions rs
         WHERE rs.request_id = p_request_id AND rs.status = 'pending'
    ) THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ003', MESSAGE = 'Booking request is already complete';
    END IF;
    SELECT public.approve_booking_request_sessions(p_request_id, p_admin_id, p_player_id, p_create_new,
        (SELECT pg_catalog.to_jsonb(pg_catalog.array_agg(rs.session_id ORDER BY rs.session_id)) FROM public.booking_request_sessions rs WHERE rs.request_id = p_request_id AND rs.status = 'pending'))
      INTO v_result;
    v_id := (v_result->'booking_ids'->>0)::uuid;
    RETURN v_id;
END
$function$;

CREATE OR REPLACE FUNCTION public.reject_booking_request(p_request_id uuid, p_admin_id uuid)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path = '' AS $function$
DECLARE v_request public.booking_requests;
BEGIN
    IF p_admin_id IS NULL THEN RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Admin identity required'; END IF;
    SELECT br.* INTO v_request FROM public.booking_requests br WHERE br.id = p_request_id FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'PZ006', MESSAGE = 'Booking request not found'; END IF;
    IF v_request.status NOT IN ('pending', 'partially_decided') THEN RAISE EXCEPTION USING ERRCODE = 'PZ003', MESSAGE = 'Booking request is already complete'; END IF;
    UPDATE public.booking_request_sessions SET status = 'rejected', decided_by = p_admin_id, decided_at = pg_catalog.clock_timestamp()
     WHERE request_id = p_request_id AND status = 'pending';
    UPDATE public.booking_requests AS br
       SET decided_by = p_admin_id, decided_at = pg_catalog.clock_timestamp(),
           status = CASE
             WHEN EXISTS (SELECT 1 FROM public.booking_request_sessions rs WHERE rs.request_id = p_request_id AND rs.status = 'approved') THEN 'partially_decided'
             ELSE 'rejected' END
     WHERE br.id = p_request_id;
    RETURN true;
END
$function$;

CREATE FUNCTION public.admin_reject_pending_booking_sessions(p_request_id uuid, p_admin_id uuid)
RETURNS text LANGUAGE plpgsql SECURITY DEFINER SET search_path = '' AS $function$
DECLARE v_status text;
BEGIN
    PERFORM public.reject_booking_request(p_request_id, p_admin_id);
    SELECT br.status INTO v_status FROM public.booking_requests br WHERE br.id = p_request_id;
    RETURN v_status;
END
$function$;

-- Include each slot's decision state in the admin queue.
DROP FUNCTION public.admin_list_pending_booking_requests_with_slots();
CREATE FUNCTION public.admin_list_pending_booking_requests_with_slots()
RETURNS TABLE(id uuid, session_id uuid, full_name text, email text, phone text, status text,
              created_at timestamptz, starts_at timestamptz, ends_at timestamptz, location text,
              session_status text, training_focus text, training_focus_notes text,
              requested_slots jsonb, resolved_player_id uuid)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = ''
AS $function$
    SELECT br.id, br.session_id, br.full_name, br.email, br.phone, br.status, br.created_at,
           pg_catalog.min(s.starts_at), pg_catalog.max(s.ends_at), pg_catalog.max(s.location),
           CASE WHEN pg_catalog.bool_and(s.status = 'open') THEN 'open' ELSE 'closed' END,
           br.training_focus, br.training_focus_notes,
           pg_catalog.jsonb_agg(pg_catalog.jsonb_build_object('session_id', s.id, 'program_name', p.name,
               'starts_at', s.starts_at, 'ends_at', s.ends_at, 'location', s.location, 'status', rs.status)
               ORDER BY s.starts_at, s.id), br.resolved_player_id
      FROM public.booking_requests br
      JOIN public.booking_request_sessions rs ON rs.request_id = br.id
      JOIN public.training_sessions s ON s.id = rs.session_id
      JOIN public.training_programs p ON p.id = s.program_id
     WHERE br.status IN ('pending', 'partially_decided')
     GROUP BY br.id ORDER BY br.created_at, br.id
$function$;
REVOKE ALL ON FUNCTION public.admin_list_pending_booking_requests_with_slots()
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_api, mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.admin_list_pending_booking_requests_with_slots() TO mmsu_booking_admin_api;

REVOKE ALL ON FUNCTION public.approve_booking_request_sessions(uuid, uuid, uuid, boolean, jsonb)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.approve_booking_request_sessions(uuid, uuid, uuid, boolean, jsonb)
    TO mmsu_booking_admin_api;
REVOKE ALL ON FUNCTION public.approve_booking_request(uuid, uuid, uuid, boolean)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.approve_booking_request(uuid, uuid, uuid, boolean) TO mmsu_booking_admin_api;
REVOKE ALL ON FUNCTION public.reject_booking_request(uuid, uuid)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.reject_booking_request(uuid, uuid) TO mmsu_booking_admin_api;
REVOKE ALL ON FUNCTION public.admin_reject_pending_booking_sessions(uuid, uuid)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_api;
GRANT EXECUTE ON FUNCTION public.admin_reject_pending_booking_sessions(uuid, uuid) TO mmsu_booking_admin_api;

CREATE OR REPLACE FUNCTION public.record_booking_approval_notification()
RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path = '' AS $function$
BEGIN
    IF NEW.status IN ('approved', 'partially_decided') AND OLD.status IS DISTINCT FROM NEW.status THEN
        INSERT INTO public.booking_approval_notifications(request_id, support_reference, program_name, starts_at, ends_at, location, status, detail_code)
        SELECT NEW.id, 'MMSU-' || pg_catalog.upper(NEW.id::text), pg_catalog.max(p.name),
               pg_catalog.min(s.starts_at), pg_catalog.max(s.ends_at), pg_catalog.max(s.location),
               CASE WHEN NULLIF(pg_catalog.btrim(NEW.email), '') IS NULL THEN 'not_configured' ELSE 'pending' END,
               CASE WHEN NULLIF(pg_catalog.btrim(NEW.email), '') IS NULL THEN 'missing_email' ELSE NULL END
          FROM public.booking_request_sessions rs
          JOIN public.training_sessions s ON s.id = rs.session_id
          JOIN public.training_programs p ON p.id = s.program_id
         WHERE rs.request_id = NEW.id AND rs.status = 'approved'
        ON CONFLICT (request_id) DO NOTHING;
    END IF;
    RETURN NEW;
END
$function$;
ALTER FUNCTION public.record_booking_approval_notification() OWNER TO mmsu_booking_function_owner;

CREATE OR REPLACE FUNCTION public.claim_booking_approval_notification(p_request_id uuid, p_resend_configured boolean)
RETURNS TABLE(notification_status text, notification_detail text, recipient_email text, program_name text,
              starts_at timestamptz, ends_at timestamptz, location text, support_reference text,
              attempt_count integer, claimed boolean)
LANGUAGE plpgsql SECURITY DEFINER SET search_path = ''
AS $function$
DECLARE v_request_status text; v_email text; v_notification public.booking_approval_notifications;
BEGIN
    SELECT br.status, NULLIF(pg_catalog.btrim(br.email), '') INTO v_request_status, v_email
      FROM public.booking_requests br WHERE br.id = p_request_id;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'PZ006', MESSAGE = 'Booking request not found'; END IF;
    IF v_request_status NOT IN ('approved', 'partially_decided') THEN
        RAISE EXCEPTION USING ERRCODE = 'PZ003', MESSAGE = 'Booking request decisions are not complete';
    END IF;
    SELECT n.* INTO v_notification FROM public.booking_approval_notifications n WHERE n.request_id = p_request_id FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'PZ006', MESSAGE = 'Approval notification record not found'; END IF;
    IF v_notification.status = 'sent' THEN
        RETURN QUERY SELECT v_notification.status, v_notification.detail_code, v_email, v_notification.program_name,
            v_notification.starts_at, v_notification.ends_at, v_notification.location, v_notification.support_reference,
            v_notification.attempt_count, false; RETURN;
    END IF;
    IF v_email IS NULL OR NOT COALESCE(p_resend_configured, false) THEN
        UPDATE public.booking_approval_notifications n
           SET status = 'not_configured',
               detail_code = CASE WHEN v_email IS NULL THEN 'missing_email' ELSE 'resend_not_configured' END,
               send_started_at = NULL, updated_at = pg_catalog.clock_timestamp()
         WHERE n.request_id = p_request_id RETURNING n.* INTO v_notification;
        RETURN QUERY SELECT v_notification.status, v_notification.detail_code, v_email, v_notification.program_name,
            v_notification.starts_at, v_notification.ends_at, v_notification.location, v_notification.support_reference,
            v_notification.attempt_count, false; RETURN;
    END IF;
    IF v_notification.status = 'sending' AND v_notification.send_started_at > pg_catalog.clock_timestamp() - INTERVAL '2 minutes' THEN
        RETURN QUERY SELECT v_notification.status, v_notification.detail_code, v_email, v_notification.program_name,
            v_notification.starts_at, v_notification.ends_at, v_notification.location, v_notification.support_reference,
            v_notification.attempt_count, false; RETURN;
    END IF;
    UPDATE public.booking_approval_notifications n SET status = 'sending', detail_code = NULL,
        attempt_count = n.attempt_count + 1, send_started_at = pg_catalog.clock_timestamp(), updated_at = pg_catalog.clock_timestamp()
     WHERE n.request_id = p_request_id RETURNING n.* INTO v_notification;
    RETURN QUERY SELECT v_notification.status, v_notification.detail_code, v_email, v_notification.program_name,
        v_notification.starts_at, v_notification.ends_at, v_notification.location, v_notification.support_reference,
        v_notification.attempt_count, true;
END
$function$;

CREATE OR REPLACE FUNCTION public.get_booking_calendar_event(p_request_id uuid)
RETURNS TABLE(request_id uuid, request_reference text, participant_name text, program_name text,
              starts_at timestamptz, ends_at timestamptz, location text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = ''
AS $function$
    SELECT br.id, COALESCE(br.public_reference, 'MMSU-' || pg_catalog.upper(br.id::text)),
           br.full_name, n.program_name, n.starts_at, n.ends_at, n.location
      FROM public.booking_requests br JOIN public.booking_approval_notifications n ON n.request_id = br.id
     WHERE br.id = p_request_id AND br.status IN ('approved', 'partially_decided')
$function$;
ALTER FUNCTION public.claim_booking_approval_notification(uuid, boolean) OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.get_booking_calendar_event(uuid) OWNER TO mmsu_booking_function_owner;

-- Preserve request status on public lookup and expose each slot decision.
DROP FUNCTION public.get_public_booking_status(bytea);
CREATE FUNCTION public.get_public_booking_status(p_tracking_token_hash bytea)
RETURNS TABLE(request_reference text, request_status text, program_name text, starts_at timestamptz,
              ends_at timestamptz, location text, is_expired boolean, training_focus text,
              training_focus_notes text, requested_sessions jsonb)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = ''
AS $function$
    SELECT br.public_reference, br.status, pg_catalog.max(p.name), pg_catalog.min(s.starts_at),
           pg_catalog.max(s.ends_at), pg_catalog.max(s.location),
           br.tracking_token_expires_at <= pg_catalog.statement_timestamp(),
           br.training_focus, br.training_focus_notes,
           pg_catalog.jsonb_agg(pg_catalog.jsonb_build_object('session_id', s.id, 'program_name', p.name,
               'starts_at', s.starts_at, 'ends_at', s.ends_at, 'location', s.location, 'status', rs.status)
               ORDER BY s.starts_at, s.id)
      FROM public.booking_requests br
      JOIN public.booking_request_sessions rs ON rs.request_id = br.id
      JOIN public.training_sessions s ON s.id = rs.session_id
      JOIN public.training_programs p ON p.id = s.program_id
     WHERE pg_catalog.octet_length(p_tracking_token_hash) = 32
       AND br.tracking_token_hash = p_tracking_token_hash
     GROUP BY br.id
$function$;
REVOKE ALL ON FUNCTION public.get_public_booking_status(bytea)
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader, mmsu_booking_admin_api;
GRANT EXECUTE ON FUNCTION public.get_public_booking_status(bytea) TO mmsu_booking_api;

-- The API role cannot read or approve rows directly; SECURITY DEFINER functions
-- run as the restricted owner and EXECUTE is role-scoped above.
ALTER FUNCTION public.approve_booking_request_sessions(uuid, uuid, uuid, boolean, jsonb) OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.approve_booking_request(uuid, uuid, uuid, boolean) OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.reject_booking_request(uuid, uuid) OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.admin_reject_pending_booking_sessions(uuid, uuid) OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.admin_list_pending_booking_requests_with_slots() OWNER TO mmsu_booking_function_owner;
ALTER FUNCTION public.get_public_booking_status(bytea) OWNER TO mmsu_booking_function_owner;

RESET ROLE;
REVOKE CREATE ON SCHEMA public FROM mmsu_booking_function_owner;
REVOKE mmsu_booking_function_owner FROM CURRENT_USER;
