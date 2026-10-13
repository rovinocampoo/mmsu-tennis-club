package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/training"
)

func TestSchedulingWorkflowKeepsAvailabilitySeparateAndPublishesAdminSessions(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to the disposable local database")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect test database (error type %T)", err)
	}
	t.Cleanup(db.Close)

	var isDisposableTarget bool
	if err := db.QueryRow(ctx, `SELECT current_database() = 'mmsu_booking_test'
		AND inet_server_addr() = '127.0.0.1'::inet
		AND inet_server_port() = 55432`).Scan(&isDisposableTarget); err != nil {
		t.Fatalf("verify test database (error type %T)", err)
	}
	if !isDisposableTarget {
		t.Fatal("refusing integration test writes outside 127.0.0.1:55432/mmsu_booking_test")
	}

	var runID string
	if err := db.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&runID); err != nil {
		t.Fatal("generate integration identifier")
	}
	var programID string
	if err := db.QueryRow(ctx, `INSERT INTO public.training_programs(name) VALUES ('Scheduling integration ' || $1) RETURNING id::text`, runID).Scan(&programID); err != nil {
		t.Fatal("create integration program")
	}
	var availabilityID, sessionID string
	t.Cleanup(func() {
		if sessionID != "" {
			if _, err := db.Exec(context.Background(), `DELETE FROM public.training_sessions WHERE id = $1::uuid`, sessionID); err != nil {
				t.Errorf("delete integration session: %v", err)
			}
		}

		if availabilityID != "" {
			if _, err := db.Exec(context.Background(), `DELETE FROM public.coach_availability WHERE id = $1::uuid`, availabilityID); err != nil {
				t.Errorf("delete integration availability: %v", err)
			}
		}
		if _, err := db.Exec(context.Background(), `DELETE FROM public.training_programs WHERE id = $1::uuid`, programID); err != nil {
			t.Errorf("delete integration program: %v", err)
		}
	})

	repository := NewBookingRepository(db)
	coachID := runID
	startsAt := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
	endsAt := startsAt.Add(90 * time.Minute)
	availabilityID, err = repository.SubmitCoachAvailability(ctx, coachID, "Integration Coach", startsAt, endsAt, "Planning reference")
	if err != nil {
		t.Fatalf("submit availability: %v", err)
	}
	availability, err := repository.ListCoachAvailability(ctx, coachID)
	if err != nil || len(availability) != 1 || availability[0].ID != availabilityID {
		t.Fatalf("availability=%#v err=%v", availability, err)
	}
	var sessionCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM public.training_sessions WHERE program_id = $1::uuid`, programID).Scan(&sessionCount); err != nil {
		t.Fatal("count sessions after availability submission")
	}
	if sessionCount != 0 {
		t.Fatalf("availability submission created %d training sessions", sessionCount)
	}

	capacity := 3
	sessionID, err = repository.CreateTrainingSession(ctx, programID, startsAt, endsAt, "Integration Court", &capacity, "Created by admin", false)
	if err != nil {
		t.Fatalf("create unpublished admin session: %v", err)
	}
	publicSessions, err := NewTrainingRepository(db).ListUpcoming(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("list public sessions before publication: %v", err)
	}
	if containsTrainingSession(publicSessions, sessionID) {
		t.Fatal("unpublished admin session appeared on the public schedule")
	}

	publish := true
	if err := repository.UpdateTrainingSession(ctx, sessionID, nil, nil, &publish); err != nil {
		t.Fatalf("publish admin session: %v", err)
	}
	publicSessions, err = NewTrainingRepository(db).ListUpcoming(ctx, time.Now().UTC())
	if err != nil || !containsTrainingSession(publicSessions, sessionID) {
		t.Fatalf("published session missing from public schedule: sessions=%#v err=%v", publicSessions, err)
	}

	adminSessions, err := repository.ListAdminTrainingSessions(ctx)
	if err != nil {
		t.Fatalf("admin list training sessions: %v", err)
	}
	session, found := findAdminSession(adminSessions, sessionID)
	if !found || session.ConfirmedBookingCount != 0 || !session.IsPublic || session.Status != "open" || session.AssignedCoachID != nil {
		t.Fatalf("admin session=%#v found=%t", session, found)
	}

	if err := repository.AssignTrainingSessionCoach(ctx, sessionID, &coachID); err != nil {
		t.Fatalf("assign coach to training session: %v", err)
	}
	assignedSessions, err := repository.ListAssignedTrainingSessions(ctx, coachID)
	if err != nil || len(assignedSessions) != 1 || assignedSessions[0].ID != sessionID {
		t.Fatalf("assigned sessions=%#v err=%v", assignedSessions, err)
	}
	unrelatedCoachSessions, err := repository.ListAssignedTrainingSessions(ctx, programID)
	if err != nil || len(unrelatedCoachSessions) != 0 {
		t.Fatalf("unrelated coach sessions=%#v err=%v", unrelatedCoachSessions, err)
	}
	adminSessions, err = repository.ListAdminTrainingSessions(ctx)
	if err != nil {
		t.Fatalf("list session after coach assignment: %v", err)
	}
	session, found = findAdminSession(adminSessions, sessionID)
	if !found || session.AssignedCoachID == nil || *session.AssignedCoachID != coachID || session.AssignedCoachName == nil || *session.AssignedCoachName != "Integration Coach" {
		t.Fatalf("admin assignment=%#v found=%t", session, found)
	}
	if err := repository.AssignTrainingSessionCoach(ctx, sessionID, nil); err != nil {
		t.Fatalf("remove coach assignment: %v", err)
	}
	assignedSessions, err = repository.ListAssignedTrainingSessions(ctx, coachID)
	if err != nil || len(assignedSessions) != 0 {
		t.Fatalf("sessions after unassignment=%#v err=%v", assignedSessions, err)
	}

	cancelled := "cancelled"
	exceptionNotes := "Cancelled for integration test"
	if err := repository.UpdateTrainingSession(ctx, sessionID, &cancelled, &exceptionNotes, nil); err != nil {
		t.Fatalf("apply schedule exception: %v", err)
	}
	adminSessions, err = repository.ListAdminTrainingSessions(ctx)
	if err != nil {
		t.Fatalf("list session after exception: %v", err)
	}
	session, found = findAdminSession(adminSessions, sessionID)
	if !found || session.Status != cancelled || session.Notes == nil || *session.Notes != exceptionNotes {
		t.Fatalf("updated admin session=%#v found=%t", session, found)
	}
	publicSessions, err = NewTrainingRepository(db).ListUpcoming(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("list public sessions after cancellation: %v", err)
	}
	if containsTrainingSession(publicSessions, sessionID) {
		t.Fatal("cancelled session remained on the public schedule")
	}
}

func containsTrainingSession(sessions []training.Session, id string) bool {
	for _, session := range sessions {
		if session.ID == id {
			return true
		}
	}
	return false
}

func findAdminSession(sessions []training.AdminSessionRecord, id string) (training.AdminSessionRecord, bool) {
	for _, session := range sessions {
		if session.ID == id {
			return session, true
		}
	}
	return training.AdminSessionRecord{}, false
}
