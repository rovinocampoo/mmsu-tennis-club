package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/adminauth"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/training"
)

type coachSubmitAvailabilityRequest struct {
	CoachName string    `json:"coach_name"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
	Notes     string    `json:"notes"`
}

func CoachSubmitAvailability(repo training.BookingRepository, logger *log.Logger) http.HandlerFunc {
	if logger == nil {
		logger = log.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		coachID := adminauth.UserID(r.Context())
		if coachID == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if repo == nil {
			writeError(w, http.StatusServiceUnavailable, "scheduling service is temporarily unavailable")
			return
		}

		var req coachSubmitAvailabilityRequest
		if !decodeJSON(w, r, &req, 8192) {
			return
		}
		req.CoachName = strings.TrimSpace(req.CoachName)
		req.Notes = strings.TrimSpace(req.Notes)
		if req.CoachName == "" || len(req.CoachName) > 100 || len(req.Notes) > 2000 {
			writeError(w, http.StatusBadRequest, "coach name and notes are invalid")
			return
		}
		if req.StartsAt.IsZero() || req.EndsAt.IsZero() || !req.StartsAt.Before(req.EndsAt) {
			writeError(w, http.StatusBadRequest, "start time must be before end time")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		id, err := repo.SubmitCoachAvailability(ctx, coachID, req.CoachName, req.StartsAt, req.EndsAt, req.Notes)
		if err != nil {
			logger.Printf("submit coach availability failed: %v", err)
			writeError(w, http.StatusInternalServerError, "could not submit availability")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
	}
}

func CoachListAvailability(repo training.BookingRepository, logger *log.Logger) http.HandlerFunc {
	if logger == nil {
		logger = log.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		coachID := adminauth.UserID(r.Context())
		if coachID == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if repo == nil {
			writeError(w, http.StatusServiceUnavailable, "scheduling service is temporarily unavailable")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		items, err := repo.ListCoachAvailability(ctx, coachID)
		if err != nil {
			logger.Printf("list coach availability failed: %v", err)
			writeError(w, http.StatusInternalServerError, "could not load coach availability")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"availability": items})
	}
}

func CoachListAssignedSessions(repo training.BookingRepository, logger *log.Logger) http.HandlerFunc {
	if logger == nil {
		logger = log.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		coachID := adminauth.UserID(r.Context())
		if coachID == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if repo == nil {
			writeError(w, http.StatusServiceUnavailable, "scheduling service is temporarily unavailable")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		sessions, err := repo.ListAssignedTrainingSessions(ctx, coachID)
		if err != nil {
			logger.Printf("list assigned training sessions failed: %v", err)
			writeError(w, http.StatusInternalServerError, "could not load assigned sessions")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"sessions": sessions})
	}
}

func CoachDeleteAvailability(repo training.BookingRepository, logger *log.Logger) http.HandlerFunc {
	if logger == nil {
		logger = log.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		coachID := adminauth.UserID(r.Context())
		if coachID == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		id := r.PathValue("id")
		if !validUUID(id) {
			writeError(w, http.StatusBadRequest, "availability id is invalid")
			return
		}
		if repo == nil {
			writeError(w, http.StatusServiceUnavailable, "scheduling service is temporarily unavailable")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := repo.DeleteCoachAvailability(ctx, id, coachID); err != nil {
			logger.Printf("delete coach availability failed: %v", err)
			writeError(w, http.StatusInternalServerError, "could not delete availability")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}
}

func AdminListCoachAvailability(repo training.BookingRepository, logger *log.Logger) http.HandlerFunc {
	if logger == nil {
		logger = log.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if repo == nil {
			writeError(w, http.StatusServiceUnavailable, "scheduling service is temporarily unavailable")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		items, err := repo.ListAllCoachAvailability(ctx)
		if err != nil {
			logger.Printf("admin list coach availability failed: %v", err)
			writeError(w, http.StatusInternalServerError, "could not load coach availability")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"availability": items})
	}
}

func AdminListTrainingPrograms(repo training.BookingRepository, logger *log.Logger) http.HandlerFunc {
	if logger == nil {
		logger = log.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if repo == nil {
			writeError(w, http.StatusServiceUnavailable, "scheduling service is temporarily unavailable")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		programs, err := repo.ListAdminTrainingPrograms(ctx)
		if err != nil {
			logger.Printf("admin list training programs failed: %v", err)
			writeError(w, http.StatusInternalServerError, "could not load training programs")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"programs": programs})
	}
}

func AdminListTrainingSessions(repo training.BookingRepository, logger *log.Logger) http.HandlerFunc {
	if logger == nil {
		logger = log.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if repo == nil {
			writeError(w, http.StatusServiceUnavailable, "scheduling service is temporarily unavailable")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		sessions, err := repo.ListAdminTrainingSessions(ctx)
		if err != nil {
			logger.Printf("admin list training sessions failed: %v", err)
			writeError(w, http.StatusInternalServerError, "could not load training sessions")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"sessions": sessions})
	}
}

type adminCreateSessionRequest struct {
	ProgramID string    `json:"program_id"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
	Location  string    `json:"location"`
	Capacity  *int      `json:"capacity"`
	Notes     string    `json:"notes"`
	IsPublic  *bool     `json:"is_public"`
}

func AdminCreateTrainingSession(repo training.BookingRepository, logger *log.Logger) http.HandlerFunc {
	if logger == nil {
		logger = log.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if repo == nil {
			writeError(w, http.StatusServiceUnavailable, "scheduling service is temporarily unavailable")
			return
		}

		var req adminCreateSessionRequest
		if !decodeJSON(w, r, &req, 8192) {
			return
		}
		req.ProgramID = strings.ToLower(strings.TrimSpace(req.ProgramID))
		req.Location = strings.TrimSpace(req.Location)
		req.Notes = strings.TrimSpace(req.Notes)
		if !validUUID(req.ProgramID) || req.StartsAt.IsZero() || req.EndsAt.IsZero() || !req.StartsAt.Before(req.EndsAt) || req.Location == "" || len(req.Location) > 200 || len(req.Notes) > 2000 {
			writeError(w, http.StatusBadRequest, "program, valid start/end times, and location are required")
			return
		}
		if req.Capacity != nil && *req.Capacity <= 0 {
			writeError(w, http.StatusBadRequest, "capacity must be positive")
			return
		}

		isPublic := true
		if req.IsPublic != nil {
			isPublic = *req.IsPublic
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		id, err := repo.CreateTrainingSession(ctx, req.ProgramID, req.StartsAt, req.EndsAt, req.Location, req.Capacity, req.Notes, isPublic)
		if err != nil {
			logger.Printf("admin create training session failed: %v", err)
			writeError(w, http.StatusInternalServerError, "could not create training session")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
	}
}

type adminUpdateSessionRequest struct {
	Status   *string `json:"status"`
	Notes    *string `json:"notes"`
	IsPublic *bool   `json:"is_public"`
}

func AdminUpdateTrainingSession(repo training.BookingRepository, logger *log.Logger) http.HandlerFunc {
	if logger == nil {
		logger = log.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := strings.ToLower(r.PathValue("id"))
		if !validUUID(sessionID) {
			writeError(w, http.StatusBadRequest, "session id is invalid")
			return
		}
		if repo == nil {
			writeError(w, http.StatusServiceUnavailable, "scheduling service is temporarily unavailable")
			return
		}

		var req adminUpdateSessionRequest
		if !decodeJSON(w, r, &req, 8192) {
			return
		}
		if req.Status != nil && *req.Status != "open" && *req.Status != "closed" && *req.Status != "cancelled" {
			writeError(w, http.StatusBadRequest, "status must be open, closed, or cancelled")
			return
		}
		if req.Status == nil && req.Notes == nil && req.IsPublic == nil {
			writeError(w, http.StatusBadRequest, "at least one session field is required")
			return
		}
		if req.Notes != nil {
			trimmedNotes := strings.TrimSpace(*req.Notes)
			if len(trimmedNotes) > 2000 {
				writeError(w, http.StatusBadRequest, "notes are too long")
				return
			}
			req.Notes = &trimmedNotes
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := repo.UpdateTrainingSession(ctx, sessionID, req.Status, req.Notes, req.IsPublic); err != nil {
			logger.Printf("admin update training session failed: %v", err)
			writeError(w, http.StatusInternalServerError, "could not update training session")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}
}

func AdminAssignTrainingSessionCoach(repo training.BookingRepository, logger *log.Logger) http.HandlerFunc {
	if logger == nil {
		logger = log.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := strings.ToLower(r.PathValue("id"))
		if !validUUID(sessionID) {
			writeError(w, http.StatusBadRequest, "session id is invalid")
			return
		}
		if repo == nil {
			writeError(w, http.StatusServiceUnavailable, "scheduling service is temporarily unavailable")
			return
		}

		var req struct {
			CoachID json.RawMessage `json:"coach_id"`
		}
		if !decodeJSON(w, r, &req, 8192) {
			return
		}
		if len(req.CoachID) == 0 {
			writeError(w, http.StatusBadRequest, "coach_id is required and may be null to unassign")
			return
		}
		var coachID *string
		if string(req.CoachID) != "null" {
			var value string
			if err := json.Unmarshal(req.CoachID, &value); err != nil {
				writeError(w, http.StatusBadRequest, "coach_id must be a UUID or null")
				return
			}
			value = strings.ToLower(strings.TrimSpace(value))
			if !validUUID(value) {
				writeError(w, http.StatusBadRequest, "coach_id must be a UUID or null")
				return
			}
			coachID = &value
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := repo.AssignTrainingSessionCoach(ctx, sessionID, coachID); err != nil {
			logger.Printf("assign coach to training session failed: %v", err)
			writeError(w, http.StatusInternalServerError, "could not assign coach to training session")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
