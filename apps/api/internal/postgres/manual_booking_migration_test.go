package postgres

import (
	"os"
	"strings"
	"testing"
)

// These are migration source checks only. They do not execute against PostgreSQL.
func TestManualBookingMigrationKeepsPendingRequestsOutOfCapacity(t *testing.T) {
	sql := readManualBookingMigration(t)
	create := migrationFunction(t, sql, "create_public_booking_request")
	if strings.Contains(create, "INSERT INTO public.bookings") || strings.Contains(create, "count(*)") || strings.Contains(create, "capacity") {
		t.Fatal("public request creation must not create a booking or inspect/consume capacity")
	}
	approve := migrationFunction(t, sql, "approve_booking_request")
	lock := strings.Index(approve, "FOR UPDATE OF s")
	count := strings.Index(approve, "count(*)")
	insert := strings.Index(approve, "INSERT INTO public.bookings")
	decision := strings.Index(approve, "UPDATE public.booking_requests")
	if lock < 0 || count <= lock || insert <= count || decision <= insert {
		t.Fatal("approval must lock the session, count confirmed bookings, create the booking, and record the decision in order")
	}
	if !strings.Contains(approve, "b.status = 'confirmed'") || !strings.Contains(approve, "s.status = 'open'") || !strings.Contains(approve, "s.is_public IS TRUE") || !strings.Contains(approve, "s.starts_at >") {
		t.Fatal("approval must recheck confirmed capacity and session eligibility")
	}
}

func TestManualBookingMigrationRejectionDoesNotCreateBooking(t *testing.T) {
	reject := migrationFunction(t, readManualBookingMigration(t), "reject_booking_request")
	if strings.Contains(reject, "INSERT INTO public.bookings") || !strings.Contains(reject, "status = 'rejected'") || !strings.Contains(reject, "decided_by") || !strings.Contains(reject, "decided_at") {
		t.Fatal("rejection must record its decision without creating a booking")
	}
}

func TestApprovalNotificationMigrationPersistsStatusAndRestrictsExecution(t *testing.T) {
	content, err := os.ReadFile("../../../../supabase/migrations/20261012100000_add_booking_approval_notifications.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	for _, expected := range []string{
		"CREATE TABLE public.booking_approval_notifications",
		"REFERENCES public.booking_requests (id) ON DELETE RESTRICT",
		"AFTER UPDATE OF status ON public.booking_requests",
		"OLD.status IS DISTINCT FROM 'approved'",
		"public.claim_booking_approval_notification",
		"public.complete_booking_approval_notification",
		"attempt_count = n.attempt_count + 1",
		"n.status = 'sending'",
		"TO mmsu_booking_admin_api",
	} {
		if !strings.Contains(sql, expected) {
			t.Errorf("notification migration missing %q", expected)
		}
	}
	if strings.Contains(sql, "GRANT EXECUTE ON FUNCTION public.claim_booking_approval_notification(uuid, boolean) TO mmsu_booking_api") || strings.Contains(sql, "GRANT EXECUTE ON FUNCTION public.complete_booking_approval_notification(uuid, integer, boolean, text) TO mmsu_booking_api") {
		t.Fatal("public database role must not execute admin notification functions")
	}
}

func TestPrivateBookingStatusMigrationStoresOnlyTokenHashesAndScopesStatusOutput(t *testing.T) {
	content, err := os.ReadFile("../../../../supabase/migrations/20261012140000_add_private_booking_status.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	status := migrationFunction(t, sql, "get_public_booking_status")
	for _, expected := range []string{
		"tracking_token_hash bytea",
		"tracking_token_expires_at timestamptz",
		"pg_catalog.octet_length(p_tracking_token_hash) = 32",
		"br.tracking_token_hash = p_tracking_token_hash",
		"br.tracking_token_expires_at <= pg_catalog.statement_timestamp()",
	} {
		if !strings.Contains(sql+status, expected) {
			t.Errorf("private status migration missing %q", expected)
		}
	}
	if strings.Contains(status, "stored_token_hash") || strings.Contains(status, "br.full_name") || strings.Contains(status, "br.email") || strings.Contains(status, "br.phone") || strings.Contains(status, "br.decided_by") {
		t.Fatal("public status function returns private request or token data")
	}
	if strings.Contains(sql, "GRANT EXECUTE ON FUNCTION public.get_public_booking_status(bytea) TO PUBLIC") ||
		!strings.Contains(sql, "TO mmsu_booking_api") {
		t.Fatal("status function execution must remain restricted to the booking API role")
	}
}

func TestPrivateBookingStatusMigrationRejectsRequestsForFullSessions(t *testing.T) {
	content, err := os.ReadFile("../../../../supabase/migrations/20261012140000_add_private_booking_status.sql")
	if err != nil {
		t.Fatal(err)
	}
	create := migrationFunction(t, string(content), "create_public_booking_request_with_status_token")
	lock := strings.Index(create, "FOR SHARE OF s")
	count := strings.Index(create, "count(*)")
	insert := strings.Index(create, "INSERT INTO public.booking_requests")
	if lock < 0 || count <= lock || insert <= count {
		t.Fatal("public requests for full sessions must be rejected after locking/checking the session and before inserting the request")
	}
	for _, expected := range []string{
		"SELECT s.program_id, s.capacity INTO v_locked_program_id, v_capacity",
		"b.status = 'confirmed'",
		"v_confirmed_count >= v_capacity",
		"ERRCODE = 'PZ002'",
		"Training session is full",
	} {
		if !strings.Contains(create, expected) {
			t.Errorf("full-session request guard missing %q", expected)
		}
	}
}

func TestCoachSchedulingMigrationSeparatesAvailabilityFromPublicSessions(t *testing.T) {
	content, err := os.ReadFile("../../../../supabase/migrations/20261012160000_add_coach_scheduling_workflow.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	submit := migrationFunction(t, sql, "submit_coach_availability")
	create := migrationFunction(t, sql, "admin_create_training_session")
	if !strings.Contains(sql, "CREATE TABLE public.coach_availability") {
		t.Fatal("coach availability table is missing")
	}
	if !strings.Contains(submit, "INSERT INTO public.coach_availability") ||
		strings.Contains(submit, "training_sessions") {
		t.Fatal("submitting coach availability must not create a training session")
	}
	if strings.Contains(sql, "CREATE TRIGGER") {
		t.Fatal("coach availability must not be automatically promoted to a training session")
	}
	for _, expected := range []string{
		"INSERT INTO public.training_sessions",
		"COALESCE(p_is_public, true)",
		"TO mmsu_booking_api, mmsu_booking_admin_api",
		"public.admin_create_training_session",
		"TO mmsu_booking_admin_api",
	} {
		if !strings.Contains(sql, expected) && !strings.Contains(create, expected) {
			t.Errorf("coach scheduling migration missing %q", expected)
		}
	}
	if strings.Contains(sql, "GRANT EXECUTE ON FUNCTION public.admin_create_training_session(uuid, timestamptz, timestamptz, text, integer, text, boolean) TO mmsu_booking_api") {
		t.Fatal("public booking API role must not create training sessions")
	}
}

func TestAdminScheduleFunctionsUseDedicatedDatabaseOwner(t *testing.T) {
	content, err := os.ReadFile("../../../../supabase/migrations/20261012170000_isolate_admin_schedule_functions.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	for _, expected := range []string{
		"CREATE ROLE mmsu_schedule_admin_function_owner",
		"GRANT SELECT (id, program_id, starts_at, ends_at, location, capacity, status, notes, is_public)",
		"CREATE POLICY training_sessions_schedule_admin_manage",
		"CREATE POLICY bookings_schedule_admin_select",
		"OWNER TO mmsu_schedule_admin_function_owner",
		"REVOKE CREATE ON SCHEMA public FROM mmsu_schedule_admin_function_owner",
		"REVOKE mmsu_schedule_admin_function_owner FROM CURRENT_USER",
	} {
		if !strings.Contains(sql, expected) {
			t.Errorf("admin schedule owner migration missing %q", expected)
		}
	}
	for _, function := range []string{
		"ALTER FUNCTION public.admin_create_training_session",
		"ALTER FUNCTION public.admin_update_training_session",
		"ALTER FUNCTION public.admin_list_training_sessions",
	} {
		if !strings.Contains(sql, function) {
			t.Errorf("admin schedule owner migration does not isolate %s", function)
		}
	}
}

func TestAdminScheduleOwnerCanMaintainSessionUpdateTimestamp(t *testing.T) {
	content, err := os.ReadFile("../../../../supabase/migrations/20261012180000_grant_schedule_updated_at.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "GRANT UPDATE (updated_at) ON TABLE public.training_sessions") ||
		!strings.Contains(string(content), "TO mmsu_schedule_admin_function_owner") {
		t.Fatal("admin schedule owner must be granted the timestamp column updated by session exceptions")
	}
}

func TestAdminApprovedClientsMigrationRestrictsAccess(t *testing.T) {
	content, err := os.ReadFile("../../../../supabase/migrations/20261015083917_add_admin_approved_clients.sql")
	if err != nil {
		t.Fatal(err)
	}

	sql := string(content)
	list := migrationFunction(t, sql, "admin_list_approved_clients")

	for _, expected := range []string{
		"SECURITY DEFINER",
		"SET search_path = ''",
		"b.status = 'confirmed'",
		"JOIN public.players AS pl ON pl.id = b.player_id",
		"JOIN public.training_sessions AS s ON s.id = b.session_id",
		"JOIN public.training_programs AS p ON p.id = s.program_id",
		"LEFT JOIN public.coach_session_assignments AS a",
	} {
		if !strings.Contains(list, expected) {
			t.Errorf("approved clients function missing %q", expected)
		}
	}

	if strings.Contains(list, "b.status = 'pending'") {
		t.Fatal("approved clients function must not return pending bookings")
	}

	for _, expected := range []string{
		"REVOKE ALL ON FUNCTION public.admin_list_approved_clients()",
		"TO mmsu_booking_admin_api",
		"GRANT SELECT (id, full_name, email, phone)",
		"TO mmsu_schedule_admin_function_owner",
		"CREATE POLICY players_schedule_admin_select",
		"FOR SELECT",
		"USING (true)",
		"OWNER TO mmsu_schedule_admin_function_owner",
		"GRANT CREATE ON SCHEMA public TO mmsu_schedule_admin_function_owner",
		"REVOKE CREATE ON SCHEMA public FROM mmsu_schedule_admin_function_owner",
		"GRANT mmsu_schedule_admin_function_owner TO CURRENT_USER",
		"REVOKE mmsu_schedule_admin_function_owner FROM CURRENT_USER",
	} {
		if !strings.Contains(sql, expected) {
			t.Errorf("approved clients migration missing %q", expected)
		}
	}

	playerGrant := strings.Index(sql, "GRANT SELECT (id, full_name, email, phone)")
	playerPolicy := strings.Index(sql, "CREATE POLICY players_schedule_admin_select")
	schemaCreate := strings.Index(sql, "GRANT CREATE ON SCHEMA public TO mmsu_schedule_admin_function_owner")
	roleGrant := strings.Index(sql, "GRANT mmsu_schedule_admin_function_owner TO CURRENT_USER")
	ownerChange := strings.Index(sql, "ALTER FUNCTION public.admin_list_approved_clients()")
	schemaRevoke := strings.Index(sql, "REVOKE CREATE ON SCHEMA public FROM mmsu_schedule_admin_function_owner")
	roleRevoke := strings.LastIndex(sql, "REVOKE mmsu_schedule_admin_function_owner FROM CURRENT_USER")

	if playerGrant < 0 ||
		playerPolicy <= playerGrant ||
		schemaCreate <= playerPolicy ||
		roleGrant <= schemaCreate ||
		ownerChange <= roleGrant ||
		schemaRevoke <= ownerChange ||
		roleRevoke <= schemaRevoke {
		t.Fatal("approved clients migration must grant minimal player access, prepare ownership transfer, then revoke temporary privileges in order")
	}

	if strings.Contains(sql, "GRANT SELECT ON TABLE public.players TO mmsu_booking_admin_api") {
		t.Fatal("admin API role must not receive direct player-table access")
	}
}

func TestCoachSessionAssignmentsAreAdminManagedAndCoachScoped(t *testing.T) {
	content, err := os.ReadFile("../../../../supabase/migrations/20261012190000_add_coach_session_assignments.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	assign := migrationFunction(t, sql, "admin_assign_training_session_coach")
	list := migrationFunction(t, sql, "coach_list_assigned_training_sessions")
	for _, expected := range []string{
		"CREATE TABLE public.coach_session_assignments",
		"session_id uuid PRIMARY KEY REFERENCES public.training_sessions(id) ON DELETE CASCADE",
		"CREATE POLICY coach_session_assignments_admin_manage",
		"OWNER TO mmsu_schedule_admin_function_owner",
		"TO mmsu_booking_admin_api",
		"TO mmsu_booking_api",
		"REVOKE CREATE ON SCHEMA public FROM mmsu_schedule_admin_function_owner",
	} {
		if !strings.Contains(sql, expected) {
			t.Errorf("coach assignment migration missing %q", expected)
		}
	}
	if !strings.Contains(assign, "FROM public.coach_availability") ||
		!strings.Contains(assign, "ON CONFLICT (session_id) DO UPDATE") ||
		!strings.Contains(list, "a.coach_id = p_coach_id") ||
		!strings.Contains(list, "SET search_path = ''") {
		t.Fatal("assignments must use known coaches and coach sessions must be identity-scoped with a fixed search path")
	}
	if strings.Contains(sql, "GRANT EXECUTE ON FUNCTION public.admin_assign_training_session_coach(uuid, uuid) TO mmsu_booking_api") ||
		strings.Contains(sql, "GRANT EXECUTE ON FUNCTION public.coach_list_assigned_training_sessions(uuid) TO anon") {
		t.Fatal("assignment writes must stay admin-only and session reads must not be public")
	}
}

func readManualBookingMigration(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile("../../../../supabase/migrations/20261010110000_add_manual_booking_approval.sql")
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func migrationFunction(t *testing.T, sql, name string) string {
	t.Helper()
	start := strings.Index(sql, "CREATE OR REPLACE FUNCTION public."+name+"(")
	if start < 0 {
		t.Fatalf("function %s missing", name)
	}
	relativeEnd := strings.Index(sql[start:], "$function$;")
	if relativeEnd < 0 {
		t.Fatalf("function %s body end missing", name)
	}
	return sql[start : start+relativeEnd]
}
