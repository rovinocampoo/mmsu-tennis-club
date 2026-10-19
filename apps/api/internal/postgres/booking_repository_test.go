package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/notifications"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/training"
)

type queryRowStub struct {
	query    string
	args     []any
	value    any
	err      error
	queryErr error
}

func (s *queryRowStub) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	s.query, s.args = query, args
	return queryRowFixture{value: s.value, err: s.err}
}
func (s *queryRowStub) Query(_ context.Context, query string, _ ...any) (pgx.Rows, error) {
	s.query = query
	if s.queryErr != nil {
		return nil, s.queryErr
	}
	return nil, errors.New("unexpected query")
}

type queryRowFixture struct {
	value any
	err   error
}

func (r queryRowFixture) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != 1 {
		return errors.New("unexpected scan destination count")
	}
	switch target := dest[0].(type) {
	case *string:
		value, ok := r.value.(string)
		if !ok {
			return errors.New("unexpected row value type")
		}
		*target = value
	case *bool:
		value, ok := r.value.(bool)
		if !ok {
			return errors.New("unexpected row value type")
		}
		*target = value
	case *[]byte:
		value, ok := r.value.([]byte)
		if !ok {
			return errors.New("unexpected row value type")
		}
		*target = append([]byte(nil), value...)
	default:
		return errors.New("unexpected scan destination")
	}
	return nil
}

type notificationQueryStub struct {
	query string
	args  []any
	row   pgx.Row
}

func (s *notificationQueryStub) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	s.query, s.args = query, args
	return s.row
}
func (s *notificationQueryStub) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("unexpected query")
}

type notificationClaimRow struct{}

func (notificationClaimRow) Scan(dest ...any) error {
	if len(dest) != 10 {
		return errors.New("unexpected notification scan destination count")
	}
	*dest[0].(*string) = "sending"
	*dest[1].(*string) = ""
	*dest[2].(*string) = "requester@example.test"
	*dest[3].(*string) = "MMSU Training"
	*dest[4].(*time.Time) = time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	*dest[5].(*time.Time) = time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC)
	*dest[6].(*string) = "MMSU Courts"
	*dest[7].(*string) = "MMSU-REQUEST"
	*dest[8].(*int) = 1
	*dest[9].(*bool) = true
	return nil
}

type publicBookingStatusRow struct {
	status  training.PublicBookingStatus
	expired bool
}

func (r publicBookingStatusRow) Scan(dest ...any) error {
	if len(dest) != 10 {
		return errors.New("unexpected public status scan destination count")
	}
	*dest[0].(*string) = r.status.RequestReference
	*dest[1].(*string) = r.status.Status
	*dest[2].(*string) = r.status.ProgramName
	*dest[3].(*time.Time) = r.status.StartsAt
	*dest[4].(*time.Time) = r.status.EndsAt
	*dest[5].(*string) = r.status.Location
	*dest[6].(*bool) = r.expired
	*dest[7].(*string) = r.status.TrainingFocus
	*dest[8].(*string) = r.status.TrainingFocusNotes
	*dest[9].(*string) = r.status.TrainingProgramName
	return nil
}

func TestBookingRepositoryClaimsApprovalNotificationThroughAdminFunction(t *testing.T) {
	db := &notificationQueryStub{row: notificationClaimRow{}}
	claim, err := NewBookingRepository(db).ClaimApprovalNotification(context.Background(), "request-id", true)
	if err != nil {
		t.Fatal(err)
	}
	if claim.Status != notifications.StatusSending || !claim.Claimed || claim.AttemptCount != 1 || claim.RecipientEmail != "requester@example.test" {
		t.Fatalf("claim=%#v", claim)
	}
	if !strings.Contains(db.query, "claim_booking_approval_notification") || !reflect.DeepEqual(db.args, []any{"request-id", true}) {
		t.Fatalf("query=%q args=%#v", db.query, db.args)
	}
}

func TestBookingRepositoryCompletesApprovalNotificationThroughAdminFunction(t *testing.T) {
	db := &queryRowStub{value: true}
	resendID := "resend-id"
	updated, err := NewBookingRepository(db).CompleteApprovalNotification(context.Background(), "request-id", 2, true, resendID)
	if err != nil || !updated {
		t.Fatalf("updated=%t err=%v", updated, err)
	}
	if !strings.Contains(db.query, "complete_booking_approval_notification") || !reflect.DeepEqual(db.args, []any{"request-id", 2, true, &resendID}) {
		t.Fatalf("query=%q args=%#v", db.query, db.args)
	}
}

func TestBookingRepositoryCreatesPendingRequestFunction(t *testing.T) {
	db := &queryRowStub{value: "request-id"}
	hash := make([]byte, 32)
	id, err := NewBookingRepository(db).CreateRequest(context.Background(), training.BookingRequest{SessionID: "session-id", FullName: "Alex Player", Email: "alex@example.com", TrainingFocus: "beginner_fundamentals", TrainingFocusNotes: ""}, hash)
	if err != nil || id != "request-id" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if !strings.Contains(db.query, "create_public_booking_request_with_participants_token") || len(db.args) != 7 {
		t.Fatalf("query=%q args count=%d", db.query, len(db.args))
	}
}

func TestBookingRepositoryNormalizesNullableTrainingFocusForAdminList(t *testing.T) {
	db := &queryRowStub{queryErr: errors.New("query fixture")}
	_, err := NewBookingRepository(db).ListPendingRequests(context.Background())
	if err == nil {
		t.Fatal("expected fixture query error")
	}
	if !strings.Contains(db.query, "COALESCE(requests.training_focus, '')") || !strings.Contains(db.query, "COALESCE(requests.training_focus_notes, '')") || !strings.Contains(db.query, "admin_list_training_program_catalog()") || !strings.Contains(db.query, "program.id::text = requests.training_focus") {
		t.Fatalf("admin list query must normalize nullable training-focus fields: %q", db.query)
	}
}

func TestBookingRepositorySendsEveryParticipantToTheRequestFunction(t *testing.T) {
	db := &queryRowStub{value: "request-id"}
	hash := make([]byte, 32)
	_, err := NewBookingRepository(db).CreateRequest(context.Background(), training.BookingRequest{
		SessionIDs:         []string{"session-1"},
		FullName:           "Alex, Sam",
		Participants:       []string{"Alex", "Sam"},
		TrainingFocus:      "forehand_backhand",
		TrainingFocusNotes: "",
	}, hash)
	if err != nil {
		t.Fatal(err)
	}
	if len(db.args) != 7 {
		t.Fatalf("expected 7 args, got %d", len(db.args))
	}
}

func TestBookingRepositoryCreatesSessionBatchInOneFunctionCall(t *testing.T) {
	db := &queryRowStub{value: []byte(`["session-1","session-2"]`)}
	start := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	ids, err := NewBookingRepository(db).CreateTrainingSessions(context.Background(), []training.TrainingSessionCreateInput{
		{ProgramID: "program-1", StartsAt: start, EndsAt: start.Add(time.Hour), Location: "Court 1", IsPublic: true},
		{ProgramID: "program-1", StartsAt: start.Add(time.Hour), EndsAt: start.Add(2 * time.Hour), Location: "Court 1", IsPublic: true},
	})
	if err != nil || !reflect.DeepEqual(ids, []string{"session-1", "session-2"}) {
		t.Fatalf("ids=%#v err=%v", ids, err)
	}
	if !strings.Contains(db.query, "admin_create_training_sessions_batch") || !strings.Contains(db.query, "$1::jsonb") || len(db.args) != 1 {
		t.Fatalf("query=%q args=%#v", db.query, db.args)
	}
	var payload []training.TrainingSessionCreateInput
	if err := json.Unmarshal([]byte(db.args[0].(string)), &payload); err != nil || len(payload) != 2 {
		t.Fatalf("payload=%#v err=%v", payload, err)
	}
}

func TestBookingRepositoryEditsFullSessionWithAcknowledgement(t *testing.T) {
	db := &queryRowStub{value: true}
	start := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	capacity := 6
	err := NewBookingRepository(db).EditTrainingSession(context.Background(), "session-id", training.TrainingSessionEditInput{
		ProgramID: "program-id", StartsAt: start, EndsAt: start.Add(2 * time.Hour), Location: "Court 2",
		Capacity: &capacity, Notes: "updated", IsPublic: true, AcknowledgeBookedChanges: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(db.query, "admin_edit_training_session") || len(db.args) != 9 || db.args[0] != "session-id" || db.args[1] != "program-id" || db.args[8] != true {
		t.Fatalf("query=%q args=%#v", db.query, db.args)
	}
}

func TestBookingRepositoryGetsPublicStatusAndMapsExpiry(t *testing.T) {
	want := training.PublicBookingStatus{
		RequestReference:    "MMSU-1234567890ABCDEF",
		Status:              "pending",
		ProgramName:         "Junior Training",
		StartsAt:            time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC),
		EndsAt:              time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC),
		Location:            "MMSU Courts",
		TrainingFocus:       "beginner_fundamentals",
		TrainingProgramName: "Beginner Fundamentals",
		TrainingFocusNotes:  "",
	}
	db := &notificationQueryStub{row: publicBookingStatusRow{status: want}}
	hash := make([]byte, 32)
	got, err := NewBookingRepository(db).GetPublicStatus(context.Background(), hash)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("status=%#v err=%v", got, err)
	}
	if !strings.Contains(db.query, "get_public_booking_status") || !strings.Contains(db.query, "COALESCE(status.training_focus, '')") || !strings.Contains(db.query, "COALESCE(status.training_focus_notes, '')") || !strings.Contains(db.query, "public.get_training_program_name(status.training_focus)") || strings.Contains(db.query, "public.training_programs") || !reflect.DeepEqual(db.args, []any{hash}) {
		t.Fatalf("query=%q args=%#v", db.query, db.args)
	}

	// Older requests and requests without an optional selection return empty
	// strings after the SQL COALESCE expressions, rather than failing the scan.
	unselected := training.PublicBookingStatus{
		RequestReference: "MMSU-OLDREQUEST123456",
		Status:           "pending",
		ProgramName:      "Junior Training",
		StartsAt:         want.StartsAt,
		EndsAt:           want.EndsAt,
		Location:         want.Location,
	}
	db.row = publicBookingStatusRow{status: unselected}
	got, err = NewBookingRepository(db).GetPublicStatus(context.Background(), hash)
	if err != nil || !reflect.DeepEqual(got, unselected) {
		t.Fatalf("unselected status=%#v err=%v", got, err)
	}

	missingOrInactive := unselected
	missingOrInactive.TrainingFocus = "missing-or-inactive-program-id"
	db.row = publicBookingStatusRow{status: missingOrInactive}
	got, err = NewBookingRepository(db).GetPublicStatus(context.Background(), hash)
	if err != nil || !reflect.DeepEqual(got, missingOrInactive) {
		t.Fatalf("missing or inactive selected program status=%#v err=%v", got, err)
	}

	db.row = publicBookingStatusRow{status: want, expired: true}
	if _, err := NewBookingRepository(db).GetPublicStatus(context.Background(), hash); !errors.Is(err, training.ErrBookingStatusExpired) {
		t.Fatalf("expired status error=%v", err)
	}
}

func TestBookingRepositoryUsesProtectedReviewFunctions(t *testing.T) {
	db := &queryRowStub{value: "booking-id"}
	repo := NewBookingRepository(db)
	playerID := "player-id"
	id, err := repo.ApproveRequest(context.Background(), "request-id", "admin-id", &playerID, false)
	if err != nil || id != "booking-id" || !strings.Contains(db.query, "approve_booking_request") {
		t.Fatalf("id=%q err=%v query=%q", id, err, db.query)
	}
	if !reflect.DeepEqual(db.args, []any{"request-id", "admin-id", &playerID, false}) {
		t.Fatalf("args=%#v", db.args)
	}
	db.value = true
	if err := repo.RejectRequest(context.Background(), "request-id", "admin-id"); err != nil || !strings.Contains(db.query, "reject_booking_request") {
		t.Fatalf("err=%v query=%q", err, db.query)
	}
}

func ptr(value string) *string { return &value }
