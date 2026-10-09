package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/training"
)

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// TrainingRepository loads public training sessions from PostgreSQL.
type TrainingRepository struct {
	db queryer
}

func NewTrainingRepository(db queryer) *TrainingRepository {
	return &TrainingRepository{db: db}
}

const listUpcomingTrainingSessions = `
SELECT
    s.id::text,
    p.name,
    p.description,
    s.starts_at,
    s.ends_at,
    s.location,
    s.capacity,
    COALESCE(cb.confirmed_booking_count, 0),
    CASE
        WHEN s.capacity IS NULL THEN NULL
        ELSE GREATEST(s.capacity::bigint - COALESCE(cb.confirmed_booking_count, 0), 0)::integer
    END,
    s.status,
    s.is_public
FROM public.training_sessions AS s
JOIN public.training_programs AS p ON p.id = s.program_id
LEFT JOIN LATERAL (
    SELECT count(*)::bigint AS confirmed_booking_count
    FROM public.bookings AS b
    WHERE b.session_id = s.id
      AND b.status = 'confirmed'
) AS cb ON true
WHERE s.starts_at >= GREATEST($1, pg_catalog.statement_timestamp())
  AND s.status <> 'cancelled'
  AND s.is_public = true
  AND p.is_active IS TRUE
ORDER BY s.starts_at ASC`

func (r *TrainingRepository) ListUpcoming(ctx context.Context, from time.Time) ([]training.Session, error) {
	rows, err := r.db.Query(ctx, listUpcomingTrainingSessions, from)
	if err != nil {
		return nil, fmt.Errorf("query upcoming training sessions: %w", err)
	}
	defer rows.Close()

	sessions := make([]training.Session, 0)
	for rows.Next() {
		var session training.Session
		if err := rows.Scan(
			&session.ID,
			&session.ProgramName,
			&session.ProgramDescription,
			&session.StartsAt,
			&session.EndsAt,
			&session.Location,
			&session.Capacity,
			&session.ConfirmedBookingCount,
			&session.RemainingCapacity,
			&session.Status,
			&session.IsPublic,
		); err != nil {
			return nil, fmt.Errorf("scan upcoming training session: %w", err)
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate upcoming training sessions: %w", err)
	}
	return sessions, nil
}
