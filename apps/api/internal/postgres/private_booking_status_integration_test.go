package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/training"
)

func newTestTrackingTokenHash(t *testing.T) []byte {
	t.Helper()
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(token[:])
	return hash[:]
}

func TestPrivateBookingStatusPostgresWorkflow(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to the disposable local database")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var isDisposableTarget bool
	if err := db.QueryRow(ctx, `SELECT current_database() = 'mmsu_booking_test'
		AND inet_server_addr() = '127.0.0.1'::inet
		AND inet_server_port() = 55432`).Scan(&isDisposableTarget); err != nil {
		t.Fatal(err)
	}
	if !isDisposableTarget {
		t.Fatal("refusing integration test writes outside 127.0.0.1:55432/mmsu_booking_test")
	}
	var bookingAPIStatus, adminAPIStatus, anonStatus, authenticatedStatus, bookingAPIWithoutToken bool
	var bookingAPICanLookupProgram, adminAPICanLookupProgram, bookingAPICanSelectPrograms bool
	var bookingAPICanListPending, adminAPICanListPending, bookingAPICanListPrograms, adminAPICanListPrograms bool
	if err := db.QueryRow(ctx, `SELECT
		has_function_privilege('mmsu_booking_api', 'public.get_public_booking_status(bytea)', 'EXECUTE'),
		has_function_privilege('mmsu_booking_admin_api', 'public.get_public_booking_status(bytea)', 'EXECUTE'),
		has_function_privilege('anon', 'public.get_public_booking_status(bytea)', 'EXECUTE'),
		has_function_privilege('authenticated', 'public.get_public_booking_status(bytea)', 'EXECUTE'),
		has_function_privilege('mmsu_booking_api', 'public.create_public_booking_request(uuid,text,text,text)', 'EXECUTE'),
		has_function_privilege('mmsu_booking_api', 'public.get_active_training_program_name(text)', 'EXECUTE'),
		has_function_privilege('mmsu_booking_admin_api', 'public.get_active_training_program_name(text)', 'EXECUTE'),
		has_table_privilege('mmsu_booking_api', 'public.training_programs', 'SELECT'),
		has_function_privilege('mmsu_booking_api', 'public.list_pending_booking_requests_with_slots()', 'EXECUTE'),
		has_function_privilege('mmsu_booking_admin_api', 'public.list_pending_booking_requests_with_slots()', 'EXECUTE'),
		has_function_privilege('mmsu_booking_api', 'public.admin_list_training_programs()', 'EXECUTE'),
		has_function_privilege('mmsu_booking_admin_api', 'public.admin_list_training_programs()', 'EXECUTE')`).Scan(
		&bookingAPIStatus, &adminAPIStatus, &anonStatus, &authenticatedStatus, &bookingAPIWithoutToken,
		&bookingAPICanLookupProgram, &adminAPICanLookupProgram, &bookingAPICanSelectPrograms,
		&bookingAPICanListPending, &adminAPICanListPending, &bookingAPICanListPrograms, &adminAPICanListPrograms,
	); err != nil {
		t.Fatal(err)
	}
	if !bookingAPIStatus || adminAPIStatus || anonStatus || authenticatedStatus || bookingAPIWithoutToken {
		t.Fatalf("status grants: booking API=%t admin API=%t anon=%t authenticated=%t legacy create=%t",
			bookingAPIStatus, adminAPIStatus, anonStatus, authenticatedStatus, bookingAPIWithoutToken)
	}
	if !bookingAPICanLookupProgram || adminAPICanLookupProgram || bookingAPICanSelectPrograms {
		t.Fatalf("program lookup grants: booking API function=%t admin API function=%t booking API table SELECT=%t",
			bookingAPICanLookupProgram, adminAPICanLookupProgram, bookingAPICanSelectPrograms)
	}
	if bookingAPICanListPending || !adminAPICanListPending || bookingAPICanListPrograms || !adminAPICanListPrograms {
		t.Fatalf("admin request-list grants: booking API pending=%t admin API pending=%t booking API programs=%t admin API programs=%t",
			bookingAPICanListPending, adminAPICanListPending, bookingAPICanListPrograms, adminAPICanListPrograms)
	}

	repository := NewBookingRepository(db)
	var runID, adminID, programID, selectedProgramID, sessionID string
	if err := db.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	adminID = runID
	contactPrefix := "private-status-" + runID
	if err := db.QueryRow(ctx, `INSERT INTO public.training_programs(name) VALUES ('Status integration ' || $1) RETURNING id::text`, runID).Scan(&programID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO public.training_programs(name) VALUES ('Selected status integration ' || $1) RETURNING id::text`, runID).Scan(&selectedProgramID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `
		INSERT INTO public.training_sessions(program_id, starts_at, ends_at, location, capacity, status, is_public)
		VALUES ($1::uuid, now() + interval '1 day', now() + interval '1 day 1 hour', 'Status Test Court', NULL, 'open', true)
		RETURNING id::text`, programID).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}

	requestIDs := make([]string, 0, 4)
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM public.booking_approval_notifications WHERE request_id = ANY($1::uuid[])`, requestIDs)
		_, _ = db.Exec(context.Background(), `DELETE FROM public.bookings WHERE session_id = $1::uuid`, sessionID)
		_, _ = db.Exec(context.Background(), `DELETE FROM public.booking_requests WHERE id = ANY($1::uuid[])`, requestIDs)
		_, _ = db.Exec(context.Background(), `DELETE FROM public.players WHERE email LIKE $1`, contactPrefix+"-%")
		_, _ = db.Exec(context.Background(), `DELETE FROM public.training_sessions WHERE id = $1::uuid`, sessionID)
		_, _ = db.Exec(context.Background(), `DELETE FROM public.training_programs WHERE id = $1::uuid`, selectedProgramID)
		_, _ = db.Exec(context.Background(), `DELETE FROM public.training_programs WHERE id = $1::uuid`, programID)
	})

	newRequest := func(suffix string, trainingFocus ...string) (string, string, []byte) {
		t.Helper()
		var token [32]byte
		if _, err := rand.Read(token[:]); err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(token[:])
		email := fmt.Sprintf("%s-%s@example.test", contactPrefix, suffix)
		request := training.BookingRequest{
			SessionID: sessionID,
			FullName:  "Status " + suffix,
			Email:     email,
		}
		if len(trainingFocus) > 0 {
			request.TrainingFocus = trainingFocus[0]
		}
		reference, err := repository.CreateRequest(ctx, request, hash[:])
		if err != nil {
			t.Fatal(err)
		}
		var requestID string
		if err := db.QueryRow(ctx, `SELECT id::text FROM public.booking_requests WHERE public_reference = $1`, reference).Scan(&requestID); err != nil {
			t.Fatal(err)
		}
		requestIDs = append(requestIDs, requestID)
		return requestID, reference, token[:]
	}

	_, pendingReference, pendingToken := newRequest("pending")
	_, _, selectedToken := newRequest("selected", selectedProgramID)
	approvedID, approvedReference, approvedToken := newRequest("approved")
	rejectedID, _, rejectedToken := newRequest("rejected")
	_, _, expiredToken := newRequest("expired")

	var storedHash []byte
	var tokenExpiry time.Time
	if err := db.QueryRow(ctx, `SELECT tracking_token_hash, tracking_token_expires_at FROM public.booking_requests WHERE public_reference = $1`, pendingReference).Scan(&storedHash, &tokenExpiry); err != nil {
		t.Fatal(err)
	}
	expectedHash := sha256.Sum256(pendingToken)
	if len(storedHash) != sha256.Size || string(storedHash) != string(expectedHash[:]) || !tokenExpiry.After(time.Now().Add(29*24*time.Hour)) {
		t.Fatalf("token hash or expiry is not stored as expected (hash length=%d expiry=%s)", len(storedHash), tokenExpiry)
	}

	var bookingID string
	if err := db.QueryRow(ctx, `SELECT public.approve_booking_request($1::uuid, $2::uuid, NULL, true)::text`, approvedID, adminID).Scan(&bookingID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `SELECT public.reject_booking_request($1::uuid, $2::uuid)`, rejectedID, adminID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE public.booking_requests SET tracking_token_expires_at = now() - interval '1 second' WHERE id = $1::uuid`, requestIDs[3]); err != nil {
		t.Fatal(err)
	}

	readStatus := func(token []byte) (training.PublicBookingStatus, error) {
		hash := sha256.Sum256(token)
		return repository.GetPublicStatus(ctx, hash[:])
	}
	for _, tc := range []struct {
		name, reference, state string
		token                  []byte
	}{
		{"pending", pendingReference, "pending", pendingToken},
		{"approved", approvedReference, "approved", approvedToken},
		{"rejected", "", "rejected", rejectedToken},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, err := readStatus(tc.token)
			if err != nil {
				t.Fatal(err)
			}
			if status.Status != tc.state || (tc.reference != "" && status.RequestReference != tc.reference) ||
				status.ProgramName != "Status integration "+runID || status.Location != "Status Test Court" {
				t.Fatalf("status=%#v, want state=%q reference=%q", status, tc.state, tc.reference)
			}
			if status.StartsAt.IsZero() || status.EndsAt.IsZero() {
				t.Fatalf("missing session time in status: %#v", status)
			}
		})
	}
	unselectedStatus, err := readStatus(pendingToken)
	if err != nil || unselectedStatus.TrainingFocus != "" || unselectedStatus.TrainingFocusNotes != "" || unselectedStatus.TrainingProgramName != "" {
		t.Fatalf("legacy request with NULL training focus fields: status=%#v err=%v", unselectedStatus, err)
	}
	selectedStatus, err := readStatus(selectedToken)
	if err != nil || selectedStatus.TrainingProgramName != "Selected status integration "+runID {
		t.Fatalf("selected training program status=%#v err=%v", selectedStatus, err)
	}
	for _, programID := range []any{"missing-program-id", nil} {
		var name string
		if err := db.QueryRow(ctx, `SELECT COALESCE(public.get_active_training_program_name($1::text), '')`, programID).Scan(&name); err != nil || name != "" {
			t.Fatalf("missing/NULL program lookup returned name=%q err=%v", name, err)
		}
	}
	if _, err := db.Exec(ctx, `UPDATE public.training_programs SET is_active = false WHERE id = $1::uuid`, selectedProgramID); err != nil {
		t.Fatal(err)
	}
	selectedStatus, err = readStatus(selectedToken)
	if err != nil || selectedStatus.TrainingProgramName != "" {
		t.Fatalf("inactive historical program should safely omit its display name: status=%#v err=%v", selectedStatus, err)
	}

	pendingStatus, err := readStatus(pendingToken)
	if err != nil || pendingStatus.RequestReference == approvedReference {
		t.Fatalf("pending token returned another request: status=%#v err=%v", pendingStatus, err)
	}
	if _, err := readStatus([]byte("no matching token in the database")); !errors.Is(err, training.ErrBookingStatusNotFound) {
		t.Fatalf("invalid token error=%v", err)
	}
	if _, err := readStatus(expiredToken); !errors.Is(err, training.ErrBookingStatusExpired) {
		t.Fatalf("expired token error=%v", err)
	}
}
