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
// Participants holds the normalized per-person names; FullName is the
// comma-joined display fallback persisted for backward compatibility.
type BookingRequest struct {
	SessionID    string
	SessionIDs   []string
	FullName     string
	Participants []string
	Email        string
	Phone        string
}

type TrainingSessionCreateInput struct {
	ProgramID string    `json:"program_id"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
	Location  string    `json:"location"`
	Capacity  *int      `json:"capacity"`
	Notes     string    `json:"notes"`
	IsPublic  bool      `json:"is_public"`
	CoachID   *string   `json:"coach_id"`
}

type TrainingSessionEditInput struct {
	ProgramID                string
	StartsAt                 time.Time
	EndsAt                   time.Time
	Location                 string
	Capacity                 *int
	Notes                    string
	IsPublic                 bool
	AcknowledgeBookedChanges bool
}

// PublicationResult reports the outcome of one atomic bulk-publication run.
// Published is the number of hidden, open sessions made public; Skipped is the
// number of hidden sessions in range that were left alone because they were
// closed or cancelled. Failed is always zero because the run is a single
// statement that either commits entirely or errors.
type PublicationResult struct {
	Published int `json:"published"`
	Skipped   int `json:"skipped"`
	Failed    int `json:"failed"`
}

type PublicBookingStatus struct {
	RequestReference string
	Status           string
	ProgramName      string
	StartsAt         time.Time
	EndsAt           time.Time
	Location         string
}

type CalendarEvent struct {
	RequestID        string
	RequestReference string
	ParticipantName  string
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
	EndsAt        time.Time `json:"ends_at"`
	Location      string    `json:"location"`
	SessionStatus string    `json:"session_status"`
}

type PlayerCandidate struct {
	ID       string  `json:"id"`
	FullName string  `json:"full_name"`
	Email    *string `json:"email"`
	Phone    *string `json:"phone"`
}

// FeedbackSubmission is a public suggestion or feedback entry. Contact fields
// are optional so anonymous submissions are supported.
type FeedbackSubmission struct {
	Category string
	Message  string
	Name     string
	Email    string
}

// FeedbackRecord is an admin-visible feedback submission.
type FeedbackRecord struct {
	ID         string     `json:"id"`
	Category   string     `json:"category"`
	Message    string     `json:"message"`
	Name       *string    `json:"name"`
	Email      *string    `json:"email"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	ReviewedAt *time.Time `json:"reviewed_at"`
	ReviewedBy *string    `json:"reviewed_by"`
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

type ApprovedClientRecord struct {
	BookingID         string    `json:"booking_id"`
	PlayerID          string    `json:"player_id"`
	PlayerName        string    `json:"player_name"`
	PlayerEmail       *string   `json:"player_email"`
	PlayerPhone       *string   `json:"player_phone"`
	SessionID         string    `json:"session_id"`
	ProgramName       string    `json:"program_name"`
	StartsAt          time.Time `json:"starts_at"`
	EndsAt            time.Time `json:"ends_at"`
	Location          string    `json:"location"`
	Capacity          *int      `json:"capacity"`
	AssignedCoachID   *string   `json:"assigned_coach_id"`
	AssignedCoachName *string   `json:"assigned_coach_name"`
	BookingCreatedAt  time.Time `json:"booking_created_at"`
	ParticipantNames  []string  `json:"participant_names"`
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

// Coach is an admin-managed directory record. Coach records exist
// independently of any login.
type Coach struct {
	ID                   string    `json:"id"`
	FullName             string    `json:"full_name"`
	Email                *string   `json:"email"`
	Phone                *string   `json:"phone"`
	Notes                *string   `json:"notes"`
	IsActive             bool      `json:"is_active"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
	AssignedSessionCount int64     `json:"assigned_session_count"`
}

// CoachDirectoryEntry is the read-only coach directory shown to the coach portal.
type CoachDirectoryEntry struct {
	ID       string  `json:"id"`
	FullName string  `json:"full_name"`
	Email    *string `json:"email"`
	Phone    *string `json:"phone"`
	Notes    *string `json:"notes"`
	IsActive bool    `json:"is_active"`
}

// CoachScheduleSession is a read-only schedule row for the shared coach portal,
// including the single assigned coach and the confirmed participant roster.
type CoachScheduleSession struct {
	ID                    string    `json:"id"`
	ProgramName           string    `json:"program_name"`
	ProgramDescription    *string   `json:"program_description"`
	StartsAt              time.Time `json:"starts_at"`
	EndsAt                time.Time `json:"ends_at"`
	Location              string    `json:"location"`
	Status                string    `json:"status"`
	IsPublic              bool      `json:"is_public"`
	AssignedCoachID       *string   `json:"assigned_coach_id"`
	AssignedCoachName     *string   `json:"assigned_coach_name"`
	ParticipantNames      []string  `json:"participant_names"`
	ConfirmedBookingCount int64     `json:"confirmed_booking_count"`
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
	ListApprovedClients(ctx context.Context) ([]ApprovedClientRecord, error)
	ListCoaches(ctx context.Context) ([]Coach, error)
	CreateCoach(ctx context.Context, fullName, email, phone, notes string) (string, error)
	UpdateCoach(ctx context.Context, id, fullName, email, phone, notes string, isActive bool) error
	ListActiveCoaches(ctx context.Context) ([]CoachDirectoryEntry, error)
	ListCoachSchedule(ctx context.Context) ([]CoachScheduleSession, error)
	CreateFeedback(ctx context.Context, submission FeedbackSubmission) (string, error)
	ListFeedbackSubmissions(ctx context.Context) ([]FeedbackRecord, error)
	UpdateFeedbackStatus(ctx context.Context, id, status, adminID string) (bool, error)
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
