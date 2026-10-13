package postgres

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/notifications"
)

func TestBookingApprovalNotificationPostgresWorkflowAndRetryDeduplication(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to the disposable local PostgreSQL database")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect test database (error type %T)", err)
	}
	defer db.Close()
	var isDisposableTarget bool
	if err := db.QueryRow(ctx, `SELECT current_database() = 'mmsu_booking_test'
		AND inet_server_addr() = '127.0.0.1'::inet
		AND inet_server_port() = 55432`).Scan(&isDisposableTarget); err != nil {
		t.Fatalf("verify test database (error type %T)", err)
	}
	if !isDisposableTarget {
		t.Fatal("refusing integration test writes outside 127.0.0.1:55432/mmsu_booking_test")
	}

	var runID, adminID string
	if err := db.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&runID); err != nil {
		t.Fatal("generate integration identifiers")
	}
	adminID = runID
	contactPrefix := "approval-notification-" + runID
	var publicCanClaim, publicCanComplete, adminCanClaim, adminCanComplete bool
	if err := db.QueryRow(ctx, `SELECT
		has_function_privilege('mmsu_booking_api','public.claim_booking_approval_notification(uuid,boolean)','EXECUTE'),
		has_function_privilege('mmsu_booking_api','public.complete_booking_approval_notification(uuid,integer,boolean,text)','EXECUTE'),
		has_function_privilege('mmsu_booking_admin_api','public.claim_booking_approval_notification(uuid,boolean)','EXECUTE'),
		has_function_privilege('mmsu_booking_admin_api','public.complete_booking_approval_notification(uuid,integer,boolean,text)','EXECUTE')`).Scan(&publicCanClaim, &publicCanComplete, &adminCanClaim, &adminCanComplete); err != nil {
		t.Fatal("inspect notification function grants")
	}
	if publicCanClaim || publicCanComplete || !adminCanClaim || !adminCanComplete {
		t.Fatalf("notification grants: public=(%t,%t) admin=(%t,%t)", publicCanClaim, publicCanComplete, adminCanClaim, adminCanComplete)
	}

	var programID string
	if err := db.QueryRow(ctx, `INSERT INTO public.training_programs(name) VALUES ('notification integration ' || $1) RETURNING id::text`, runID).Scan(&programID); err != nil {
		t.Fatal("create integration program")
	}
	createSession := func(capacity int, status string) string {
		t.Helper()
		var id string
		err := db.QueryRow(ctx, `INSERT INTO public.training_sessions(program_id, starts_at, ends_at, location, capacity, status, is_public)
			VALUES ($1::uuid, now() + interval '3 days', now() + interval '3 days 90 minutes', 'Notification test court', $2, $3, true)
			RETURNING id::text`, programID, capacity, status).Scan(&id)
		if err != nil {
			t.Fatal("create integration session")
		}
		return id
	}
	approvedSession := createSession(3, "open")
	rejectedSession := createSession(2, "open")
	fullSession := createSession(1, "open")
	staleSession := createSession(2, "open")

	var approvedRequest, rejectedRequest, fullFirst, fullSecond, staleRequest string
	createRequest := func(sessionID, suffix string, withEmail bool) string {
		t.Helper()
		var id string
		var reference string
		var err error
		if withEmail {
			err = db.QueryRow(ctx, `SELECT public.create_public_booking_request_with_status_token($1::uuid, 'Notification Player', $2, NULL, $3)`,
				sessionID, contactPrefix+"-"+suffix+"@example.test", newTestTrackingTokenHash(t)).Scan(&reference)
		} else {
			err = db.QueryRow(ctx, `SELECT public.create_public_booking_request_with_status_token($1::uuid, 'Notification Player', NULL, $2, $3)`,
				sessionID, "+1555000"+suffix, newTestTrackingTokenHash(t)).Scan(&reference)
		}
		if err != nil {
			t.Fatal("create integration request")
		}
		if err := db.QueryRow(ctx, `SELECT id::text FROM public.booking_requests WHERE public_reference = $1`, reference).Scan(&id); err != nil {
			t.Fatal("look up integration request")
		}
		return id
	}
	approvedRequest = createRequest(approvedSession, "approved", true)
	rejectedRequest = createRequest(rejectedSession, "rejected", true)
	fullFirst = createRequest(fullSession, "full-first", true)
	fullSecond = createRequest(fullSession, "full-second", true)
	staleRequest = createRequest(staleSession, "stale", true)
	t.Cleanup(func() {
		requestIDs := []string{approvedRequest, rejectedRequest, fullFirst, fullSecond, staleRequest}
		sessionIDs := []string{approvedSession, rejectedSession, fullSession, staleSession}
		_, _ = db.Exec(context.Background(), `DELETE FROM public.booking_approval_notifications WHERE request_id = ANY($1::uuid[])`, requestIDs)
		_, _ = db.Exec(context.Background(), `DELETE FROM public.booking_requests WHERE id = ANY($1::uuid[])`, requestIDs)
		_, _ = db.Exec(context.Background(), `DELETE FROM public.bookings WHERE session_id = ANY($1::uuid[])`, sessionIDs)
		_, _ = db.Exec(context.Background(), `DELETE FROM public.players WHERE email LIKE $1`, contactPrefix+"-%")
		_, _ = db.Exec(context.Background(), `DELETE FROM public.training_sessions WHERE id = ANY($1::uuid[])`, sessionIDs)
		_, _ = db.Exec(context.Background(), `DELETE FROM public.training_programs WHERE id = $1::uuid`, programID)
	})

	var bookingID string
	if err := db.QueryRow(ctx, `SELECT public.approve_booking_request($1::uuid,$2::uuid,NULL,true)::text`, approvedRequest, adminID).Scan(&bookingID); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	var notificationStatus, programName, reference string
	var attempts int
	if err := db.QueryRow(ctx, `SELECT status, program_name, support_reference, attempt_count FROM public.booking_approval_notifications WHERE request_id=$1::uuid`, approvedRequest).Scan(&notificationStatus, &programName, &reference, &attempts); err != nil {
		t.Fatal("read approval snapshot")
	}
	if notificationStatus != "pending" || attempts != 0 || programName == "" || reference == "" || bookingID == "" {
		t.Fatalf("snapshot status=%q attempts=%d program=%q ref=%q booking=%q", notificationStatus, attempts, programName, reference, bookingID)
	}

	var claim struct {
		status, detail, recipient, name, location, supportReference string
		startsAt, endsAt                                            time.Time
		attempt                                                     int
		claimed                                                     bool
	}
	claimNotification := func(configured bool) {
		t.Helper()
		err := db.QueryRow(ctx, `SELECT notification_status, COALESCE(notification_detail,''), COALESCE(recipient_email,''), program_name, starts_at, ends_at, location, support_reference, attempt_count, claimed
			FROM public.claim_booking_approval_notification($1::uuid,$2)`, approvedRequest, configured).Scan(
			&claim.status, &claim.detail, &claim.recipient, &claim.name, &claim.startsAt, &claim.endsAt,
			&claim.location, &claim.supportReference, &claim.attempt, &claim.claimed,
		)
		if err != nil {
			t.Fatal("claim approval notification")
		}
	}
	claimNotification(false)
	if claim.status != "not_configured" || claim.detail != "resend_not_configured" || claim.claimed || claim.attempt != 0 {
		t.Fatalf("missing configuration claim=%#v", claim)
	}
	claimNotification(true)
	if claim.status != "sending" || !claim.claimed || claim.attempt != 1 || claim.recipient != contactPrefix+"-approved@example.test" || claim.startsAt.IsZero() || claim.endsAt.IsZero() {
		t.Fatalf("configured claim=%#v", claim)
	}
	claimNotification(true)
	if claim.status != "sending" || claim.claimed || claim.attempt != 1 {
		t.Fatalf("concurrent claim was not deduplicated: %#v", claim)
	}
	var updated bool
	if err := db.QueryRow(ctx, `SELECT public.complete_booking_approval_notification($1::uuid,1,false,NULL)`, approvedRequest).Scan(&updated); err != nil || !updated {
		t.Fatalf("record failed provider attempt updated=%t err=%v", updated, err)
	}
	claimNotification(true)
	if claim.status != "sending" || !claim.claimed || claim.attempt != 2 {
		t.Fatalf("retry claim=%#v", claim)
	}
	if err := db.QueryRow(ctx, `SELECT public.complete_booking_approval_notification($1::uuid,2,true,'resend-test-id')`, approvedRequest).Scan(&updated); err != nil || !updated {
		t.Fatalf("record accepted email updated=%t err=%v", updated, err)
	}
	claimNotification(true)
	if claim.status != "sent" || claim.claimed || claim.attempt != 2 {
		t.Fatalf("already-sent notification was claimable again: %#v", claim)
	}

	if _, err := db.Exec(ctx, `SELECT public.reject_booking_request($1::uuid,$2::uuid)`, rejectedRequest, adminID); err != nil {
		t.Fatal("reject request")
	}
	assertNoApprovalNotification(t, ctx, db, rejectedRequest)
	if err := db.QueryRow(ctx, `SELECT public.approve_booking_request($1::uuid,$2::uuid,NULL,true)::text`, fullFirst, adminID).Scan(&bookingID); err != nil {
		t.Fatal("approve first full-session request")
	}
	assertNotificationSQLState(t, func() error {
		return db.QueryRow(ctx, `SELECT public.approve_booking_request($1::uuid,$2::uuid,NULL,true)::text`, fullSecond, adminID).Scan(&bookingID)
	}, "PZ002")
	assertNoApprovalNotification(t, ctx, db, fullSecond)
	if _, err := db.Exec(ctx, `UPDATE public.training_sessions SET status='cancelled' WHERE id=$1::uuid`, staleSession); err != nil {
		t.Fatal("make session stale")
	}
	assertNotificationSQLState(t, func() error {
		return db.QueryRow(ctx, `SELECT public.approve_booking_request($1::uuid,$2::uuid,NULL,true)::text`, staleRequest, adminID).Scan(&bookingID)
	}, "PZ001")
	assertNoApprovalNotification(t, ctx, db, staleRequest)
}

func assertNoApprovalNotification(t *testing.T, ctx context.Context, db *pgxpool.Pool, requestID string) {
	t.Helper()
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM public.booking_approval_notifications WHERE request_id=$1::uuid`, requestID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("request %s notification count=%d err=%v", requestID, count, err)
	}
}

func assertNotificationSQLState(t *testing.T, run func() error, want string) {
	t.Helper()
	err := run()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != want {
		t.Fatalf("error=%v, want SQLSTATE %s", err, want)
	}
}

func TestBookingApprovalNotificationConcurrentClaim(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to the disposable local PostgreSQL database")
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
		t.Fatal("generate integration identifiers")
	}
	adminID := runID
	contactPrefix := "approval-concurrent-" + runID
	recipientEmail := contactPrefix + "@example.test"

	var programID string
	if err := db.QueryRow(ctx, `INSERT INTO public.training_programs(name) VALUES ('notification concurrent ' || $1) RETURNING id::text`, runID).Scan(&programID); err != nil {
		t.Fatal("create integration program")
	}
	var sessionIDs []string
	var references []string
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if len(references) > 0 {
			if _, err := db.Exec(cleanupCtx, `DELETE FROM public.booking_approval_notifications AS n
				USING public.booking_requests AS br
				WHERE n.request_id = br.id AND br.public_reference = ANY($1::text[])`, references); err != nil {
				t.Errorf("delete integration notifications: %v", err)
			}
			if _, err := db.Exec(cleanupCtx, `DELETE FROM public.booking_requests WHERE public_reference = ANY($1::text[])`, references); err != nil {
				t.Errorf("delete integration booking requests: %v", err)
			}
		}
		if len(sessionIDs) > 0 {
			if _, err := db.Exec(cleanupCtx, `DELETE FROM public.bookings WHERE session_id = ANY($1::uuid[])`, sessionIDs); err != nil {
				t.Errorf("delete integration bookings: %v", err)
			}
		}
		if _, err := db.Exec(cleanupCtx, `DELETE FROM public.players WHERE email LIKE $1`, contactPrefix+"%"); err != nil {
			t.Errorf("delete integration players: %v", err)
		}
		if len(sessionIDs) > 0 {
			if _, err := db.Exec(cleanupCtx, `DELETE FROM public.training_sessions WHERE id = ANY($1::uuid[])`, sessionIDs); err != nil {
				t.Errorf("delete integration sessions: %v", err)
			}
		}
		if _, err := db.Exec(cleanupCtx, `DELETE FROM public.training_programs WHERE id = $1::uuid`, programID); err != nil {
			t.Errorf("delete integration program: %v", err)
		}
	})

	var sessionID string
	err = db.QueryRow(ctx, `INSERT INTO public.training_sessions(program_id, starts_at, ends_at, location, capacity, status, is_public)
		VALUES ($1::uuid, now() + interval '3 days', now() + interval '3 days 90 minutes', 'Notification concurrent court', 2, 'open', true)
		RETURNING id::text`, programID).Scan(&sessionID)
	if err != nil {
		t.Fatal("create integration session")
	}
	sessionIDs = append(sessionIDs, sessionID)

	var reference, requestID string
	err = db.QueryRow(ctx, `SELECT public.create_public_booking_request_with_status_token($1::uuid, 'Concurrent Requester', $2, NULL, $3)`,
		sessionID, recipientEmail, newTestTrackingTokenHash(t)).Scan(&reference)
	if err != nil {
		t.Fatal("create integration request")
	}
	references = append(references, reference)
	if err := db.QueryRow(ctx, `SELECT id::text FROM public.booking_requests WHERE public_reference = $1`, reference).Scan(&requestID); err != nil {
		t.Fatal("look up integration request")
	}

	var bookingID string
	if err := db.QueryRow(ctx, `SELECT public.approve_booking_request($1::uuid, $2::uuid, NULL, true)::text`, requestID, adminID).Scan(&bookingID); err != nil {
		t.Fatalf("approve request: %v", err)
	}

	var initialStatus string
	var initialAttempts int
	if err := db.QueryRow(ctx, `SELECT status, attempt_count FROM public.booking_approval_notifications WHERE request_id = $1::uuid`, requestID).Scan(&initialStatus, &initialAttempts); err != nil {
		t.Fatalf("read initial notification: %v", err)
	}
	if initialStatus != "pending" || initialAttempts != 0 {
		t.Fatalf("initial snapshot status=%q attempts=%d, want status=pending attempts=0", initialStatus, initialAttempts)
	}

	conn1, err := db.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire connection 1: %v", err)
	}
	defer conn1.Release()

	conn2, err := db.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire connection 2: %v", err)
	}
	defer conn2.Release()

	lockConn, err := db.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire notification lock connection: %v", err)
	}
	defer lockConn.Release()

	pid1 := conn1.Conn().PgConn().PID()
	pid2 := conn2.Conn().PgConn().PID()
	lockPID := lockConn.Conn().PgConn().PID()
	if pid1 == 0 || pid2 == 0 || lockPID == 0 || pid1 == pid2 || pid1 == lockPID || pid2 == lockPID {
		t.Fatalf("claims and lock holder must use distinct database backend connections; got pid1=%d, pid2=%d, lockPID=%d", pid1, pid2, lockPID)
	}

	repo1 := NewBookingRepository(conn1)
	repo2 := NewBookingRepository(conn2)

	type claimResult struct {
		claim notifications.Claim
		err   error
	}

	runConcurrentClaims := func(expectedAttempt int) (winner notifications.Claim, loser notifications.Claim) {
		lockTx, err := lockConn.Begin(ctx)
		if err != nil {
			t.Fatalf("begin notification lock transaction: %v", err)
		}
		defer func() {
			_ = lockTx.Rollback(context.Background())
		}()
		var lockedRequestID string
		if err := lockTx.QueryRow(ctx, `SELECT request_id::text
			FROM public.booking_approval_notifications
			WHERE request_id = $1::uuid
			FOR UPDATE`, requestID).Scan(&lockedRequestID); err != nil {
			t.Fatalf("lock notification row: %v", err)
		}
		if lockedRequestID != requestID {
			t.Fatalf("locked request ID=%q, want %q", lockedRequestID, requestID)
		}

		results := make(chan claimResult, 2)
		var ready sync.WaitGroup
		ready.Add(2)
		start := make(chan struct{})

		runAttempt := func(repo *BookingRepository) {
			ready.Done()
			<-start
			claim, claimErr := repo.ClaimApprovalNotification(ctx, requestID, true)
			results <- claimResult{claim: claim, err: claimErr}
		}

		go runAttempt(repo1)
		go runAttempt(repo2)

		ready.Wait()
		close(start)

		waitDeadline := time.Now().Add(5 * time.Second)
		for {
			var waitingClaims, notificationTableLocks int
			err := db.QueryRow(ctx, `
				WITH RECURSIVE blocker_chain(claim_pid, current_pid, depth) AS (
					SELECT claim.pid, unnest(pg_catalog.pg_blocking_pids(claim.pid)), 1
					FROM unnest($1::int[]) AS claim(pid)
					JOIN pg_catalog.pg_stat_activity AS activity ON activity.pid = claim.pid
					WHERE activity.state = 'active'
					  AND activity.wait_event_type = 'Lock'
					  AND activity.query LIKE '%claim_booking_approval_notification%'
					UNION
					SELECT c.claim_pid, unnest(pg_catalog.pg_blocking_pids(c.current_pid)), c.depth + 1
					FROM blocker_chain c
					WHERE c.depth < 10
				)
				SELECT
					count(DISTINCT b.claim_pid),
					(SELECT count(DISTINCT l.pid)
					 FROM pg_catalog.pg_locks l
					 WHERE l.pid = ANY($1::int[])
					   AND l.relation = 'public.booking_approval_notifications'::regclass
					   AND l.granted = true)
				FROM blocker_chain b
				WHERE b.current_pid = $2::int`,
				[]int32{int32(pid1), int32(pid2)}, int32(lockPID)).Scan(&waitingClaims, &notificationTableLocks)
			if err != nil {
				t.Fatalf("inspect concurrent claim waits: %v", err)
			}

			if waitingClaims == 2 && notificationTableLocks == 2 {
				break
			}
			if time.Now().After(waitDeadline) {
				t.Fatalf("timed out waiting for both claims to block on the notification row held by PID %d; blocked claims=%d, table locks=%d",
					lockPID, waitingClaims, notificationTableLocks)
			}
			time.Sleep(10 * time.Millisecond)
		}
		if err := lockTx.Commit(ctx); err != nil {
			t.Fatalf("release notification row lock: %v", err)
		}

		var res1, res2 claimResult
		select {
		case res1 = <-results:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for first claim attempt")
		}
		select {
		case res2 = <-results:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for second claim attempt")
		}

		if res1.err != nil {
			t.Fatalf("claim attempt 1 error: %v", res1.err)
		}
		if res2.err != nil {
			t.Fatalf("claim attempt 2 error: %v", res2.err)
		}

		switch {
		case res1.claim.Claimed && !res2.claim.Claimed:
			winner, loser = res1.claim, res2.claim
		case !res1.claim.Claimed && res2.claim.Claimed:
			winner, loser = res2.claim, res1.claim
		default:
			t.Fatalf("expected exactly one claim to succeed at attempt %d, got res1.Claimed=%t res2.Claimed=%t (res1=%#v res2=%#v)",
				expectedAttempt, res1.claim.Claimed, res2.claim.Claimed, res1.claim, res2.claim)
		}

		if winner.Status != notifications.StatusSending || winner.AttemptCount != expectedAttempt || winner.RecipientEmail != recipientEmail {
			t.Fatalf("winning claim mismatch at attempt %d: %#v", expectedAttempt, winner)
		}
		if loser.Status != notifications.StatusSending || loser.AttemptCount != expectedAttempt {
			t.Fatalf("losing claim mismatch at attempt %d: %#v", expectedAttempt, loser)
		}

		var dbStatus string
		var dbAttempts int
		var sendStartedAt *time.Time
		if err := db.QueryRow(ctx, `SELECT status, attempt_count, send_started_at FROM public.booking_approval_notifications WHERE request_id = $1::uuid`, requestID).Scan(&dbStatus, &dbAttempts, &sendStartedAt); err != nil {
			t.Fatalf("query persisted notification status: %v", err)
		}
		if dbStatus != "sending" || dbAttempts != expectedAttempt || sendStartedAt == nil {
			t.Fatalf("persisted notification state at attempt %d: status=%q attempts=%d send_started_at=%v",
				expectedAttempt, dbStatus, dbAttempts, sendStartedAt)
		}

		return winner, loser
	}

	// Verify initial concurrent claim attempt deduplication.
	runConcurrentClaims(1)

	// Complete attempt 1 as failed to make notification eligible for retry.
	updated, err := repo1.CompleteApprovalNotification(ctx, requestID, 1, false, "")
	if err != nil || !updated {
		t.Fatalf("record failed provider attempt updated=%t err=%v", updated, err)
	}

	var failedStatus string
	var failedAttempts int
	var failedSendStartedAt *time.Time
	if err := db.QueryRow(ctx, `SELECT status, attempt_count, send_started_at
		FROM public.booking_approval_notifications
		WHERE request_id = $1::uuid`, requestID).Scan(&failedStatus, &failedAttempts, &failedSendStartedAt); err != nil {
		t.Fatalf("read failed notification state: %v", err)
	}
	if failedStatus != "failed" || failedAttempts != 1 || failedSendStartedAt != nil {
		t.Fatalf("failed notification state: status=%q attempts=%d send_started_at=%v, want failed/1/nil",
			failedStatus, failedAttempts, failedSendStartedAt)
	}

	// Verify retry concurrent claim attempt deduplication.
	runConcurrentClaims(2)
}
