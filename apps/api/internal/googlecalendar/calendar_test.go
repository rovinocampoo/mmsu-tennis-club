package googlecalendar

import (
	"testing"

	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/training"
)

func TestSyncEventUsesStablePerSessionIDAndLegacySingleSessionID(t *testing.T) {
	first := training.CalendarEvent{EventID: "mmsurequest-session1", RequestID: "request-id"}
	second := training.CalendarEvent{EventID: "mmsurequest-session2", RequestID: "request-id"}
	if syncEventID(first) != first.EventID || syncEventID(second) != second.EventID || syncEventID(first) == syncEventID(second) {
		t.Fatal("sessions in one request must have distinct retry-stable calendar IDs")
	}
	if got, want := syncEventID(training.CalendarEvent{RequestID: "request-id"}), eventID("request-id"); got != want {
		t.Fatalf("legacy single-session event ID=%q, want %q", got, want)
	}
}
