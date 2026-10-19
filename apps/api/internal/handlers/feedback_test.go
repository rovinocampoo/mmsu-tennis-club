package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/training"
)

func discardLogger() *log.Logger { return log.New(io.Discard, "", 0) }

func TestFeedbackSubmissionStoresValidEntry(t *testing.T) {
	repo := &bookingStub{}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/feedback-submissions", strings.NewReader(`{"category":"Suggestion","message":"  More weekend sessions please  ","name":"  Ana  ","email":"ANA@Example.com"}`))
	FeedbackSubmission(repo, discardLogger())(w, r)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	if repo.feedbackCalls != 1 {
		t.Fatalf("create calls=%d", repo.feedbackCalls)
	}
	if repo.feedback.Category != "suggestion" || repo.feedback.Message != "More weekend sessions please" ||
		repo.feedback.Name != "Ana" || repo.feedback.Email != "ana@example.com" {
		t.Fatalf("stored feedback = %+v", repo.feedback)
	}
}

func TestFeedbackSubmissionAllowsAnonymousEntry(t *testing.T) {
	repo := &bookingStub{}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/feedback-submissions", strings.NewReader(`{"category":"other","message":"Court lights were great tonight."}`))
	FeedbackSubmission(repo, discardLogger())(w, r)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	if repo.feedback.Name != "" || repo.feedback.Email != "" {
		t.Fatalf("expected empty contact fields, got %+v", repo.feedback)
	}
}

func TestFeedbackSubmissionRejectsInvalidInput(t *testing.T) {
	longMessage, _ := json.Marshal(strings.Repeat("a", feedbackMessageMaxRunes+1))
	cases := map[string]string{
		"unknown category": `{"category":"billing","message":"hi"}`,
		"missing category": `{"message":"hi"}`,
		"empty message":    `{"category":"feedback","message":"   "}`,
		"invalid email":    `{"category":"feedback","message":"hi","email":"not-an-email"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &bookingStub{}
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/feedback-submissions", strings.NewReader(body))
			FeedbackSubmission(repo, discardLogger())(w, r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
			if repo.feedbackCalls != 0 {
				t.Fatalf("invalid submission reached repository")
			}
		})
	}

	t.Run("over-long message", func(t *testing.T) {
		repo := &bookingStub{}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/feedback-submissions", strings.NewReader(`{"category":"feedback","message":`+string(longMessage)+`}`))
		FeedbackSubmission(repo, discardLogger())(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", w.Code, w.Body)
		}
	})
}

func TestFeedbackSubmissionUnavailableWithoutRepository(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/feedback-submissions", strings.NewReader(`{"category":"feedback","message":"hello"}`))
	FeedbackSubmission(nil, discardLogger())(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
}

func TestAdminFeedbackSubmissionsListsRecords(t *testing.T) {
	repo := &bookingStub{feedbackRecords: []training.FeedbackRecord{{ID: "11111111-1111-4111-8111-111111111111", Category: "facility", Message: "Fix the net", Status: "new"}}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/admin/feedback-submissions", nil)
	AdminFeedbackSubmissions(repo, discardLogger())(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	var body struct {
		Submissions []training.FeedbackRecord `json:"submissions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Submissions) != 1 || body.Submissions[0].Category != "facility" {
		t.Fatalf("body = %s", w.Body)
	}
}

func TestAdminUpdateFeedbackSubmissionValidatesAndRecordsAdmin(t *testing.T) {
	const adminID = "22222222-2222-4222-8222-222222222222"
	const id = "11111111-1111-4111-8111-111111111111"

	t.Run("valid status records admin identity", func(t *testing.T) {
		repo := &bookingStub{feedbackUpdated: true}
		w := httptest.NewRecorder()
		r := withUserContext(httptest.NewRequest(http.MethodPatch, "/api/admin/feedback-submissions/"+id, strings.NewReader(`{"status":"reviewed"}`)), adminID)
		r.SetPathValue("id", id)
		AdminUpdateFeedbackSubmission(repo, discardLogger())(w, r)

		if w.Code != http.StatusOK || repo.feedbackStatus != "reviewed" || repo.feedbackAdminID != adminID || repo.feedbackUpdateID != id {
			t.Fatalf("status=%d update=%q admin=%q id=%q body=%s", w.Code, repo.feedbackStatus, repo.feedbackAdminID, repo.feedbackUpdateID, w.Body)
		}
	})

	t.Run("invalid status is rejected", func(t *testing.T) {
		repo := &bookingStub{feedbackUpdated: true}
		w := httptest.NewRecorder()
		r := withUserContext(httptest.NewRequest(http.MethodPatch, "/api/admin/feedback-submissions/"+id, strings.NewReader(`{"status":"done"}`)), adminID)
		r.SetPathValue("id", id)
		AdminUpdateFeedbackSubmission(repo, discardLogger())(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", w.Code, w.Body)
		}
	})

	t.Run("invalid id is rejected", func(t *testing.T) {
		repo := &bookingStub{feedbackUpdated: true}
		w := httptest.NewRecorder()
		r := withUserContext(httptest.NewRequest(http.MethodPatch, "/api/admin/feedback-submissions/not-a-uuid", strings.NewReader(`{"status":"reviewed"}`)), adminID)
		r.SetPathValue("id", "not-a-uuid")
		AdminUpdateFeedbackSubmission(repo, discardLogger())(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", w.Code, w.Body)
		}
	})

	t.Run("missing record returns not found", func(t *testing.T) {
		repo := &bookingStub{feedbackUpdated: false}
		w := httptest.NewRecorder()
		r := withUserContext(httptest.NewRequest(http.MethodPatch, "/api/admin/feedback-submissions/"+id, strings.NewReader(`{"status":"archived"}`)), adminID)
		r.SetPathValue("id", id)
		AdminUpdateFeedbackSubmission(repo, discardLogger())(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status=%d body=%s", w.Code, w.Body)
		}
	})
}
