package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/training"
)

type trainingSessionsResponse struct {
	Error string `json:"error"`
}

// TrainingSessions serves the public upcoming training schedule.
func TrainingSessions(service *training.Service, logger *log.Logger) http.HandlerFunc {
	if logger == nil {
		logger = log.Default()
	}

	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		sessions, err := service.ListUpcoming(ctx)
		if err != nil {
			if errors.Is(err, training.ErrUnavailable) {
				logger.Printf("training sessions unavailable; check DATABASE_URL, PostgreSQL connectivity, and RLS read privileges (error type %T)", err)
				writeTrainingSessionsError(w, http.StatusServiceUnavailable, "training sessions are temporarily unavailable")
				return
			}

			logger.Printf("unexpected training sessions error (type %T)", err)
			writeTrainingSessionsError(w, http.StatusInternalServerError, "could not load training sessions")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(sessions); err != nil {
			logger.Printf("encode training sessions response: %v", err)
		}
	}
}

func writeTrainingSessionsError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(trainingSessionsResponse{Error: message})
}
