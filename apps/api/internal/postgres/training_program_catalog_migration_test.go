package postgres

import (
	"os"
	"strings"
	"testing"
)

func TestTrainingProgramCatalogMigrationUsesRestrictedAdminFunctions(t *testing.T) {
	content, err := os.ReadFile("../../../../supabase/migrations/20261019230000_admin_training_program_catalog.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	for _, name := range []string{
		"get_training_program_name",
		"admin_list_training_program_catalog",
		"admin_create_training_program",
		"admin_update_training_program",
	} {
		function := migrationFunction(t, sql, name)
		if !strings.Contains(function, "SECURITY DEFINER") || !strings.Contains(function, "SET search_path = ''") {
			t.Errorf("%s must be SECURITY DEFINER with an empty search_path", name)
		}
	}
	historyLookup := migrationFunction(t, sql, "get_training_program_name")
	if strings.Contains(historyLookup, "is_active") {
		t.Fatal("historical booking display must resolve names for deactivated programs")
	}
	if !strings.Contains(sql, "SET ROLE mmsu_schedule_admin_function_owner") ||
		!strings.Contains(sql, "TO mmsu_booking_admin_api") {
		t.Fatal("catalog functions must be created under the restricted schedule admin owner and executable by the admin API")
	}
	if !strings.Contains(sql, "GRANT EXECUTE ON FUNCTION public.get_training_program_name(text)\n    TO mmsu_booking_api") ||
		strings.Contains(sql, "GRANT EXECUTE ON FUNCTION public.get_training_program_name(text)\n    TO mmsu_booking_admin_api") {
		t.Fatal("historical name lookup must be callable only by the public booking API among application roles")
	}
	if strings.Contains(strings.ToUpper(sql), "DELETE FROM PUBLIC.TRAINING_PROGRAMS") ||
		strings.Contains(strings.ToUpper(sql), "GRANT DELETE ON TABLE PUBLIC.TRAINING_PROGRAMS") {
		t.Fatal("program removal must use the is_active soft-delete field")
	}
	for _, expected := range []string{
		"CREATE POLICY training_programs_schedule_admin_insert",
		"CREATE POLICY training_programs_schedule_admin_update",
		"UPDATE public.training_programs",
		"is_active = p_is_active",
		"ORDER BY tp.is_active DESC, tp.name ASC",
		"REVOKE mmsu_schedule_admin_function_owner FROM CURRENT_USER",
	} {
		if !strings.Contains(sql, expected) {
			t.Errorf("catalog migration is missing %q", expected)
		}
	}
}
