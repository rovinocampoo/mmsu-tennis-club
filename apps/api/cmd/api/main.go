package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/config"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/handlers"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load configuration: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handlers.Health)

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
