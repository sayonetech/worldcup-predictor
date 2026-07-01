package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadReadsEnvAndAppliesDefaults(t *testing.T) {
	t.Setenv("HTTP_PORT", "")
	t.Setenv("APP_ENV", "")
	t.Setenv("SESSION_SECRET", "secret")
	t.Setenv("GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("ALLOWED_EMAIL_DOMAIN", "sayonetech.com")
	t.Setenv("SEED_ADMIN_EMAILS", "a@sayonetech.com, b@sayonetech.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPPort != "8000" {
		t.Errorf("HTTPPort default = %q, want 8000", cfg.HTTPPort)
	}
	if cfg.AppEnv != "development" {
		t.Errorf("AppEnv default = %q, want development", cfg.AppEnv)
	}
	if cfg.IsProduction() {
		t.Errorf("IsProduction() = true, want false for development")
	}
	if len(cfg.SeedAdminEmails) != 2 || cfg.SeedAdminEmails[0] != "a@sayonetech.com" {
		t.Errorf("SeedAdminEmails = %v, want trimmed 2-element slice", cfg.SeedAdminEmails)
	}
}

func TestLoadRequiresSessionSecret(t *testing.T) {
	t.Setenv("SESSION_SECRET", "")
	t.Setenv("GOOGLE_CLIENT_ID", "client-id")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error for missing SESSION_SECRET")
	}
}

func TestLoadRequiresGoogleClientID(t *testing.T) {
	t.Setenv("SESSION_SECRET", "secret")
	t.Setenv("GOOGLE_CLIENT_ID", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error for missing GOOGLE_CLIENT_ID")
	}
}

func TestLoadSeedDataDirDefault(t *testing.T) {
	t.Setenv("SESSION_SECRET", "secret")
	t.Setenv("GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("SEED_DATA_DIR", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.SeedDataDir != "./data" {
		t.Errorf("SeedDataDir default = %q, want ./data", cfg.SeedDataDir)
	}

	t.Setenv("SEED_DATA_DIR", "/srv/data")
	cfg, _ = Load()
	if cfg.SeedDataDir != "/srv/data" {
		t.Errorf("SeedDataDir override = %q, want /srv/data", cfg.SeedDataDir)
	}
}

func TestConfig_GameDefaults(t *testing.T) {
	// Required vars so Load() succeeds.
	t.Setenv("SESSION_SECRET", "x")
	t.Setenv("GOOGLE_CLIENT_ID", "y")
	// Ensure GAME_* vars are unset so defaults apply.
	_ = os.Unsetenv("GAME_TOKEN_TTL")
	_ = os.Unsetenv("GAME_DURATION_SLACK_MS")
	_ = os.Unsetenv("GAME_DIST_EPS_M")
	_ = os.Unsetenv("GAME_DIST_EPS_FRAC")
	_ = os.Unsetenv("GAME_COIN_MIN_SPACING_M")
	_ = os.Unsetenv("GAME_COIN_SLACK")
	_ = os.Unsetenv("GAME_MAX_DISTANCE")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.GameTokenTTL != 10*time.Minute {
		t.Errorf("GameTokenTTL default = %v, want 10m", cfg.GameTokenTTL)
	}
	if cfg.GameDurationSlackMs != 1500 {
		t.Errorf("GameDurationSlackMs default = %v, want 1500", cfg.GameDurationSlackMs)
	}
	if cfg.GameDistEpsM != 25 {
		t.Errorf("GameDistEpsM default = %v, want 25", cfg.GameDistEpsM)
	}
	if cfg.GameDistEpsFrac != 0.02 {
		t.Errorf("GameDistEpsFrac default = %v, want 0.02", cfg.GameDistEpsFrac)
	}
	if cfg.GameCoinMinSpacingM != 300 {
		t.Errorf("GameCoinMinSpacingM default = %v, want 300", cfg.GameCoinMinSpacingM)
	}
	if cfg.GameCoinSlack != 3 {
		t.Errorf("GameCoinSlack default = %v, want 3", cfg.GameCoinSlack)
	}
	if cfg.GameMaxDistance != 0 {
		t.Errorf("GameMaxDistance default = %v, want 0", cfg.GameMaxDistance)
	}
}

func TestLoad_OpenAIDefaults(t *testing.T) {
	// Required vars so Load() succeeds.
	t.Setenv("SESSION_SECRET", "x")
	t.Setenv("GOOGLE_CLIENT_ID", "y")
	// Ensure OpenAI vars are unset for the default case.
	_ = os.Unsetenv("OPENAI_API_KEY")
	_ = os.Unsetenv("OPENAI_SYSTEM_PROMPT_FILE")
	_ = os.Unsetenv("OPENAI_MODEL")
	_ = os.Unsetenv("OPENAI_TEMPERATURE")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.OpenAIAPIKey != "" {
		t.Errorf("OpenAIAPIKey = %q, want empty", c.OpenAIAPIKey)
	}
	if c.OpenAISystemPromptFile != "" {
		t.Errorf("OpenAISystemPromptFile = %q, want empty", c.OpenAISystemPromptFile)
	}
	if c.OpenAIModel != "gpt-4.1-mini-2025-04-14" {
		t.Errorf("OpenAIModel = %q, want gpt-4.1-mini-2025-04-14", c.OpenAIModel)
	}
	if c.OpenAITemperature != 0.8 {
		t.Errorf("OpenAITemperature = %v, want 0.8", c.OpenAITemperature)
	}
}
