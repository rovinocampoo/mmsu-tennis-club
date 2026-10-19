package postgres

import (
	"os"
	"strings"
	"testing"
)

func readMigrationFile(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile("../../../../supabase/migrations/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestCoachAssignmentMigrationRestoresConcurrencyGuards(t *testing.T) {
	sql := readMigrationFile(t, "20261019160000_restore_coach_assignment_guards.sql")
	assign := migrationFunction(t, sql, "admin_assign_training_session_coach")
	for _, expected := range []string{
		"pg_catalog.pg_advisory_xact_lock",
		"hashtextextended('mmsu-training-schedule-write', 0)",
		"FOR UPDATE",
		"FROM public.coaches AS c",
		"c.is_active IS TRUE",
		"ERRCODE = 'PZ004'",
		"ERRCODE = 'PZ007'",
		"ERRCODE = 'P0002'",
		"ERRCODE = 'PZ001'",
		"s.status <> 'cancelled'",
	} {
		if !strings.Contains(assign, expected) {
			t.Errorf("restored assignment function missing %q", expected)
		}
	}
	// The unassign branch must run before the cancelled-session guard so an
	// admin can always clear an assignment.
	unassign := strings.Index(assign, "DELETE FROM public.coach_session_assignments")
	cancelled := strings.Index(assign, "v_status = 'cancelled'")
	if unassign < 0 || cancelled < 0 || unassign > cancelled {
		t.Fatal("unassign must be allowed before the cancelled-session guard")
	}
}

func TestBulkPublishMigrationIsConservativeAndAtomic(t *testing.T) {
	sql := readMigrationFile(t, "20261019170000_add_bulk_publish_training_sessions.sql")
	publish := migrationFunction(t, sql, "admin_publish_training_sessions")
	for _, expected := range []string{
		"pg_catalog.pg_advisory_xact_lock",
		"is_public IS NOT TRUE",
		"s.status = 'open'",
		"s.starts_at >= p_starts_at",
		"s.starts_at < p_ends_at",
		"'published'",
		"'skipped'",
		"'failed'",
		"ERRCODE = '22023'",
	} {
		if !strings.Contains(publish, expected) {
			t.Errorf("bulk publish function missing %q", expected)
		}
	}
	if !strings.Contains(sql, "GRANT EXECUTE ON FUNCTION public.admin_publish_training_sessions(timestamptz, timestamptz)\n    TO mmsu_booking_admin_api") {
		t.Fatal("bulk publish function must be executable only by the admin API role")
	}
	if strings.Contains(sql, "TO mmsu_booking_api;") {
		t.Fatal("bulk publish must not be granted to the public booking role")
	}
}
