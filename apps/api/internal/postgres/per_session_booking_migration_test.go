package postgres

import (
	"os"
	"strings"
	"testing"
)

func TestPerSessionBookingMigrationRemovesAdjacencyRuleAndKeepsSlotSafety(t *testing.T) {
	content, err := os.ReadFile("../../../../supabase/migrations/20261029100000_per_session_booking_decisions.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	create := migrationFunction(t, sql, "create_public_booking_request_with_participants_token")
	for _, expected := range []string{"count(DISTINCT (s.starts_at AT TIME ZONE 'Asia/Manila')::date", "v_session_count > 16", "ERRCODE = 'PZ013'", "ERRCODE = 'PZ015'", "s.status <> 'open'", "p.is_active IS NOT TRUE", "s.capacity IS NOT NULL", "INSERT INTO public.booking_request_sessions"} {
		if !strings.Contains(create, expected) {
			t.Errorf("updated booking validation is missing %q", expected)
		}
	}
	if strings.Contains(create, "lead(") {
		t.Fatal("independent non-overlapping sessions must not require adjacency")
	}
	if !strings.Contains(create, "count(DISTINCT s.program_id)") || !strings.Contains(create, "count(DISTINCT pg_catalog.lower(pg_catalog.btrim(s.location)))") {
		t.Fatal("requests must preserve the existing same-program and same-location business rules")
	}
}

func TestPerSessionApprovalMigrationIsAtomicAndRestricted(t *testing.T) {
	content, err := os.ReadFile("../../../../supabase/migrations/20261029100000_per_session_booking_decisions.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	approve := migrationFunction(t, sql, "approve_booking_request_sessions")
	for _, expected := range []string{"FOR UPDATE", "rs.status = 'pending'", "FOR UPDATE OF s", "b.status = 'confirmed'", "INSERT INTO public.bookings", "status = 'approved'", "'request_status'"} {
		if !strings.Contains(approve, expected) {
			t.Errorf("selective approval function is missing %q", expected)
		}
	}
	lock := strings.Index(approve, "FOR UPDATE OF s")
	capacity := strings.Index(approve, "b.status = 'confirmed'")
	insert := strings.Index(approve, "INSERT INTO public.bookings")
	decision := strings.Index(approve, "UPDATE public.booking_request_sessions SET status = 'approved'")
	aggregate := strings.Index(approve, "UPDATE public.booking_requests AS br")
	if lock < 0 || capacity <= lock || insert <= capacity || decision <= insert || aggregate <= decision {
		t.Fatal("selected approval must lock, validate capacity, insert atomically, then update session and request decisions")
	}
	if !strings.Contains(sql, "GRANT EXECUTE ON FUNCTION public.approve_booking_request_sessions(uuid, uuid, uuid, boolean, jsonb)\n    TO mmsu_booking_admin_api") ||
		strings.Contains(sql, "GRANT EXECUTE ON FUNCTION public.approve_booking_request_sessions(uuid, uuid, uuid, boolean, jsonb)\n    TO mmsu_booking_api") {
		t.Fatal("selective approval must be executable only by the admin API role")
	}
	if !strings.Contains(sql, "status IN ('pending', 'partially_decided', 'approved', 'rejected')") || !strings.Contains(sql, "WHERE br.status IN ('pending', 'partially_decided')") {
		t.Fatal("mixed outcomes and still-pending requests must remain reviewable")
	}
	if !strings.Contains(approve, "WHEN EXISTS (SELECT 1 FROM public.booking_request_sessions rs WHERE rs.request_id = p_request_id AND rs.status = 'pending') THEN 'pending'") ||
		!strings.Contains(approve, "THEN 'partially_decided'") ||
		!strings.Contains(approve, "rs.session_id = ANY(v_session_ids) AND rs.status = 'pending'") {
		t.Fatal("aggregate status must retain pending unselected sessions and approve only the submitted session IDs")
	}
	if !strings.Contains(sql, "'status', rs.status") || !strings.Contains(sql, "public.get_public_booking_status") {
		t.Fatal("public status must report individual session decisions")
	}

	reject := migrationFunction(t, sql, "reject_booking_request")
	if !strings.Contains(reject, "WHERE request_id = p_request_id AND status = 'pending'") ||
		!strings.Contains(reject, "rs.status = 'approved') THEN 'partially_decided'") {
		t.Fatal("rejecting pending slots must preserve approved slots and aggregate to partially_decided")
	}
	if !strings.Contains(sql, "DROP CONSTRAINT booking_requests_decision_consistent") ||
		!strings.Contains(sql, "ADD CONSTRAINT booking_requests_decision_consistent CHECK") ||
		!strings.Contains(sql, "status = 'pending' AND decided_by IS NOT NULL AND decided_at IS NOT NULL AND resolved_player_id IS NOT NULL") ||
		!strings.Contains(sql, "status = 'partially_decided' AND decided_by IS NOT NULL AND decided_at IS NOT NULL AND resolved_player_id IS NOT NULL") ||
		!strings.Contains(sql, "status IN ('approved', 'rejected') AND decided_by IS NOT NULL AND decided_at IS NOT NULL") {
		t.Fatal("request decision constraint must accept partial decisions while requiring complete decision fields")
	}
}
