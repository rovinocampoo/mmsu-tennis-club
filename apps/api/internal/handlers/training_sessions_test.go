package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/training"
)

type trainingRepositoryStub struct {
	sessions []training.Session
	err      error
}

func (r trainingRepositoryStub) ListUpcoming(context.Context, time.Time) ([]training.Session, error) {
	return r.sessions, r.err
}

func trainingSessionsRequest(service *training.Service) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/training-sessions", nil)
	response := httptest.NewRecorder()
	TrainingSessions(service, log.New(io.Discard, "", 0))(response, request)
	return response
}

func TestTrainingSessionsReturnsPublicScheduleFields(t *testing.T) {
	description := "Group lessons for beginners"
	capacity := 8
	remaining := 5
	unlimitedDescription := "Open practice"
	start := time.Date(2026, 10, 10, 9, 0, 0, 0, time.FixedZone("Asia/Manila", 8*60*60))
	sessions := []training.Session{
		{
			ID:                    "11111111-1111-4111-8111-111111111111",
			ProgramName:           "Beginner Tennis",
			ProgramDescription:    &description,
			StartsAt:              start,
			EndsAt:                start.Add(time.Hour),
			Location:              "Court 1",
			Capacity:              &capacity,
			ConfirmedBookingCount: 3,
			RemainingCapacity:     &remaining,
			Status:                "open",
			IsPublic:              true,
		},
		{
			ID:                    "22222222-2222-4222-8222-222222222222",
			ProgramName:           "Open Court",
			ProgramDescription:    &unlimitedDescription,
			StartsAt:              start.Add(24 * time.Hour),
			EndsAt:                start.Add(25 * time.Hour),
			Location:              "Court 2",
			ConfirmedBookingCount: 1,
			Status:                "open",
			IsPublic:              true,
		},
		{
			ID:                    "33333333-3333-4333-8333-333333333333",
			ProgramName:           "Full Session",
			StartsAt:              start.Add(48 * time.Hour),
			EndsAt:                start.Add(49 * time.Hour),
			Location:              "Court 3",
			Capacity:              &capacity,
			ConfirmedBookingCount: 8,
			RemainingCapacity:     new(int),
			Status:                "open",
			IsPublic:              true,
		},
	}
	response := trainingSessionsRequest(training.NewService(trainingRepositoryStub{sessions: sessions}))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusOK, response.Body)
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	var got []map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("returned %d sessions, want 3", len(got))
	}
	assertJSONNumber(t, got[0], "confirmed_booking_count", "3")
	assertJSONNumber(t, got[0], "remaining_capacity", "5")
	assertJSONNumber(t, got[0], "capacity", "8")
	assertJSONNull(t, got[1], "remaining_capacity")
	assertJSONNull(t, got[1], "capacity")
	assertJSONNumber(t, got[2], "confirmed_booking_count", "8")
	assertJSONNumber(t, got[2], "remaining_capacity", "0")
	for _, excluded := range []string{"players", "email", "phone", "notes", "bookings", "external_booking_reference"} {
		if strings.Contains(response.Body.String(), `"`+excluded+`"`) {
			t.Errorf("response exposes excluded field %q", excluded)
		}
	}
}

func TestTrainingSessionsReturnsEmptyArrayWhenNoneExist(t *testing.T) {
	response := trainingSessionsRequest(training.NewService(trainingRepositoryStub{}))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := strings.TrimSpace(response.Body.String()); got != "[]" {
		t.Errorf("body = %q, want []", got)
	}
}

func TestTrainingSessionsReturnsSafe503WhenDatabaseUnavailable(t *testing.T) {
	response := trainingSessionsRequest(training.NewService(nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if body["error"] != "training sessions are temporarily unavailable" {
		t.Errorf("error = %q, want safe temporary-unavailable message", body["error"])
	}
}

func TestTrainingSessionsDoesNotExposeRepositoryError(t *testing.T) {
	databaseError := "connection failed: password=do-not-return"
	response := trainingSessionsRequest(training.NewService(trainingRepositoryStub{err: errors.New(databaseError)}))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if strings.Contains(response.Body.String(), databaseError) {
		t.Errorf("response exposed supplied repository error %q: %s", databaseError, response.Body)
	}
}

func assertJSONNumber(t *testing.T, fields map[string]json.RawMessage, name, expected string) {
	t.Helper()
	var got string
	if err := json.Unmarshal(fields[name], &got); err == nil {
		t.Fatalf("%s unexpectedly decoded as string %q", name, got)
	}
	if got := strings.TrimSpace(string(fields[name])); got != expected {
		t.Errorf("%s = %s, want %s", name, got, expected)
	}
}

func assertJSONNull(t *testing.T, fields map[string]json.RawMessage, name string) {
	t.Helper()
	if got := strings.TrimSpace(string(fields[name])); got != "null" {
		t.Errorf("%s = %s, want null", name, got)
	}
}
