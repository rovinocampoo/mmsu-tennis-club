package postgres

import (
	"os"
	"strings"
	"testing"
)

func TestApprovedClientsMigrationReturnsParticipantSelectedProgramSafely(t *testing.T) {
	content, err := os.ReadFile("../../../../supabase/migrations/20261020000000_show_selected_program_in_approved_clients.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	for _, expected := range []string{
		"DROP FUNCTION public.admin_list_approved_clients()",
		"selected_training_program_name text",
		"LEFT JOIN public.booking_requests AS br",
		"LEFT JOIN public.training_programs AS selected_program",
		"selected_program.id::text = br.training_focus",
		"COALESCE(selected_program.name, '')",
		"SECURITY DEFINER",
		"SET search_path = ''",
		"GRANT EXECUTE ON FUNCTION public.admin_list_approved_clients()",
		"TO mmsu_booking_admin_api",
		"REVOKE mmsu_schedule_admin_function_owner FROM CURRENT_USER",
	} {
		if !strings.Contains(sql, expected) {
			t.Errorf("approved-client display migration is missing %q", expected)
		}
	}
	if strings.Contains(sql, "GRANT SELECT ON TABLE public.training_programs TO mmsu_booking_admin_api") {
		t.Fatal("admin API must not receive direct training-program table access")
	}
	if strings.Contains(sql, "selected_program.is_active") {
		t.Fatal("historical bookings must retain the display name after a program is deactivated")
	}
}
