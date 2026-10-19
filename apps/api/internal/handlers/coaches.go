package handlers

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/training"
)

// CoachListSchedule returns the whole-club read-only schedule, including the
// assigned coach and participant roster, for the shared coach portal.
func CoachListSchedule(repo training.BookingRepository, logger *log.Logger) http.HandlerFunc {
	if logger == nil {
		logger = log.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if repo == nil {
			writeError(w, http.StatusServiceUnavailable, "coach schedule is temporarily unavailable")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		sessions, err := repo.ListCoachSchedule(ctx)
		if err != nil {
			logger.Printf("list coach schedule failed: %v", err)
			writeError(w, http.StatusInternalServerError, "could not load the training schedule")
			return
		}
		if sessions == nil {
			sessions = []training.CoachScheduleSession{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
	}
}

// CoachListCoaches returns the active coach directory for the coach portal.
func CoachListCoaches(repo training.BookingRepository, logger *log.Logger) http.HandlerFunc {
	if logger == nil {
		logger = log.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if repo == nil {
			writeError(w, http.StatusServiceUnavailable, "coach directory is temporarily unavailable")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		coaches, err := repo.ListActiveCoaches(ctx)
		if err != nil {
			logger.Printf("list coach directory failed: %v", err)
			writeError(w, http.StatusInternalServerError, "could not load the coach directory")
			return
		}
		if coaches == nil {
			coaches = []training.CoachDirectoryEntry{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"coaches": coaches})
	}
}

func AdminListCoaches(repo training.BookingRepository, logger *log.Logger) http.HandlerFunc {
	if logger == nil {
		logger = log.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if repo == nil {
			writeError(w, http.StatusServiceUnavailable, "coach directory is temporarily unavailable")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		coaches, err := repo.ListCoaches(ctx)
		if err != nil {
			logger.Printf("admin list coaches failed: %v", err)
			writeError(w, http.StatusInternalServerError, "could not load coaches")
			return
		}
		if coaches == nil {
			coaches = []training.Coach{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"coaches": coaches})
	}
}

type coachInput struct {
	FullName string `json:"full_name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Notes    string `json:"notes"`
}

func (in *coachInput) normalize() {
	in.FullName = strings.TrimSpace(in.FullName)
	in.Email = strings.TrimSpace(in.Email)
	in.Phone = strings.TrimSpace(in.Phone)
	in.Notes = strings.TrimSpace(in.Notes)
}

func (in coachInput) validate() string {
	if utf8.RuneCountInString(in.FullName) < 1 || utf8.RuneCountInString(in.FullName) > 120 {
		return "coach name must be 1 to 120 characters"
	}
	if len(in.Email) > 254 {
		return "email is too long"
	}
	if len(in.Phone) > 40 {
		return "phone is too long"
	}
	if len(in.Notes) > 2000 {
		return "notes are too long"
	}
	return ""
}

func AdminCreateCoach(repo training.BookingRepository, logger *log.Logger) http.HandlerFunc {
	if logger == nil {
		logger = log.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if repo == nil {
			writeError(w, http.StatusServiceUnavailable, "coach directory is temporarily unavailable")
			return
		}
		var input coachInput
		if !decodeJSON(w, r, &input, 8192) {
			return
		}
		input.normalize()
		if message := input.validate(); message != "" {
			writeError(w, http.StatusBadRequest, message)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		id, err := repo.CreateCoach(ctx, input.FullName, input.Email, input.Phone, input.Notes)
		if err != nil {
			writeCoachRepositoryError(w, logger, "create coach", err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": id})
	}
}

func AdminUpdateCoach(repo training.BookingRepository, logger *log.Logger) http.HandlerFunc {
	if logger == nil {
		logger = log.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		coachID := strings.ToLower(strings.TrimSpace(r.PathValue("id")))
		if !validUUID(coachID) {
			writeError(w, http.StatusBadRequest, "coach id is invalid")
			return
		}
		if repo == nil {
			writeError(w, http.StatusServiceUnavailable, "coach directory is temporarily unavailable")
			return
		}
		var input struct {
			coachInput
			IsActive *bool `json:"is_active"`
		}
		if !decodeJSON(w, r, &input, 8192) {
			return
		}
		input.normalize()
		if message := input.validate(); message != "" {
			writeError(w, http.StatusBadRequest, message)
			return
		}
		if input.IsActive == nil {
			writeError(w, http.StatusBadRequest, "is_active is required")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := repo.UpdateCoach(ctx, coachID, input.FullName, input.Email, input.Phone, input.Notes, *input.IsActive); err != nil {
			writeCoachRepositoryError(w, logger, "update coach", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	}
}

func writeCoachRepositoryError(w http.ResponseWriter, logger *log.Logger, operation string, err error) {
	if logger != nil {
		logger.Printf("%s failed (error type %T)", operation, err)
	}
	writeError(w, http.StatusInternalServerError, "could not save the coach")
}
