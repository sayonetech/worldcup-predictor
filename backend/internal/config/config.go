// Package config loads 12-factor environment configuration.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv             string
	HTTPPort           string
	DBHost             string
	DBPort             string
	DBUser             string
	DBPassword         string
	DBName             string
	SessionSecret      string
	GoogleClientID     string
	AllowedEmailDomain string
	SeedAdminEmails    []string
	SeedDataDir        string

	FootballDataAPIKey  string
	FootballDataBaseURL string
	ResultsCron         string
	WeeklyCron          string
	// ResultsCronEnabled gates the SCHEDULED results-ingest job. When false the
	// cron does not start, but the manual admin trigger still works (debug). The
	// Docker stack sets this false so demo data isn't auto-overwritten.
	ResultsCronEnabled bool

	SlackWebhookURL string

	OpenAIAPIKey           string
	OpenAISystemPromptFile string
	OpenAIModel            string
	OpenAITemperature      float64

	BonusLockAt time.Time

	// GOAT mini-game config (§14 / §18).
	GameTokenTTL        time.Duration
	GameDurationSlackMs float64
	GameDistEpsM        float64
	GameDistEpsFrac     float64
	GameCoinMinSpacingM int
	GameCoinSlack       int
	GameMaxDistance     int
}

func (c Config) IsProduction() bool { return c.AppEnv == "production" }

// DSN returns a go-sql-driver/mysql DSN (parseTime so DATETIME scans into time.Time).
func (c Config) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&loc=UTC",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
}

func Load() (Config, error) {
	c := Config{
		AppEnv:             getenv("APP_ENV", "development"),
		HTTPPort:           getenv("HTTP_PORT", "8000"),
		DBHost:             getenv("DB_HOST", "127.0.0.1"),
		DBPort:             getenv("DB_PORT", "3306"),
		DBUser:             getenv("DB_USER", "wcp"),
		DBPassword:         getenv("DB_PASSWORD", "wcp"),
		DBName:             getenv("DB_NAME", "wcp"),
		SessionSecret:      os.Getenv("SESSION_SECRET"),
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		AllowedEmailDomain: getenv("ALLOWED_EMAIL_DOMAIN", "sayonetech.com"),
		SeedAdminEmails:    splitTrim(os.Getenv("SEED_ADMIN_EMAILS")),
		// Static WC dataset (committed CSVs). Override only if data/ moves.
		SeedDataDir: getenv("SEED_DATA_DIR", "./data"),

		FootballDataAPIKey:  os.Getenv("FOOTBALL_DATA_API_KEY"),
		FootballDataBaseURL: getenv("FOOTBALL_DATA_BASE_URL", "https://api.football-data.org/v4"),
		ResultsCron:         getenv("RESULTS_CRON", "0 3,8,13 * * *"),
		WeeklyCron:          getenv("WEEKLY_CRON", "0 9 * * 1"),
		ResultsCronEnabled:  getbool("RESULTS_CRON_ENABLED", true),
		// Optional Slack Incoming Webhook for cron-completion notifications.
		// Empty disables Slack (jobs still run; nothing is posted).
		SlackWebhookURL: os.Getenv("SLACK_WEBHOOK_URL"),

		OpenAIAPIKey:           os.Getenv("OPENAI_API_KEY"),
		OpenAISystemPromptFile: os.Getenv("OPENAI_SYSTEM_PROMPT_FILE"),
		OpenAIModel:            getenv("OPENAI_MODEL", "gpt-4.1-mini-2025-04-14"),
		OpenAITemperature:      getfloat("OPENAI_TEMPERATURE", 0.8),

		// GOAT mini-game knobs (§14 defaults).
		GameTokenTTL:        getduration("GAME_TOKEN_TTL", 10*time.Minute),
		GameDurationSlackMs: getfloat("GAME_DURATION_SLACK_MS", 1500),
		GameDistEpsM:        getfloat("GAME_DIST_EPS_M", 25),
		GameDistEpsFrac:     getfloat("GAME_DIST_EPS_FRAC", 0.02),
		GameCoinMinSpacingM: getint("GAME_COIN_MIN_SPACING_M", 300),
		GameCoinSlack:       getint("GAME_COIN_SLACK", 3),
		GameMaxDistance:     getint("GAME_MAX_DISTANCE", 0),
	}
	lockStr := getenv("BONUS_LOCK_AT", "2026-06-28T23:59:00+05:30")
	lockAt, err := time.Parse(time.RFC3339, lockStr)
	if err != nil {
		return Config{}, fmt.Errorf("config: BONUS_LOCK_AT must be RFC3339: %w", err)
	}
	c.BonusLockAt = lockAt

	if c.SessionSecret == "" {
		return Config{}, fmt.Errorf("config: SESSION_SECRET is required")
	}
	if c.GoogleClientID == "" {
		return Config{}, fmt.Errorf("config: GOOGLE_CLIENT_ID is required")
	}
	return c, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// getfloat parses a float env var. Empty/unparseable falls back to def.
func getfloat(key string, def float64) float64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

// getbool parses a boolean env var. Empty/unrecognised falls back to def.
func getbool(key string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

// getduration parses a Go duration string env var. Empty/unparseable falls back to def.
func getduration(key string, def time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// getint parses an integer env var. Empty/unparseable falls back to def.
func getint(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

func splitTrim(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
