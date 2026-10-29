package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/notifications"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/training"
)

type bookingDB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type BookingRepository struct{ db bookingDB }

func NewBookingRepository(db bookingDB) *BookingRepository { return &BookingRepository{db: db} }

func (r *BookingRepository) CreateRequest(ctx context.Context, request training.BookingRequest, tokenHash []byte) (string, error) {
	sessionIDs := request.SessionIDs
	if len(sessionIDs) == 0 && request.SessionID != "" {
		sessionIDs = []string{request.SessionID}
	}
	encodedIDs, err := json.Marshal(sessionIDs)
	if err != nil {
		return "", fmt.Errorf("encode requested training sessions: %w", err)
	}
	participants := request.Participants
	if len(participants) == 0 {
		if trimmed := strings.TrimSpace(request.FullName); trimmed != "" {
			participants = []string{trimmed}
		}
	}
	encodedParticipants, err := json.Marshal(participants)
	if err != nil {
		return "", fmt.Errorf("encode booking participants: %w", err)
	}
	var reference string
	err = r.db.QueryRow(ctx, `SELECT public.create_public_booking_request_with_participants_token($1::jsonb, $2::jsonb, $3, $4, $5, $6, $7)`, string(encodedIDs), string(encodedParticipants), nullable(request.Email), nullable(request.Phone), tokenHash, nullable(request.TrainingFocus), nullable(request.TrainingFocusNotes)).Scan(&reference)
	if err != nil {
		return "", fmt.Errorf("create public booking request: %w", err)
	}
	return reference, nil
}

func (r *BookingRepository) GetPublicStatus(ctx context.Context, tokenHash []byte) (training.PublicBookingStatus, error) {
	var status training.PublicBookingStatus
	var expired bool
	var requestedSessions []byte
	err := r.db.QueryRow(ctx, `
		SELECT status.request_reference, status.request_status, status.program_name,
		       status.starts_at, status.ends_at, status.location, status.is_expired,
		       COALESCE(status.training_focus, ''), COALESCE(status.training_focus_notes, ''),
		       COALESCE(public.get_training_program_name(status.training_focus), ''), status.requested_sessions
		FROM public.get_public_booking_status($1) AS status
		`, tokenHash).Scan(
		&status.RequestReference, &status.Status, &status.ProgramName,
		&status.StartsAt, &status.EndsAt, &status.Location, &expired,
		&status.TrainingFocus, &status.TrainingFocusNotes, &status.TrainingProgramName, &requestedSessions,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return training.PublicBookingStatus{}, training.ErrBookingStatusNotFound
	}
	if err != nil {
		return training.PublicBookingStatus{}, fmt.Errorf("get public booking status: %w", err)
	}
	if err := json.Unmarshal(requestedSessions, &status.RequestedSessions); err != nil {
		return training.PublicBookingStatus{}, fmt.Errorf("decode public booking sessions: %w", err)
	}
	if expired {
		return training.PublicBookingStatus{}, training.ErrBookingStatusExpired
	}
	return status, nil
}

func (r *BookingRepository) ListPendingRequests(ctx context.Context) ([]training.BookingRequestRecord, error) {
	rows, err := r.db.Query(ctx, `
		SELECT requests.id, requests.session_id, requests.full_name, requests.email, requests.phone,
		       requests.status, requests.created_at, requests.starts_at, requests.ends_at,
		       requests.location, requests.session_status,
		       COALESCE(requests.training_focus, ''), COALESCE(requests.training_focus_notes, ''),
		       COALESCE(selected_program.name, ''), requests.requested_slots,
		       requests.resolved_player_id
		FROM public.admin_list_pending_booking_requests_with_slots() AS requests
		LEFT JOIN LATERAL (
		    SELECT program.name
		    FROM public.admin_list_training_program_catalog() AS program
		    WHERE program.id::text = requests.training_focus
		    LIMIT 1
		) AS selected_program ON true
		ORDER BY requests.created_at ASC, requests.id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list pending booking requests: %w", err)
	}
	defer rows.Close()
	result := make([]training.BookingRequestRecord, 0)
	for rows.Next() {
		var row training.BookingRequestRecord
		var requestedSlots []byte
		if err := rows.Scan(&row.ID, &row.SessionID, &row.FullName, &row.Email, &row.Phone, &row.Status, &row.CreatedAt, &row.StartsAt, &row.EndsAt, &row.Location, &row.SessionStatus, &row.TrainingFocus, &row.TrainingFocusNotes, &row.TrainingProgramName, &requestedSlots, &row.ResolvedPlayerID); err != nil {
			return nil, fmt.Errorf("scan pending booking request: %w", err)
		}
		if err := json.Unmarshal(requestedSlots, &row.RequestedSlots); err != nil {
			return nil, fmt.Errorf("decode pending booking request slots: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read pending booking requests: %w", err)
	}
	return result, nil
}

func (r *BookingRepository) SearchPlayers(ctx context.Context, query string) ([]training.PlayerCandidate, error) {
	rows, err := r.db.Query(ctx, `SELECT id, full_name, email, phone FROM public.search_booking_players($1)`, query)
	if err != nil {
		return nil, fmt.Errorf("search booking players: %w", err)
	}
	defer rows.Close()
	result := make([]training.PlayerCandidate, 0)
	for rows.Next() {
		var row training.PlayerCandidate
		if err := rows.Scan(&row.ID, &row.FullName, &row.Email, &row.Phone); err != nil {
			return nil, fmt.Errorf("scan booking player: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read booking players: %w", err)
	}
	return result, nil
}

func (r *BookingRepository) ApproveRequest(ctx context.Context, requestID, adminID string, playerID *string, createNew bool) (string, error) {
	var bookingID string
	err := r.db.QueryRow(ctx, `SELECT public.approve_booking_request($1::uuid, $2::uuid, $3::uuid, $4)`, requestID, adminID, playerID, createNew).Scan(&bookingID)
	if err != nil {
		return "", fmt.Errorf("approve booking request: %w", err)
	}
	return bookingID, nil
}

func (r *BookingRepository) ApproveSelectedSessions(ctx context.Context, requestID, adminID string, playerID *string, createNew bool, sessionIDs []string) (training.ApprovalDecision, error) {
	encodedIDs, err := json.Marshal(sessionIDs)
	if err != nil {
		return training.ApprovalDecision{}, fmt.Errorf("encode selected booking sessions: %w", err)
	}
	var raw []byte
	err = r.db.QueryRow(ctx, `SELECT public.approve_booking_request_sessions($1::uuid, $2::uuid, $3::uuid, $4, $5::jsonb)`, requestID, adminID, playerID, createNew, string(encodedIDs)).Scan(&raw)
	if err != nil {
		return training.ApprovalDecision{}, fmt.Errorf("approve selected booking sessions: %w", err)
	}
	var result training.ApprovalDecision
	if err := json.Unmarshal(raw, &result); err != nil {
		return training.ApprovalDecision{}, fmt.Errorf("decode selected booking approval: %w", err)
	}
	return result, nil
}

func (r *BookingRepository) GetBookingCalendarEvents(ctx context.Context, requestID string) ([]training.CalendarEvent, error) {
	rows, err := r.db.Query(ctx, `SELECT event_id, request_id::text, request_reference, participant_name, program_name, starts_at, ends_at, location FROM public.list_booking_calendar_events($1::uuid)`, requestID)
	if err != nil {
		return nil, fmt.Errorf("list booking calendar events: %w", err)
	}
	defer rows.Close()
	var events []training.CalendarEvent
	for rows.Next() {
		var e training.CalendarEvent
		if err := rows.Scan(&e.EventID, &e.RequestID, &e.RequestReference, &e.ParticipantName, &e.ProgramName, &e.StartsAt, &e.EndsAt, &e.Location); err != nil {
			return nil, fmt.Errorf("read booking calendar event: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read booking calendar events: %w", err)
	}
	return events, nil
}

func (r *BookingRepository) RejectRequest(ctx context.Context, requestID, adminID string) error {
	var rejected bool
	err := r.db.QueryRow(ctx, `SELECT public.reject_booking_request($1::uuid, $2::uuid)`, requestID, adminID).Scan(&rejected)
	if err != nil {
		return fmt.Errorf("reject booking request: %w", err)
	}
	return nil
}

func (r *BookingRepository) RejectPendingSessions(ctx context.Context, requestID, adminID string) (string, error) {
	var status string
	err := r.db.QueryRow(ctx, `SELECT public.admin_reject_pending_booking_sessions($1::uuid, $2::uuid)`, requestID, adminID).Scan(&status)
	if err != nil {
		return "", fmt.Errorf("reject pending booking sessions: %w", err)
	}
	return status, nil
}

func (r *BookingRepository) ClaimApprovalNotification(ctx context.Context, requestID string, resendConfigured bool) (notifications.Claim, error) {
	var claim notifications.Claim
	var status string
	err := r.db.QueryRow(ctx, `
		SELECT notification_status,
		       COALESCE(notification_detail, ''),
		       COALESCE(recipient_email, ''),
		       program_name, starts_at, ends_at, location, support_reference,
		       attempt_count, claimed
		FROM public.claim_booking_approval_notification($1::uuid, $2)`, requestID, resendConfigured).Scan(
		&status, &claim.Detail, &claim.RecipientEmail, &claim.ProgramName,
		&claim.StartsAt, &claim.EndsAt, &claim.Location, &claim.SupportReference,
		&claim.AttemptCount, &claim.Claimed,
	)
	if err != nil {
		return notifications.Claim{}, fmt.Errorf("claim approval notification: %w", err)
	}
	claim.Status = notifications.Status(status)
	return claim, nil
}

func (r *BookingRepository) CompleteApprovalNotification(ctx context.Context, requestID string, attemptCount int, succeeded bool, resendEmailID string) (bool, error) {
	var updated bool
	err := r.db.QueryRow(ctx, `SELECT public.complete_booking_approval_notification($1::uuid, $2, $3, $4)`, requestID, attemptCount, succeeded, nullable(resendEmailID)).Scan(&updated)
	if err != nil {
		return false, fmt.Errorf("complete approval notification: %w", err)
	}
	return updated, nil
}

func nullable(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (r *BookingRepository) SubmitCoachAvailability(ctx context.Context, coachID, coachName string, startsAt, endsAt time.Time, notes string) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `SELECT public.submit_coach_availability($1::uuid, $2, $3, $4, $5)`,
		coachID, coachName, startsAt, endsAt, nullable(notes)).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("submit coach availability: %w", err)
	}
	return id, nil
}

func (r *BookingRepository) ListCoachAvailability(ctx context.Context, coachID string) ([]training.CoachAvailability, error) {
	rows, err := r.db.Query(ctx, `SELECT id, coach_id, coach_name, starts_at, ends_at, notes, created_at FROM public.list_coach_availability($1::uuid)`, coachID)
	if err != nil {
		return nil, fmt.Errorf("list coach availability: %w", err)
	}
	defer rows.Close()
	result := make([]training.CoachAvailability, 0)
	for rows.Next() {
		var item training.CoachAvailability
		if err := rows.Scan(&item.ID, &item.CoachID, &item.CoachName, &item.StartsAt, &item.EndsAt, &item.Notes, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan coach availability: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read coach availability: %w", err)
	}
	return result, nil
}

func (r *BookingRepository) DeleteCoachAvailability(ctx context.Context, id, coachID string) error {
	var deleted bool
	err := r.db.QueryRow(ctx, `SELECT public.delete_coach_availability($1::uuid, $2::uuid)`, id, coachID).Scan(&deleted)
	if err != nil {
		return fmt.Errorf("delete coach availability: %w", err)
	}
	if !deleted {
		return errors.New("availability not found")
	}
	return nil
}

func (r *BookingRepository) ListAllCoachAvailability(ctx context.Context) ([]training.CoachAvailability, error) {
	rows, err := r.db.Query(ctx, `SELECT id, coach_id, coach_name, starts_at, ends_at, notes, created_at FROM public.admin_list_coach_availability()`)
	if err != nil {
		return nil, fmt.Errorf("list all coach availability: %w", err)
	}
	defer rows.Close()
	result := make([]training.CoachAvailability, 0)
	for rows.Next() {
		var item training.CoachAvailability
		if err := rows.Scan(&item.ID, &item.CoachID, &item.CoachName, &item.StartsAt, &item.EndsAt, &item.Notes, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan coach availability: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read coach availability: %w", err)
	}
	return result, nil
}

func (r *BookingRepository) ListAdminTrainingPrograms(ctx context.Context) ([]training.TrainingProgram, error) {
	rows, err := r.db.Query(ctx, `SELECT id, name, description, is_active FROM public.admin_list_training_programs()`)
	if err != nil {
		return nil, fmt.Errorf("list admin training programs: %w", err)
	}
	defer rows.Close()
	result := make([]training.TrainingProgram, 0)
	for rows.Next() {
		var p training.TrainingProgram
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.IsActive); err != nil {
			return nil, fmt.Errorf("scan admin training program: %w", err)
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read admin training programs: %w", err)
	}
	return result, nil
}

func (r *BookingRepository) ListAdminTrainingProgramCatalog(ctx context.Context) ([]training.TrainingProgram, error) {
	rows, err := r.db.Query(ctx, `SELECT id, name, description, is_active FROM public.admin_list_training_program_catalog()`)
	if err != nil {
		return nil, fmt.Errorf("list admin training program catalog: %w", err)
	}
	defer rows.Close()
	result := make([]training.TrainingProgram, 0)
	for rows.Next() {
		var program training.TrainingProgram
		if err := rows.Scan(&program.ID, &program.Name, &program.Description, &program.IsActive); err != nil {
			return nil, fmt.Errorf("scan admin training program catalog: %w", err)
		}
		result = append(result, program)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read admin training program catalog: %w", err)
	}
	return result, nil
}

func (r *BookingRepository) CreateAdminTrainingProgram(ctx context.Context, name, description string) (string, error) {
	var id string
	if err := r.db.QueryRow(ctx, `SELECT public.admin_create_training_program($1, $2)::text`, name, description).Scan(&id); err != nil {
		return "", fmt.Errorf("create admin training program: %w", err)
	}
	return id, nil
}

func (r *BookingRepository) UpdateAdminTrainingProgram(ctx context.Context, id, name, description string, isActive bool) (bool, error) {
	var updated bool
	if err := r.db.QueryRow(ctx, `SELECT public.admin_update_training_program($1::uuid, $2, $3, $4)`, id, name, description, isActive).Scan(&updated); err != nil {
		return false, fmt.Errorf("update admin training program: %w", err)
	}
	return updated, nil
}

func (r *BookingRepository) ListAdminTrainingSessions(ctx context.Context) ([]training.AdminSessionRecord, error) {
	rows, err := r.db.Query(ctx, `SELECT id, program_id, program_name, starts_at, ends_at, location, capacity, confirmed_booking_count, status, notes, is_public, assigned_coach_id, assigned_coach_name FROM public.admin_list_training_sessions()`)
	if err != nil {
		return nil, fmt.Errorf("list admin training sessions: %w", err)
	}
	defer rows.Close()
	result := make([]training.AdminSessionRecord, 0)
	for rows.Next() {
		var s training.AdminSessionRecord
		if err := rows.Scan(&s.ID, &s.ProgramID, &s.ProgramName, &s.StartsAt, &s.EndsAt, &s.Location, &s.Capacity, &s.ConfirmedBookingCount, &s.Status, &s.Notes, &s.IsPublic, &s.AssignedCoachID, &s.AssignedCoachName); err != nil {
			return nil, fmt.Errorf("scan admin training session: %w", err)
		}
		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read admin training sessions: %w", err)
	}
	return result, nil
}

func (r *BookingRepository) ListApprovedClients(ctx context.Context) ([]training.ApprovedClientRecord, error) {
	rows, err := r.db.Query(ctx, `
		SELECT booking_id, player_id, player_name, player_email, player_phone,
		       session_id, program_name, starts_at, ends_at, location, capacity,
		       assigned_coach_id, assigned_coach_name, booking_created_at, participant_names,
		       selected_training_program_name
		FROM public.admin_list_approved_clients()
	`)
	if err != nil {
		return nil, fmt.Errorf("list approved clients: %w", err)
	}
	defer rows.Close()

	result := make([]training.ApprovedClientRecord, 0)
	for rows.Next() {
		var client training.ApprovedClientRecord
		var participantNames []byte
		if err := rows.Scan(
			&client.BookingID,
			&client.PlayerID,
			&client.PlayerName,
			&client.PlayerEmail,
			&client.PlayerPhone,
			&client.SessionID,
			&client.ProgramName,
			&client.StartsAt,
			&client.EndsAt,
			&client.Location,
			&client.Capacity,
			&client.AssignedCoachID,
			&client.AssignedCoachName,
			&client.BookingCreatedAt,
			&participantNames,
			&client.SelectedTrainingProgramName,
		); err != nil {
			return nil, fmt.Errorf("scan approved client: %w", err)
		}
		client.ParticipantNames, err = decodeNameList(participantNames)
		if err != nil {
			return nil, fmt.Errorf("decode approved client participants: %w", err)
		}
		result = append(result, client)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read approved clients: %w", err)
	}

	return result, nil
}

func (r *BookingRepository) CreateFeedback(ctx context.Context, submission training.FeedbackSubmission) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `SELECT public.submit_feedback($1, $2, $3, $4)`,
		submission.Category, submission.Message, nullable(submission.Name), nullable(submission.Email)).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("submit feedback: %w", err)
	}
	return id, nil
}

func (r *BookingRepository) ListFeedbackSubmissions(ctx context.Context) ([]training.FeedbackRecord, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, category, message, name, email, status, created_at, reviewed_at, reviewed_by
		FROM public.admin_list_feedback_submissions()`)
	if err != nil {
		return nil, fmt.Errorf("list feedback submissions: %w", err)
	}
	defer rows.Close()
	result := make([]training.FeedbackRecord, 0)
	for rows.Next() {
		var record training.FeedbackRecord
		if err := rows.Scan(&record.ID, &record.Category, &record.Message, &record.Name, &record.Email, &record.Status, &record.CreatedAt, &record.ReviewedAt, &record.ReviewedBy); err != nil {
			return nil, fmt.Errorf("scan feedback submission: %w", err)
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read feedback submissions: %w", err)
	}
	return result, nil
}

func (r *BookingRepository) UpdateFeedbackStatus(ctx context.Context, id, status, adminID string) (bool, error) {
	var updated bool
	err := r.db.QueryRow(ctx, `SELECT public.admin_update_feedback_status($1::uuid, $2, $3::uuid)`, id, status, adminID).Scan(&updated)
	if err != nil {
		return false, fmt.Errorf("update feedback status: %w", err)
	}
	return updated, nil
}

func (r *BookingRepository) ListAssignedTrainingSessions(ctx context.Context, coachID string) ([]training.AssignedTrainingSession, error) {
	rows, err := r.db.Query(ctx, `SELECT id, program_name, program_description, starts_at, ends_at, location, status, is_public FROM public.coach_list_assigned_training_sessions($1::uuid)`, coachID)
	if err != nil {
		return nil, fmt.Errorf("list assigned training sessions: %w", err)
	}
	defer rows.Close()
	result := make([]training.AssignedTrainingSession, 0)
	for rows.Next() {
		var session training.AssignedTrainingSession
		if err := rows.Scan(&session.ID, &session.ProgramName, &session.ProgramDescription, &session.StartsAt, &session.EndsAt, &session.Location, &session.Status, &session.IsPublic); err != nil {
			return nil, fmt.Errorf("scan assigned training session: %w", err)
		}
		result = append(result, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read assigned training sessions: %w", err)
	}
	return result, nil
}

func (r *BookingRepository) AssignTrainingSessionCoach(ctx context.Context, sessionID string, coachID *string) error {
	var assigned bool
	err := r.db.QueryRow(ctx, `SELECT public.admin_assign_training_session_coach($1::uuid, $2::uuid)`, sessionID, coachID).Scan(&assigned)
	if err != nil {
		return fmt.Errorf("assign coach to training session: %w", err)
	}
	return nil
}

func (r *BookingRepository) CreateTrainingSession(ctx context.Context, programID string, startsAt, endsAt time.Time, location string, capacity *int, notes string, isPublic bool) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `SELECT public.admin_create_training_session($1::uuid, $2, $3, $4, $5, $6, $7)`,
		programID, startsAt, endsAt, location, capacity, nullable(notes), isPublic).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create training session: %w", err)
	}
	return id, nil
}

func (r *BookingRepository) CreateTrainingSessions(ctx context.Context, sessions []training.TrainingSessionCreateInput) ([]string, error) {
	payload, err := json.Marshal(sessions)
	if err != nil {
		return nil, fmt.Errorf("encode training sessions batch: %w", err)
	}
	var encodedIDs []byte
	if err := r.db.QueryRow(ctx, `SELECT public.admin_create_training_sessions_batch($1::jsonb)`, string(payload)).Scan(&encodedIDs); err != nil {
		return nil, fmt.Errorf("create training sessions batch: %w", err)
	}
	var ids []string
	if err := json.Unmarshal(encodedIDs, &ids); err != nil {
		return nil, fmt.Errorf("decode created training session IDs: %w", err)
	}
	return ids, nil
}

func (r *BookingRepository) PublishTrainingSessions(ctx context.Context, startsAt, endsAt time.Time) (training.PublicationResult, error) {
	var encoded []byte
	if err := r.db.QueryRow(ctx, `SELECT public.admin_publish_training_sessions($1, $2)`, startsAt, endsAt).Scan(&encoded); err != nil {
		return training.PublicationResult{}, fmt.Errorf("publish training sessions: %w", err)
	}
	var result training.PublicationResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		return training.PublicationResult{}, fmt.Errorf("decode training session publication result: %w", err)
	}
	return result, nil
}

func (r *BookingRepository) EditTrainingSession(ctx context.Context, sessionID string, input training.TrainingSessionEditInput) error {
	var updated bool
	err := r.db.QueryRow(ctx, `SELECT public.admin_edit_training_session($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9)`,
		sessionID, input.ProgramID, input.StartsAt, input.EndsAt, input.Location, input.Capacity, nullable(input.Notes), input.IsPublic, input.AcknowledgeBookedChanges).Scan(&updated)
	if err != nil {
		return fmt.Errorf("edit training session: %w", err)
	}
	if !updated {
		return errors.New("training session was not updated")
	}
	return nil
}

func (r *BookingRepository) UpdateTrainingSession(ctx context.Context, sessionID string, status *string, notes *string, isPublic *bool) error {
	var updated bool
	err := r.db.QueryRow(ctx, `SELECT public.admin_update_training_session($1::uuid, $2, $3, $4)`,
		sessionID, status, notes, isPublic).Scan(&updated)
	if err != nil {
		return fmt.Errorf("update training session: %w", err)
	}
	return nil
}

func decodeNameList(encoded []byte) ([]string, error) {
	names := make([]string, 0)
	if len(encoded) == 0 {
		return names, nil
	}
	if err := json.Unmarshal(encoded, &names); err != nil {
		return nil, err
	}
	if names == nil {
		names = []string{}
	}
	return names, nil
}

func (r *BookingRepository) ListCoaches(ctx context.Context) ([]training.Coach, error) {
	rows, err := r.db.Query(ctx, `SELECT id, full_name, email, phone, notes, is_active, created_at, updated_at, assigned_session_count FROM public.admin_list_coaches()`)
	if err != nil {
		return nil, fmt.Errorf("list coaches: %w", err)
	}
	defer rows.Close()
	result := make([]training.Coach, 0)
	for rows.Next() {
		var coach training.Coach
		if err := rows.Scan(&coach.ID, &coach.FullName, &coach.Email, &coach.Phone, &coach.Notes, &coach.IsActive, &coach.CreatedAt, &coach.UpdatedAt, &coach.AssignedSessionCount); err != nil {
			return nil, fmt.Errorf("scan coach: %w", err)
		}
		result = append(result, coach)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read coaches: %w", err)
	}
	return result, nil
}

func (r *BookingRepository) CreateCoach(ctx context.Context, fullName, email, phone, notes string) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `SELECT public.admin_create_coach($1, $2, $3, $4)`,
		fullName, nullable(email), nullable(phone), nullable(notes)).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create coach: %w", err)
	}
	return id, nil
}

func (r *BookingRepository) UpdateCoach(ctx context.Context, id, fullName, email, phone, notes string, isActive bool) error {
	var updated bool
	err := r.db.QueryRow(ctx, `SELECT public.admin_update_coach($1::uuid, $2, $3, $4, $5, $6)`,
		id, fullName, nullable(email), nullable(phone), nullable(notes), isActive).Scan(&updated)
	if err != nil {
		return fmt.Errorf("update coach: %w", err)
	}
	if !updated {
		return errors.New("coach was not updated")
	}
	return nil
}

func (r *BookingRepository) ListActiveCoaches(ctx context.Context) ([]training.CoachDirectoryEntry, error) {
	rows, err := r.db.Query(ctx, `SELECT id, full_name, email, phone, notes, is_active FROM public.coach_list_coaches()`)
	if err != nil {
		return nil, fmt.Errorf("list coach directory: %w", err)
	}
	defer rows.Close()
	result := make([]training.CoachDirectoryEntry, 0)
	for rows.Next() {
		var coach training.CoachDirectoryEntry
		if err := rows.Scan(&coach.ID, &coach.FullName, &coach.Email, &coach.Phone, &coach.Notes, &coach.IsActive); err != nil {
			return nil, fmt.Errorf("scan coach directory entry: %w", err)
		}
		result = append(result, coach)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read coach directory: %w", err)
	}
	return result, nil
}

func (r *BookingRepository) ListCoachSchedule(ctx context.Context) ([]training.CoachScheduleSession, error) {
	rows, err := r.db.Query(ctx, `SELECT id, program_name, program_description, starts_at, ends_at, location, status, is_public, assigned_coach_id, assigned_coach_name, participant_names, confirmed_booking_count FROM public.coach_list_training_schedule()`)
	if err != nil {
		return nil, fmt.Errorf("list coach schedule: %w", err)
	}
	defer rows.Close()
	result := make([]training.CoachScheduleSession, 0)
	for rows.Next() {
		var session training.CoachScheduleSession
		var participantNames []byte
		if err := rows.Scan(&session.ID, &session.ProgramName, &session.ProgramDescription, &session.StartsAt, &session.EndsAt, &session.Location, &session.Status, &session.IsPublic, &session.AssignedCoachID, &session.AssignedCoachName, &participantNames, &session.ConfirmedBookingCount); err != nil {
			return nil, fmt.Errorf("scan coach schedule session: %w", err)
		}
		session.ParticipantNames, err = decodeNameList(participantNames)
		if err != nil {
			return nil, fmt.Errorf("decode coach schedule participants: %w", err)
		}
		result = append(result, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read coach schedule: %w", err)
	}
	return result, nil
}
