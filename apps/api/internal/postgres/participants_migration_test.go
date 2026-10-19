package postgres

import (
	"os"
	"strings"
	"testing"
)

func readParticipantsMigration(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile("../../../../supabase/migrations/20261019140000_add_booking_request_participants.sql")
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestParticipantsMigrationNormalizesRequestAndBookingParticipants(t *testing.T) {
	sql := readParticipantsMigration(t)
	for _, expected := range []string{
		"CREATE TABLE public.booking_request_participants",
		"CREATE TABLE public.booking_participants",
		"REFERENCES public.booking_requests(id) ON DELETE CASCADE",
		"REFERENCES public.bookings(id) ON DELETE CASCADE",
		"PRIMARY KEY (request_id, position)",
		"PRIMARY KEY (booking_id, position)",
		"INSERT INTO public.booking_request_participants",
	} {
		if !strings.Contains(sql, expected) {
			t.Errorf("participants migration missing %q", expected)
		}
	}
	if strings.Contains(sql, "DROP TABLE") {
		t.Fatal("participants migration must not drop existing data")
	}
}

func TestParticipantsCreateFunctionValidatesAndStoresEachName(t *testing.T) {
	create := migrationFunction(t, readParticipantsMigration(t), "create_public_booking_request_with_participants_token")
	for _, expected := range []string{
		"pg_catalog.jsonb_typeof(p_participants) IS DISTINCT FROM 'array'",
		"jsonb_array_elements_text(p_participants)",
		"v_participant_count < 1",
		"array_to_string(v_participants, ', ')",
		"INSERT INTO public.booking_request_participants",
		"booking-request-name:",
		"ERRCODE = 'PZ004'",
		"ERRCODE = 'PZ002'",
		"FOR SHARE OF s",
	} {
		if !strings.Contains(create, expected) {
			t.Errorf("participants create function missing %q", expected)
		}
	}
	if strings.Contains(create, "v_email IS NULL AND v_phone IS NULL") {
		t.Fatal("participants create function must not require a contact method")
	}
}

func TestParticipantsApprovalCopiesParticipantsBeforeDecision(t *testing.T) {
	approve := migrationFunction(t, readParticipantsMigration(t), "approve_booking_request")
	lock := strings.Index(approve, "FOR UPDATE OF s")
	insert := strings.Index(approve, "INSERT INTO public.bookings")
	copyIndex := strings.Index(approve, "INSERT INTO public.booking_participants")
	decision := strings.Index(approve, "UPDATE public.booking_requests")
	if lock < 0 || insert < lock || copyIndex <= insert || decision <= copyIndex {
		t.Fatal("approval must lock sessions, create bookings, copy participants, then mark the request approved")
	}
	if !strings.Contains(approve, "ERRCODE = 'PZ002'") || !strings.Contains(approve, "FOR v_session_id IN") {
		t.Fatal("approval must preserve capacity checks and reserve every requested slot")
	}
}

func TestLegacySlotsSignatureDelegatesToParticipants(t *testing.T) {
	slots := migrationFunction(t, readParticipantsMigration(t), "create_public_booking_request_with_slots_token")
	if !strings.Contains(slots, "create_public_booking_request_with_participants_token") {
		t.Fatal("legacy slots signature must delegate to the participants function for rolling deploys")
	}
}
