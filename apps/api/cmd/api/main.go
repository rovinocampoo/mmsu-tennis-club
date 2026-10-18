package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/adminauth"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/config"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/googlecalendar"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/handlers"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/notifications"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/postgres"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/ratelimit"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/training"
)

const (
	bookingRequestLimit       = 5
	bookingRequestWindow      = time.Hour
	bookingStatusLimit        = 30
	bookingStatusWindow       = time.Minute
	publicRateLimitMaxClients = 10_000
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load configuration: %v", err)
	}

	var repository training.Repository
	var bookingRepository training.BookingRepository
	var adminBookingRepository training.BookingRepository
	var approvalNotificationRepository notifications.Repository
	if cfg.DatabaseURL == "" {
		log.Print("DATABASE_URL is not configured; training sessions will return 503")
	} else {
		pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
		if err != nil {
			log.Printf("could not configure PostgreSQL from DATABASE_URL; training sessions will return 503 (%T)", err)
		} else {
			defer pool.Close()
			repository = postgres.NewTrainingRepository(pool)
			bookingPostgresRepository := postgres.NewBookingRepository(pool)
			bookingRepository = bookingPostgresRepository
		}
	}
	if cfg.AdminDatabaseURL == "" {
		log.Print("ADMIN_DATABASE_URL is not configured; admin booking review will return unavailable")
	} else {
		adminPool, err := pgxpool.New(context.Background(), cfg.AdminDatabaseURL)
		if err != nil {
			log.Printf("could not configure PostgreSQL for admin review (%T)", err)
		} else {
			defer adminPool.Close()
			adminPostgresRepository := postgres.NewBookingRepository(adminPool)
			adminBookingRepository = adminPostgresRepository
			approvalNotificationRepository = adminPostgresRepository
		}
	}

	var approvalNotifier notifications.ApprovalNotifier
	if approvalNotificationRepository != nil {
		approvalNotifier = notifications.NewService(
			approvalNotificationRepository,
			notifications.NewResendSender(cfg.ResendAPIKey, cfg.ResendFromEmail),
			cfg.ResendFromEmail,
			log.Default(),
		)
	}

	calendarSyncer := googlecalendar.New(cfg.GoogleServiceAccountFile, cfg.GoogleCalendarID)
	trainingService := training.NewService(repository)
	listenAddr := net.JoinHostPort(cfg.APIHost, cfg.Port)
	server := &http.Server{
		Addr:              listenAddr,
		Handler:           newAPIHandlerWithCalendar(cfg, trainingService, bookingRepository, adminBookingRepository, log.Default(), approvalNotifier, calendarSyncer),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("API listening on http://%s", listenAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	}
}

func newAPIHandler(cfg config.Config, trainingService *training.Service, bookingRepository, adminBookingRepository training.BookingRepository, logger *log.Logger, notifiers ...notifications.ApprovalNotifier) http.Handler {
 var notifier notifications.ApprovalNotifier; if len(notifiers)>0 { notifier=notifiers[0] }
 return newAPIHandlerWithCalendar(cfg, trainingService, bookingRepository, adminBookingRepository, logger, notifier, nil)
}
func newAPIHandlerWithCalendar(cfg config.Config, trainingService *training.Service, bookingRepository, adminBookingRepository training.BookingRepository, logger *log.Logger, approvalNotifier notifications.ApprovalNotifier, calendarSyncer *googlecalendar.Client) http.Handler {
	if logger == nil {
		logger = log.Default()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handlers.Health)
	mux.Handle("GET /api/training-programs", handlers.TrainingPrograms(trainingService, logger))
	mux.Handle("GET /api/training-sessions", handlers.TrainingSessions(trainingService, logger))
	mux.Handle("POST /api/booking-requests", ratelimit.MiddlewareWithProxySecret(
		ratelimit.New(bookingRequestLimit, bookingRequestWindow, publicRateLimitMaxClients),
		handlers.BookingRequest(bookingRepository, logger),
		cfg.APIProxySharedSecret,
	))
	mux.Handle("POST /api/booking-status", ratelimit.MiddlewareWithProxySecret(
		ratelimit.New(bookingStatusLimit, bookingStatusWindow, publicRateLimitMaxClients),
		handlers.PublicBookingStatus(bookingRepository, logger),
		cfg.APIProxySharedSecret,
	))
	var authValidator *adminauth.Validator
	if cfg.SupabaseURL != "" && cfg.SupabasePublishableKey != "" {
		var err error
		authValidator, err = adminauth.NewValidator(cfg.SupabaseURL, cfg.SupabasePublishableKey)
		if err != nil {
			logger.Printf("Supabase Auth validation is unavailable (%T)", err)
		}
	}
	protectAdmin := func(handler http.Handler) http.Handler {
		return adminauth.Protect(handler, authValidator, cfg.AdminUserIDs)
	}
	protectCoach := func(handler http.Handler) http.Handler {
		return adminauth.Protect(handler, authValidator, cfg.CoachUserIDs)
	}

	mux.Handle("GET /api/coach/availability", protectCoach(handlers.CoachListAvailability(bookingRepository, logger)))
	mux.Handle("POST /api/coach/availability", protectCoach(handlers.CoachSubmitAvailability(bookingRepository, logger)))
	mux.Handle("DELETE /api/coach/availability/{id}", protectCoach(handlers.CoachDeleteAvailability(bookingRepository, logger)))
	mux.Handle("GET /api/coach/sessions", protectCoach(handlers.CoachListAssignedSessions(bookingRepository, logger)))

	mux.Handle("GET /api/admin/coach-availability", protectAdmin(handlers.AdminListCoachAvailability(adminBookingRepository, logger)))
	mux.Handle("GET /api/admin/training-programs", protectAdmin(handlers.AdminListTrainingPrograms(adminBookingRepository, logger)))
	mux.Handle("GET /api/admin/training-sessions", protectAdmin(handlers.AdminListTrainingSessions(adminBookingRepository, logger)))
	mux.Handle("POST /api/admin/training-sessions", protectAdmin(handlers.AdminCreateTrainingSession(adminBookingRepository, logger)))
	mux.Handle("POST /api/admin/training-sessions/batch", protectAdmin(handlers.AdminCreateTrainingSessionsBatch(adminBookingRepository, logger)))
	mux.Handle("PUT /api/admin/training-sessions/{id}", protectAdmin(handlers.AdminEditTrainingSession(adminBookingRepository, logger)))
	mux.Handle("PATCH /api/admin/training-sessions/{id}", protectAdmin(handlers.AdminUpdateTrainingSession(adminBookingRepository, logger)))
	mux.Handle("PATCH /api/admin/training-sessions/{id}/coach", protectAdmin(handlers.AdminAssignTrainingSessionCoach(adminBookingRepository, logger)))

	mux.Handle("GET /api/admin/booking-requests", protectAdmin(handlers.AdminBookingRequests(adminBookingRepository, logger)))
	mux.Handle("GET /api/admin/approved-clients", protectAdmin(handlers.AdminApprovedClients(adminBookingRepository, logger)))
	mux.Handle("GET /api/admin/players", protectAdmin(handlers.AdminPlayerSearch(adminBookingRepository, logger)))
	mux.Handle("POST /api/admin/booking-requests/{id}/approve", protectAdmin(handlers.AdminApproveBookingRequestWithCalendar(adminBookingRepository, logger, calendarSyncer, approvalNotifier)))
	mux.Handle("POST /api/admin/booking-requests/{id}/calendar/retry", protectAdmin(handlers.AdminRetryBookingCalendarSync(adminBookingRepository, calendarSyncer, logger)))
	mux.Handle("POST /api/admin/booking-requests/{id}/reject", protectAdmin(handlers.AdminRejectBookingRequest(adminBookingRepository, logger)))
	mux.Handle("POST /api/admin/booking-requests/{id}/notification/retry", protectAdmin(handlers.AdminRetryBookingApprovalNotification(approvalNotifier, logger)))
	return mux
}
