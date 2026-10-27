package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/adminauth"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/notifications"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/training"
)

type bookingRequestInput struct {
	SessionID          string   `json:"session_id"`
	SessionIDs         []string `json:"session_ids"`
	FullName           string   `json:"full_name"`
	Participants       []string `json:"participants"`
	Email              string   `json:"email"`
	Phone              string   `json:"phone"`
	TrainingFocus      string   `json:"training_focus"`
	TrainingFocusNotes string   `json:"training_focus_notes"`
}

// maxBookingParticipants bounds the per-request participant list. The joined
// names must also fit the 200-character booking_requests.full_name column.
const maxBookingParticipants = 20
const maxBookingSlots = 16

// normalizeParticipants prefers the explicit participant list and falls back to
// the single full_name still sent by older clients.
func normalizeParticipants(participants []string, fullName string) []string {
	names := make([]string, 0, len(participants))
	for _, name := range participants {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	if len(names) == 0 {
		names = append(names, strings.TrimSpace(fullName))
	}
	return names
}

func validateParticipants(participants []string) string {
	if len(participants) < 1 || len(participants) > maxBookingParticipants {
		return "provide between 1 and 20 participant names"
	}
	total := 0
	for _, name := range participants {
		if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 200 {
			return "each participant name must be 1 to 200 characters"
		}
		total += utf8.RuneCountInString(name)
	}
	if total+2*(len(participants)-1) > 200 {
		return "participant names are too long; use fewer or shorter names"
	}
	return ""
}

type bookingRequestAccepted struct {
	RequestReference string `json:"request_reference"`
	TrackingToken    string `json:"tracking_token"`
	Status           string `json:"status"`
	Message          string `json:"message"`
}

// BookingRequest accepts an unverified request for manual review only.
func BookingRequest(repo training.BookingRepository, logger interface{ Printf(string, ...any) }) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		var input bookingRequestInput
		if !decodeJSON(w, r, &input, 8192) {
			return
		}
		input.SessionID = strings.ToLower(strings.TrimSpace(input.SessionID))
		input.FullName = strings.TrimSpace(input.FullName)
		input.Email = strings.ToLower(strings.TrimSpace(input.Email))
		input.Phone = strings.TrimSpace(input.Phone)
		if len(input.SessionIDs) == 0 && input.SessionID != "" {
			input.SessionIDs = []string{input.SessionID}
		}
		if input.SessionID != "" && len(input.SessionIDs) > 0 && input.SessionID != input.SessionIDs[0] {
			writeBookingError(w, http.StatusBadRequest, "provide either session_id or matching session_ids")
			return
		}
		if len(input.SessionIDs) < 1 {
			writeBookingError(w, http.StatusBadRequest, "select at least one session slot")
			return
		}
		if len(input.SessionIDs) > maxBookingSlots {
			writeBookingError(w, http.StatusRequestEntityTooLarge, "booking size limit exceeded: select no more than 16 session slots across 8 dates")
			return
		}
		participants := normalizeParticipants(input.Participants, input.FullName)
		if message := validateParticipants(participants); message != "" {
			writeBookingError(w, http.StatusBadRequest, message)
			return
		}
		input.Participants = participants
		input.FullName = strings.Join(participants, ", ")
		input.TrainingFocus = strings.TrimSpace(input.TrainingFocus)
		input.TrainingFocusNotes = strings.TrimSpace(input.TrainingFocusNotes)
		seenSessionIDs := make(map[string]struct{}, len(input.SessionIDs))
		for index, id := range input.SessionIDs {
			id = strings.ToLower(strings.TrimSpace(id))
			if !validUUID(id) {
				writeBookingError(w, http.StatusBadRequest, "one or more selected session IDs are invalid")
				return
			}
			if _, exists := seenSessionIDs[id]; exists {
				writeBookingError(w, http.StatusBadRequest, "a session slot was selected more than once")
				return
			}
			seenSessionIDs[id] = struct{}{}
			input.SessionIDs[index] = id
		}
		input.SessionID = input.SessionIDs[0]
		// Email and phone are independently optional; a request may contain only
		// a full name. When a value is supplied it must still be well formed.
		if input.Email != "" {
			parsed, err := mail.ParseAddress(input.Email)
			if err != nil || parsed.Address != input.Email || len(input.Email) > 254 {
				writeBookingError(w, http.StatusBadRequest, "email is invalid")
				return
			}
		}
		if len(input.Phone) > 40 || !utf8.ValidString(input.Phone) {
			writeBookingError(w, http.StatusBadRequest, "phone is invalid")
			return
		}
		if repo == nil {
			writeBookingError(w, http.StatusServiceUnavailable, "booking requests are temporarily unavailable")
			return
		}
		var tokenBytes [32]byte
		if _, err := io.ReadFull(rand.Reader, tokenBytes[:]); err != nil {
			logBookingError(logger, "generate booking status token", err)
			writeBookingError(w, http.StatusInternalServerError, "booking request could not be processed")
			return
		}
		token := base64.RawURLEncoding.EncodeToString(tokenBytes[:])
		tokenHash := sha256.Sum256(tokenBytes[:])
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		reference, err := repo.CreateRequest(ctx, training.BookingRequest{
			SessionID: input.SessionID, SessionIDs: input.SessionIDs, FullName: input.FullName,
			Participants: input.Participants, Email: input.Email, Phone: input.Phone,
			TrainingFocus: input.TrainingFocus, TrainingFocusNotes: input.TrainingFocusNotes,
		}, tokenHash[:])
		if err != nil {
			writeBookingRepoError(w, logger, "create booking request", err)
			return
		}
		writeJSON(w, http.StatusAccepted, bookingRequestAccepted{RequestReference: reference, TrackingToken: token, Status: "pending", Message: "Your request is pending admin review. This is not a confirmed reservation."})
	}
}

type publicBookingStatusInput struct {
	TrackingToken string `json:"tracking_token"`
}

type publicBookingStatusResponse struct {
	RequestReference    string   `json:"request_reference"`
	Status              string   `json:"status"`
	ProgramName         string   `json:"program_name"`
	StartsAt            string   `json:"starts_at"`
	EndsAt              string   `json:"ends_at"`
	Location            string   `json:"location"`
	NextSteps           []string `json:"next_steps"`
	TrainingFocus       string   `json:"training_focus"`
	TrainingProgramName string   `json:"training_program_name"`
	TrainingFocusNotes  string   `json:"training_focus_notes"`
}

func PublicBookingStatus(repo training.BookingRepository, logger interface{ Printf(string, ...any) }) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		var input publicBookingStatusInput
		if !decodeJSON(w, r, &input, 1024) {
			return
		}
		tokenBytes, err := base64.RawURLEncoding.Strict().DecodeString(input.TrackingToken)
		if err != nil || len(tokenBytes) != 32 || base64.RawURLEncoding.EncodeToString(tokenBytes) != input.TrackingToken {
			writeBookingError(w, http.StatusNotFound, "status link is invalid")
			return
		}
		if repo == nil {
			writeBookingError(w, http.StatusServiceUnavailable, "booking status is temporarily unavailable")
			return
		}
		tokenHash := sha256.Sum256(tokenBytes)
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		status, err := repo.GetPublicStatus(ctx, tokenHash[:])
		if errors.Is(err, training.ErrBookingStatusNotFound) {
			writeBookingError(w, http.StatusNotFound, "status link is invalid")
			return
		}
		if errors.Is(err, training.ErrBookingStatusExpired) {
			writeBookingError(w, http.StatusGone, "status link has expired")
			return
		}
		if err != nil {
			writeBookingRepoError(w, logger, "get public booking status", err)
			return
		}
		nextSteps := map[string][]string{
			"pending":  {"A club admin will review your request. Wait for a decision; this is not a confirmed booking."},
			"approved": {"Your request is approved. Please arrive at the scheduled time and check in with the club."},
			"rejected": {"This request was not approved. You may contact the club or submit a request for another session."},
		}[status.Status]
		if nextSteps == nil {
			logBookingError(logger, "get public booking status", errors.New("unknown booking status"))
			writeBookingError(w, http.StatusInternalServerError, "booking status could not be processed")
			return
		}
		writeJSON(w, http.StatusOK, publicBookingStatusResponse{
			RequestReference:    status.RequestReference,
			Status:              status.Status,
			ProgramName:         status.ProgramName,
			StartsAt:            status.StartsAt.UTC().Format(time.RFC3339),
			EndsAt:              status.EndsAt.UTC().Format(time.RFC3339),
			Location:            status.Location,
			NextSteps:           nextSteps,
			TrainingFocus:       status.TrainingFocus,
			TrainingProgramName: status.TrainingProgramName,
			TrainingFocusNotes:  status.TrainingFocusNotes,
		})
	}
}

func AdminBookingRequests(repo training.BookingRepository, logger interface{ Printf(string, ...any) }) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if repo == nil {
			writeBookingError(w, http.StatusServiceUnavailable, "booking requests are temporarily unavailable")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		rows, err := repo.ListPendingRequests(ctx)
		if err != nil {
			writeBookingRepoError(w, logger, "list booking requests", err)
			return
		}
		if rows == nil {
			rows = []training.BookingRequestRecord{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"requests": rows})
	}
}

func AdminApprovedClients(repo training.BookingRepository, logger interface{ Printf(string, ...any) }) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if repo == nil {
			writeBookingError(w, http.StatusServiceUnavailable, "approved clients are temporarily unavailable")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		clients, err := repo.ListApprovedClients(ctx)
		if err != nil {
			writeBookingRepoError(w, logger, "list approved clients", err)
			return
		}

		if clients == nil {
			clients = []training.ApprovedClientRecord{}
		}

		writeJSON(w, http.StatusOK, map[string]any{"clients": clients})
	}
}

func AdminPlayerSearch(repo training.BookingRepository, logger interface{ Printf(string, ...any) }) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if repo == nil {
			writeBookingError(w, http.StatusServiceUnavailable, "player search is temporarily unavailable")
			return
		}
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		if utf8.RuneCountInString(query) < 2 || utf8.RuneCountInString(query) > 100 {
			writeBookingError(w, http.StatusBadRequest, "q must contain 2 to 100 characters")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		players, err := repo.SearchPlayers(ctx, query)
		if err != nil {
			writeBookingRepoError(w, logger, "search players", err)
			return
		}
		if players == nil {
			players = []training.PlayerCandidate{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"players": players})
	}
}

type approveRequestInput struct {
	PlayerID  string `json:"player_id"`
	CreateNew bool   `json:"create_new"`
}

type CalendarSyncer interface {
	IsConfigured() bool
	Sync(context.Context, training.CalendarEvent) error
}
type calendarEventRepository interface {
	GetBookingCalendarEvent(context.Context, string) (training.CalendarEvent, error)
}
type calendarSyncOutcome struct {
	Status    string `json:"status"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func syncCalendar(ctx context.Context, repo training.BookingRepository, syncer CalendarSyncer, id string, logger interface{ Printf(string, ...any) }) calendarSyncOutcome {
	if syncer == nil || !syncer.IsConfigured() {
		return calendarSyncOutcome{"not_configured", "Google Calendar is not configured; the booking is still approved.", false}
	}
	r, ok := repo.(calendarEventRepository)
	if !ok {
		return calendarSyncOutcome{"failed", "Calendar booking details could not be loaded. Retry the sync.", true}
	}
	e, err := r.GetBookingCalendarEvent(ctx, id)
	if err == nil {
		err = syncer.Sync(ctx, e)
	}
	if err != nil {
		if logger != nil {
			logger.Printf("Google Calendar sync failed for booking request %s: %v", id, err)
		}
		return calendarSyncOutcome{"failed", "Booking approved, but Google Calendar sync failed. You can retry.", true}
	}
	return calendarSyncOutcome{"synced", "Booking successfully added to Google Calendar.", false}
}
func AdminApproveBookingRequest(repo training.BookingRepository, logger interface{ Printf(string, ...any) }, notifiers ...notifications.ApprovalNotifier) http.HandlerFunc {
	return AdminApproveBookingRequestWithCalendar(repo, logger, nil, notifiers...)
}
func AdminApproveBookingRequestWithCalendar(repo training.BookingRepository, logger interface{ Printf(string, ...any) }, syncer CalendarSyncer, notifiers ...notifications.ApprovalNotifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if repo == nil {
			writeBookingError(w, http.StatusServiceUnavailable, "booking requests are temporarily unavailable")
			return
		}
		requestID := strings.ToLower(r.PathValue("id"))
		if !validUUID(requestID) {
			writeBookingError(w, http.StatusBadRequest, "request id is invalid")
			return
		}
		var input approveRequestInput
		if !decodeJSON(w, r, &input, 2048) {
			return
		}
		input.PlayerID = strings.ToLower(strings.TrimSpace(input.PlayerID))
		if (input.PlayerID == "") == !input.CreateNew || (input.PlayerID != "" && !validUUID(input.PlayerID)) {
			writeBookingError(w, http.StatusBadRequest, "choose an existing player or explicitly create a new player")
			return
		}
		var playerID *string
		if input.PlayerID != "" {
			playerID = &input.PlayerID
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		_, err := repo.ApproveRequest(ctx, requestID, adminauth.UserID(r.Context()), playerID, input.CreateNew)
		if err != nil {
			writeBookingRepoError(w, logger, "approve booking request", err)
			return
		}
		calendarCtx, calendarCancel := context.WithTimeout(context.WithoutCancel(r.Context()), 12*time.Second)
		calendarResult := syncCalendar(calendarCtx, repo, syncer, requestID, logger)
		calendarCancel()
		result := notifications.Result{
			Status:    notifications.StatusNotConfigured,
			Message:   "Approval was saved, but Resend is unavailable. Please contact the requester manually.",
			Retryable: true,
		}
		if len(notifiers) > 0 && notifiers[0] != nil {
			notificationCtx, notificationCancel := context.WithTimeout(context.WithoutCancel(r.Context()), 12*time.Second)
			result = notifiers[0].NotifyApproved(notificationCtx, requestID)
			notificationCancel()
		}
		writeJSON(w, http.StatusOK, struct {
			Status       string              `json:"status"`
			CalendarSync calendarSyncOutcome `json:"calendar_sync"`
			notifications.Result
		}{Status: "approved", CalendarSync: calendarResult, Result: result})
	}
}

func AdminRetryBookingCalendarSync(repo training.BookingRepository, syncer CalendarSyncer, logger interface{ Printf(string, ...any) }) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.ToLower(r.PathValue("id"))
		if !validUUID(id) {
			writeBookingError(w, http.StatusBadRequest, "request id is invalid")
			return
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 12*time.Second)
		defer cancel()
		writeJSON(w, http.StatusOK, syncCalendar(ctx, repo, syncer, id, logger))
	}
}

func AdminRetryBookingApprovalNotification(notifier notifications.ApprovalNotifier, logger interface{ Printf(string, ...any) }) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.ToLower(r.PathValue("id"))
		if !validUUID(requestID) {
			writeBookingError(w, http.StatusBadRequest, "request id is invalid")
			return
		}
		if notifier == nil {
			writeBookingError(w, http.StatusServiceUnavailable, "approval notifications are temporarily unavailable")
			return
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 12*time.Second)
		defer cancel()
		result := notifier.NotifyApproved(ctx, requestID)
		if result.Status == notifications.StatusFailed && result.Retryable {
			loggerBookingNotificationError(logger)
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func loggerBookingNotificationError(logger interface{ Printf(string, ...any) }) {
	if logger != nil {
		logger.Printf("approval notification retry did not complete successfully")
	}
}

func AdminRejectBookingRequest(repo training.BookingRepository, logger interface{ Printf(string, ...any) }) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if repo == nil {
			writeBookingError(w, http.StatusServiceUnavailable, "booking requests are temporarily unavailable")
			return
		}
		requestID := strings.ToLower(r.PathValue("id"))
		if !validUUID(requestID) {
			writeBookingError(w, http.StatusBadRequest, "request id is invalid")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := repo.RejectRequest(ctx, requestID, adminauth.UserID(r.Context())); err != nil {
			writeBookingRepoError(w, logger, "reject booking request", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any, maxBytes int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeBookingError(w, http.StatusRequestEntityTooLarge, "request is too large")
		} else {
			writeBookingError(w, http.StatusBadRequest, "invalid request")
		}
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeBookingError(w, http.StatusBadRequest, "invalid request")
		return false
	}
	return true
}

func writeBookingRepoError(w http.ResponseWriter, logger interface{ Printf(string, ...any) }, operation string, err error) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "PZ001", "P0002":
			writeBookingError(w, http.StatusConflict, "session is unavailable")
		case "PZ002":
			writeBookingError(w, http.StatusConflict, "session is full")
		case "PZ003":
			writeBookingError(w, http.StatusConflict, "request has already been decided")
		case "PZ004", "23505":
			writeBookingError(w, http.StatusConflict, "a matching request or booking already exists")
		case "PZ005":
			writeBookingError(w, http.StatusConflict, "selected player is unavailable")
		case "PZ006":
			writeBookingError(w, http.StatusNotFound, "booking request not found")
		case "PZ007":
			writeBookingError(w, http.StatusConflict, "one or more selected slots conflict with the schedule")
		case "PZ008":
			writeBookingError(w, http.StatusConflict, "schedule changes require explicit acknowledgement because bookings already exist")
		case "PZ009":
			writeBookingError(w, http.StatusConflict, "capacity cannot be lower than the number of confirmed bookings")
		case "PZ010":
			writeBookingError(w, http.StatusBadRequest, "within each date, selected slots must be adjacent and from the same program and location")
		case "PZ011":
			writeBookingError(w, http.StatusBadRequest, "select an active training program or leave the selection empty")
		case "PZ012":
			writeBookingError(w, http.StatusRequestEntityTooLarge, "booking size limit exceeded: select slots on no more than 8 dates")
		case "22023":
			writeBookingError(w, http.StatusBadRequest, "booking details are invalid")
		default:
			logBookingError(logger, operation, err)
			writeBookingError(w, http.StatusInternalServerError, "booking request could not be processed")
		}
		return
	}
	logBookingError(logger, operation, err)
	writeBookingError(w, http.StatusInternalServerError, "booking request could not be processed")
}

func logBookingError(logger interface{ Printf(string, ...any) }, operation string, err error) {
	if logger != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			logger.Printf("%s failed (database SQLSTATE %s)", operation, pgErr.Code)
			return
		}
		logger.Printf("%s failed (error type %T)", operation, err)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func validUUID(s string) bool {
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func writeBookingError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
