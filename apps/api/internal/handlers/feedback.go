package handlers

import (
	"context"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/adminauth"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/training"
)

const (
	feedbackMessageMaxRunes = 2000
	feedbackNameMaxRunes    = 200
	feedbackEmailMaxBytes   = 254
)

// feedbackCategories is the closed set accepted from the public form.
var feedbackCategories = map[string]struct{}{
	"suggestion": {},
	"feedback":   {},
	"facility":   {},
	"training":   {},
	"scheduling": {},
	"other":      {},
}

type feedbackSubmissionInput struct {
	Category string `json:"category"`
	Message  string `json:"message"`
	Name     string `json:"name"`
	Email    string `json:"email"`
}

type feedbackSubmissionAccepted struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// FeedbackSubmission stores a public suggestion or feedback entry. Submissions
// are never returned to the public; only authorized admins can read them.
func FeedbackSubmission(repo training.BookingRepository, logger interface{ Printf(string, ...any) }) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		var input feedbackSubmissionInput
		if !decodeJSON(w, r, &input, 8192) {
			return
		}
		input.Category = strings.ToLower(strings.TrimSpace(input.Category))
		input.Message = strings.TrimSpace(input.Message)
		input.Name = strings.TrimSpace(input.Name)
		input.Email = strings.ToLower(strings.TrimSpace(input.Email))

		if _, ok := feedbackCategories[input.Category]; !ok ||
			input.Message == "" || !utf8.ValidString(input.Message) ||
			utf8.RuneCountInString(input.Message) > feedbackMessageMaxRunes {
			writeBookingError(w, http.StatusBadRequest, "choose a category and provide a message of 1 to 2000 characters")
			return
		}
		if input.Name != "" && (!utf8.ValidString(input.Name) || utf8.RuneCountInString(input.Name) > feedbackNameMaxRunes) {
			writeBookingError(w, http.StatusBadRequest, "name must be 200 characters or fewer")
			return
		}
		if input.Email != "" {
			parsed, err := mail.ParseAddress(input.Email)
			if err != nil || parsed.Address != input.Email || len(input.Email) > feedbackEmailMaxBytes {
				writeBookingError(w, http.StatusBadRequest, "email is invalid")
				return
			}
		}
		if repo == nil {
			writeBookingError(w, http.StatusServiceUnavailable, "feedback is temporarily unavailable")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if _, err := repo.CreateFeedback(ctx, training.FeedbackSubmission{
			Category: input.Category,
			Message:  input.Message,
			Name:     input.Name,
			Email:    input.Email,
		}); err != nil {
			writeBookingRepoError(w, logger, "submit feedback", err)
			return
		}

		writeJSON(w, http.StatusAccepted, feedbackSubmissionAccepted{
			Status:  "received",
			Message: "Thank you. Your submission has been received by the club.",
		})
	}
}

// AdminFeedbackSubmissions lists submissions for authorized club admins.
func AdminFeedbackSubmissions(repo training.BookingRepository, logger interface{ Printf(string, ...any) }) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if repo == nil {
			writeBookingError(w, http.StatusServiceUnavailable, "feedback submissions are temporarily unavailable")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		records, err := repo.ListFeedbackSubmissions(ctx)
		if err != nil {
			writeBookingRepoError(w, logger, "list feedback submissions", err)
			return
		}
		if records == nil {
			records = []training.FeedbackRecord{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"submissions": records})
	}
}

type feedbackStatusInput struct {
	Status string `json:"status"`
}

// AdminUpdateFeedbackSubmission changes a submission's review status.
func AdminUpdateFeedbackSubmission(repo training.BookingRepository, logger interface{ Printf(string, ...any) }) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if repo == nil {
			writeBookingError(w, http.StatusServiceUnavailable, "feedback submissions are temporarily unavailable")
			return
		}
		id := strings.ToLower(r.PathValue("id"))
		if !validUUID(id) {
			writeBookingError(w, http.StatusBadRequest, "submission id is invalid")
			return
		}
		var input feedbackStatusInput
		if !decodeJSON(w, r, &input, 1024) {
			return
		}
		input.Status = strings.ToLower(strings.TrimSpace(input.Status))
		if input.Status != "new" && input.Status != "reviewed" && input.Status != "archived" {
			writeBookingError(w, http.StatusBadRequest, "status must be new, reviewed, or archived")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		updated, err := repo.UpdateFeedbackStatus(ctx, id, input.Status, adminauth.UserID(r.Context()))
		if err != nil {
			writeBookingRepoError(w, logger, "update feedback status", err)
			return
		}
		if !updated {
			writeBookingError(w, http.StatusNotFound, "submission not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": input.Status})
	}
}
