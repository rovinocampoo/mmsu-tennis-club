package postgres

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type queryerStub struct {
	err   error
	args  []any
	query string
	rows  pgx.Rows
}

func (q *queryerStub) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	q.query = sql
	q.args = args
	if q.err != nil {
		return nil, q.err
	}
	if q.rows != nil {
		return q.rows, nil
	}
	return nil, errors.New("unexpected query without row fixture")
}

type rowsFixture struct {
	rows      [][]any
	current   []any
	index     int
	iteration error
	closed    bool
}

func (r *rowsFixture) Close()                                       { r.closed = true }
func (r *rowsFixture) Err() error                                   { return r.iteration }
func (r *rowsFixture) CommandTag() pgconn.CommandTag                { return pgconn.NewCommandTag("SELECT") }
func (r *rowsFixture) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *rowsFixture) Next() bool {
	if r.index >= len(r.rows) {
		return false
	}
	r.current = r.rows[r.index]
	r.index++
	return true
}
func (r *rowsFixture) Scan(dest ...any) error {
	if len(dest) != len(r.current) {
		return errors.New("scan destination count mismatch")
	}
	for i := range dest {
		if err := assignScanValue(dest[i], r.current[i]); err != nil {
			return err
		}
	}
	return nil
}
func (r *rowsFixture) Values() ([]any, error) { return r.current, nil }
func (r *rowsFixture) RawValues() [][]byte    { return nil }
func (r *rowsFixture) Conn() *pgx.Conn        { return nil }
func (r *rowsFixture) TypeMap() *pgtype.Map   { return pgtype.NewMap() }

func assignScanValue(dest, src any) error {
	target := reflect.ValueOf(dest)
	if target.Kind() != reflect.Pointer || target.IsNil() {
		return errors.New("scan destination must be a non-nil pointer")
	}
	target = target.Elem()
	if src == nil {
		if target.Kind() != reflect.Pointer {
			return errors.New("cannot scan NULL into non-pointer destination")
		}
		target.SetZero()
		return nil
	}
	if target.Kind() == reflect.Pointer {
		target.Set(reflect.New(target.Type().Elem()))
		target = target.Elem()
	}
	source := reflect.ValueOf(src)
	if source.Type().AssignableTo(target.Type()) {
		target.Set(source)
		return nil
	}
	if source.Type().ConvertibleTo(target.Type()) {
		target.Set(source.Convert(target.Type()))
		return nil
	}
	return errors.New("row value has an incompatible type")
}

func TestUpcomingQueryFiltersPublicUpcomingSessionsAndCountsConfirmedBookings(t *testing.T) {
	for _, fragment := range []string{
		"JOIN public.training_programs AS p ON p.id = s.program_id",
		"FROM public.bookings",
		"b.status = 'confirmed'",
		"s.starts_at >= GREATEST($1, pg_catalog.statement_timestamp())",
		"s.status <> 'cancelled'",
		"s.is_public = true",
		"p.is_active IS TRUE",
		"ORDER BY s.starts_at ASC",
	} {
		if !strings.Contains(listUpcomingTrainingSessions, fragment) {
			t.Errorf("query does not contain required clause %q", fragment)
		}
	}
	if strings.Contains(strings.ToLower(listUpcomingTrainingSessions), "concat(") {
		t.Fatal("query should use parameters rather than construct SQL dynamically")
	}
	if strings.Count(strings.ToLower(listUpcomingTrainingSessions), "public.bookings") != 1 {
		t.Errorf("query should use one SQL booking aggregate; query references bookings %d times", strings.Count(strings.ToLower(listUpcomingTrainingSessions), "public.bookings"))
	}
}

func TestRepositoryPassesUpcomingCutoffAsQueryParameter(t *testing.T) {
	query := &queryerStub{err: errors.New("database unavailable")}
	repository := NewTrainingRepository(query)
	cutoff := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	_, err := repository.ListUpcoming(context.Background(), cutoff)
	if err == nil {
		t.Fatal("ListUpcoming() succeeded, want query error")
	}
	if len(query.args) != 1 || query.args[0] != cutoff {
		t.Errorf("query args = %#v, want cutoff time parameter", query.args)
	}
	if !strings.Contains(err.Error(), "query upcoming training sessions") {
		t.Errorf("error = %q, want repository context", err)
	}
}

func TestRepositoryScansNullableCapacityAndBookingCounts(t *testing.T) {
	start := time.Date(2026, 10, 10, 1, 0, 0, 0, time.UTC)
	description := "Public group session"
	rows := &rowsFixture{rows: [][]any{
		{
			"11111111-1111-4111-8111-111111111111", "Open Court", nil,
			start, start.Add(time.Hour), "Court 1", nil, int64(3), nil, "open", true,
		},
		{
			"22222222-2222-4222-8222-222222222222", "Group Training", description,
			start.Add(24 * time.Hour), start.Add(25 * time.Hour), "Court 2", int32(8), int64(3), int32(5), "closed", true,
		},
	}}
	repository := NewTrainingRepository(&queryerStub{rows: rows})
	sessions, err := repository.ListUpcoming(context.Background(), start.Add(-time.Hour))
	if err != nil {
		t.Fatalf("ListUpcoming() returned an unexpected error: %v", err)
	}
	if !rows.closed {
		t.Error("rows were not closed")
	}
	if len(sessions) != 2 {
		t.Fatalf("returned %d sessions, want 2", len(sessions))
	}
	if sessions[0].Capacity != nil || sessions[0].RemainingCapacity != nil {
		t.Errorf("unlimited capacity mapped as capacity=%v remaining=%v, want both nil", sessions[0].Capacity, sessions[0].RemainingCapacity)
	}
	if sessions[0].ConfirmedBookingCount != 3 {
		t.Errorf("unlimited session confirmed count = %d, want 3", sessions[0].ConfirmedBookingCount)
	}
	if sessions[0].ProgramDescription != nil {
		t.Errorf("NULL description mapped as %q, want nil", *sessions[0].ProgramDescription)
	}
	if sessions[1].Capacity == nil || *sessions[1].Capacity != 8 {
		t.Errorf("finite capacity = %v, want 8", sessions[1].Capacity)
	}
	if sessions[1].ConfirmedBookingCount != 3 {
		t.Errorf("finite session confirmed count = %d, want 3", sessions[1].ConfirmedBookingCount)
	}
	if sessions[1].RemainingCapacity == nil || *sessions[1].RemainingCapacity != 5 {
		t.Errorf("remaining capacity = %v, want 5", sessions[1].RemainingCapacity)
	}
	if sessions[1].ProgramDescription == nil || *sessions[1].ProgramDescription != description {
		t.Errorf("program description = %v, want %q", sessions[1].ProgramDescription, description)
	}
}
