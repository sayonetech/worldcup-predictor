package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/sayonetech/worldcup-predictor/backend/internal/auth"
	"github.com/sayonetech/worldcup-predictor/backend/internal/chat"
	"github.com/sayonetech/worldcup-predictor/backend/internal/game"
	"github.com/sayonetech/worldcup-predictor/backend/internal/store"
)

const sessionCookieName = "sayscore_session"
const sessionTTL = 7 * 24 * time.Hour

// sessionRefreshThreshold: re-issue the cookie once more than this duration has
// elapsed since it was issued — i.e. when remaining TTL < sessionTTL - 24h (≈6d).
// This slides the 7-day window forward on activity so active users stay logged in;
// idle users whose cookie is never refreshed expire naturally after 7 days.
const sessionRefreshThreshold = 24 * time.Hour

type ctxKey int

const userCtxKey ctxKey = iota

// SettingsProvider is the handler-facing interface for runtime-editable settings.
// *settings.Service satisfies this interface.
type SettingsProvider interface {
	BonusLockAt(ctx context.Context) (time.Time, error)
	All(ctx context.Context) (map[string]string, error)
	SetAll(ctx context.Context, kv map[string]string) error
}

// RecomputeRunner triggers the idempotent points recompute job.
// An adapter wrapping jobs.Recompute satisfies this interface.
type RecomputeRunner interface {
	Run(ctx context.Context) (any, error)
}

// Deps holds everything the HTTP layer needs. Built in cmd/server.
type Deps struct {
	Store              store.Store
	Matches            store.MatchStore
	Predictions        store.PredictionStore
	Leaderboard        store.LeaderboardStore
	Bonus              store.BonusStore
	Players            store.PlayerStore
	AdminMatches       store.AdminMatchStore
	AdminUsers         store.AdminUserStore
	Results            store.ResultsStore
	Celebrations       store.CelebrationStore
	Chat               chat.Streamer
	Settings           SettingsProvider
	Recompute          RecomputeRunner
	JobRunner          JobRunner
	Sessions           *auth.SessionManager
	Verifier           auth.TokenVerifier
	AllowedEmailDomain string
	Secure             bool // Secure flag on the cookie (false for local http)

	// GOAT mini-game (§3.10 / §11)
	Game         store.GameStore
	GameTokens   *game.TokenManager
	GameLimits   game.Limits
	GameTokenTTL time.Duration
	gameJTI      *seenJTI
}

func userFromContext(ctx context.Context) (store.User, bool) {
	u, ok := ctx.Value(userCtxKey).(store.User)
	return u, ok
}

// RequireAuth loads the user from the session cookie or returns 401.
func (d *Deps) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		sess, err := d.Sessions.Decode(c.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid session")
			return
		}
		u, err := d.Store.GetUserByID(r.Context(), sess.UserID)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "user not found")
			return
		}
		// Sliding refresh: re-issue the cookie once sessionRefreshThreshold (24h)
		// has elapsed since it was issued (remaining TTL < sessionTTL - 24h ≈ 6d),
		// sliding the 7-day window forward on activity. Idle users expire naturally.
		if time.Unix(sess.ExpiresAt, 0).Sub(now()) < sessionTTL-sessionRefreshThreshold {
			d.setSessionCookie(w, u.ID)
		}
		ctx := context.WithValue(r.Context(), userCtxKey, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// JobRunner runs a named background job on demand (debug trigger). nil in prod.
type JobRunner interface {
	RunResultsIngest(ctx context.Context) (any, error)
	RunWeeklyWinner(ctx context.Context) (any, error)
	RunBonusScore(ctx context.Context) (any, error)
}

// RequireAdmin must follow RequireAuth; it 403s non-admin users.
func (d *Deps) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := userFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		if u.Role != store.RoleAdmin {
			writeError(w, http.StatusForbidden, "admin only")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (d *Deps) setSessionCookie(w http.ResponseWriter, userID int64) {
	token := d.Sessions.Encode(auth.Session{UserID: userID}, sessionTTL)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   d.Secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

// maxBodyBytes caps request body size; an over-limit body makes the handler's
// json.Decode fail (→ 400). Applied once on the /api group.
func maxBodyBytes(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}

const maxBodyBytesLimit int64 = 1 << 20 // 1 MiB

func (d *Deps) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/",
		HttpOnly: true, Secure: d.Secure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
