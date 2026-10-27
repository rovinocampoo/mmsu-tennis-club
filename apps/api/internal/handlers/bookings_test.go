package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/notifications"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/ratelimit"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/training"
)

type bookingStub struct {
	request           training.BookingRequest
	tokenHash         []byte
	createdID         string
	err               error
	status            training.PublicBookingStatus
	statusErr         error
	createCalls       int
	statusCalls       int
	listCalls         int
	searchCalls       int
	approveCalls      int
	rejectCalls       int
	adminID           string
	playerID          *string
	createNew         bool
	approvalCommitted bool
	approvedClients   []training.ApprovedClientRecord
	feedback          training.FeedbackSubmission
	feedbackRecords   []training.FeedbackRecord
	feedbackCalls     int
	feedbackListCalls int
	feedbackUpdateID  string
	feedbackStatus    string
	feedbackAdminID   string
	feedbackUpdated   bool
}

func (s *bookingStub) CreateRequest(_ context.Context, request training.BookingRequest, tokenHash []byte) (string, error) {
	s.createCalls++
	s.request = request
	s.tokenHash = append([]byte(nil), tokenHash...)
	return s.createdID, s.err
}
func (s *bookingStub) GetPublicStatus(_ context.Context, tokenHash []byte) (training.PublicBookingStatus, error) {
	s.statusCalls++
	s.tokenHash = append([]byte(nil), tokenHash...)
	return s.status, s.statusErr
}
func (s *bookingStub) ListPendingRequests(context.Context) ([]training.BookingRequestRecord, error) {
	s.listCalls++
	return nil, s.err
}
func (s *bookingStub) SearchPlayers(context.Context, string) ([]training.PlayerCandidate, error) {
	s.searchCalls++
	return nil, s.err
}
func (s *bookingStub) ApproveRequest(_ context.Context, _, adminID string, playerID *string, createNew bool) (string, error) {
	s.approveCalls++
	s.adminID, s.playerID, s.createNew = adminID, playerID, createNew
	s.approvalCommitted = s.err == nil
	return "booking-ref", s.err
}
func (s *bookingStub) RejectRequest(_ context.Context, _, adminID string) error {
	s.rejectCalls++
	s.adminID = adminID
	return s.err
}
func (s *bookingStub) SubmitCoachAvailability(context.Context, string, string, time.Time, time.Time, string) (string, error) {
	return "avail-id", s.err
}
func (s *bookingStub) ListCoachAvailability(context.Context, string) ([]training.CoachAvailability, error) {
	return nil, s.err
}
func (s *bookingStub) DeleteCoachAvailability(context.Context, string, string) error {
	return s.err
}
func (s *bookingStub) ListAllCoachAvailability(context.Context) ([]training.CoachAvailability, error) {
	return nil, s.err
}
func (s *bookingStub) ListAdminTrainingPrograms(context.Context) ([]training.TrainingProgram, error) {
	return nil, s.err
}
func (s *bookingStub) ListAdminTrainingSessions(context.Context) ([]training.AdminSessionRecord, error) {
	return nil, s.err
}
func (s *bookingStub) ListApprovedClients(context.Context) ([]training.ApprovedClientRecord, error) {
	return s.approvedClients, s.err
}
func (s *bookingStub) CreateFeedback(_ context.Context, submission training.FeedbackSubmission) (string, error) {
	s.feedbackCalls++
	s.feedback = submission
	return "feedback-id", s.err
}
func (s *bookingStub) ListFeedbackSubmissions(context.Context) ([]training.FeedbackRecord, error) {
	s.feedbackListCalls++
	return s.feedbackRecords, s.err
}
func (s *bookingStub) UpdateFeedbackStatus(_ context.Context, id, status, adminID string) (bool, error) {
	s.feedbackUpdateID, s.feedbackStatus, s.feedbackAdminID = id, status, adminID
	return s.feedbackUpdated, s.err
}
func (s *bookingStub) ListAssignedTrainingSessions(context.Context, string) ([]training.AssignedTrainingSession, error) {
	return nil, s.err
}
func (s *bookingStub) AssignTrainingSessionCoach(context.Context, string, *string) error {
	return s.err
}
func (s *bookingStub) CreateTrainingSession(context.Context, string, time.Time, time.Time, string, *int, string, bool) (string, error) {
	return "session-id", s.err
}
func (s *bookingStub) UpdateTrainingSession(context.Context, string, *string, *string, *bool) error {
	return s.err
}
func (s *bookingStub) ListCoaches(context.Context) ([]training.Coach, error) {
	return nil, s.err
}
func (s *bookingStub) CreateCoach(context.Context, string, string, string, string) (string, error) {
	return "coach-id", s.err
}
func (s *bookingStub) UpdateCoach(context.Context, string, string, string, string, string, bool) error {
	return s.err
}
func (s *bookingStub) ListActiveCoaches(context.Context) ([]training.CoachDirectoryEntry, error) {
	return nil, s.err
}
func (s *bookingStub) ListCoachSchedule(context.Context) ([]training.CoachScheduleSession, error) {
	return nil, s.err
}

type notifierStub struct {
	result         notifications.Result
	calls          int
	requestID      string
	approvedAtCall bool
	repo           *bookingStub
}

func (s *notifierStub) NotifyApproved(_ context.Context, requestID string) notifications.Result {
	s.calls++
	s.requestID = requestID
	s.approvedAtCall = s.repo == nil || s.repo.approvalCommitted
	return s.result
}

func TestBookingRequestCreatesPendingReferenceWithoutConfirming(t *testing.T) {
	repo := &bookingStub{createdID: "MMSU-1234567890ABCDEF"}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/booking-requests", strings.NewReader(`{"session_id":"11111111-1111-4111-8111-111111111111","full_name":"Alex Player","email":"alex@example.com"}`))
	BookingRequest(repo, log.New(io.Discard, "", 0))(w, r)
	var response bookingRequestAccepted
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	rawToken, err := base64.RawURLEncoding.DecodeString(response.TrackingToken)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(rawToken)
	if w.Code != http.StatusAccepted || response.Status != "pending" || !strings.Contains(w.Body.String(), "not a confirmed reservation") || response.RequestReference != repo.createdID {
		t.Fatalf("response = %d %s", w.Code, w.Body)
	}
	if len(repo.tokenHash) != sha256.Size || string(repo.tokenHash) != string(hash[:]) {
		t.Fatalf("stored token hash length=%d does not match generated token", len(repo.tokenHash))
	}
	if repo.createCalls != 1 || repo.request.Email != "alex@example.com" || repo.request.SessionID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("request=%#v calls=%d", repo.request, repo.createCalls)
	}
}

func TestBookingRequestAcceptsOptionalContactAndRejectsInvalidInput(t *testing.T) {
	repo := &bookingStub{createdID: "request-id"}
	for _, body := range []string{
		`{"session_id":"bad","full_name":"Alex","phone":"123"}`,
		`{"session_id":"11111111-1111-4111-8111-111111111111","full_name":"Alex","email":"bad"}`,
		`{"session_id":"11111111-1111-4111-8111-111111111111","full_name":"Alex","phone":"123","unknown":true}`,
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/booking-requests", strings.NewReader(body))
		BookingRequest(repo, nil)(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s status=%d", body, w.Code)
		}
	}
	if repo.createCalls != 0 {
		t.Fatalf("invalid inputs created %d requests", repo.createCalls)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/booking-requests", strings.NewReader(`{"session_id":"11111111-1111-4111-8111-111111111111","full_name":"Alex","phone":"+1 555 0100"}`))
	BookingRequest(repo, nil)(w, r)
	if w.Code != http.StatusAccepted || repo.request.Phone != "+1 555 0100" {
		t.Fatalf("phone request response=%d %#v", w.Code, repo.request)
	}

	repo = &bookingStub{createdID: "request-id"}
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/booking-requests", strings.NewReader(`{"session_id":"11111111-1111-4111-8111-111111111111","full_name":"Alex"}`))
	BookingRequest(repo, nil)(w, r)
	if w.Code != http.StatusAccepted || repo.createCalls != 1 {
		t.Fatalf("no-contact request response=%d calls=%d %s", w.Code, repo.createCalls, w.Body)
	}
	if repo.request.Email != "" || repo.request.Phone != "" {
		t.Fatalf("no-contact request persisted contact=%#v", repo.request)
	}
}

func TestBookingRequestForwardsNormalizedParticipants(t *testing.T) {
	repo := &bookingStub{createdID: "request-id"}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/booking-requests", strings.NewReader(`{"session_ids":["11111111-1111-4111-8111-111111111111"],"participants":["  Alex  ","Sam",""],"email":"alex@example.com"}`))
	BookingRequest(repo, nil)(w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("participants response=%d %s", w.Code, w.Body)
	}
	if len(repo.request.Participants) != 2 || repo.request.Participants[0] != "Alex" || repo.request.Participants[1] != "Sam" {
		t.Fatalf("participants not normalized: %#v", repo.request.Participants)
	}
	if repo.request.FullName != "Alex, Sam" {
		t.Fatalf("full_name fallback = %q", repo.request.FullName)
	}
	if repo.request.Email != "alex@example.com" {
		t.Fatalf("email = %q", repo.request.Email)
	}
}

func TestBookingRequestForwardsOptionalTrainingProgramSelection(t *testing.T) {
	repo := &bookingStub{createdID: "request-id"}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/booking-requests", strings.NewReader(
		`{"session_id":"11111111-1111-4111-8111-111111111111","full_name":"Alex","training_focus":"22222222-2222-4222-8222-222222222222","training_focus_notes":"  Improve serve  "}`,
	))
	BookingRequest(repo, nil)(w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("request response=%d %s", w.Code, w.Body)
	}
	if repo.request.TrainingFocus != "22222222-2222-4222-8222-222222222222" || repo.request.TrainingFocusNotes != "Improve serve" {
		t.Fatalf("training program fields were not forwarded: %#v", repo.request)
	}
}

func TestBookingRequestFallsBackToFullNameForOlderClients(t *testing.T) {
	repo := &bookingStub{createdID: "request-id"}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/booking-requests", strings.NewReader(`{"session_id":"11111111-1111-4111-8111-111111111111","full_name":"  Alex Player  "}`))
	BookingRequest(repo, nil)(w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("legacy response=%d %s", w.Code, w.Body)
	}
	if len(repo.request.Participants) != 1 || repo.request.Participants[0] != "Alex Player" {
		t.Fatalf("legacy participants = %#v", repo.request.Participants)
	}
	if repo.request.FullName != "Alex Player" {
		t.Fatalf("legacy full_name = %q", repo.request.FullName)
	}
}

func TestBookingRequestRejectsInvalidParticipants(t *testing.T) {
	tooLong := `"` + strings.Repeat("a", 201) + `"`
	tooLongCombined := `"` + strings.Repeat("b", 150) + `","` + strings.Repeat("c", 150) + `"`
	tooMany := make([]string, maxBookingParticipants+1)
	for index := range tooMany {
		tooMany[index] = `"Player"`
	}
	for _, body := range []string{
		`{"session_id":"11111111-1111-4111-8111-111111111111","participants":["  ",""]}`,
		`{"session_id":"11111111-1111-4111-8111-111111111111","participants":[` + tooLong + `]}`,
		`{"session_id":"11111111-1111-4111-8111-111111111111","participants":[` + tooLongCombined + `]}`,
		`{"session_id":"11111111-1111-4111-8111-111111111111","participants":[` + strings.Join(tooMany, ",") + `]}`,
		`{"session_id":"11111111-1111-4111-8111-111111111111"}`,
	} {
		repo := &bookingStub{createdID: "request-id"}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/booking-requests", strings.NewReader(body))
		BookingRequest(repo, nil)(w, r)
		if w.Code != http.StatusBadRequest || repo.createCalls != 0 {
			t.Errorf("body %s status=%d calls=%d", body, w.Code, repo.createCalls)
		}
	}
}

func bookingRequestBodyWithSessionCount(t *testing.T, count int) string {
	t.Helper()
	ids := make([]string, count)
	for index := range ids {
		ids[index] = fmt.Sprintf("00000000-0000-4000-8000-%012d", index+1)
	}
	body, err := json.Marshal(map[string]any{
		"session_ids":    ids,
		"participants":   []string{"Alex Player"},
		"training_focus": "22222222-2222-4222-8222-222222222222",
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestBookingRequestAcceptsSixteenSlotsInOneSubmission(t *testing.T) {
	repo := &bookingStub{createdID: "MMSU-1234567890ABCDEF"}
	request := httptest.NewRequest(http.MethodPost, "/api/booking-requests", strings.NewReader(bookingRequestBodyWithSessionCount(t, 16)))
	response := httptest.NewRecorder()
	BookingRequest(repo, nil)(response, request)
	if response.Code != http.StatusAccepted || repo.createCalls != 1 || len(repo.request.SessionIDs) != 16 {
		t.Fatalf("status=%d calls=%d slots=%d body=%s", response.Code, repo.createCalls, len(repo.request.SessionIDs), response.Body)
	}
}

func TestBookingRequestRejectsOversizedSlotList(t *testing.T) {
	repo := &bookingStub{createdID: "MMSU-1234567890ABCDEF"}
	request := httptest.NewRequest(http.MethodPost, "/api/booking-requests", strings.NewReader(bookingRequestBodyWithSessionCount(t, maxBookingSlots+1)))
	response := httptest.NewRecorder()
	BookingRequest(repo, nil)(response, request)
	if response.Code != http.StatusRequestEntityTooLarge || repo.createCalls != 0 || !strings.Contains(response.Body.String(), "16 session slots") {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, repo.createCalls, response.Body)
	}
}

func TestBookingRequestRejectsDuplicateSlotIDs(t *testing.T) {
	repo := &bookingStub{createdID: "MMSU-1234567890ABCDEF"}
	body := `{"session_ids":["11111111-1111-4111-8111-111111111111","11111111-1111-4111-8111-111111111111"],"participants":["Alex Player"]}`
	request := httptest.NewRequest(http.MethodPost, "/api/booking-requests", strings.NewReader(body))
	response := httptest.NewRecorder()
	BookingRequest(repo, nil)(response, request)
	if response.Code != http.StatusBadRequest || repo.createCalls != 0 || !strings.Contains(response.Body.String(), "selected more than once") {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, repo.createCalls, response.Body)
	}
}

func TestMultiSlotSubmissionCountsOnceTowardRateLimit(t *testing.T) {
	repo := &bookingStub{createdID: "MMSU-1234567890ABCDEF"}
	handler := ratelimit.Middleware(ratelimit.New(2, time.Hour, 10), BookingRequest(repo, nil))
	for attempt := 1; attempt <= 3; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/api/booking-requests", strings.NewReader(bookingRequestBodyWithSessionCount(t, 16)))
		request.RemoteAddr = "192.0.2.17:4000"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if attempt <= 2 && response.Code != http.StatusAccepted {
			t.Fatalf("attempt %d status=%d body=%s", attempt, response.Code, response.Body)
		}
		if attempt == 3 && (response.Code != http.StatusTooManyRequests || !strings.Contains(response.Body.String(), "too many requests")) {
			t.Fatalf("rapid repeated request status=%d body=%s", response.Code, response.Body)
		}
	}
	if repo.createCalls != 2 {
		t.Fatalf("created requests=%d, want one per accepted HTTP submission", repo.createCalls)
	}
}

func TestBookingRequestMapsDuplicateAndIneligibleSession(t *testing.T) {
	for _, tc := range []struct {
		code    string
		want    int
		message string
	}{{"PZ004", http.StatusConflict, "matching request"}, {"PZ001", http.StatusConflict, "unavailable"}, {"P0002", http.StatusConflict, "unavailable"}, {"PZ002", http.StatusConflict, "full"}, {"PZ010", http.StatusBadRequest, "adjacent"}, {"PZ012", http.StatusRequestEntityTooLarge, "booking size limit"}} {
		repo := &bookingStub{err: &pgconn.PgError{Code: tc.code, Message: "internal"}}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/booking-requests", strings.NewReader(`{"session_id":"11111111-1111-4111-8111-111111111111","full_name":"Alex","email":"alex@example.com"}`))
		BookingRequest(repo, nil)(w, r)
		if w.Code != tc.want || !strings.Contains(w.Body.String(), tc.message) || strings.Contains(w.Body.String(), "internal") {
			t.Errorf("code %s response=%d %s", tc.code, w.Code, w.Body)
		}
	}
}

func TestPublicBookingStatusValidTokenReturnsOnlyPublicFields(t *testing.T) {
	tokenBytes := []byte("12345678901234567890123456789012")
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	startsAt := time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC)
	repo := &bookingStub{status: training.PublicBookingStatus{
		RequestReference:    "MMSU-1234567890ABCDEF",
		Status:              "approved",
		ProgramName:         "Junior Training",
		StartsAt:            startsAt,
		EndsAt:              startsAt.Add(time.Hour),
		Location:            "MMSU Courts",
		TrainingFocus:       "22222222-2222-4222-8222-222222222222",
		TrainingProgramName: "Beginner Tennis",
	}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/booking-status", strings.NewReader(`{"tracking_token":"`+token+`"}`))
	PublicBookingStatus(repo, nil)(w, r)
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d headers=%v body=%s", w.Code, w.Header(), w.Body)
	}
	for _, expected := range []string{`"request_reference":"MMSU-1234567890ABCDEF"`, `"status":"approved"`, `"program_name":"Junior Training"`, `"location":"MMSU Courts"`, `"training_program_name":"Beginner Tennis"`, `"next_steps"`} {
		if !strings.Contains(w.Body.String(), expected) {
			t.Errorf("status response missing %q: %s", expected, w.Body)
		}
	}
	for _, forbidden := range []string{"full_name", "email", "phone", "decided_by", "notification", "tracking_token_hash", "request_id"} {
		if strings.Contains(w.Body.String(), forbidden) {
			t.Errorf("status response exposed %q: %s", forbidden, w.Body)
		}
	}
	hash := sha256.Sum256(tokenBytes)
	if repo.statusCalls != 1 || string(repo.tokenHash) != string(hash[:]) {
		t.Fatalf("status calls=%d hash=%x", repo.statusCalls, repo.tokenHash)
	}
}

func TestPublicBookingStatusInvalidExpiredAndCrossRequestInputs(t *testing.T) {
	validToken := base64.RawURLEncoding.EncodeToString([]byte("12345678901234567890123456789012"))
	for _, tc := range []struct {
		name       string
		body       string
		repoError  error
		wantStatus int
		wantCalls  int
	}{
		{name: "invalid token", body: `{"tracking_token":"invalid"}`, wantStatus: http.StatusNotFound},
		{name: "expired token", body: `{"tracking_token":"` + validToken + `"}`, repoError: training.ErrBookingStatusExpired, wantStatus: http.StatusGone, wantCalls: 1},
		{name: "cannot select another request", body: `{"tracking_token":"` + validToken + `","request_reference":"MMSU-OTHER"}`, wantStatus: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &bookingStub{statusErr: tc.repoError}
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/booking-status", strings.NewReader(tc.body))
			PublicBookingStatus(repo, nil)(w, r)
			if w.Code != tc.wantStatus || repo.statusCalls != tc.wantCalls {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, repo.statusCalls, w.Body)
			}
		})
	}
}

func TestAdminActionRequiresExplicitPlayerReconciliation(t *testing.T) {
	for _, body := range []string{`{}`, `{"create_new":false}`, `{"player_id":"11111111-1111-4111-8111-111111111111","create_new":true}`} {
		repo := &bookingStub{}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/admin/booking-requests/22222222-2222-4222-8222-222222222222/approve", strings.NewReader(body))
		r.SetPathValue("id", "22222222-2222-4222-8222-222222222222")
		AdminApproveBookingRequest(repo, nil)(w, r)
		if w.Code != http.StatusBadRequest || repo.approveCalls != 0 {
			t.Errorf("body=%s status=%d calls=%d", body, w.Code, repo.approveCalls)
		}
	}
}

func TestApprovalConflictWhenDatabaseReportsFullAndRejectDoesNotBook(t *testing.T) {
	fullRepo := &bookingStub{err: &pgconn.PgError{Code: "PZ002"}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/admin/booking-requests/22222222-2222-4222-8222-222222222222/approve", strings.NewReader(`{"create_new":true}`))
	r.SetPathValue("id", "22222222-2222-4222-8222-222222222222")
	AdminApproveBookingRequest(fullRepo, nil)(w, r)
	if w.Code != http.StatusConflict || fullRepo.approveCalls != 1 {
		t.Fatalf("approve status=%d calls=%d", w.Code, fullRepo.approveCalls)
	}

	rejectRepo := &bookingStub{}
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/admin/booking-requests/22222222-2222-4222-8222-222222222222/reject", strings.NewReader(`{}`))
	r.SetPathValue("id", "22222222-2222-4222-8222-222222222222")
	AdminRejectBookingRequest(rejectRepo, nil)(w, r)
	if w.Code != http.StatusOK || rejectRepo.rejectCalls != 1 || rejectRepo.approveCalls != 0 {
		t.Fatalf("reject status=%d calls=%d", w.Code, rejectRepo.rejectCalls)
	}
}

func TestRepeatedDecisionReturnsConflict(t *testing.T) {
	for _, action := range []struct {
		name    string
		handler func(training.BookingRepository, interface{ Printf(string, ...any) }) http.HandlerFunc
	}{
		{name: "approve", handler: func(repo training.BookingRepository, logger interface{ Printf(string, ...any) }) http.HandlerFunc {
			return AdminApproveBookingRequest(repo, logger)
		}},
		{name: "reject", handler: AdminRejectBookingRequest},
	} {
		t.Run(action.name, func(t *testing.T) {
			repo := &bookingStub{err: &pgconn.PgError{Code: "PZ003"}}
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"create_new":true}`))
			r.SetPathValue("id", "22222222-2222-4222-8222-222222222222")
			action.handler(repo, nil)(w, r)
			if w.Code != http.StatusConflict {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
		})
	}
}

func TestApprovalNotificationRunsAfterCommittedApprovalAndFailureDoesNotUndoIt(t *testing.T) {
	repo := &bookingStub{}
	notifier := &notifierStub{repo: repo, result: notifications.Result{Status: notifications.StatusFailed, Message: "Please contact the requester manually.", Retryable: true}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/admin/booking-requests/22222222-2222-4222-8222-222222222222/approve", strings.NewReader(`{"create_new":true}`))
	r.SetPathValue("id", "22222222-2222-4222-8222-222222222222")
	AdminApproveBookingRequest(repo, nil, notifier)(w, r)
	if w.Code != http.StatusOK || repo.approveCalls != 1 || notifier.calls != 1 || !notifier.approvedAtCall {
		t.Fatalf("status=%d approval calls=%d notification calls=%d after commit=%t body=%s", w.Code, repo.approveCalls, notifier.calls, notifier.approvedAtCall, w.Body)
	}
	for _, expected := range []string{`"status":"approved"`, `"notification_status":"failed"`, `"notification_retryable":true`} {
		if !strings.Contains(w.Body.String(), expected) {
			t.Errorf("response %s missing %q", w.Body, expected)
		}
	}
	if strings.Contains(w.Body.String(), "booking_id") {
		t.Fatal("approval response exposed an unnecessary internal booking ID")
	}
}

func TestRejectedFullStaleOrAlreadyDecidedApprovalDoesNotNotify(t *testing.T) {
	for _, code := range []string{"PZ002", "PZ001", "PZ003", "PZ006"} {
		t.Run(code, func(t *testing.T) {
			repo := &bookingStub{err: &pgconn.PgError{Code: code}}
			notifier := &notifierStub{repo: repo}
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/admin/booking-requests/22222222-2222-4222-8222-222222222222/approve", strings.NewReader(`{"create_new":true}`))
			r.SetPathValue("id", "22222222-2222-4222-8222-222222222222")
			AdminApproveBookingRequest(repo, nil, notifier)(w, r)
			if notifier.calls != 0 || w.Code == http.StatusOK {
				t.Fatalf("status=%d notification calls=%d body=%s", w.Code, notifier.calls, w.Body)
			}
		})
	}
}

func TestRetryNotificationUsesOnlyApprovedRequestReference(t *testing.T) {
	notifier := &notifierStub{result: notifications.Result{Status: notifications.StatusSent, Message: "accepted by provider"}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/admin/booking-requests/22222222-2222-4222-8222-222222222222/notification/retry", nil)
	r.SetPathValue("id", "22222222-2222-4222-8222-222222222222")
	AdminRetryBookingApprovalNotification(notifier, nil)(w, r)
	if w.Code != http.StatusOK || notifier.calls != 1 || notifier.requestID != "22222222-2222-4222-8222-222222222222" {
		t.Fatalf("status=%d calls=%d id=%s body=%s", w.Code, notifier.calls, notifier.requestID, w.Body)
	}
}

func TestBookingRequestDoesNotLeakDatabaseErrors(t *testing.T) {
	repo := &bookingStub{err: errors.New("password=secret")}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/booking-requests", strings.NewReader(`{"session_id":"11111111-1111-4111-8111-111111111111","full_name":"Alex","email":"alex@example.com"}`))
	BookingRequest(repo, log.New(io.Discard, "", 0))(w, r)
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("response=%d %s", w.Code, w.Body)
	}
}

func TestLogBookingErrorIncludesSafeDatabaseCode(t *testing.T) {
	var output strings.Builder
	logger := log.New(&output, "", 0)
	err := fmt.Errorf("wrapped database error: %w", &pgconn.PgError{Code: "42883", Message: "sensitive detail"})
	logBookingError(logger, "create booking request", err)
	if !strings.Contains(output.String(), "SQLSTATE 42883") || strings.Contains(output.String(), "sensitive detail") {
		t.Fatalf("log should include only the safe database code: %q", output.String())
	}
}

func TestAdminApprovedClientsReturnsRecords(t *testing.T) {
	startsAt := time.Date(2026, 10, 20, 1, 0, 0, 0, time.UTC)
	email := "alex@example.com"
	repo := &bookingStub{
		approvedClients: []training.ApprovedClientRecord{
			{
				BookingID:                   "booking-1",
				PlayerID:                    "player-1",
				PlayerName:                  "Alex Player",
				PlayerEmail:                 &email,
				SessionID:                   "session-1",
				ProgramName:                 "Junior Training",
				SelectedTrainingProgramName: "Advanced Tennis Training",
				StartsAt:                    startsAt,
				EndsAt:                      startsAt.Add(time.Hour),
				Location:                    "MMSU Courts",
				BookingCreatedAt:            startsAt.Add(-24 * time.Hour),
			},
		},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/admin/approved-clients", nil)
	AdminApprovedClients(repo, log.New(io.Discard, "", 0))(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}

	var response struct {
		Clients []training.ApprovedClientRecord `json:"clients"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Clients) != 1 {
		t.Fatalf("clients=%d body=%s", len(response.Clients), w.Body)
	}
	if response.Clients[0].PlayerName != "Alex Player" ||
		response.Clients[0].ProgramName != "Junior Training" ||
		response.Clients[0].SelectedTrainingProgramName != "Advanced Tennis Training" ||
		response.Clients[0].BookingID != "booking-1" {
		t.Fatalf("unexpected client record: %#v", response.Clients[0])
	}
}

func TestAdminApprovedClientsReturnsEmptyArray(t *testing.T) {
	repo := &bookingStub{}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/admin/approved-clients", nil)
	AdminApprovedClients(repo, nil)(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), `"clients":[]`) {
		t.Fatalf("expected an empty clients array, body=%s", w.Body)
	}
}

func TestAdminApprovedClientsHandlesUnavailableRepository(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/admin/approved-clients", nil)
	AdminApprovedClients(nil, nil)(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
}

func TestAdminApprovedClientsHandlesRepositoryError(t *testing.T) {
	repo := &bookingStub{err: errors.New("database unavailable")}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/admin/approved-clients", nil)
	AdminApprovedClients(repo, log.New(io.Discard, "", 0))(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "database unavailable") {
		t.Fatalf("internal error leaked to response: %s", w.Body)
	}
}
