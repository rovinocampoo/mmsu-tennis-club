package config

import "testing"

func TestLoadReadsDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example.invalid/test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an unexpected error: %v", err)
	}
	if cfg.DatabaseURL != "postgres://example.invalid/test" {
		t.Errorf("Load().DatabaseURL = %q, want configured value", cfg.DatabaseURL)
	}
}

func TestLoadAllowsMissingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() without DATABASE_URL returned an unexpected error: %v", err)
	}
	if cfg.DatabaseURL != "" {
		t.Errorf("Load().DatabaseURL = %q, want empty", cfg.DatabaseURL)
	}
}

func TestLoadUsesDefaultPort(t *testing.T) {
	t.Setenv("PORT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an unexpected error: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("Load().Port = %q, want %q", cfg.Port, "8080")
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	for _, port := range []string{"not-a-port", "0", "65536"} {
		t.Run(port, func(t *testing.T) {
			t.Setenv("PORT", port)

			if _, err := Load(); err == nil {
				t.Fatalf("Load() with PORT=%q succeeded, want an error", port)
			}
		})
	}
}
