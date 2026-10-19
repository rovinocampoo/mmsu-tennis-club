package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
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
	coaches             []training.Coach
	coachDirectory      []training.CoachDirectoryEntry
	coachSchedule       []training.CoachScheduleSession
	createdCoachName    string
	updatedCoachID      string
	updatedCoachActive  bool
	publishedStart      time.Time
	publishedEnd        time.Time
	publishCalls        int
	publicationResult   training.PublicationResult
}

const (
	testCoachID        = "11111111-1111-4111-8111-111111111111"
	testAvailabilityID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	testProgramID      = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	testSessionID      = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	testCoachRecordID  = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
)

func (s *schedulingRepoStub) ListCoaches(context.Context) ([]training.Coach, error) {
	return s.coaches, nil
}

func (s *schedulingRepoStub) CreateCoach(_ context.Context, fullName, _, _, _ string) (string, error) {
	s.createdCoachName = fullName
	return testCoachRecordID, nil
}

func (s *schedulingRepoStub) UpdateCoach(_ context.Context, id, _, _, _, _ string, isActive bool) error {
	s.updatedCoachID = id
	s.updatedCoachActive = isActive
	return nil
}

func (s *schedulingRepoStub) ListActiveCoaches(context.Context) ([]training.CoachDirectoryEntry, error) {
	return s.coachDirectory, nil
}

func (s *schedulingRepoStub) ListCoachSchedule(context.Context) ([]training.CoachScheduleSession, error) {
	return s.coachSchedule, nil
}

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

func (s *schedulingRepoStub) PublishTrainingSessions(_ context.Context, startsAt, endsAt time.Time) (training.PublicationResult, error) {
	s.publishedStart = startsAt
	s.publishedEnd = endsAt
	s.publishCalls++
	return s.publicationResult, nil
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

func TestCoachPortalScheduleAndDirectory(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	coachName := "Coach Maria"
	repo := &schedulingRepoStub{
		coachDirectory: []training.CoachDirectoryEntry{{ID: testCoachRecordID, FullName: coachName, IsActive: true}},
		coachSchedule: []training.CoachScheduleSession{{
			ID:                    testSessionID,
			ProgramName:           "Weekend Junior Clinic",
			StartsAt:              time.Now().UTC(),
			EndsAt:                time.Now().UTC().Add(time.Hour),
			Location:              "MMSU Court 2",
			Status:                "open",
			IsPublic:              true,
			AssignedCoachID:       &coachName,
			AssignedCoachName:     &coachName,
			ParticipantNames:      []string{"Alex", "Bea"},
			ConfirmedBookingCount: 2,
		}},
	}

	rec := httptest.NewRecorder()
	req := withUserContext(httptest.NewRequest(http.MethodGet, "/api/coach/schedule", nil), testCoachID)
	CoachListSchedule(repo, logger).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("coach schedule status=%d body=%s", rec.Code, rec.Body.String())
	}
	var scheduleResp struct {
		Sessions []training.CoachScheduleSession `json:"sessions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &scheduleResp); err != nil {
		t.Fatal(err)
	}
	if len(scheduleResp.Sessions) != 1 || len(scheduleResp.Sessions[0].ParticipantNames) != 2 {
		t.Fatalf("unexpected schedule response: %#v", scheduleResp.Sessions)
	}

	rec = httptest.NewRecorder()
	req = withUserContext(httptest.NewRequest(http.MethodGet, "/api/coach/coaches", nil), testCoachID)
	CoachListCoaches(repo, logger).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("coach directory status=%d body=%s", rec.Code, rec.Body.String())
	}
	var directoryResp struct {
		Coaches []training.CoachDirectoryEntry `json:"coaches"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &directoryResp); err != nil {
		t.Fatal(err)
	}
	if len(directoryResp.Coaches) != 1 || directoryResp.Coaches[0].FullName != coachName {
		t.Fatalf("unexpected directory response: %#v", directoryResp.Coaches)
	}
}

func TestAdminCoachDirectoryCreateAndUpdate(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	repo := &schedulingRepoStub{}

	createHandler := AdminCreateCoach(repo, logger)
	createPayload, _ := json.Marshal(map[string]any{"full_name": "  Coach Rafael  ", "email": "rafael@example.com"})
	rec := httptest.NewRecorder()
	createHandler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/admin/coaches", bytes.NewReader(createPayload)))
	if rec.Code != http.StatusCreated || repo.createdCoachName != "Coach Rafael" {
		t.Fatalf("create coach status=%d body=%s name=%q", rec.Code, rec.Body.String(), repo.createdCoachName)
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.ID != testCoachRecordID {
		t.Fatalf("created coach id=%q", created.ID)
	}

	// Missing is_active is rejected.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/coaches/"+testCoachRecordID, bytes.NewBufferString(`{"full_name":"Coach Rafael"}`))
	req.SetPathValue("id", testCoachRecordID)
	AdminUpdateCoach(repo, logger).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("update coach without is_active status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Valid update deactivates the record.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/admin/coaches/"+testCoachRecordID, bytes.NewBufferString(`{"full_name":"Coach Rafael","is_active":false}`))
	req.SetPathValue("id", testCoachRecordID)
	AdminUpdateCoach(repo, logger).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || repo.updatedCoachID != testCoachRecordID || repo.updatedCoachActive {
		t.Fatalf("update coach status=%d body=%s id=%q active=%v", rec.Code, rec.Body.String(), repo.updatedCoachID, repo.updatedCoachActive)
	}

	// Validation rejects an empty name.
	rec = httptest.NewRecorder()
	createHandler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/admin/coaches", bytes.NewBufferString(`{"full_name":"   "}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create coach with blank name status=%d", rec.Code)
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

func TestAdminPublishTrainingSessionsDerivesManilaWindows(t *testing.T) {
	repo := &schedulingRepoStub{publicationResult: training.PublicationResult{Published: 3, Skipped: 1}}
	handler := AdminPublishTrainingSessions(repo, log.New(io.Discard, "", 0))

	monthPayload := bytes.NewReader([]byte(`{"scope":"month","date":"2026-10-19"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/admin/training-sessions/publish", monthPayload))
	if rec.Code != http.StatusOK {
		t.Fatalf("month status=%d body=%s", rec.Code, rec.Body.String())
	}
	manila := time.FixedZone("Asia/Manila", 8*60*60)
	if !repo.publishedStart.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, manila)) ||
		!repo.publishedEnd.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, manila)) {
		t.Fatalf("month window = %s..%s", repo.publishedStart, repo.publishedEnd)
	}
	if !strings.Contains(rec.Body.String(), `"published":3`) {
		t.Fatalf("response body = %s", rec.Body.String())
	}

	dayPayload := bytes.NewReader([]byte(`{"scope":"day","date":"2026-10-19"}`))
	dayRec := httptest.NewRecorder()
	handler.ServeHTTP(dayRec, httptest.NewRequest(http.MethodPost, "/api/admin/training-sessions/publish", dayPayload))
	if dayRec.Code != http.StatusOK {
		t.Fatalf("day status=%d body=%s", dayRec.Code, dayRec.Body.String())
	}
	if !repo.publishedStart.Equal(time.Date(2026, 10, 19, 0, 0, 0, 0, manila)) ||
		!repo.publishedEnd.Equal(time.Date(2026, 10, 20, 0, 0, 0, 0, manila)) {
		t.Fatalf("day window = %s..%s", repo.publishedStart, repo.publishedEnd)
	}
}

func TestAdminPublishTrainingSessionsRejectsInvalidScopeAndDate(t *testing.T) {
	repo := &schedulingRepoStub{}
	handler := AdminPublishTrainingSessions(repo, log.New(io.Discard, "", 0))
	for _, body := range []string{
		`{"scope":"year","date":"2026-10-19"}`,
		`{"scope":"day","date":"not-a-date"}`,
		`{"scope":"day","date":""}`,
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/admin/training-sessions/publish", bytes.NewReader([]byte(body))))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d", body, rec.Code)
		}
	}
	if repo.publishCalls != 0 {
		t.Fatal("invalid publication requests must not reach the repository")
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
