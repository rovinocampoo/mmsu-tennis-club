package config

import "testing"

func clearEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"API_HOST", "PORT", "DATABASE_URL", "ADMIN_DATABASE_URL", "SUPABASE_URL", "SUPABASE_PUBLISHABLE_KEY", "SUPABASE_ANON_KEY", "ADMIN_USER_IDS", "COACH_USER_IDS", "API_PROXY_SHARED_SECRET", "RESEND_API_KEY", "RESEND_FROM_EMAIL"} {
		t.Setenv(name, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIHost != "" || cfg.Port != "8080" || cfg.DatabaseURL != "" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestLoadAPIProxySharedSecret(t *testing.T) {
	clearEnv(t)
	const secret = "0123456789abcdef0123456789abcdef"
	t.Setenv("API_PROXY_SHARED_SECRET", " "+secret+" ")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIProxySharedSecret != secret {
		t.Fatal("API proxy shared secret was not trimmed and loaded")
	}
}

func TestLoadRejectsShortAPIProxySharedSecret(t *testing.T) {
	clearEnv(t)
	t.Setenv("API_PROXY_SHARED_SECRET", "too-short")
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted a short API proxy shared secret")
	}
}

func TestLoadOptionalResendConfiguration(t *testing.T) {
	clearEnv(t)
	t.Setenv("RESEND_API_KEY", " server-only-key ")
	t.Setenv("RESEND_FROM_EMAIL", " club@example.test ")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ResendAPIKey != "server-only-key" || cfg.ResendFromEmail != "club@example.test" {
		t.Fatal("Resend configuration was not trimmed and loaded")
	}
}

func TestLoadAPIHost(t *testing.T) {
	clearEnv(t)
	t.Setenv("API_HOST", " 127.0.0.1 ")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIHost != "127.0.0.1" {
		t.Fatalf("APIHost = %q, want 127.0.0.1", cfg.APIHost)
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	for _, port := range []string{"nope", "0", "65536"} {
		t.Run(port, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("PORT", port)
			if _, err := Load(); err == nil {
				t.Fatal("Load accepted invalid port")
			}
		})
	}
}

func TestLoadAdminAllowlistAndLegacyKey(t *testing.T) {
	clearEnv(t)
	t.Setenv("SUPABASE_URL", "https://example.supabase.co/")
	t.Setenv("SUPABASE_ANON_KEY", "public-anon-key")
	t.Setenv("ADMIN_USER_IDS", "11111111-1111-4111-8111-111111111111, ABCDEFAB-CDEF-4ABC-8DEF-ABCDEFABCDEF")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SupabaseURL != "https://example.supabase.co" || cfg.SupabasePublishableKey != "public-anon-key" || len(cfg.AdminUserIDs) != 2 {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if _, ok := cfg.AdminUserIDs["abcdefab-cdef-4abc-8def-abcdefabcdef"]; !ok {
		t.Fatal("ADMIN_USER_IDS was not normalized to lowercase UUIDs")
	}
}

func TestLoadRejectsInvalidOrIncompleteAdminConfig(t *testing.T) {
	t.Run("invalid uuid", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("ADMIN_USER_IDS", "not-a-uuid")
		if _, err := Load(); err == nil {
			t.Fatal("Load accepted invalid allowlist")
		}
	})
	t.Run("missing URL and key", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("ADMIN_USER_IDS", "11111111-1111-4111-8111-111111111111")
		if _, err := Load(); err == nil {
			t.Fatal("Load accepted incomplete Auth config")
		}
	})
}

func TestLoadCoachAllowlist(t *testing.T) {
	clearEnv(t)
	t.Setenv("SUPABASE_URL", "https://example.supabase.co")
	t.Setenv("SUPABASE_PUBLISHABLE_KEY", "public-key")
	t.Setenv("COACH_USER_IDS", "ABCDEFAB-CDEF-4ABC-8DEF-ABCDEFABCDEF")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.CoachUserIDs["abcdefab-cdef-4abc-8def-abcdefabcdef"]; !ok {
		t.Fatalf("coach allowlist was not normalized: %#v", cfg.CoachUserIDs)
	}
}

func TestLoadRejectsInvalidOrIncompleteCoachConfig(t *testing.T) {
	t.Run("invalid uuid", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("COACH_USER_IDS", "not-a-uuid")
		if _, err := Load(); err == nil {
			t.Fatal("Load accepted invalid coach allowlist")
		}
	})
	t.Run("missing Supabase configuration", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("COACH_USER_IDS", "11111111-1111-4111-8111-111111111111")
		if _, err := Load(); err == nil {
			t.Fatal("Load accepted a coach allowlist without Supabase Auth configuration")
		}
	})
}
