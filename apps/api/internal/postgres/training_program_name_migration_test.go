package postgres

import (
	"os"
	"strings"
	"testing"
)

func TestActiveTrainingProgramNameLookupMigrationKeepsLeastPrivilege(t *testing.T) {
	content, err := os.ReadFile("../../../../supabase/migrations/20261019220000_add_active_training_program_name_lookup.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	lookup := migrationFunction(t, sql, "get_active_training_program_name")
	for _, expected := range []string{
		"RETURNS text",
		"STABLE",
		"STRICT",
		"SECURITY DEFINER",
		"SET search_path = ''",
		"FROM public.training_programs AS tp",
		"tp.id::text = p_program_id",
		"tp.is_active IS TRUE",
	} {
		if !strings.Contains(lookup, expected) {
			t.Errorf("active program name lookup is missing %q", expected)
		}
	}
	if !strings.Contains(sql, "SET ROLE mmsu_booking_function_owner") {
		t.Fatal("lookup must be created under its restricted function owner")
	}
	if !strings.Contains(sql, "GRANT EXECUTE ON FUNCTION public.get_active_training_program_name(text)\n    TO mmsu_booking_api") {
		t.Fatal("lookup must be executable by the public booking API role")
	}
	if !strings.Contains(sql, "REVOKE ALL ON FUNCTION public.get_active_training_program_name(text)\n    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,") {
		t.Fatal("lookup must revoke the default/public function grants")
	}
	if strings.Contains(sql, "GRANT EXECUTE ON FUNCTION public.get_active_training_program_name(text)\n    TO mmsu_booking_admin_api") ||
		strings.Contains(sql, "GRANT SELECT ON TABLE public.training_programs TO mmsu_booking_api") ||
		strings.Contains(sql, "GRANT SELECT (id, name, is_active) ON TABLE public.training_programs TO mmsu_booking_api") {
		t.Fatal("lookup must not broaden public or admin database permissions")
	}
	if !strings.Contains(sql, "REVOKE CREATE ON SCHEMA public FROM mmsu_booking_function_owner") ||
		!strings.Contains(sql, "REVOKE mmsu_booking_function_owner FROM CURRENT_USER") {
		t.Fatal("temporary ownership-transfer privileges must be revoked")
	}
}
