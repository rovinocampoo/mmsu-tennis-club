package training

import (
	"context"
	"errors"
	"time"
)

// ErrUnavailable indicates that training-session data could not be read from
// PostgreSQL, including when DATABASE_URL is not configured.
var ErrUnavailable = errors.New("training session data unavailable")

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
