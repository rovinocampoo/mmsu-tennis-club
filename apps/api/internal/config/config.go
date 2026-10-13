package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config contains settings used to start the API.
type Config struct {
	APIHost                string
	Port                   string
	DatabaseURL            string
	AdminDatabaseURL       string
	SupabaseURL            string
	SupabasePublishableKey string
	AdminUserIDs           map[string]struct{}
	CoachUserIDs           map[string]struct{}
	APIProxySharedSecret   string
	ResendAPIKey           string
	ResendFromEmail        string
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

	cfg := Config{
		APIHost:                strings.TrimSpace(os.Getenv("API_HOST")),
		Port:                   port,
		DatabaseURL:            os.Getenv("DATABASE_URL"),
		AdminDatabaseURL:       os.Getenv("ADMIN_DATABASE_URL"),
		SupabaseURL:            strings.TrimRight(strings.TrimSpace(os.Getenv("SUPABASE_URL")), "/"),
		SupabasePublishableKey: strings.TrimSpace(os.Getenv("SUPABASE_PUBLISHABLE_KEY")),
		APIProxySharedSecret:   strings.TrimSpace(os.Getenv("API_PROXY_SHARED_SECRET")),
		ResendAPIKey:           strings.TrimSpace(os.Getenv("RESEND_API_KEY")),
		ResendFromEmail:        strings.TrimSpace(os.Getenv("RESEND_FROM_EMAIL")),
	}
	if cfg.APIProxySharedSecret != "" && len(cfg.APIProxySharedSecret) < 32 {
		return Config{}, fmt.Errorf("API_PROXY_SHARED_SECRET must contain at least 32 characters")
	}
	if cfg.SupabasePublishableKey == "" {
		cfg.SupabasePublishableKey = strings.TrimSpace(os.Getenv("SUPABASE_ANON_KEY"))
	}
	cfg.AdminUserIDs, err = parseUserIDs(os.Getenv("ADMIN_USER_IDS"), "ADMIN_USER_IDS")
	if err != nil {
		return Config{}, err
	}
	cfg.CoachUserIDs, err = parseUserIDs(os.Getenv("COACH_USER_IDS"), "COACH_USER_IDS")
	if err != nil {
		return Config{}, err
	}
	if (len(cfg.AdminUserIDs) > 0 || len(cfg.CoachUserIDs) > 0) && (cfg.SupabaseURL == "" || cfg.SupabasePublishableKey == "") {
		return Config{}, fmt.Errorf("SUPABASE_URL and SUPABASE_PUBLISHABLE_KEY (or SUPABASE_ANON_KEY) are required with ADMIN_USER_IDS or COACH_USER_IDS")
	}
	return cfg, nil
}

func parseUserIDs(raw, setting string) (map[string]struct{}, error) {
	ids := make(map[string]struct{})
	for _, id := range strings.Split(raw, ",") {
		id = strings.ToLower(strings.TrimSpace(id))
		if id == "" {
			continue
		}
		if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
			return nil, fmt.Errorf("%s must contain comma-separated UUIDs", setting)
		}
		for i, c := range id {
			if i == 8 || i == 13 || i == 18 || i == 23 {
				continue
			}
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				return nil, fmt.Errorf("%s must contain comma-separated UUIDs", setting)
			}
		}
		ids[id] = struct{}{}
	}
	return ids, nil
}
