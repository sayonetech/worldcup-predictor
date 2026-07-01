package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sayonetech/worldcup-predictor/backend/internal/game"
	"github.com/sayonetech/worldcup-predictor/backend/internal/store"
)

type fakeGameStore struct {
	inserted []struct{ uid, dist, coins int32 }
	me       store.GameMe
}

func (f *fakeGameStore) InsertGameRun(_ context.Context, uid int64, d, c int32) error {
	f.inserted = append(f.inserted, struct{ uid, dist, coins int32 }{int32(uid), d, c})
	f.me.BestDistance = int64(d)
	f.me.CoinPool += int64(c)
	return nil
}
func (f *fakeGameStore) GameDistanceBoard(context.Context) ([]store.GameDistanceRow, error) {
	return nil, nil
}
func (f *fakeGameStore) GameCoinBoard(context.Context) ([]store.GameCoinRow, error) {
	return nil, nil
}
func (f *fakeGameStore) GameMeStanding(context.Context, int64) (store.GameMe, error) {
	return f.me, nil
}

func testGameDeps(nowFn func() time.Time) (*Deps, *fakeGameStore) {
	fs := &fakeGameStore{}
	d := &Deps{
		Game:       fs,
		GameTokens: game.NewTokenManager("test-secret", 10*time.Minute, nowFn),
		GameLimits: game.Limits{DurationSlackMs: 1500, DistEpsM: 25, DistEpsFrac: 0.02, CoinMinSpacingM: 300, CoinSlack: 3},
	}
	d.initGameJTISet()
	return d, fs
}

// reqWithUser injects an authenticated user into the request context the way
// RequireAuth would (matches the existing handler-test helper convention).
func reqWithUser(method, target string, body []byte, uid int64) *http.Request {
	r := httptest.NewRequest(method, target, bytes.NewReader(body))
	return r.WithContext(context.WithValue(r.Context(), userCtxKey, store.User{ID: uid, Name: "Renjith"}))
}

func TestPostGameRun_AcceptsPlausibleAndStores(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	d, fs := testGameDeps(func() time.Time { return base.Add(61 * time.Second) })
	// token issued at base, validated 61s later (within 10-minute TTL)
	tok := game.NewTokenManager("test-secret", 10*time.Minute, func() time.Time { return base }).Issue(7, "jti-A")

	body, _ := json.Marshal(map[string]any{"run_token": tok, "distance": game.PaceDistance(60000), "coins": 4, "duration_ms": 60000})
	w := httptest.NewRecorder()
	d.PostGameRun(w, reqWithUser(http.MethodPost, "/api/game/runs", body, 7))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(fs.inserted) != 1 {
		t.Fatalf("expected 1 stored run, got %d", len(fs.inserted))
	}
}

func TestPostGameRun_RejectsReusedToken(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	d, _ := testGameDeps(func() time.Time { return base.Add(61 * time.Second) })
	tok := game.NewTokenManager("test-secret", 10*time.Minute, func() time.Time { return base }).Issue(7, "jti-B")
	body, _ := json.Marshal(map[string]any{"run_token": tok, "distance": game.PaceDistance(60000), "coins": 4, "duration_ms": 60000})

	w1 := httptest.NewRecorder()
	d.PostGameRun(w1, reqWithUser(http.MethodPost, "/api/game/runs", body, 7))
	w2 := httptest.NewRecorder()
	d.PostGameRun(w2, reqWithUser(http.MethodPost, "/api/game/runs", body, 7))
	if w2.Code != http.StatusForbidden {
		t.Fatalf("reused token status = %d, want 403", w2.Code)
	}
}

func TestPostGameRun_RejectsImplausibleDistance(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	d, _ := testGameDeps(func() time.Time { return base.Add(31 * time.Second) })
	tok := game.NewTokenManager("test-secret", 10*time.Minute, func() time.Time { return base }).Issue(7, "jti-C")
	body, _ := json.Marshal(map[string]any{"run_token": tok, "distance": 99999, "coins": 0, "duration_ms": 30000})
	w := httptest.NewRecorder()
	d.PostGameRun(w, reqWithUser(http.MethodPost, "/api/game/runs", body, 7))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", w.Code)
	}
}

func TestPostGameRun_RejectsTokenUserMismatch(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	d, _ := testGameDeps(func() time.Time { return base.Add(61 * time.Second) })
	tok := game.NewTokenManager("test-secret", 10*time.Minute, func() time.Time { return base }).Issue(99, "jti-D")
	body, _ := json.Marshal(map[string]any{"run_token": tok, "distance": game.PaceDistance(60000), "coins": 1, "duration_ms": 60000})
	w := httptest.NewRecorder()
	d.PostGameRun(w, reqWithUser(http.MethodPost, "/api/game/runs", body, 7)) // caller 7 ≠ token 99
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

// TestGetGameLeaderboard_ReturnsBoards asserts the leaderboard handler:
//   (a) returns 200 with the distance/coins boards and the caller's me standing, and
//   (b) includes a non-empty run_token issued for the caller.
func TestGetGameLeaderboard_ReturnsBoards(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	d, fs := testGameDeps(func() time.Time { return base })
	// Seed the fake store with a non-trivial me standing so the response is
	// distinguishable from a zero value.
	fs.me = store.GameMe{BestDistance: 42, CoinPool: 7}

	w := httptest.NewRecorder()
	d.GetGameLeaderboard(w, reqWithUser(http.MethodGet, "/api/game/leaderboard", nil, 5))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	// Empty boards (the fake store returns nil) must serialize as JSON `[]`,
	// never `null` — the client maps over these arrays unconditionally.
	if body := w.Body.String(); strings.Contains(body, `"distance":null`) || strings.Contains(body, `"coins":null`) {
		t.Fatalf("empty boards must serialize as [] not null; body=%s", body)
	}
	var resp gameLeaderboardResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Me.BestDistance != 42 {
		t.Errorf("me.best_distance = %d, want 42", resp.Me.BestDistance)
	}
	if resp.Me.CoinPool != 7 {
		t.Errorf("me.coin_pool = %d, want 7", resp.Me.CoinPool)
	}
	if resp.RunToken == "" {
		t.Error("run_token must be non-empty")
	}
	// Verify the token was issued for the caller (uid=5).
	claims, err := d.GameTokens.Verify(resp.RunToken)
	if err != nil {
		t.Fatalf("run_token verification failed: %v", err)
	}
	if claims.UserID != 5 {
		t.Errorf("run_token uid = %d, want 5", claims.UserID)
	}
}
