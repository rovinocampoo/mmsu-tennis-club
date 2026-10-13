package training

import (
	"context"
	"errors"
	"time"
)

// ErrUnavailable indicates that training-session data could not be read from
// PostgreSQL, including when DATABASE_URL is not configured.
var ErrUnavailable = errors.New("training session data unavailable")
var ErrBookingStatusNotFound = errors.New("booking status not found")
var ErrBookingStatusExpired = errors.New("booking status link expired")

// Session contains only fields intended for the public training schedule.
type Session struct {
	ID                    string    `json:"id"`
	ProgramName           string    `json:"program_name"`
	ProgramDescription    *string   `json:"program_description"`
	StartsAt              time.Time `json:"starts_at"`
	EndsAt                time.Time `json:"ends_at"`
	Location              string    `json:"location"`
	Capacity              *int      `json:"capacity"`
	ConfirmedBookingCount int64     `json:"confirmed_booking_count"`
	RemainingCapacity     *int      `json:"remaining_capacity"`
	Status                string    `json:"status"`
	IsPublic              bool      `json:"is_public"`
}

// Repository reads upcoming public schedule rows from the database.
type Repository interface {
	ListUpcoming(context.Context, time.Time) ([]Session, error)
	ListActivePrograms(context.Context) ([]Program, error)
}

// BookingRequest is a public request for a spot; it does not reserve capacity.
type BookingRequest struct {
	SessionID string
	FullName  string
	Email     string
	Phone     string
}

type PublicBookingStatus struct {
	RequestReference string
	Status           string
	ProgramName      string
	StartsAt         time.Time
	EndsAt           time.Time
	Location         string
}

type BookingRequestRecord struct {
	ID            string    `json:"id"`
	SessionID     string    `json:"session_id"`
	FullName      string    `json:"full_name"`
	Email         *string   `json:"email"`
	Phone         *string   `json:"phone"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	StartsAt      time.Time `json:"starts_at"`
	Location      string    `json:"location"`
	SessionStatus string    `json:"session_status"`
}

type PlayerCandidate struct {
	ID       string  `json:"id"`
	FullName string  `json:"full_name"`
	Email    *string `json:"email"`
	Phone    *string `json:"phone"`
}

type CoachAvailability struct {
	ID        string    `json:"id"`
	CoachID   string    `json:"coach_id"`
	CoachName string    `json:"coach_name"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
	Notes     *string   `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
}

type TrainingProgram struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	IsActive    bool    `json:"is_active"`
}

type Program struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type AdminSessionRecord struct {
	ID                    string    `json:"id"`
	ProgramID             string    `json:"program_id"`
	ProgramName           string    `json:"program_name"`
	StartsAt              time.Time `json:"starts_at"`
	EndsAt                time.Time `json:"ends_at"`
	Location              string    `json:"location"`
	Capacity              *int      `json:"capacity"`
	ConfirmedBookingCount int64     `json:"confirmed_booking_count"`
	Status                string    `json:"status"`
	Notes                 *string   `json:"notes"`
	IsPublic              bool      `json:"is_public"`
	AssignedCoachID       *string   `json:"assigned_coach_id"`
	AssignedCoachName     *string   `json:"assigned_coach_name"`
}

type AssignedTrainingSession struct {
	ID                 string    `json:"id"`
	ProgramName        string    `json:"program_name"`
	ProgramDescription *string   `json:"program_description"`
	StartsAt           time.Time `json:"starts_at"`
	EndsAt             time.Time `json:"ends_at"`
	Location           string    `json:"location"`
	Status             string    `json:"status"`
	IsPublic           bool      `json:"is_public"`
}

type BookingRepository interface {
	CreateRequest(context.Context, BookingRequest, []byte) (string, error)
	GetPublicStatus(context.Context, []byte) (PublicBookingStatus, error)
	ListPendingRequests(context.Context) ([]BookingRequestRecord, error)
	SearchPlayers(context.Context, string) ([]PlayerCandidate, error)
	ApproveRequest(context.Context, string, string, *string, bool) (string, error)
	RejectRequest(context.Context, string, string) error

	SubmitCoachAvailability(ctx context.Context, coachID, coachName string, startsAt, endsAt time.Time, notes string) (string, error)
	ListCoachAvailability(ctx context.Context, coachID string) ([]CoachAvailability, error)
	DeleteCoachAvailability(ctx context.Context, id, coachID string) error
	ListAllCoachAvailability(ctx context.Context) ([]CoachAvailability, error)
	ListAdminTrainingPrograms(ctx context.Context) ([]TrainingProgram, error)
	ListAdminTrainingSessions(ctx context.Context) ([]AdminSessionRecord, error)
	ListAssignedTrainingSessions(ctx context.Context, coachID string) ([]AssignedTrainingSession, error)
	AssignTrainingSessionCoach(ctx context.Context, sessionID string, coachID *string) error
	CreateTrainingSession(ctx context.Context, programID string, startsAt, endsAt time.Time, location string, capacity *int, notes string, isPublic bool) (string, error)
	UpdateTrainingSession(ctx context.Context, sessionID string, status *string, notes *string, isPublic *bool) error
}

// Service contains training schedule business logic.
type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository, now: time.Now}
}

func (s *Service) ListUpcoming(ctx context.Context) ([]Session, error) {
	if s.repository == nil {
		return nil, ErrUnavailable
	}

	sessions, err := s.repository.ListUpcoming(ctx, s.now().UTC())
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	if sessions == nil {
		sessions = []Session{}
	}
	return sessions, nil
}

func (s *Service) ListActivePrograms(ctx context.Context) ([]Program, error) {
	if s.repository == nil {
		return nil, ErrUnavailable
	}
	programs, err := s.repository.ListActivePrograms(ctx)
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	if programs == nil {
		programs = []Program{}
	}
	return programs, nil
}
