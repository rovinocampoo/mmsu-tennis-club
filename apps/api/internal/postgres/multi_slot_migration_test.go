package postgres

import (
	"os"
	"strings"
	"testing"
)

func TestMultiSlotMigrationKeepsBatchCreationAtomicAndConflictsServerValidated(t *testing.T) {
	content, err := os.ReadFile("../../../../supabase/migrations/20261016081200_add_multi_slot_training_bookings.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	batch := migrationFunction(t, sql, "admin_create_training_sessions_batch")
	for _, expected := range []string{
		"pg_advisory_xact_lock",
		"jsonb_array_elements(p_sessions)",
		"INSERT INTO public.training_sessions",
		"INSERT INTO public.coach_session_assignments",
		"ERRCODE = 'PZ007'",
		"RETURN v_created_ids",
	} {
		if !strings.Contains(batch, expected) {
			t.Errorf("batch schedule function missing %q", expected)
		}
	}
}

func TestLegacySessionStatusUpdatesShareTheScheduleLockAndGuardCancelledReopen(t *testing.T) {
	content, err := os.ReadFile("../../../../supabase/migrations/20261016081200_add_multi_slot_training_bookings.sql")
	if err != nil {
		t.Fatal(err)
	}
	update := migrationFunction(t, string(content), "admin_update_training_session")
	for _, expected := range []string{
		"pg_advisory_xact_lock",
		"v_old_status = 'cancelled' AND p_status = 'open'",
		"ERRCODE = 'PZ007'",
		"coach_session_assignments",
	} {
		if !strings.Contains(update, expected) {
			t.Errorf("legacy session update function missing %q", expected)
		}
	}
}

func TestMultiSlotMigrationRequiresAdjacentSlotsAndApprovesEverySlotTogether(t *testing.T) {
	content, err := os.ReadFile("../../../../supabase/migrations/20261016081200_add_multi_slot_training_bookings.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	create := migrationFunction(t, sql, "create_public_booking_request_with_slots_token")
	for _, expected := range []string{
		"CREATE TABLE public.booking_request_sessions",
		"cardinality(v_session_ids)",
		"FOR SHARE OF s",
		"lead(s.starts_at)",
		"ERRCODE = 'PZ010'",
		"INSERT INTO public.booking_request_sessions",
	} {
		if !strings.Contains(sql+create, expected) {
			t.Errorf("multi-slot request migration missing %q", expected)
		}
	}
	approve := migrationFunction(t, sql, "approve_booking_request")
	lock := strings.Index(approve, "FOR UPDATE OF s")
	capacity := strings.Index(approve, "b.status = 'confirmed'")
	insert := strings.Index(approve, "INSERT INTO public.bookings")
	decision := strings.Index(approve, "UPDATE public.booking_requests")
	if lock < 0 || capacity <= lock || insert <= capacity || decision <= insert {
		t.Fatal("approval must lock all sessions, validate capacity, create all bookings, then mark the request approved")
	}
	if !strings.Contains(approve, "FOR v_session_id IN") || !strings.Contains(approve, "ANY(v_session_ids)") {
		t.Fatal("multi-slot approval must loop over and reserve every requested session")
	}
}

func TestMultiSlotMigrationGuardsEditsToBookedSessionsAndSendsCombinedApprovalSummary(t *testing.T) {
	content, err := os.ReadFile("../../../../supabase/migrations/20261016081200_add_multi_slot_training_bookings.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	edit := migrationFunction(t, sql, "admin_edit_training_session")
	for _, expected := range []string{
		"p_acknowledge_booked_changes",
		"ERRCODE = 'PZ008'",
		"ERRCODE = 'PZ009'",
		"ERRCODE = 'PZ007'",
	} {
		if !strings.Contains(edit, expected) {
			t.Errorf("session edit function missing %q", expected)
		}
	}
	notification := migrationFunction(t, sql, "record_booking_approval_notification")
	if !strings.Contains(notification, "booking_request_sessions") || !strings.Contains(notification, "min(s.starts_at)") || !strings.Contains(notification, "max(s.ends_at)") {
		t.Fatal("approval notification must summarize all adjacent slots in the request")
	}
}
