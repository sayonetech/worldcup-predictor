package httpapi

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter wires the API routes. The trailing bool is ignored (it formerly
// gated the dev-only job-run route, now registered in all environments); it is
// kept so existing call sites/tests need no change.
func NewRouter(d *Deps, _ bool) chi.Router {
	// Initialise the in-memory JTI set for the GOAT game if it hasn't been set
	// up already (e.g. in tests that call initGameJTISet directly).
	if d.gameJTI == nil {
		d.initGameJTISet()
	}

	r := chi.NewRouter()
	// RealIP normalizes r.RemoteAddr from proxy headers so the per-IP auth limiter
	// keys on the real client. SA1019 flags X-Forwarded-For spoofing risk; that's
	// acceptable here — this internal app runs behind a trusted nginx that sets the
	// header, and the limiter is a best-effort throttle, not an authz boundary.
	r.Use(middleware.RealIP) //nolint:staticcheck // trusted reverse proxy; see comment above
	r.Use(middleware.Recoverer)

	r.Get("/healthz", Healthz)

	// API reference (Scalar) + the OpenAPI document it renders. Public.
	r.Get("/docs", GetDocs)
	r.Get("/openapi.yaml", GetOpenAPISpec)

	authLimiter     := newKeyedLimiter(authRate, authBurst)
	writeLimiter    := newKeyedLimiter(writeRate, writeBurst)
	chatLimiter     := newKeyedLimiter(chatRate, chatBurst)
	gameReadLimiter := newKeyedLimiter(gameReadRate, gameReadBurst)

	r.Route("/api", func(api chi.Router) {
		api.Use(maxBodyBytes(maxBodyBytesLimit))
		api.With(rateLimitIP(authLimiter)).Post("/auth/google", d.PostAuthGoogle)
		api.With(rateLimitIP(authLimiter)).Post("/auth/logout", d.PostAuthLogout)

		api.Group(func(priv chi.Router) {
			priv.Use(d.RequireAuth)
			priv.Use(rateLimitWrites(writeLimiter)) // after RequireAuth: needs user in context
			priv.Get("/me", d.GetMe)
			priv.Get("/matches", d.GetMatches)
			priv.Put("/matches/{id}/prediction", d.PutPrediction)
			priv.Get("/matches/{id}/predictions", d.GetMatchPredictions)
			priv.Get("/leaderboard", d.GetLeaderboard)
			priv.Get("/winners", d.GetWinners)
			priv.Get("/celebrations", d.GetCelebrations)
			priv.Post("/celebrations/seen", d.PostCelebrationsSeen)
			priv.With(d.RequireAdmin).Put("/admin/winners/paid", d.PutWinnerPaid)

			priv.Get("/bonus", d.GetBonus)
			priv.Put("/bonus", d.PutBonus)
			priv.Get("/teams", d.GetTeams)
			priv.Get("/players", d.GetPlayers)
			priv.With(d.RequireAdmin).Get("/admin/bonus/results", d.GetBonusResults)
			priv.With(d.RequireAdmin).Put("/admin/bonus/results", d.PutBonusResults)

			// Admin match management (all environments — not debug-gated).
			priv.With(d.RequireAdmin).Get("/admin/matches", d.GetAdminMatches)
			priv.With(d.RequireAdmin).Post("/admin/matches", d.PostAdminMatch)
			priv.With(d.RequireAdmin).Put("/admin/matches/{id}", d.PutAdminMatch)
			priv.With(d.RequireAdmin).Put("/admin/matches/{id}/result", d.PutAdminMatchResult)
			priv.With(d.RequireAdmin).Delete("/admin/matches/{id}", d.DeleteAdminMatch)

			// Admin user management (all environments — not debug-gated).
			priv.With(d.RequireAdmin).Get("/admin/users", d.GetAdminUsers)
			priv.With(d.RequireAdmin).Post("/admin/users/{id}/role", d.PostUserRole)

			// Admin settings + recompute (all environments — not debug-gated).
			priv.With(d.RequireAdmin).Get("/admin/settings", d.GetAdminSettings)
			priv.With(d.RequireAdmin).Put("/admin/settings", d.PutAdminSettings)
			priv.With(d.RequireAdmin).Post("/admin/recompute", d.PostRecompute)

			// Manual job trigger — admin-only, registered in all environments so an
			// admin can run a missed cron (e.g. results-ingest) from production.
			priv.With(d.RequireAdmin).Post("/admin/jobs/run", d.PostRunJob)

			priv.With(rateLimitWrites(chatLimiter)).Post("/chat", d.PostChat)

			// GOAT mini-game (§3.10 / §11)
			// GET issues a single-use run token on every call so it carries its own
			// per-user rate limiter (rateLimitWrites skips GET). POST inherits the
			// group-level write limiter (rateLimitWrites) applied above.
			priv.With(rateLimitUser(gameReadLimiter)).Get("/game/leaderboard", d.GetGameLeaderboard)
			priv.Post("/game/runs", d.PostGameRun)
		})
	})

	return r
}
