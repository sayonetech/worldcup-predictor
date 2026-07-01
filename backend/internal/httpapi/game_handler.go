package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sayonetech/worldcup-predictor/backend/internal/game"
)

// firstName returns the first token of the user's name for the small game
// leaderboard discs. When there's no stored name, it derives one from the
// email's local part ("hiba.kareem@sayonetech.com" -> "Hiba") instead of
// showing "Unknown".
func firstName(name, email string) string {
	if f := strings.Fields(name); len(f) > 0 {
		return f[0]
	}
	local := email
	if i := strings.IndexByte(local, '@'); i >= 0 {
		local = local[:i]
	}
	for _, p := range strings.FieldsFunc(local, func(r rune) bool { return r == '.' || r == '_' || r == '-' }) {
		if p != "" {
			return strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return "Player"
}

// seenJTI is a single-use guard for run-token jtis (in-memory, single-instance —
// consistent with the rate limiter). Entries expire after the token TTL.
type seenJTI struct {
	mu  sync.Mutex
	at  map[string]time.Time
	ttl time.Duration
	now func() time.Time
}

func (s *seenJTI) consume(jti string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for k, t := range s.at { // opportunistic GC
		if now.Sub(t) > s.ttl {
			delete(s.at, k)
		}
	}
	if _, used := s.at[jti]; used {
		return false
	}
	s.at[jti] = now
	return true
}

func (d *Deps) initGameJTISet() {
	now := func() time.Time { return time.Now().UTC() }
	ttl := d.GameTokenTTL
	if ttl <= 0 {
		ttl = 10 * time.Minute // safe default when unset (e.g. in unit tests)
	}
	ttl += 5 * time.Minute // margin beyond token validity
	d.gameJTI = &seenJTI{at: map[string]time.Time{}, ttl: ttl, now: now}
}

func newJTI() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type gameBoardRowDTO struct {
	UserID    int64  `json:"user_id"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Team      string `json:"team,omitempty"`
	Distance  int64  `json:"distance,omitempty"`
	Coins     int64  `json:"coins,omitempty"`
}

type gameLeaderboardResponse struct {
	Distance []gameBoardRowDTO `json:"distance"`
	Coins    []gameBoardRowDTO `json:"coins"`
	Me       struct {
		BestDistance int64 `json:"best_distance"`
		CoinPool     int64 `json:"coin_pool"`
	} `json:"me"`
	RunToken string `json:"run_token"`
}

func (d *Deps) GetGameLeaderboard(w http.ResponseWriter, r *http.Request) {
	u, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	dist, err := d.Game.GameDistanceBoard(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load game boards")
		return
	}
	coins, err := d.Game.GameCoinBoard(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load game boards")
		return
	}
	me, err := d.Game.GameMeStanding(r.Context(), u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load game boards")
		return
	}
	// Initialise to empty (non-nil) slices so an empty board marshals as JSON
	// `[]`, not `null` — the client maps over these arrays unconditionally.
	resp := gameLeaderboardResponse{
		Distance: make([]gameBoardRowDTO, 0, len(dist)),
		Coins:    make([]gameBoardRowDTO, 0, len(coins)),
	}
	// Show only the first name on the (small) leaderboard discs; derive from the
	// email when there's no stored name.
	for _, row := range dist {
		resp.Distance = append(resp.Distance, gameBoardRowDTO{UserID: row.UserID, Name: firstName(row.Name, row.Email), AvatarURL: row.AvatarURL, Team: teamForEmail(row.Email), Distance: row.Distance})
	}
	for _, row := range coins {
		resp.Coins = append(resp.Coins, gameBoardRowDTO{UserID: row.UserID, Name: firstName(row.Name, row.Email), AvatarURL: row.AvatarURL, Team: teamForEmail(row.Email), Coins: row.Coins})
	}
	resp.Me.BestDistance, resp.Me.CoinPool = me.BestDistance, me.CoinPool
	resp.RunToken = d.GameTokens.Issue(u.ID, newJTI())
	writeJSON(w, http.StatusOK, resp)
}

type postGameRunRequest struct {
	RunToken   string  `json:"run_token"`
	Distance   int     `json:"distance"`
	Coins      int     `json:"coins"`
	DurationMs float64 `json:"duration_ms"`
}

func (d *Deps) PostGameRun(w http.ResponseWriter, r *http.Request) {
	u, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var req postGameRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Distance < 0 || req.Coins < 0 || req.DurationMs < 0 {
		writeError(w, http.StatusBadRequest, "invalid run fields")
		return
	}
	if req.Distance > math.MaxInt32 || req.Coins > math.MaxInt32 {
		writeError(w, http.StatusBadRequest, "value out of range")
		return
	}
	claims, err := d.GameTokens.Verify(req.RunToken)
	if err != nil {
		writeError(w, http.StatusForbidden, "invalid run token")
		return
	}
	if claims.UserID != u.ID {
		writeError(w, http.StatusForbidden, "run token does not match user")
		return
	}
	if !d.gameJTI.consume(claims.JTI) {
		writeError(w, http.StatusForbidden, "run token already used")
		return
	}
	run := game.Run{Distance: req.Distance, Coins: req.Coins, DurationMs: req.DurationMs}
	if err := game.ValidateRun(run, claims.IssuedAt, time.Now().UTC(), d.GameLimits); err != nil {
		switch {
		case errors.Is(err, game.ErrBadRunFields):
			writeError(w, http.StatusBadRequest, "invalid run fields")
		default:
			writeError(w, http.StatusUnprocessableEntity, "run rejected as implausible")
		}
		return
	}
	if err := d.Game.InsertGameRun(r.Context(), u.ID, int32(req.Distance), int32(req.Coins)); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save run")
		return
	}
	me, err := d.Game.GameMeStanding(r.Context(), u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load standing")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"best_distance": me.BestDistance,
		"coin_pool":     me.CoinPool,
		"run_token":     d.GameTokens.Issue(u.ID, newJTI()),
	})
}
