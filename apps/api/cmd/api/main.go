package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/config"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/handlers"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/postgres"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/training"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load configuration: %v", err)
	}

	var repository training.Repository
	if cfg.DatabaseURL == "" {
		log.Print("DATABASE_URL is not configured; training sessions will return 503")
	} else {
		pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
		if err != nil {
			log.Printf("could not configure PostgreSQL from DATABASE_URL; training sessions will return 503 (%T)", err)
		} else {
			defer pool.Close()
			repository = postgres.NewTrainingRepository(pool)
		}
	}

	trainingService := training.NewService(repository)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handlers.Health)
	mux.Handle("GET /api/training-sessions", handlers.TrainingSessions(trainingService, log.Default()))

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("API listening on http://localhost:%s", cfg.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	}
}
