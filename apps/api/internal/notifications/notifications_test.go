package notifications

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestBuildApprovalEmailUsesManilaTimeAndSafeSupportDetails(t *testing.T) {
	claim := Claim{
		RecipientEmail:   "unverified@example.test",
		ProgramName:      "Adult <Beginner>",
		StartsAt:         time.Date(2026, 10, 12, 1, 0, 0, 0, time.UTC),
		EndsAt:           time.Date(2026, 10, 12, 2, 30, 0, 0, time.UTC),
		Location:         "MMSU Courts",
		SupportReference: "MMSU-ABC123",
	}
	email, err := BuildApprovalEmail("club@example.test", claim)
	if err != nil {
		t.Fatal(err)
	}
	if email.From != "MMSU Tennis Club <club@example.test>" || email.To != "unverified@example.test" {
		t.Fatalf("unexpected email routing: from=%q to=%q", email.From, email.To)
	}
	for _, expected := range []string{"Your booking is approved", "Adult &lt;Beginner&gt;", "Monday, 12 October 2026, 9:00 AM", "10:30 AM", "Asia/Manila", "MMSU-ABC123", "does not replace, personal confirmation"} {
		if !strings.Contains(email.HTML+email.Text, expected) {
			t.Errorf("email does not contain %q", expected)
		}
	}
}

func TestApprovalEmailPreservesEmDashThroughResendPayload(t *testing.T) {
	const title = "MMSU Tennis Club — Local Test 2"
	claim := Claim{
		RecipientEmail:   "requester@example.test",
		ProgramName:      title,
		StartsAt:         time.Date(2026, 10, 14, 9, 36, 0, 0, time.UTC),
		EndsAt:           time.Date(2026, 10, 14, 11, 6, 0, 0, time.UTC),
		Location:         "MMSU Lawn Tennis Courts (Local Test)",
		SupportReference: "MMSU-TEST123",
	}
	email, err := BuildApprovalEmail("club@example.test", claim)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(email.HTML, title) || !strings.Contains(email.Text, title) {
		t.Fatal("generated approval email changed the em dash in the program title")
	}
	if strings.ContainsRune(email.HTML+email.Text, '\uFFFD') {
		t.Fatal("generated approval email contains the Unicode replacement character")
	}

	var rawPayload []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		rawPayload, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read Resend payload: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"email_unicode_test"}`))
	}))
	defer server.Close()
	sender := NewResendSender("test-key", "club@example.test")
	sender.endpointForTest(server.URL, server.Client())
	if _, err := sender.Send(context.Background(), email, "booking-approval/unicode-test"); err != nil {
		t.Fatalf("send generated email: %v", err)
	}
	if !utf8.Valid(rawPayload) || strings.Contains(string(rawPayload), "\uFFFD") {
		t.Fatal("serialized Resend payload is invalid UTF-8 or contains a replacement character")
	}
	var payload struct {
		HTML string `json:"html"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		t.Fatalf("decode serialized Resend payload: %v", err)
	}
	if !strings.Contains(payload.HTML, title) || !strings.Contains(payload.Text, title) || strings.ContainsRune(payload.HTML+payload.Text, '\uFFFD') {
		t.Fatal("serialized Resend payload changed the program title")
	}
}

func TestResendSenderUsesStableIdempotencyKeyAndParsesAcceptedID(t *testing.T) {
	var gotKey, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Idempotency-Key")
		gotAuth = r.Header.Get("Authorization")
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload["to"] == nil || payload["from"] == nil {
			t.Error("missing email payload fields")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"email_test_123"}`))
	}))
	defer server.Close()
	sender := NewResendSender("test-key", "club@example.test")
	sender.endpointForTest(server.URL, server.Client())
	id, err := sender.Send(context.Background(), Email{From: "Club", To: "person@example.test", Subject: "Approved", HTML: "<p>Approved</p>", Text: "Approved"}, "booking-approval/request-123")
	if err != nil {
		t.Fatal("send failed")
	}
	if id != "email_test_123" || gotKey != "booking-approval/request-123" || gotAuth != "Bearer test-key" {
		t.Fatalf("id=%q key=%q auth present=%t", id, gotKey, gotAuth != "")
	}
}

func TestResendSenderRejectsProviderFailureWithoutLeakingResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "secret provider details", http.StatusBadRequest)
	}))
	defer server.Close()
	sender := NewResendSender("test-key", "club@example.test")
	sender.endpointForTest(server.URL, server.Client())
	_, err := sender.Send(context.Background(), Email{}, "stable-key")
	if err == nil || strings.Contains(err.Error(), "secret provider details") || strings.Contains(err.Error(), "test-key") {
		t.Fatalf("unexpected provider error content: %v", err)
	}
}

type notificationRepoStub struct {
	claim                 Claim
	claimErr              error
	complete              bool
	completeErr           error
	claimedWithConfigured bool
	completeSucceeded     bool
	completeCalls         int
}

func (r *notificationRepoStub) ClaimApprovalNotification(_ context.Context, _ string, configured bool) (Claim, error) {
	r.claimedWithConfigured = configured
	return r.claim, r.claimErr
}
func (r *notificationRepoStub) CompleteApprovalNotification(_ context.Context, _ string, _ int, succeeded bool, _ string) (bool, error) {
	r.completeCalls++
	r.completeSucceeded = succeeded
	return r.complete, r.completeErr
}

type notificationSenderStub struct {
	id    string
	err   error
	calls int
}

func (s *notificationSenderStub) Send(context.Context, Email, string) (string, error) {
	s.calls++
	return s.id, s.err
}

func TestMissingResendConfigurationAndProviderFailureAreNonFatalAndRetryable(t *testing.T) {
	repo := &notificationRepoStub{claim: Claim{Status: StatusNotConfigured, Detail: "resend_not_configured"}}
	sender := &notificationSenderStub{}
	service := NewService(repo, sender, "", nil)
	result := service.NotifyApproved(context.Background(), "request-id")
	if result.Status != StatusNotConfigured || !result.Retryable || sender.calls != 0 || repo.claimedWithConfigured {
		t.Fatalf("missing configuration result=%#v", result)
	}

	repo = &notificationRepoStub{claim: Claim{Status: StatusSending, Claimed: true, AttemptCount: 1, RecipientEmail: "person@example.test", ProgramName: "Program", StartsAt: time.Now(), EndsAt: time.Now().Add(time.Hour), Location: "Court", SupportReference: "MMSU-REF"}, complete: true}
	sender = &notificationSenderStub{err: context.DeadlineExceeded}
	result = NewService(repo, sender, "club@example.test", nil).NotifyApproved(context.Background(), "request-id")
	if result.Status != StatusFailed || !result.Retryable || !strings.Contains(result.Message, "contact the requester manually") || repo.completeSucceeded || repo.completeCalls != 1 {
		t.Fatalf("provider failure result=%#v", result)
	}
}

func TestSuccessfulProviderAcceptanceIsNotClaimedAsDelivered(t *testing.T) {
	repo := &notificationRepoStub{claim: Claim{Status: StatusSending, Claimed: true, AttemptCount: 1, RecipientEmail: "person@example.test", ProgramName: "Program", StartsAt: time.Now(), EndsAt: time.Now().Add(time.Hour), Location: "Court", SupportReference: "MMSU-REF"}, complete: true}
	sender := &notificationSenderStub{id: "resend-id"}
	result := NewService(repo, sender, "club@example.test", nil).NotifyApproved(context.Background(), "request-id")
	if result.Status != StatusSent || result.Retryable || !strings.Contains(result.Message, "Delivery is not confirmed") || !repo.completeSucceeded {
		t.Fatalf("result=%#v", result)
	}
}
