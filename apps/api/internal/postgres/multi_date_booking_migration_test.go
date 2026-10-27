package postgres

import (
	"os"
	"strings"
	"testing"
)

func TestMultiDateBookingMigrationKeepsAtomicValidationAndRestrictedRoles(t *testing.T) {
	content, err := os.ReadFile("../../../../supabase/migrations/20261027090000_multi_date_booking_requests.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	create := migrationFunction(t, sql, "create_public_booking_request_with_participants_token")
	for _, expected := range []string{
		"v_session_count > 16",
		"ERRCODE = 'PZ012'",
		"AT TIME ZONE 'Asia/Manila'",
		"count(DISTINCT (s.starts_at AT TIME ZONE 'Asia/Manila')::date",
		") > 8",
		"PARTITION BY (s.starts_at AT TIME ZONE 'Asia/Manila')::date",
		"first_slot.starts_at < second_slot.ends_at",
		"second_slot.starts_at < first_slot.ends_at",
		"s.status <> 'open'",
		"p.is_active IS NOT TRUE",
		"s.capacity IS NOT NULL",
		"count(DISTINCT id)",
		"INSERT INTO public.booking_request_sessions",
		"SECURITY DEFINER",
		"SET search_path = ''",
	} {
		if !strings.Contains(create, expected) {
			t.Errorf("multi-date booking validation is missing %q", expected)
		}
	}
	if !strings.Contains(sql, "GRANT mmsu_booking_function_owner TO CURRENT_USER") ||
		!strings.Contains(sql, "SET ROLE mmsu_booking_function_owner") ||
		!strings.Contains(sql, "REVOKE mmsu_booking_function_owner FROM CURRENT_USER") {
		t.Fatal("booking functions must be updated under the restricted owner with temporary membership cleanup")
	}
	if !strings.Contains(sql, "GRANT EXECUTE ON FUNCTION public.admin_list_pending_booking_requests_with_slots()\n    TO mmsu_booking_admin_api") ||
		strings.Contains(sql, "GRANT EXECUTE ON FUNCTION public.admin_list_pending_booking_requests_with_slots()\n    TO mmsu_booking_api") {
		t.Fatal("detailed pending requests must remain restricted to the admin API role")
	}
}

func TestPendingBookingDetailsMigrationReturnsSubmissionFIFOAndSlotDetails(t *testing.T) {
	content, err := os.ReadFile("../../../../supabase/migrations/20261027090000_multi_date_booking_requests.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	start := strings.Index(sql, "CREATE FUNCTION public.admin_list_pending_booking_requests_with_slots()")
	if start < 0 {
		t.Fatal("detailed admin queue function is missing")
	}
	function := sql[start:]
	for _, expected := range []string{
		"created_at timestamptz",
		"requested_slots jsonb",
		"jsonb_agg(",
		"'session_id', s.id",
		"'program_name', p.name",
		"ORDER BY br.created_at ASC, br.id ASC",
		"SECURITY DEFINER",
		"SET search_path = ''",
	} {
		if !strings.Contains(function, expected) {
			t.Errorf("admin queue function is missing %q", expected)
		}
	}
}
