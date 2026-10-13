package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This test runs the real migration functions when TEST_DATABASE_URL points at
// a disposable PostgreSQL database with repository migrations already applied.
func TestManualApprovalPostgresWorkflowAndConcurrentCapacity(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable migrated PostgreSQL database")
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
	var runID string
	if err := db.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	adminID := runID
	var publicRoleCanReview, adminRoleCanReview bool
	if err := db.QueryRow(ctx, `SELECT has_function_privilege('mmsu_booking_api','public.approve_booking_request(uuid,uuid,uuid,boolean)','EXECUTE'), has_function_privilege('mmsu_booking_admin_api','public.approve_booking_request(uuid,uuid,uuid,boolean)','EXECUTE')`).Scan(&publicRoleCanReview, &adminRoleCanReview); err != nil {
		t.Fatal(err)
	}
	if publicRoleCanReview || !adminRoleCanReview {
		t.Fatalf("review function grants: public API=%t admin API=%t", publicRoleCanReview, adminRoleCanReview)
	}
	var programID, capacitySession, emptySession, concurrentSession, cancelledSession string
	if err := db.QueryRow(ctx, `INSERT INTO public.training_programs(name) VALUES ('integration ' || $1) RETURNING id::text`, runID).Scan(&programID); err != nil {
		t.Fatal(err)
	}
	createSession := func(capacity *int, status string) string {
		t.Helper()
		var id string
		err := db.QueryRow(ctx, `INSERT INTO public.training_sessions(program_id, starts_at, ends_at, location, capacity, status, is_public) VALUES ($1::uuid, now() + interval '1 day', now() + interval '1 day 1 hour', 'Integration Court', $2, $3, true) RETURNING id::text`, programID, capacity, status).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	one := 1
	capacitySession = createSession(&one, "open")
	emptySession = createSession(&one, "open")
	concurrentSession = createSession(&one, "open")
	cancelledSession = createSession(&one, "cancelled")
	contactPrefix := "integration-" + runID
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM public.booking_requests WHERE session_id = ANY($1::uuid[])`, []string{capacitySession, emptySession, concurrentSession, cancelledSession})
		_, _ = db.Exec(context.Background(), `DELETE FROM public.bookings WHERE session_id = ANY($1::uuid[])`, []string{capacitySession, emptySession, concurrentSession, cancelledSession})
		_, _ = db.Exec(context.Background(), `DELETE FROM public.players WHERE email LIKE $1`, contactPrefix+"-%")
		_, _ = db.Exec(context.Background(), `DELETE FROM public.training_sessions WHERE id = ANY($1::uuid[])`, []string{capacitySession, emptySession, concurrentSession, cancelledSession})
		_, _ = db.Exec(context.Background(), `DELETE FROM public.training_programs WHERE id = $1::uuid`, programID)
	})

	request := func(sessionID, suffix string) string {
		t.Helper()
		var id string
		var reference string
		err := db.QueryRow(ctx, `SELECT public.create_public_booking_request_with_status_token($1::uuid, $2, $3, NULL, $4)`,
			sessionID, "Player "+suffix, contactPrefix+"-"+suffix+"@example.test", newTestTrackingTokenHash(t)).Scan(&reference)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(ctx, `SELECT id::text FROM public.booking_requests WHERE public_reference = $1`, reference).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	firstRequest := request(capacitySession, "first")
	secondRequest := request(capacitySession, "second")
	var pendingCount, bookingCount, playerCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM public.bookings WHERE session_id=$1::uuid`, capacitySession).Scan(&bookingCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM public.players WHERE email LIKE $1`, contactPrefix+"-%").Scan(&playerCount); err != nil {
		t.Fatal(err)
	}
	if bookingCount != 0 || playerCount != 0 {
		t.Fatalf("pending requests created booking=%d players=%d", bookingCount, playerCount)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM public.booking_requests WHERE session_id=$1::uuid AND status='pending'`, capacitySession).Scan(&pendingCount); err != nil {
		t.Fatal(err)
	}
	if pendingCount != 2 {
		t.Fatalf("pending count=%d", pendingCount)
	}

	// A same-email player record is not claimed automatically; the admin must
	// explicitly select it when approving.
	var reconciledPlayer string
	if err := db.QueryRow(ctx, `INSERT INTO public.players(full_name,email) VALUES ('Correct Existing Player',$1) RETURNING id::text`, contactPrefix+"-first@example.test").Scan(&reconciledPlayer); err != nil {
		t.Fatal(err)
	}
	var firstBooking string
	if err := db.QueryRow(ctx, `SELECT public.approve_booking_request($1::uuid,$2::uuid,$3::uuid,false)::text`, firstRequest, adminID, reconciledPlayer).Scan(&firstBooking); err != nil {
		t.Fatalf("approve with capacity: %v", err)
	}
	assertSQLState(t, func() error {
		var value string
		return db.QueryRow(ctx, `SELECT public.create_public_booking_request_with_status_token($1::uuid,'Late Request',$2,NULL,$3)`,
			capacitySession, contactPrefix+"-late@example.test", newTestTrackingTokenHash(t)).Scan(&value)
	}, "PZ002")
	var bookedPlayer string
	if err := db.QueryRow(ctx, `SELECT player_id::text FROM public.bookings WHERE id=$1::uuid`, firstBooking).Scan(&bookedPlayer); err != nil || bookedPlayer != reconciledPlayer {
		t.Fatalf("booking player=%q err=%v, want explicitly selected %q", bookedPlayer, err, reconciledPlayer)
	}
	var recordedAdmin string
	var decidedAt *time.Time
	if err := db.QueryRow(ctx, `SELECT decided_by::text, decided_at FROM public.booking_requests WHERE id=$1::uuid`, firstRequest).Scan(&recordedAdmin, &decidedAt); err != nil {
		t.Fatal(err)
	}
	if recordedAdmin != adminID || decidedAt == nil {
		t.Fatalf("decision metadata admin=%q decided_at=%v", recordedAdmin, decidedAt)
	}
	assertSQLState(t, func() error {
		var value string
		return db.QueryRow(ctx, `SELECT public.approve_booking_request($1::uuid,$2::uuid,NULL,true)::text`, firstRequest, adminID).Scan(&value)
	}, "PZ003")
	assertSQLState(t, func() error {
		var value string
		return db.QueryRow(ctx, `SELECT public.approve_booking_request($1::uuid,$2::uuid,NULL,true)::text`, secondRequest, adminID).Scan(&value)
	}, "PZ002")
	var secondStatus string
	if err := db.QueryRow(ctx, `SELECT status FROM public.booking_requests WHERE id=$1::uuid`, secondRequest).Scan(&secondStatus); err != nil {
		t.Fatal(err)
	}
	if secondStatus != "pending" {
		t.Fatalf("full approval changed request status to %s", secondStatus)
	}
	if _, err := db.Exec(ctx, `SELECT public.reject_booking_request($1::uuid,$2::uuid)`, secondRequest, adminID); err != nil {
		t.Fatalf("reject pending request: %v", err)
	}
	assertSQLState(t, func() error {
		_, err := db.Exec(ctx, `SELECT public.reject_booking_request($1::uuid,$2::uuid)`, secondRequest, adminID)
		return err
	}, "PZ003")
	if err := db.QueryRow(ctx, `SELECT count(*) FROM public.bookings WHERE session_id=$1::uuid`, capacitySession).Scan(&bookingCount); err != nil {
		t.Fatal(err)
	}
	if bookingCount != 1 {
		t.Fatalf("booking count after rejection=%d, want 1", bookingCount)
	}

	duplicateRequest := request(emptySession, "duplicate")
	_ = duplicateRequest
	assertSQLState(t, func() error {
		var value string
		return db.QueryRow(ctx, `SELECT public.create_public_booking_request_with_status_token($1::uuid,'Different Name',$2,NULL,$3)`,
			emptySession, contactPrefix+"-duplicate@example.test", newTestTrackingTokenHash(t)).Scan(&value)
	}, "PZ004")
	assertSQLState(t, func() error {
		var value string
		return db.QueryRow(ctx, `SELECT public.create_public_booking_request_with_status_token($1::uuid,'Cancelled',$2,NULL,$3)`,
			cancelledSession, contactPrefix+"-cancelled@example.test", newTestTrackingTokenHash(t)).Scan(&value)
	}, "PZ001")

	// A pending request does not prevent an independent confirmed booking from
	// consuming the last spot; pending requests are excluded from capacity.
	_ = request(emptySession, "while-pending")
	var independentPlayer, directBooking string
	if err := db.QueryRow(ctx, `INSERT INTO public.players(full_name,email) VALUES ('Independent', $1) RETURNING id::text`, contactPrefix+"-independent@example.test").Scan(&independentPlayer); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT (public.book_training_session($1::uuid,$2::uuid,NULL)).id::text`, independentPlayer, emptySession).Scan(&directBooking); err != nil {
		t.Fatalf("pending request consumed capacity: %v", err)
	}
	_ = directBooking

	// Keep approval one uncommitted while a second connection approves another
	// request for the same one-seat session; the second transaction must wait.
	concurrentFirst := request(concurrentSession, "concurrent-first")
	concurrentSecond := request(concurrentSession, "concurrent-second")
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var concurrentBooking string
	if err := tx.QueryRow(ctx, `SELECT public.approve_booking_request($1::uuid,$2::uuid,NULL,true)::text`, concurrentFirst, adminID).Scan(&concurrentBooking); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	blockedResult := make(chan error, 1)
	go func() {
		var value string
		blockedResult <- db.QueryRow(ctx, `SELECT public.approve_booking_request($1::uuid,$2::uuid,NULL,true)::text`, concurrentSecond, adminID).Scan(&value)
	}()
	deadline := time.Now().Add(5 * time.Second)
	blocked := false
	for time.Now().Before(deadline) {
		var waiting bool
		err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity a WHERE a.query LIKE '%approve_booking_request%' AND cardinality(pg_blocking_pids(a.pid)) > 0)`).Scan(&waiting)
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(err)
		}
		if waiting {
			blocked = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !blocked {
		_ = tx.Rollback(ctx)
		t.Fatal("second approval did not wait on the locked session")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-blockedResult:
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "PZ002" {
			t.Fatalf("concurrent approval error=%v, want PZ002", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second approval did not finish after first transaction committed")
	}
}

func TestPublicBookingRequestRejectsConfirmedFullSessionButIgnoresPendingRequests(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable migrated PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
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

	var runID, programID, sessionID, playerID string
	if err := db.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&runID); err != nil {
		t.Fatal("generate integration identifier")
	}
	if err := db.QueryRow(ctx, `INSERT INTO public.training_programs(name) VALUES ('capacity guard ' || $1) RETURNING id::text`, runID).Scan(&programID); err != nil {
		t.Fatal("create capacity-guard integration program")
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if sessionID != "" {
			if _, err := db.Exec(cleanupCtx, `DELETE FROM public.booking_requests WHERE session_id = $1::uuid`, sessionID); err != nil {
				t.Errorf("delete capacity-guard requests: %v", err)
			}
			if _, err := db.Exec(cleanupCtx, `DELETE FROM public.bookings WHERE session_id = $1::uuid`, sessionID); err != nil {
				t.Errorf("delete capacity-guard bookings: %v", err)
			}
			if _, err := db.Exec(cleanupCtx, `DELETE FROM public.training_sessions WHERE id = $1::uuid`, sessionID); err != nil {
				t.Errorf("delete capacity-guard session: %v", err)
			}
		}
		if playerID != "" {
			if _, err := db.Exec(cleanupCtx, `DELETE FROM public.players WHERE id = $1::uuid`, playerID); err != nil {
				t.Errorf("delete capacity-guard player: %v", err)
			}
		}
		if programID != "" {
			if _, err := db.Exec(cleanupCtx, `DELETE FROM public.training_programs WHERE id = $1::uuid`, programID); err != nil {
				t.Errorf("delete capacity-guard program: %v", err)
			}
		}
	})

	if err := db.QueryRow(ctx, `INSERT INTO public.training_sessions
		(program_id, starts_at, ends_at, location, capacity, status, is_public)
		VALUES ($1::uuid, pg_catalog.statement_timestamp() + interval '1 day',
		        pg_catalog.statement_timestamp() + interval '1 day 1 hour',
		        'Capacity Guard Integration Court', 1, 'open', true)
		RETURNING id::text`, programID).Scan(&sessionID); err != nil {
		t.Fatal("create capacity-guard integration session")
	}
	contact := "capacity-guard-" + runID
	if err := db.QueryRow(ctx, `SELECT public.create_public_booking_request_with_status_token(
		$1::uuid, 'Pending Integration Request', $2, NULL, $3)`,
		sessionID, contact+"-pending@example.test", newTestTrackingTokenHash(t)).Scan(new(string)); err != nil {
		t.Fatalf("pending request should be accepted before any confirmed booking: %v", err)
	}

	if err := db.QueryRow(ctx, `INSERT INTO public.players(full_name, email)
		VALUES ('Confirmed Integration Player', $1) RETURNING id::text`, contact+"-confirmed@example.test").Scan(&playerID); err != nil {
		t.Fatal("create confirmed integration player")
	}
	if err := db.QueryRow(ctx, `SELECT (public.book_training_session($1::uuid, $2::uuid, NULL)).id::text`,
		playerID, sessionID).Scan(new(string)); err != nil {
		t.Fatalf("confirm booking despite the existing pending request: %v", err)
	}

	var confirmedCount, pendingCount int
	if err := db.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM public.bookings WHERE session_id = $1::uuid AND status = 'confirmed'),
		(SELECT count(*) FROM public.booking_requests WHERE session_id = $1::uuid AND status = 'pending')`,
		sessionID).Scan(&confirmedCount, &pendingCount); err != nil {
		t.Fatal("count confirmed bookings and pending requests")
	}
	if confirmedCount != 1 || pendingCount != 1 {
		t.Fatalf("confirmed bookings=%d pending requests=%d, want one of each", confirmedCount, pendingCount)
	}

	assertSQLState(t, func() error {
		var reference string
		return db.QueryRow(ctx, `SELECT public.create_public_booking_request_with_status_token(
			$1::uuid, 'Rejected Full Session Request', $2, NULL, $3)`,
			sessionID, contact+"-full@example.test", newTestTrackingTokenHash(t)).Scan(&reference)
	}, "PZ002")

	if err := db.QueryRow(ctx, `SELECT count(*) FROM public.booking_requests
		WHERE session_id = $1::uuid AND status = 'pending'`, sessionID).Scan(&pendingCount); err != nil {
		t.Fatal("verify rejected full-session request was not persisted")
	}
	if pendingCount != 1 {
		t.Fatalf("pending requests after full-session rejection=%d, want original request only", pendingCount)
	}
}

func assertSQLState(t *testing.T, run func() error, want string) {
	t.Helper()
	err := run()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != want {
		t.Fatalf("error=%v, want SQLSTATE %s", err, want)
	}
}
