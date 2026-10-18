package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/adminauth"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/training"
)

type schedulingRepoStub struct {
	bookingStub
	coachAvailabilities []training.CoachAvailability
	programs            []training.TrainingProgram
	adminSessions       []training.AdminSessionRecord
	assignedSessions    []training.AssignedTrainingSession
	assignedCoachID     *string
	assignmentSessionID string
	createdSessionID    string
	createdBatch        []training.TrainingSessionCreateInput
	editedSessionID     string
	editedSession       training.TrainingSessionEditInput
}

const (
	testCoachID        = "11111111-1111-4111-8111-111111111111"
	testAvailabilityID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	testProgramID      = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	testSessionID      = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
)

func (s *schedulingRepoStub) SubmitCoachAvailability(_ context.Context, coachID, coachName string, startsAt, endsAt time.Time, notes string) (string, error) {
	s.coachAvailabilities = append(s.coachAvailabilities, training.CoachAvailability{
		ID:        testAvailabilityID,
		CoachID:   coachID,
		CoachName: coachName,
		StartsAt:  startsAt,
		EndsAt:    endsAt,
		Notes:     &notes,
	})
	return testAvailabilityID, nil
}

func (s *schedulingRepoStub) ListCoachAvailability(_ context.Context, coachID string) ([]training.CoachAvailability, error) {
	res := make([]training.CoachAvailability, 0)
	for _, a := range s.coachAvailabilities {
		if a.CoachID == coachID {
			res = append(res, a)
		}
	}
	return res, nil
}

func (s *schedulingRepoStub) DeleteCoachAvailability(_ context.Context, id, coachID string) error {
	for i, a := range s.coachAvailabilities {
		if a.ID == id && a.CoachID == coachID {
			s.coachAvailabilities = append(s.coachAvailabilities[:i], s.coachAvailabilities[i+1:]...)
			return nil
		}
	}
	return nil
}

func (s *schedulingRepoStub) ListAllCoachAvailability(context.Context) ([]training.CoachAvailability, error) {
	return s.coachAvailabilities, nil
}

func (s *schedulingRepoStub) ListAdminTrainingPrograms(context.Context) ([]training.TrainingProgram, error) {
	return s.programs, nil
}

func (s *schedulingRepoStub) ListAdminTrainingSessions(context.Context) ([]training.AdminSessionRecord, error) {
	return s.adminSessions, nil
}

func (s *schedulingRepoStub) ListAssignedTrainingSessions(_ context.Context, coachID string) ([]training.AssignedTrainingSession, error) {
	if coachID != testCoachID {
		return nil, nil
	}
	return s.assignedSessions, nil
}

func (s *schedulingRepoStub) AssignTrainingSessionCoach(_ context.Context, sessionID string, coachID *string) error {
	s.assignmentSessionID = sessionID
	s.assignedCoachID = coachID
	return nil
}

func (s *schedulingRepoStub) CreateTrainingSession(_ context.Context, programID string, startsAt, endsAt time.Time, location string, capacity *int, notes string, isPublic bool) (string, error) {
	s.createdSessionID = testSessionID
	s.adminSessions = append(s.adminSessions, training.AdminSessionRecord{
		ID:        s.createdSessionID,
		ProgramID: programID,
		StartsAt:  startsAt,
		EndsAt:    endsAt,
		Location:  location,
		Capacity:  capacity,
		Status:    "open",
		Notes:     &notes,
		IsPublic:  isPublic,
	})
	return s.createdSessionID, nil
}

func (s *schedulingRepoStub) CreateTrainingSessions(_ context.Context, sessions []training.TrainingSessionCreateInput) ([]string, error) {
	s.createdBatch = append([]training.TrainingSessionCreateInput(nil), sessions...)
	ids := make([]string, len(sessions))
	for i := range sessions {
		ids[i] = testSessionID
	}
	return ids, nil
}

func (s *schedulingRepoStub) EditTrainingSession(_ context.Context, sessionID string, input training.TrainingSessionEditInput) error {
	s.editedSessionID = sessionID
	s.editedSession = input
	return nil
}

func (s *schedulingRepoStub) UpdateTrainingSession(_ context.Context, sessionID string, status *string, notes *string, isPublic *bool) error {
	for i, ssn := range s.adminSessions {
		if ssn.ID == sessionID {
			if status != nil {
				s.adminSessions[i].Status = *status
			}
			if notes != nil {
				s.adminSessions[i].Notes = notes
			}
			if isPublic != nil {
				s.adminSessions[i].IsPublic = *isPublic
			}
			return nil
		}
	}
	return nil
}

func withUserContext(r *http.Request, userID string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), adminauth.UserIDKeyForTest(), userID))
}

func TestCoachAvailabilityWorkflow(t *testing.T) {
	repo := &schedulingRepoStub{}
	logger := log.New(io.Discard, "", 0)
	coachID := testCoachID

	// 1. Submit availability
	submitHandler := CoachSubmitAvailability(repo, logger)
	starts := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Minute)
	ends := starts.Add(2 * time.Hour)
	payload, _ := json.Marshal(map[string]any{
		"coach_name": "Coach Maria",
		"starts_at":  starts,
		"ends_at":    ends,
		"notes":      "Available for morning clinic",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/coach/availability", bytes.NewReader(payload))
	req = withUserContext(req, coachID)
	rec := httptest.NewRecorder()
	submitHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit availability: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// 2. List availability
	listHandler := CoachListAvailability(repo, logger)
	req = withUserContext(httptest.NewRequest(http.MethodGet, "/api/coach/availability", nil), coachID)
	rec = httptest.NewRecorder()
	listHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list availability: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var listResp struct {
		Availability []training.CoachAvailability `json:"availability"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listResp)
	if len(listResp.Availability) != 1 || listResp.Availability[0].CoachName != "Coach Maria" {
		t.Fatalf("unexpected list response: %#v", listResp)
	}

	// 3. Delete availability
	delHandler := CoachDeleteAvailability(repo, logger)
	req = withUserContext(httptest.NewRequest(http.MethodDelete, "/api/coach/availability/"+testAvailabilityID, nil), coachID)
	req.SetPathValue("id", testAvailabilityID)
	rec = httptest.NewRecorder()
	delHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete availability: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Verify empty list
	rec = httptest.NewRecorder()
	listHandler.ServeHTTP(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &listResp)
	if len(listResp.Availability) != 0 {
		t.Fatalf("expected 0 availability items, got %d", len(listResp.Availability))
	}
}

func TestAdminSchedulingWorkflow(t *testing.T) {
	repo := &schedulingRepoStub{
		programs: []training.TrainingProgram{
			{ID: testProgramID, Name: "Weekend Junior Clinic", IsActive: true},
		},
		coachAvailabilities: []training.CoachAvailability{
			{ID: testAvailabilityID, CoachID: testCoachID, CoachName: "Coach Rafael", StartsAt: time.Now(), EndsAt: time.Now().Add(time.Hour)},
		},
	}
	logger := log.New(io.Discard, "", 0)

	// 1. Admin lists coach availability
	listCoachHandler := AdminListCoachAvailability(repo, logger)
	rec := httptest.NewRecorder()
	listCoachHandler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/coach-availability", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list coach availability status=%d", rec.Code)
	}

	// 2. Admin lists programs
	programsHandler := AdminListTrainingPrograms(repo, logger)
	rec = httptest.NewRecorder()
	programsHandler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/training-programs", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list programs status=%d", rec.Code)
	}

	// 3. Admin creates training session
	createHandler := AdminCreateTrainingSession(repo, logger)
	starts := time.Now().Add(48 * time.Hour)
	ends := starts.Add(90 * time.Minute)
	cap := 4
	createPayload, _ := json.Marshal(map[string]any{
		"program_id": testProgramID,
		"starts_at":  starts,
		"ends_at":    ends,
		"location":   "MMSU Court 2",
		"capacity":   &cap,
		"notes":      "Beginner drills",
		"is_public":  true,
	})
	rec = httptest.NewRecorder()
	createHandler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/admin/training-sessions", bytes.NewReader(createPayload)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create session status=%d body=%s", rec.Code, rec.Body.String())
	}

	// 4. Admin lists sessions
	listSessionsHandler := AdminListTrainingSessions(repo, logger)
	rec = httptest.NewRecorder()
	listSessionsHandler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/training-sessions", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list sessions status=%d", rec.Code)
	}
	var sessResp struct {
		Sessions []training.AdminSessionRecord `json:"sessions"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &sessResp)
	if len(sessResp.Sessions) != 1 || sessResp.Sessions[0].Location != "MMSU Court 2" {
		t.Fatalf("unexpected sessions: %#v", sessResp)
	}

	// 5. Admin updates training session (schedule exception: cancel)
	updateHandler := AdminUpdateTrainingSession(repo, logger)
	newStatus := "cancelled"
	newNotes := "Cancelled due to university tournament"
	updatePayload, _ := json.Marshal(map[string]any{
		"status": &newStatus,
		"notes":  &newNotes,
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/training-sessions/"+testSessionID, bytes.NewReader(updatePayload))
	req.SetPathValue("id", testSessionID)
	rec = httptest.NewRecorder()
	updateHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update session status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Verify exception updated
	rec = httptest.NewRecorder()
	listSessionsHandler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/training-sessions", nil))
	_ = json.Unmarshal(rec.Body.Bytes(), &sessResp)
	if sessResp.Sessions[0].Status != "cancelled" || *sessResp.Sessions[0].Notes != newNotes {
		t.Fatalf("session status not updated: %#v", sessResp.Sessions[0])
	}
}

func TestCoachListAssignedSessionsUsesAuthenticatedCoachID(t *testing.T) {
	repo := &schedulingRepoStub{
		assignedSessions: []training.AssignedTrainingSession{{
			ID:          testSessionID,
			ProgramName: "Weekend Junior Clinic",
			StartsAt:    time.Now().UTC(),
			EndsAt:      time.Now().UTC().Add(time.Hour),
			Location:    "MMSU Court 2",
			Status:      "open",
			IsPublic:    false,
		}},
	}
	req := withUserContext(httptest.NewRequest(http.MethodGet, "/api/coach/sessions", nil), testCoachID)
	rec := httptest.NewRecorder()
	CoachListAssignedSessions(repo, log.New(io.Discard, "", 0)).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list assigned sessions status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Sessions []training.AssignedTrainingSession `json:"sessions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Sessions) != 1 || body.Sessions[0].ID != testSessionID || body.Sessions[0].IsPublic {
		t.Fatalf("unexpected assigned sessions response: %#v", body.Sessions)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/coach/sessions", nil)
	rec = httptest.NewRecorder()
	CoachListAssignedSessions(repo, log.New(io.Discard, "", 0)).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing coach identity status=%d", rec.Code)
	}
}

func TestAdminAssignTrainingSessionCoachValidatesAndSupportsUnassignment(t *testing.T) {
	repo := &schedulingRepoStub{}
	handler := AdminAssignTrainingSessionCoach(repo, log.New(io.Discard, "", 0))

	req := httptest.NewRequest(http.MethodPatch, "/api/admin/training-sessions/"+testSessionID+"/coach", bytes.NewBufferString(`{"coach_id":"`+testCoachID+`"}`))
	req.SetPathValue("id", testSessionID)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || repo.assignmentSessionID != testSessionID || repo.assignedCoachID == nil || *repo.assignedCoachID != testCoachID {
		t.Fatalf("assign coach status=%d body=%s session=%q coach=%v", rec.Code, rec.Body.String(), repo.assignmentSessionID, repo.assignedCoachID)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/admin/training-sessions/"+testSessionID+"/coach", bytes.NewBufferString(`{"coach_id":null}`))
	req.SetPathValue("id", testSessionID)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || repo.assignedCoachID != nil {
		t.Fatalf("unassign coach status=%d body=%s coach=%v", rec.Code, rec.Body.String(), repo.assignedCoachID)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/admin/training-sessions/"+testSessionID+"/coach", bytes.NewBufferString(`{}`))
	req.SetPathValue("id", testSessionID)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing coach_id status=%d", rec.Code)
	}
}

func TestAdminCreateTrainingSessionsBatchValidatesAndSubmitsEverySlotTogether(t *testing.T) {
	repo := &schedulingRepoStub{}
	start := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	payload, _ := json.Marshal(map[string]any{"sessions": []map[string]any{
		{"program_id": testProgramID, "starts_at": start, "ends_at": start.Add(time.Hour), "location": "MMSU Court 1", "capacity": 4, "notes": "morning", "is_public": false, "coach_id": nil},
		{"program_id": testProgramID, "starts_at": start.Add(time.Hour), "ends_at": start.Add(2 * time.Hour), "location": "MMSU Court 1", "capacity": 4, "notes": "morning", "is_public": false, "coach_id": nil},
	}})
	rec := httptest.NewRecorder()
	AdminCreateTrainingSessionsBatch(repo, log.New(io.Discard, "", 0)).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/admin/training-sessions/batch", bytes.NewReader(payload)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(repo.createdBatch) != 2 || repo.createdBatch[0].IsPublic || repo.createdBatch[1].Location != "MMSU Court 1" {
		t.Fatalf("batch sessions = %#v", repo.createdBatch)
	}
}

func TestAdminCreateTrainingSessionsBatchRejectsInvalidSessionWithoutCallingRepository(t *testing.T) {
	repo := &schedulingRepoStub{}
	payload, _ := json.Marshal(map[string]any{"sessions": []map[string]any{{
		"program_id": testProgramID,
		"starts_at":  time.Now().Add(time.Hour),
		"ends_at":    time.Now(),
		"location":   "MMSU Court 1",
	}}})
	rec := httptest.NewRecorder()
	AdminCreateTrainingSessionsBatch(repo, log.New(io.Discard, "", 0)).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/admin/training-sessions/batch", bytes.NewReader(payload)))
	if rec.Code != http.StatusBadRequest || len(repo.createdBatch) != 0 {
		t.Fatalf("status=%d sessions=%d body=%s", rec.Code, len(repo.createdBatch), rec.Body.String())
	}
}

func TestAdminEditTrainingSessionRequiresFullScheduleAndPassesAcknowledgement(t *testing.T) {
	repo := &schedulingRepoStub{}
	start := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	capacity := 5
	payload, _ := json.Marshal(map[string]any{
		"program_id":                 testProgramID,
		"starts_at":                  start,
		"ends_at":                    start.Add(2 * time.Hour),
		"location":                   "MMSU Court 2",
		"capacity":                   capacity,
		"notes":                      "edited",
		"is_public":                  true,
		"acknowledge_booked_changes": true,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/admin/training-sessions/"+testSessionID, bytes.NewReader(payload))
	req.SetPathValue("id", testSessionID)
	rec := httptest.NewRecorder()
	AdminEditTrainingSession(repo, log.New(io.Discard, "", 0)).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if repo.editedSessionID != testSessionID || !repo.editedSession.AcknowledgeBookedChanges || !repo.editedSession.IsPublic || *repo.editedSession.Capacity != capacity {
		t.Fatalf("edited session = %#v id=%q", repo.editedSession, repo.editedSessionID)
	}
}
