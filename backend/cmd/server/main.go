package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sayonetech/worldcup-predictor/backend/internal/auth"
	"github.com/sayonetech/worldcup-predictor/backend/internal/chat"
	"github.com/sayonetech/worldcup-predictor/backend/internal/config"
	"github.com/sayonetech/worldcup-predictor/backend/internal/game"
	"github.com/sayonetech/worldcup-predictor/backend/internal/httpapi"
	"github.com/sayonetech/worldcup-predictor/backend/internal/jobs"
	"github.com/sayonetech/worldcup-predictor/backend/internal/notify"
	"github.com/sayonetech/worldcup-predictor/backend/internal/settings"
	"github.com/sayonetech/worldcup-predictor/backend/internal/sportsapi"
	"github.com/sayonetech/worldcup-predictor/backend/internal/store"

	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Load .env for local dev. godotenv does not override variables already set
	// in the real environment, so production (env-injected) is unaffected and a
	// missing .env file is a no-op.
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config", "err", err)
		os.Exit(1)
	}

	db, err := store.OpenMySQL(cfg.DSN())
	if err != nil {
		logger.Error("db", "err", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	st := store.New(db)
	seedAdmins(context.Background(), st, cfg.SeedAdminEmails, logger)

	// Build the settings service and seed missing keys from env defaults on boot.
	settingsSvc := &settings.Service{
		Store: st,
		Defaults: map[string]string{
			settings.KeyResultsCron: cfg.ResultsCron,
			settings.KeyWeeklyCron:  cfg.WeeklyCron,
			settings.KeyBonusLockAt: cfg.BonusLockAt.Format(time.RFC3339),
		},
	}
	if err := settingsSvc.EnsureSeeded(context.Background()); err != nil {
		logger.Warn("settings seed failed (continuing)", "err", err)
	}

	// Read cron expressions from settings at boot (DB overrides env if seeded).
	bootCtx := context.Background()
	resultsCronExpr, err := settingsSvc.Get(bootCtx, settings.KeyResultsCron)
	if err != nil || resultsCronExpr == "" {
		resultsCronExpr = cfg.ResultsCron
	}
	weeklyCronExpr, err := settingsSvc.Get(bootCtx, settings.KeyWeeklyCron)
	if err != nil || weeklyCronExpr == "" {
		weeklyCronExpr = cfg.WeeklyCron
	}

	// Optional Slack notifier for cron-completion + manual job-run status.
	notifier := notify.NewSlack(cfg.SlackWebhookURL)
	if notifier.Enabled() {
		logger.Info("slack notifications enabled")
	}

	weekly := jobs.WeeklyWinner{Store: st, Now: func() time.Time { return time.Now().UTC() }}
	sj := serverJobs{weekly: weekly, bonus: jobs.BonusScore{Store: st}, notify: notifier}
	if cfg.FootballDataAPIKey != "" {
		if alias, err := loadAliasFile(cfg.SeedDataDir + "/fd_team_aliases.csv"); err == nil {
			ingest := jobs.ResultsIngest{
				API:   sportsapi.New(cfg.FootballDataBaseURL, cfg.FootballDataAPIKey),
				Store: st,
				Now:   func() time.Time { return time.Now().UTC() },
				Alias: alias,
			}
			sj.ingest = &ingest
		} else {
			logger.Warn("results ingest trigger disabled: alias load", "err", err)
		}
	}
	var jobRunner httpapi.JobRunner = sj

	// Recompute job + adapter implementing httpapi.RecomputeRunner.
	recomputeJob := jobs.Recompute{Store: st, Bonus: jobs.BonusScore{Store: st}}

	// Chat assistant (optional). Enabled only when an API key and a readable
	// prompt file are both present; otherwise POST /api/chat returns 503.
	var chatClient chat.Streamer
	if cfg.OpenAIAPIKey != "" && cfg.OpenAISystemPromptFile != "" {
		prompt, err := chat.LoadSystemPrompt(cfg.OpenAISystemPromptFile)
		if err != nil {
			logger.Warn("chat disabled: cannot read OPENAI_SYSTEM_PROMPT_FILE", "path", cfg.OpenAISystemPromptFile, "err", err)
		} else {
			chatClient = chat.New(cfg.OpenAIAPIKey, cfg.OpenAIModel, prompt, cfg.OpenAITemperature)
			logger.Info("chat assistant enabled", "model", cfg.OpenAIModel, "temperature", cfg.OpenAITemperature)
		}
	} else {
		logger.Info("chat assistant disabled (set OPENAI_API_KEY + OPENAI_SYSTEM_PROMPT_FILE to enable)")
	}

	deps := &httpapi.Deps{
		Store:              st,
		Matches:            st,
		Predictions:        st,
		Leaderboard:        st,
		Bonus:              st,
		Players:            st,
		AdminMatches:       st,
		AdminUsers:         st,
		Results:            st,
		Celebrations:       st,
		Chat:               chatClient,
		Settings:           settingsSvc,
		Recompute:          recomputeAdapter{r: recomputeJob},
		JobRunner:          jobRunner,
		Sessions:           auth.NewSessionManager(cfg.SessionSecret),
		Verifier:           auth.GoogleTokenVerifier{ClientID: cfg.GoogleClientID},
		AllowedEmailDomain: cfg.AllowedEmailDomain,
		Secure:             cfg.IsProduction(),
		Game:               st,
		GameTokens:         game.NewTokenManager(cfg.SessionSecret, cfg.GameTokenTTL, func() time.Time { return time.Now().UTC() }),
		GameTokenTTL:       cfg.GameTokenTTL,
		GameLimits: game.Limits{
			DurationSlackMs: cfg.GameDurationSlackMs,
			DistEpsM:        cfg.GameDistEpsM,
			DistEpsFrac:     cfg.GameDistEpsFrac,
			CoinMinSpacingM: cfg.GameCoinMinSpacingM,
			CoinSlack:       cfg.GameCoinSlack,
			MaxDistance:     cfg.GameMaxDistance,
		},
	}

	router := httpapi.NewRouter(deps, !cfg.IsProduction())

	srv := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Results-ingest scheduler (in-process). Only runs when an API key is set;
	// local dev without a key still boots.
	scheduler := startResultsCron(resultsCronExpr, cfg, st, notifier, logger)
	if scheduler != nil {
		defer scheduler.Stop()
	}

	weeklyScheduler := startWeeklyCron(weeklyCronExpr, cfg, weekly, notifier, logger)
	if weeklyScheduler != nil {
		defer weeklyScheduler.Stop()
	}

	go func() {
		logger.Info("listening", "port", cfg.HTTPPort, "env", cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	logger.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown", "err", err)
	}
}

// startResultsCron builds the results-ingest job and schedules it on cronExpr
// (IST). Falls back to cfg.ResultsCron if cronExpr is empty. Returns nil (and
// logs) when no API key is configured or the cron expression is invalid.
func startResultsCron(cronExpr string, cfg config.Config, st *store.SQLStore, notifier notify.Slack, logger *slog.Logger) *cron.Cron {
	if !cfg.ResultsCronEnabled {
		logger.Info("results-ingest schedule disabled (RESULTS_CRON_ENABLED=false); manual trigger still available")
		return nil
	}
	if cfg.FootballDataAPIKey == "" {
		logger.Info("results-ingest disabled (no FOOTBALL_DATA_API_KEY)")
		return nil
	}
	if cronExpr == "" {
		cronExpr = cfg.ResultsCron
	}
	alias, err := loadAliasFile(cfg.SeedDataDir + "/fd_team_aliases.csv")
	if err != nil {
		logger.Error("results-ingest disabled: load alias file", "err", err)
		return nil
	}
	job := jobs.ResultsIngest{
		API:   sportsapi.New(cfg.FootballDataBaseURL, cfg.FootballDataAPIKey),
		Store: st,
		Now:   func() time.Time { return time.Now().UTC() },
		Alias: alias,
	}
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		loc = time.FixedZone("IST", 5*3600+1800)
	}
	c := cron.New(cron.WithLocation(loc))
	if _, err := c.AddFunc(cronExpr, func() {
		ctx := context.Background()
		sum, err := job.Run(ctx)
		if err != nil {
			logger.Error("results-ingest run", "err", err)
		}
		notifyJob(ctx, notifier, "scheduled", sum, err)
	}); err != nil {
		logger.Error("results-ingest disabled: bad RESULTS_CRON", "spec", cronExpr, "err", err)
		return nil
	}
	c.Start()
	logger.Info("results-ingest scheduled", "cron", cronExpr, "tz", loc.String())
	return c
}

// startWeeklyCron schedules the weekly-winner job on cronExpr (IST). Falls back
// to cfg.WeeklyCron if cronExpr is empty. Always runs (no external API needed).
func startWeeklyCron(cronExpr string, cfg config.Config, job jobs.WeeklyWinner, notifier notify.Slack, logger *slog.Logger) *cron.Cron {
	if cronExpr == "" {
		cronExpr = cfg.WeeklyCron
	}
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		loc = time.FixedZone("IST", 5*3600+1800)
	}
	c := cron.New(cron.WithLocation(loc))
	if _, err := c.AddFunc(cronExpr, func() {
		ctx := context.Background()
		sum, err := job.Run(ctx)
		if err != nil {
			logger.Error("weekly-winner run", "err", err)
		}
		notifyJob(ctx, notifier, "scheduled", sum, err)
	}); err != nil {
		logger.Error("weekly-winner disabled: bad WEEKLY_CRON", "spec", cronExpr, "err", err)
		return nil
	}
	c.Start()
	logger.Info("weekly-winner scheduled", "cron", cronExpr, "tz", loc.String())
	return c
}

// recomputeAdapter wraps jobs.Recompute to satisfy httpapi.RecomputeRunner.
type recomputeAdapter struct{ r jobs.Recompute }

func (a recomputeAdapter) Run(ctx context.Context) (any, error) {
	return a.r.Run(ctx)
}

// loadAliasFile opens + parses the fd-team-id alias CSV.
func loadAliasFile(path string) (map[int64]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return jobs.LoadAliases(f)
}

// serverJobs adapts the background jobs to httpapi.JobRunner. ingest is nil when
// no results API key is configured; weekly-winner and bonus-score always work
// (no external API).
type serverJobs struct {
	ingest *jobs.ResultsIngest
	weekly jobs.WeeklyWinner
	bonus  jobs.BonusScore
	notify notify.Slack
}

func (s serverJobs) RunResultsIngest(ctx context.Context) (any, error) {
	if s.ingest == nil {
		return nil, errors.New("results ingest not configured (no FOOTBALL_DATA_API_KEY)")
	}
	sum, err := s.ingest.Run(ctx)
	notifyJob(ctx, s.notify, "manual", sum, err)
	return sum, err
}

func (s serverJobs) RunWeeklyWinner(ctx context.Context) (any, error) {
	sum, err := s.weekly.Run(ctx)
	notifyJob(ctx, s.notify, "manual", sum, err)
	return sum, err
}

func (s serverJobs) RunBonusScore(ctx context.Context) (any, error) {
	sum, err := s.bonus.Run(ctx)
	notifyJob(ctx, s.notify, "manual", sum, err)
	return sum, err
}

// notifyJob posts a friendly job-completion status to Slack (no-op without a
// webhook). `trigger` is "scheduled" or "manual"; the human-readable job title
// and detail are derived from the summary type. Timestamp is rendered in IST.
func notifyJob(ctx context.Context, n notify.Slack, trigger string, summary any, runErr error) {
	if !n.Enabled() {
		return
	}
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		loc = time.FixedZone("IST", 5*3600+1800)
	}
	ts := time.Now().In(loc).Format("Mon 02 Jan 2006, 3:04 PM IST")
	title, detail := jobMessage(summary)
	if runErr != nil {
		n.Send(ctx, fmt.Sprintf(":x: *SayScore — %s* (%s run) failed at %s\n> %v", title, trigger, ts, runErr))
		return
	}
	n.Send(ctx, fmt.Sprintf(":white_check_mark: *SayScore — %s* (%s run) completed at %s\n> %s", title, trigger, ts, detail))
}

// jobMessage turns a job's result summary into a human title + one-line detail.
func jobMessage(summary any) (title, detail string) {
	switch s := summary.(type) {
	case jobs.Summary: // results-ingest
		return "Match results sync",
			fmt.Sprintf("Checked football-data for finished matches — %d found, %d result(s) applied to fixtures, %d prediction(s) scored%s.",
				s.Fetched, s.Updated, s.PredictionsScored, optSkipped(s.Skipped))
	case jobs.WeeklySummary: // weekly-winner
		return "Weekly winner",
			fmt.Sprintf("Declared the week of %s — %d winner(s) from %d participant(s).",
				s.WeekStart, s.Winners, s.Participants)
	case jobs.BonusSummary: // bonus-score
		return "Tournament bonus scoring",
			fmt.Sprintf("Re-scored tournament bonus picks against the declared outcomes — %d pick(s) updated.", s.Scored)
	default:
		// Unknown summary type: never serialise the struct (would leak field
		// values to Slack). Emit a safe, generic line — known jobs are handled
		// above; this only fires if a new job type is added without a case.
		return "Background job", "Completed (see server logs for details)."
	}
}

func optSkipped(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(", %d skipped (unaligned/already corrected)", n)
}

// seedAdmins promotes any already-existing user in the seed list to admin and
// pre-creates the rows so the first login of a seed admin is already elevated.
func seedAdmins(ctx context.Context, st store.Store, emails []string, logger *slog.Logger) {
	for _, email := range emails {
		u, err := st.UpsertUser(ctx, store.UpsertUserParams{Email: email, Role: store.RoleAdmin})
		if err != nil {
			logger.Warn("seed admin failed", "email", email, "err", err)
			continue
		}
		if u.Role != store.RoleAdmin {
			if err := st.SetUserRole(ctx, u.ID, store.RoleAdmin); err != nil {
				logger.Warn("promote seed admin failed", "email", email, "err", err)
			}
		}
	}
}
