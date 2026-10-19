package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/adminauth"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/training"
)

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

type trainingProgramCatalogRepository interface {
	ListAdminTrainingProgramCatalog(context.Context) ([]training.TrainingProgram, error)
	CreateAdminTrainingProgram(context.Context, string, string) (string, error)
	UpdateAdminTrainingProgram(context.Context, string, string, string, bool) (bool, error)
}

func AdminTrainingProgramCatalog(repo training.BookingRepository, logger *log.Logger) http.HandlerFunc {
	if logger == nil {
		logger = log.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		catalog, ok := any(repo).(trainingProgramCatalogRepository)
		if repo == nil || !ok {
			writeError(w, http.StatusServiceUnavailable, "training program management is temporarily unavailable")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		switch r.Method {
		case http.MethodGet:
			programs, err := catalog.ListAdminTrainingProgramCatalog(ctx)
			if err != nil {
				logger.Printf("admin list training program catalog failed: %v", err)
				writeError(w, http.StatusInternalServerError, "could not load training programs")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"programs": programs})
		case http.MethodPost:
			var request struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			}
			if !decodeJSON(w, r, &request, 32*1024) {
				return
			}
			request.Name = strings.TrimSpace(request.Name)
			request.Description = strings.TrimSpace(request.Description)
			if request.Name == "" || utf8.RuneCountInString(request.Name) > 200 {
				writeError(w, http.StatusBadRequest, "training program name is required and must be 200 characters or fewer")
				return
			}
			if utf8.RuneCountInString(request.Description) > 2000 {
				writeError(w, http.StatusBadRequest, "training program description must be 2000 characters or fewer")
				return
			}
			id, err := catalog.CreateAdminTrainingProgram(ctx, request.Name, request.Description)
			if err != nil {
				logger.Printf("admin create training program failed: %v", err)
				writeError(w, http.StatusInternalServerError, "could not create training program")
				return
			}
			writeJSON(w, http.StatusCreated, map[string]string{"id": id})
		case http.MethodPatch:
			id := strings.ToLower(strings.TrimSpace(r.PathValue("id")))
			if !validUUID(id) {
				writeError(w, http.StatusBadRequest, "training program ID must be a UUID")
				return
			}
			var request struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				IsActive    *bool  `json:"is_active"`
			}
			if !decodeJSON(w, r, &request, 32*1024) {
				return
			}
			request.Name = strings.TrimSpace(request.Name)
			request.Description = strings.TrimSpace(request.Description)
			if request.Name == "" || utf8.RuneCountInString(request.Name) > 200 || request.IsActive == nil {
				writeError(w, http.StatusBadRequest, "training program name and status are required; name must be 200 characters or fewer")
				return
			}
			if utf8.RuneCountInString(request.Description) > 2000 {
				writeError(w, http.StatusBadRequest, "training program description must be 2000 characters or fewer")
				return
			}
			updated, err := catalog.UpdateAdminTrainingProgram(ctx, id, request.Name, request.Description, *request.IsActive)
			if err != nil {
				logger.Printf("admin update training program failed: %v", err)
				writeError(w, http.StatusInternalServerError, "could not update training program")
				return
			}
			if !updated {
				writeError(w, http.StatusNotFound, "training program not found")
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
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
	CoachID   *string   `json:"coach_id"`
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
			writeSchedulingRepositoryError(w, logger, "admin create training session", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
	}
}

type sessionBatchRepository interface {
	CreateTrainingSessions(context.Context, []training.TrainingSessionCreateInput) ([]string, error)
}

// AdminCreateTrainingSessionsBatch creates every requested slot in one database call.
// The database function runs as one statement, so a conflict rolls the entire batch back.
func AdminCreateTrainingSessionsBatch(repo training.BookingRepository, logger *log.Logger) http.HandlerFunc {
	if logger == nil {
		logger = log.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if repo == nil {
			writeError(w, http.StatusServiceUnavailable, "scheduling service is temporarily unavailable")
			return
		}
		batchRepo, ok := any(repo).(sessionBatchRepository)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, "batch scheduling is temporarily unavailable")
			return
		}
		var req struct {
			Sessions []adminCreateSessionRequest `json:"sessions"`
		}
		if !decodeJSON(w, r, &req, 512*1024) {
			return
		}
		if len(req.Sessions) < 1 || len(req.Sessions) > 500 {
			writeError(w, http.StatusBadRequest, "provide between 1 and 500 sessions")
			return
		}
		sessions := make([]training.TrainingSessionCreateInput, 0, len(req.Sessions))
		for _, item := range req.Sessions {
			item.ProgramID = strings.ToLower(strings.TrimSpace(item.ProgramID))
			item.Location = strings.TrimSpace(item.Location)
			item.Notes = strings.TrimSpace(item.Notes)
			if !validUUID(item.ProgramID) || item.StartsAt.IsZero() || item.EndsAt.IsZero() || !item.StartsAt.Before(item.EndsAt) || item.Location == "" || len(item.Location) > 200 || len(item.Notes) > 2000 {
				writeError(w, http.StatusBadRequest, "each session needs a program, valid start/end times, and a location")
				return
			}
			if item.Capacity != nil && *item.Capacity <= 0 {
				writeError(w, http.StatusBadRequest, "capacity must be a positive whole number or null")
				return
			}
			if item.CoachID != nil {
				coachID := strings.ToLower(strings.TrimSpace(*item.CoachID))
				if !validUUID(coachID) {
					writeError(w, http.StatusBadRequest, "coach_id must be a UUID or null")
					return
				}
				item.CoachID = &coachID
			}
			isPublic := true
			if item.IsPublic != nil {
				isPublic = *item.IsPublic
			}
			sessions = append(sessions, training.TrainingSessionCreateInput{
				ProgramID: item.ProgramID, StartsAt: item.StartsAt, EndsAt: item.EndsAt,
				Location: item.Location, Capacity: item.Capacity, Notes: item.Notes,
				IsPublic: isPublic, CoachID: item.CoachID,
			})
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		ids, err := batchRepo.CreateTrainingSessions(ctx, sessions)
		if err != nil {
			writeSchedulingRepositoryError(w, logger, "admin create training sessions batch", err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"ids": ids, "created": len(ids)})
	}
}

var manilaLocation = time.FixedZone("Asia/Manila", 8*60*60)

type sessionPublishRepository interface {
	PublishTrainingSessions(context.Context, time.Time, time.Time) (training.PublicationResult, error)
}

// AdminPublishTrainingSessions atomically publishes every hidden, still-open
// session inside a confirmed Manila day or month. The window is derived
// server-side from the selected date so the client cannot widen it, and the
// database re-checks eligibility so closed sessions are not reopened and
// cancelled sessions are not reactivated.
func AdminPublishTrainingSessions(repo training.BookingRepository, logger *log.Logger) http.HandlerFunc {
	if logger == nil {
		logger = log.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if repo == nil {
			writeError(w, http.StatusServiceUnavailable, "scheduling service is temporarily unavailable")
			return
		}
		publishRepo, ok := any(repo).(sessionPublishRepository)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, "bulk publication is temporarily unavailable")
			return
		}
		var req struct {
			Scope string `json:"scope"`
			Date  string `json:"date"`
		}
		if !decodeJSON(w, r, &req, 4096) {
			return
		}
		req.Scope = strings.ToLower(strings.TrimSpace(req.Scope))
		req.Date = strings.TrimSpace(req.Date)
		if req.Scope != "day" && req.Scope != "month" {
			writeError(w, http.StatusBadRequest, "scope must be day or month")
			return
		}
		day, err := time.ParseInLocation("2006-01-02", req.Date, manilaLocation)
		if err != nil {
			writeError(w, http.StatusBadRequest, "date must be a valid YYYY-MM-DD value")
			return
		}
		var startsAt, endsAt time.Time
		if req.Scope == "day" {
			startsAt = day
			endsAt = day.AddDate(0, 0, 1)
		} else {
			startsAt = time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, manilaLocation)
			endsAt = startsAt.AddDate(0, 1, 0)
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		result, err := publishRepo.PublishTrainingSessions(ctx, startsAt, endsAt)
		if err != nil {
			writeSchedulingRepositoryError(w, logger, "admin publish training sessions", err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

type sessionEditRepository interface {
	EditTrainingSession(context.Context, string, training.TrainingSessionEditInput) error
}

type adminEditSessionRequest struct {
	ProgramID                string    `json:"program_id"`
	StartsAt                 time.Time `json:"starts_at"`
	EndsAt                   time.Time `json:"ends_at"`
	Location                 string    `json:"location"`
	Capacity                 *int      `json:"capacity"`
	Notes                    string    `json:"notes"`
	IsPublic                 *bool     `json:"is_public"`
	AcknowledgeBookedChanges bool      `json:"acknowledge_booked_changes"`
}

func AdminEditTrainingSession(repo training.BookingRepository, logger *log.Logger) http.HandlerFunc {
	if logger == nil {
		logger = log.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := strings.ToLower(strings.TrimSpace(r.PathValue("id")))
		if !validUUID(sessionID) {
			writeError(w, http.StatusBadRequest, "session id is invalid")
			return
		}
		if repo == nil {
			writeError(w, http.StatusServiceUnavailable, "scheduling service is temporarily unavailable")
			return
		}
		editRepo, ok := any(repo).(sessionEditRepository)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, "session editing is temporarily unavailable")
			return
		}
		var req adminEditSessionRequest
		if !decodeJSON(w, r, &req, 8192) {
			return
		}
		req.ProgramID = strings.ToLower(strings.TrimSpace(req.ProgramID))
		req.Location = strings.TrimSpace(req.Location)
		req.Notes = strings.TrimSpace(req.Notes)
		if !validUUID(req.ProgramID) || req.StartsAt.IsZero() || req.EndsAt.IsZero() || !req.StartsAt.Before(req.EndsAt) || req.Location == "" || len(req.Location) > 200 || len(req.Notes) > 2000 || req.IsPublic == nil {
			writeError(w, http.StatusBadRequest, "program, valid start/end times, location, and publication status are required")
			return
		}
		if req.Capacity != nil && *req.Capacity <= 0 {
			writeError(w, http.StatusBadRequest, "capacity must be a positive whole number or null")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		err := editRepo.EditTrainingSession(ctx, sessionID, training.TrainingSessionEditInput{
			ProgramID: req.ProgramID, StartsAt: req.StartsAt, EndsAt: req.EndsAt,
			Location: req.Location, Capacity: req.Capacity, Notes: req.Notes,
			IsPublic: *req.IsPublic, AcknowledgeBookedChanges: req.AcknowledgeBookedChanges,
		})
		if err != nil {
			writeSchedulingRepositoryError(w, logger, "admin edit training session", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	}
}

func writeSchedulingRepositoryError(w http.ResponseWriter, logger interface{ Printf(string, ...any) }, operation string, err error) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "PZ001", "P0002":
			writeError(w, http.StatusConflict, "training session or program is unavailable")
		case "PZ002", "PZ009":
			writeError(w, http.StatusConflict, "capacity conflicts with existing bookings")
		case "PZ004":
			writeError(w, http.StatusConflict, "the selected coach is unavailable")
		case "PZ007":
			writeError(w, http.StatusConflict, "one or more selected slots conflict with the existing schedule; no sessions were created")
		case "PZ008":
			writeError(w, http.StatusConflict, "confirm the warning before changing a session that has confirmed bookings")
		case "PZ011":
			writeError(w, http.StatusConflict, "resolve pending booking requests before changing this session's date, time, program, or venue")
		case "22023":
			writeError(w, http.StatusBadRequest, "session details are invalid")
		default:
			logger.Printf("%s failed (database code %s)", operation, pgErr.Code)
			writeError(w, http.StatusInternalServerError, "could not save the training schedule")
		}
		return
	}
	if logger != nil {
		logger.Printf("%s failed (error type %T)", operation, err)
	}
	writeError(w, http.StatusInternalServerError, "could not save the training schedule")
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
			writeSchedulingRepositoryError(w, logger, "admin update training session", err)
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
			writeSchedulingRepositoryError(w, logger, "assign coach to training session", err)
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
