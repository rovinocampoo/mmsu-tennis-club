package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config contains settings used to start the API.
type Config struct {
	Port        string
	DatabaseURL string
}

// Load reads configuration from the process environment.
func Load() (Config, error) {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return Config{}, fmt.Errorf("PORT must be a number from 1 to 65535")
	}

	return Config{
		Port:        port,
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}, nil
}
