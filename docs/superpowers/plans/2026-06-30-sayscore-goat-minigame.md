# GOAT Mini-Game Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Embed the `chased-by-the-goat` arcade bundle in SayScore with two of its own leaderboards (best-distance + lifetime-coins), persisting every run through a server-authoritative anti-cheat path, fully separated from the prize leaderboards.

**Architecture:** A pure Go pacing/token/validation core (`internal/game/`, zero I/O) reproduces the bundle's published pacing curve and signs single-use run tokens; thin handlers (`internal/httpapi/game_handler.go`) depend on a new `store.GameStore` interface and the game core; an append-only `game_runs` table backs `MAX(distance)` / `SUM(coins)` aggregate boards. The React host mounts the bundle, rotates a server-issued token after each run, and renders a new **Game** tab.

**Tech Stack:** Go 1.22 · chi v5 · sqlc + MySQL 8 · `crypto/hmac` (reusing `SESSION_SECRET`) · React 19 + TypeScript + Vite · TanStack Query · Vitest + Testing Library · the prebuilt `chased-by-the-goat@0.6.0` ESM bundle.

## Global Constraints

- **Spec is the contract:** this plan implements §3.10, §10 (`game_runs`), §11 (Game endpoints), §12, §14 (`GAME_*` env), §18 of `docs/REQUIREMENTS.md`. Do not add behavior not in the spec.
- **Off the prize path:** game data must never touch `predictions`, `weekly_results`, `bonus_*`, or `GET /api/leaderboard`. No money/prize columns on `game_runs`.
- **Server is authoritative:** never store a run that fails token or plausibility validation (§18). The client clock is never trusted; bound duration by server-measured token age.
- **Append-only + recompute:** one `game_runs` row per run; boards are pure aggregates, never incremented in place.
- **Pure core has zero I/O:** `internal/game/` imports no DB, no `net/http`, no `time.Now` directly (inject a clock) — mirrors `internal/scoring/`.
- **sqlc workflow:** edit SQL in `backend/internal/store/queries/`, run `make sqlc`; never hand-edit `internal/store/sqlc/`.
- **Migrations:** numbered up/down pair; never edit an applied migration. InnoDB + utf8mb4; timestamps UTC.
- **Times:** store UTC, display IST at the frontend edge.
- **Commits:** Conventional Commits, scope `game`. Frequent, one per task minimum.
- **Pacing constants (verbatim, §18.3):** `SPEED0=11`, `MAX_SPEED=27`, `HARD_MAX=35`, `ULTRA_MAX=43`, `ACCEL=0.0024`, `SCORE_RATE=0.035`, `ULTRA_AT=10000`, `FRAME=1000.0/60.0`.

---

### Task 1: Pure pacing engine (`internal/game/pace.go`)

The highest-value, purest surface — reproduce the bundle's distance-vs-time curve exactly so the server can reject implausible distances. TDD against the bundle's worked table (§18.3).

**Files:**
- Create: `backend/internal/game/pace.go`
- Test: `backend/internal/game/pace_test.go`

**Interfaces:**
- Produces: `func PaceDistance(activeMs float64) int` — total metres after `activeMs` of active play; `≤0 → 0`.

- [ ] **Step 1: Write the failing test** (the bundle's worked table — exact, since both integrate at the logical 60fps step)

```go
package game

import "testing"

func TestPaceDistance_WorkedTable(t *testing.T) {
	cases := []struct {
		ms   float64
		want int
	}{
		{1000, 23}, {2000, 46}, {5000, 119}, {10000, 246}, {15000, 380},
		{20000, 522}, {30000, 829}, {60000, 1930}, {90000, 3303},
		{120000, 4939}, {180000, 8454}, {300000, 16357}, {600000, 42306},
	}
	for _, c := range cases {
		if got := PaceDistance(c.ms); got != c.want {
			t.Errorf("PaceDistance(%v) = %d, want %d", c.ms, got, c.want)
		}
	}
}

func TestPaceDistance_NonPositive(t *testing.T) {
	if PaceDistance(0) != 0 || PaceDistance(-5) != 0 {
		t.Fatal("non-positive activeMs must yield 0")
	}
}

func TestPaceDistance_Monotonic(t *testing.T) {
	prev := -1
	for ms := 0.0; ms <= 600000; ms += 250 {
		d := PaceDistance(ms)
		if d < prev {
			t.Fatalf("distance decreased at %vms: %d < %d", ms, d, prev)
		}
		prev = d
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/game/ -run TestPaceDistance -v`
Expected: FAIL — `undefined: PaceDistance`.

- [ ] **Step 3: Write minimal implementation** (direct port of §18.3 `paceStep` / `paceDistance`)

```go
// Package game holds the pure, I/O-free core for the GOAT mini-game (§3.10):
// the pacing curve used to validate a run's distance, the signed run token, and
// run-plausibility checks. No DB, no net/http, no wall-clock — mirrors internal/scoring.
package game

import "math"

// Pacing constants — verbatim from REQUIREMENTS.md §18.3 / the bundle's INTEGRATION.md §8.
const (
	speed0    = 11.0
	maxSpeed  = 27.0
	hardMax   = 35.0
	ultraMax  = 43.0
	accel     = 0.0024
	scoreRate = 0.035
	ultraAt   = 10000.0
	frameMs   = 1000.0 / 60.0
)

// paceStep advances one frame of size dt (logical 60fps frames; dt=1 at 60fps).
// Tier is chosen from the CURRENT state; speed is bumped first (clamped to the
// tier cap), then distance accrues using the updated speed.
func paceStep(speed, score, dt float64) (float64, float64) {
	var cap, acc float64
	switch {
	case speed < maxSpeed:
		cap, acc = maxSpeed, accel
	case score < ultraAt:
		cap, acc = hardMax, accel*0.16
	default:
		cap, acc = ultraMax, accel*0.42
	}
	speed = math.Min(cap, speed+acc*dt)
	score += speed * scoreRate * dt
	return speed, score
}

// PaceDistance returns the maximum distance (metres, floored) reachable after
// activeMs of active play, integrating from the canonical start (speed0, 0).
func PaceDistance(activeMs float64) int {
	if !(activeMs > 0) {
		return 0
	}
	frames := activeMs / frameMs
	whole := math.Floor(frames)
	speed, score := speed0, 0.0
	for i := 0; i < int(whole); i++ {
		speed, score = paceStep(speed, score, 1)
	}
	if frac := frames - whole; frac > 0 {
		_, score = paceStep(speed, score, frac)
	}
	return int(math.Floor(score))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/game/ -run TestPaceDistance -v`
Expected: PASS (all worked-table rows match exactly).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/game/pace.go backend/internal/game/pace_test.go
git commit -m "feat(game): pure pacing engine matching the bundle worked table (§18.3)"
```

---

### Task 2: Run token — sign / verify / single-use (`internal/game/token.go`)

HMAC-signed, single-use, short-TTL run token (§18.2). Reuses the auth cookie's HMAC scheme but is a self-contained type so the game core stays decoupled from `internal/auth`.

**Files:**
- Create: `backend/internal/game/token.go`
- Test: `backend/internal/game/token_test.go`

**Interfaces:**
- Produces:
  - `type TokenClaims struct { UserID int64; JTI string; IssuedAt time.Time }`
  - `type TokenManager struct { ... }`
  - `func NewTokenManager(secret string, ttl time.Duration, now func() time.Time) *TokenManager`
  - `func (m *TokenManager) Issue(userID int64, jti string) string`
  - `func (m *TokenManager) Verify(token string) (TokenClaims, error)` — checks signature + TTL, returns claims; caller checks single-use + user match.
  - Sentinel errors: `ErrTokenMalformed`, `ErrTokenBadSignature`, `ErrTokenExpired`.

- [ ] **Step 1: Write the failing test**

```go
package game

import (
	"testing"
	"time"
)

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestToken_RoundTrip(t *testing.T) {
	base := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	m := NewTokenManager("s3cr3t", 10*time.Minute, fixedClock(base))
	tok := m.Issue(42, "jti-1")
	claims, err := m.Verify(tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.UserID != 42 || claims.JTI != "jti-1" || !claims.IssuedAt.Equal(base) {
		t.Fatalf("claims mismatch: %+v", claims)
	}
}

func TestToken_RejectsTampered(t *testing.T) {
	m := NewTokenManager("s3cr3t", 10*time.Minute, fixedClock(time.Now()))
	tok := m.Issue(42, "jti-1")
	if _, err := m.Verify(tok + "x"); err == nil {
		t.Fatal("expected bad-signature error")
	}
}

func TestToken_RejectsWrongKey(t *testing.T) {
	now := fixedClock(time.Now())
	tok := NewTokenManager("key-a", 10*time.Minute, now).Issue(1, "j")
	if _, err := NewTokenManager("key-b", 10*time.Minute, now).Verify(tok); err == nil {
		t.Fatal("expected bad-signature error across keys")
	}
}

func TestToken_RejectsExpired(t *testing.T) {
	base := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	tok := NewTokenManager("s", 10*time.Minute, fixedClock(base)).Issue(1, "j")
	later := NewTokenManager("s", 10*time.Minute, fixedClock(base.Add(11*time.Minute)))
	if _, err := later.Verify(tok); err != ErrTokenExpired {
		t.Fatalf("want ErrTokenExpired, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/game/ -run TestToken -v`
Expected: FAIL — `undefined: NewTokenManager`.

- [ ] **Step 3: Write minimal implementation**

```go
package game

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrTokenMalformed    = errors.New("game: malformed run token")
	ErrTokenBadSignature = errors.New("game: bad run-token signature")
	ErrTokenExpired      = errors.New("game: run token expired")
)

// TokenClaims is the signed (not encrypted) run-token payload.
type TokenClaims struct {
	UserID   int64 `json:"uid"`
	JTI      string `json:"jti"`
	IssuedAt time.Time
}

type tokenPayload struct {
	UserID int64  `json:"uid"`
	JTI    string `json:"jti"`
	IAT    int64  `json:"iat"` // unix seconds
}

type TokenManager struct {
	key []byte
	ttl time.Duration
	now func() time.Time
}

func NewTokenManager(secret string, ttl time.Duration, now func() time.Time) *TokenManager {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &TokenManager{key: []byte(secret), ttl: ttl, now: now}
}

func (m *TokenManager) sign(b64 string) string {
	h := hmac.New(sha256.New, m.key)
	h.Write([]byte(b64))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// Issue returns "<base64url(payload)>.<base64url(hmac)>".
func (m *TokenManager) Issue(userID int64, jti string) string {
	body, _ := json.Marshal(tokenPayload{UserID: userID, JTI: jti, IAT: m.now().UTC().Unix()})
	b64 := base64.RawURLEncoding.EncodeToString(body)
	return b64 + "." + m.sign(b64)
}

// Verify checks the signature and TTL and returns the claims. Single-use (jti)
// and caller-match are enforced by the handler, not here.
func (m *TokenManager) Verify(token string) (TokenClaims, error) {
	b64, sig, ok := strings.Cut(token, ".")
	if !ok {
		return TokenClaims{}, ErrTokenMalformed
	}
	if !hmac.Equal([]byte(sig), []byte(m.sign(b64))) {
		return TokenClaims{}, ErrTokenBadSignature
	}
	body, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		return TokenClaims{}, ErrTokenMalformed
	}
	var p tokenPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return TokenClaims{}, ErrTokenMalformed
	}
	iat := time.Unix(p.IAT, 0).UTC()
	if m.now().UTC().Sub(iat) > m.ttl {
		return TokenClaims{}, ErrTokenExpired
	}
	return TokenClaims{UserID: p.UserID, JTI: p.JTI, IssuedAt: iat}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/game/ -run TestToken -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/game/token.go backend/internal/game/token_test.go
git commit -m "feat(game): HMAC-signed short-TTL run token (§18.2)"
```

---

### Task 3: Run validation (`internal/game/validate.go`)

Pure decision function combining the duration bound + pacing equality + coin band (§18.3/18.4). No token I/O — takes the already-verified `issued_at` and `now`.

**Files:**
- Create: `backend/internal/game/validate.go`
- Test: `backend/internal/game/validate_test.go`

**Interfaces:**
- Consumes: `PaceDistance` (Task 1).
- Produces:
  - `type Limits struct { DurationSlackMs float64; DistEpsM float64; DistEpsFrac float64; CoinMinSpacingM int; CoinSlack int; MaxDistance int }`
  - `type Run struct { Distance int; Coins int; DurationMs float64 }`
  - `func ValidateRun(r Run, issuedAt, now time.Time, lim Limits) error` — `nil` if plausible; otherwise a non-nil error. Sentinels: `ErrImplausibleDistance`, `ErrImplausibleCoins`, `ErrBadRunFields`.

- [ ] **Step 1: Write the failing test**

```go
package game

import (
	"testing"
	"time"
)

func defaultLimits() Limits {
	return Limits{DurationSlackMs: 1500, DistEpsM: 25, DistEpsFrac: 0.02, CoinMinSpacingM: 300, CoinSlack: 3, MaxDistance: 0}
}

func TestValidateRun_AcceptsHonest(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	// 60s of play → ~1930m → coin cap = floor(1930/300)+3 = 9; submit shortly after a 60s run.
	r := Run{Distance: PaceDistance(60000), Coins: 4, DurationMs: 60000}
	if err := ValidateRun(r, base, base.Add(61*time.Second), defaultLimits()); err != nil {
		t.Fatalf("honest run rejected: %v", err)
	}
}

func TestValidateRun_RejectsInflatedDistance(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	r := Run{Distance: 99999, Coins: 0, DurationMs: 30000}
	if err := ValidateRun(r, base, base.Add(31*time.Second), defaultLimits()); err != ErrImplausibleDistance {
		t.Fatalf("want ErrImplausibleDistance, got %v", err)
	}
}

func TestValidateRun_BoundsDurationByTokenAge(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	// Client claims a 600s run but the token is only ~5s old → distance must
	// match ~5s of pace, not 600s. A 600s distance is far over the bound.
	r := Run{Distance: PaceDistance(600000), Coins: 0, DurationMs: 600000}
	if err := ValidateRun(r, base, base.Add(5*time.Second), defaultLimits()); err != ErrImplausibleDistance {
		t.Fatalf("want ErrImplausibleDistance (duration bounded by token age), got %v", err)
	}
}

func TestValidateRun_RejectsTooManyCoins(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	d := PaceDistance(60000)
	r := Run{Distance: d, Coins: d, DurationMs: 60000} // far over 0.5/m + 5
	if err := ValidateRun(r, base, base.Add(61*time.Second), defaultLimits()); err != ErrImplausibleCoins {
		t.Fatalf("want ErrImplausibleCoins, got %v", err)
	}
}

func TestValidateRun_RejectsNegativeFields(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	r := Run{Distance: -1, Coins: 0, DurationMs: 1000}
	if err := ValidateRun(r, base, base.Add(2*time.Second), defaultLimits()); err != ErrBadRunFields {
		t.Fatalf("want ErrBadRunFields, got %v", err)
	}
}

func TestValidateRun_MaxDistanceCeiling(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	lim := defaultLimits()
	lim.MaxDistance = 1000
	// Honest per pace but over the absolute ceiling.
	r := Run{Distance: PaceDistance(120000), Coins: 0, DurationMs: 120000}
	if err := ValidateRun(r, base, base.Add(121*time.Second), lim); err != ErrImplausibleDistance {
		t.Fatalf("want ErrImplausibleDistance (over ceiling), got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/game/ -run TestValidateRun -v`
Expected: FAIL — `undefined: ValidateRun`.

- [ ] **Step 3: Write minimal implementation**

```go
package game

import (
	"errors"
	"math"
	"time"
)

var (
	ErrBadRunFields        = errors.New("game: bad run fields")
	ErrImplausibleDistance = errors.New("game: implausible distance")
	ErrImplausibleCoins    = errors.New("game: implausible coins")
)

// Limits are the configurable validation knobs (§14 / §18).
type Limits struct {
	DurationSlackMs float64
	DistEpsM        float64
	DistEpsFrac     float64
	CoinMinSpacingM int // bundle MIN_COLLECTIBLE_SPACING_M (300) — hard spawn-cadence gate
	CoinSlack       int
	MaxDistance     int // 0 = disabled (no absolute ceiling)
}

// Run is one reported run (post token-verify).
type Run struct {
	Distance   int
	Coins      int
	DurationMs float64
}

// ValidateRun returns nil if the reported run is plausible per §18.3/18.4.
func ValidateRun(r Run, issuedAt, now time.Time, lim Limits) error {
	if r.Distance < 0 || r.Coins < 0 || r.DurationMs < 0 {
		return ErrBadRunFields
	}
	elapsedMs := float64(now.Sub(issuedAt).Milliseconds())
	boundedMs := math.Min(r.DurationMs, elapsedMs+lim.DurationSlackMs)
	expected := PaceDistance(boundedMs)
	eps := math.Max(lim.DistEpsM, lim.DistEpsFrac*float64(expected))
	if math.Abs(float64(r.Distance-expected)) > eps {
		return ErrImplausibleDistance
	}
	if lim.MaxDistance > 0 && r.Distance > lim.MaxDistance {
		return ErrImplausibleDistance
	}
	// Coins gate by minimum spawn spacing (bundle MIN_COLLECTIBLE_SPACING_M): at most
	// one coin per CoinMinSpacingM metres. Guard against a misconfigured 0 spacing.
	coinCap := lim.CoinSlack
	if lim.CoinMinSpacingM > 0 {
		coinCap += r.Distance / lim.CoinMinSpacingM
	}
	if r.Coins > coinCap {
		return ErrImplausibleCoins
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/game/ -v`
Expected: PASS (all game-core tests).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/game/validate.go backend/internal/game/validate_test.go
git commit -m "feat(game): server-authoritative run validation (§18.3/18.4)"
```

---

### Task 4: Migration + sqlc queries + store wrapper (`game_runs`)

Append-only table and the three aggregate queries (§10/§11). Implemented on `*store.SQLStore` (the one struct backing every store interface).

**Files:**
- Create: `backend/migrations/0011_create_game_runs.up.sql`, `backend/migrations/0011_create_game_runs.down.sql`
- Create: `backend/internal/store/queries/game.sql`
- Create: `backend/internal/store/game.go`
- (Regenerated) `backend/internal/store/sqlc/*` via `make sqlc`

**Interfaces:**
- Produces:
  - `type GameDistanceRow struct { UserID int64; Name string; AvatarURL string; Distance int64 }`
  - `type GameCoinRow struct { UserID int64; Name string; AvatarURL string; Coins int64 }`
  - `type GameMe struct { BestDistance int64; CoinPool int64 }`
  - `store.GameStore` interface:
    - `InsertGameRun(ctx, userID int64, distance, coins int32) error`
    - `GameDistanceBoard(ctx) ([]GameDistanceRow, error)` (top 20, `MAX(distance)` desc)
    - `GameCoinBoard(ctx) ([]GameCoinRow, error)` (top 20, `SUM(coins)` desc)
    - `GameMeStanding(ctx, userID int64) (GameMe, error)` (`MAX`/`SUM` for one user; zeros if none)

- [ ] **Step 1: Write the migration (up/down)**

`backend/migrations/0011_create_game_runs.up.sql`:

```sql
CREATE TABLE game_runs (
    id         BIGINT       NOT NULL AUTO_INCREMENT,
    user_id    BIGINT       NOT NULL,
    distance   INT UNSIGNED NOT NULL,
    coins      INT UNSIGNED NOT NULL DEFAULT 0,
    played_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_game_runs_user (user_id),
    KEY idx_game_runs_distance (distance),
    CONSTRAINT fk_game_runs_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

`backend/migrations/0011_create_game_runs.down.sql`:

```sql
DROP TABLE IF EXISTS game_runs;
```

- [ ] **Step 2: Write the sqlc queries**

`backend/internal/store/queries/game.sql`:

```sql
-- name: InsertGameRun :exec
INSERT INTO game_runs (user_id, distance, coins) VALUES (?, ?, ?);

-- name: GameDistanceBoard :many
SELECT u.id AS user_id, u.name, u.avatar_url,
       CAST(MAX(r.distance) AS SIGNED) AS best_distance
FROM game_runs r JOIN users u ON u.id = r.user_id
GROUP BY u.id, u.name, u.avatar_url
ORDER BY best_distance DESC, u.id ASC
LIMIT 20;

-- name: GameCoinBoard :many
SELECT u.id AS user_id, u.name, u.avatar_url,
       CAST(SUM(r.coins) AS SIGNED) AS coin_pool
FROM game_runs r JOIN users u ON u.id = r.user_id
GROUP BY u.id, u.name, u.avatar_url
ORDER BY coin_pool DESC, u.id ASC
LIMIT 20;

-- name: GameMeStanding :one
SELECT CAST(COALESCE(MAX(distance),0) AS SIGNED) AS best_distance,
       CAST(COALESCE(SUM(coins),0)   AS SIGNED) AS coin_pool
FROM game_runs WHERE user_id = ?;
```

- [ ] **Step 3: Generate sqlc code**

Run: `cd backend && make sqlc`
Expected: `internal/store/sqlc/` gains `InsertGameRun`, `GameDistanceBoard`, `GameCoinBoard`, `GameMeStanding`. `sqlc diff` clean.

- [ ] **Step 4: Write the store wrapper + interface**

`backend/internal/store/game.go` (adapt field names to whatever `make sqlc` generated — that generated code is authoritative):

```go
package store

import (
	"context"
	"fmt"
)

type GameDistanceRow struct {
	UserID    int64
	Name      string
	AvatarURL string
	Distance  int64
}

type GameCoinRow struct {
	UserID    int64
	Name      string
	AvatarURL string
	Coins     int64
}

type GameMe struct {
	BestDistance int64
	CoinPool     int64
}

// GameStore backs the GOAT mini-game boards (§3.10). Append-only writes;
// reads are pure aggregates (never recompute on the prize path).
type GameStore interface {
	InsertGameRun(ctx context.Context, userID int64, distance, coins int32) error
	GameDistanceBoard(ctx context.Context) ([]GameDistanceRow, error)
	GameCoinBoard(ctx context.Context) ([]GameCoinRow, error)
	GameMeStanding(ctx context.Context, userID int64) (GameMe, error)
}

func (s *SQLStore) InsertGameRun(ctx context.Context, userID int64, distance, coins int32) error {
	if err := s.q.InsertGameRun(ctx, sqlcInsertGameRunParams(userID, distance, coins)); err != nil {
		return fmt.Errorf("store: insert game run: %w", err)
	}
	return nil
}

func (s *SQLStore) GameDistanceBoard(ctx context.Context) ([]GameDistanceRow, error) {
	rows, err := s.q.GameDistanceBoard(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: game distance board: %w", err)
	}
	out := make([]GameDistanceRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, GameDistanceRow{UserID: r.UserID, Name: r.Name, AvatarURL: r.AvatarUrl, Distance: r.BestDistance})
	}
	return out, nil
}

func (s *SQLStore) GameCoinBoard(ctx context.Context) ([]GameCoinRow, error) {
	rows, err := s.q.GameCoinBoard(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: game coin board: %w", err)
	}
	out := make([]GameCoinRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, GameCoinRow{UserID: r.UserID, Name: r.Name, AvatarURL: r.AvatarUrl, Coins: r.CoinPool})
	}
	return out, nil
}

func (s *SQLStore) GameMeStanding(ctx context.Context, userID int64) (GameMe, error) {
	row, err := s.q.GameMeStanding(ctx, userID)
	if err != nil {
		return GameMe{}, fmt.Errorf("store: game me standing: %w", err)
	}
	return GameMe{BestDistance: row.BestDistance, CoinPool: row.CoinPool}, nil
}
```

> **Note for the implementer:** replace `sqlcInsertGameRunParams(...)` with the actual generated params struct (e.g. `sqlc.InsertGameRunParams{UserID: userID, Distance: uint32(distance), Coins: uint32(coins)}`) and confirm the generated row field names (`AvatarUrl`, `BestDistance`, `CoinPool`) match — adapt callers to the generated code, never hand-edit it.

- [ ] **Step 5: Verify build + migration round-trips**

Run: `cd backend && go build ./... && make migrate-up && make migrate-down && make migrate-up`
Expected: builds; up/down/up all succeed (down drops the table cleanly).

- [ ] **Step 6: Commit**

```bash
git add backend/migrations/0011_* backend/internal/store/queries/game.sql backend/internal/store/sqlc backend/internal/store/game.go
git commit -m "feat(game): game_runs table + aggregate board queries (§10/§11)"
```

---

### Task 5: Handlers + router + Deps (`internal/httpapi/game_handler.go`)

`GET /api/game/leaderboard` and `POST /api/game/runs` (§11), depending on `store.GameStore` and `*game.TokenManager`, with an in-memory single-use jti set. Handler tests use a fake `GameStore` (the established pattern — handlers depend on interfaces).

**Files:**
- Create: `backend/internal/httpapi/game_handler.go`
- Create: `backend/internal/httpapi/game_handler_test.go`
- Modify: `backend/internal/httpapi/middleware.go` (add `Game store.GameStore`, `GameTokens *game.TokenManager`, `GameLimits game.Limits` to `Deps`)
- Modify: `backend/internal/httpapi/router.go` (register the two routes in the `priv` group)

**Interfaces:**
- Consumes: `store.GameStore` (Task 4); `game.TokenManager`, `game.ValidateRun`, `game.Limits`, sentinels (Tasks 2–3); `userFromContext`, `writeJSON`, `writeError` (existing).
- Produces: `Deps.GetGameLeaderboard`, `Deps.PostGameRun` handlers; an unexported `seenJTI` TTL set.

- [ ] **Step 1: Write the failing handler test** (fake store + a real TokenManager)

```go
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
func (f *fakeGameStore) GameDistanceBoard(context.Context) ([]store.GameDistanceRow, error) { return nil, nil }
func (f *fakeGameStore) GameCoinBoard(context.Context) ([]store.GameCoinRow, error)          { return nil, nil }
func (f *fakeGameStore) GameMeStanding(context.Context, int64) (store.GameMe, error)         { return f.me, nil }

func testGameDeps(now func() time.Time) (*Deps, *fakeGameStore) {
	fs := &fakeGameStore{}
	d := &Deps{
		Game:       fs,
		GameTokens: game.NewTokenManager("test-secret", 10*time.Minute, now),
		GameLimits: game.Limits{DurationSlackMs: 1500, DistEpsM: 25, DistEpsFrac: 0.02, CoinMinSpacingM: 300, CoinSlack: 3},
	}
	d.initGameJTISet()
	return d, fs
}

// withUser injects an authenticated user into the request context the way
// RequireAuth would (matches the existing handler-test helper convention).
func reqWithUser(method, target string, body []byte, uid int64) *http.Request {
	r := httptest.NewRequest(method, target, bytes.NewReader(body))
	return r.WithContext(context.WithValue(r.Context(), userCtxKey, store.User{ID: uid, Name: "Renjith"}))
}

func TestPostGameRun_AcceptsPlausibleAndStores(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	calls := 0
	now := func() time.Time { calls++; return base.Add(time.Duration(calls) * time.Millisecond) } // ~stable
	d, fs := testGameDeps(func() time.Time { return base.Add(61 * time.Second) })
	_ = now
	tok := d.GameTokens.Issue(7, "jti-A")
	// rewind issuedAt: token issued at base, validated 61s later
	d.GameTokens = game.NewTokenManager("test-secret", 10*time.Minute, func() time.Time { return base.Add(61 * time.Second) })
	tok = game.NewTokenManager("test-secret", 10*time.Minute, func() time.Time { return base }).Issue(7, "jti-A")

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
```

> **Implementer note:** the test mirrors the token's `issued_at` by issuing with a clock fixed at `base` while the `Deps` validates with a clock fixed 60s later. Keep both `TokenManager`s on the same secret. If the existing handler tests use a different user-injection helper than `reqWithUser`, reuse theirs.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/httpapi/ -run TestPostGameRun -v`
Expected: FAIL — `d.PostGameRun`, `Deps.Game`, `initGameJTISet` undefined.

- [ ] **Step 3: Add Deps fields**

In `backend/internal/httpapi/middleware.go`, add to the `Deps` struct (and import `internal/game`):

```go
	Game       store.GameStore
	GameTokens *game.TokenManager
	GameLimits game.Limits
```

- [ ] **Step 4: Write the handler**

`backend/internal/httpapi/game_handler.go`:

```go
package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/sayonetech/worldcup-predictor/backend/internal/game"
)

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
	d.gameJTI = &seenJTI{at: map[string]time.Time{}, ttl: 15 * time.Minute, now: now}
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
	var resp gameLeaderboardResponse
	for _, row := range dist {
		resp.Distance = append(resp.Distance, gameBoardRowDTO{UserID: row.UserID, Name: row.Name, AvatarURL: row.AvatarURL, Distance: row.Distance})
	}
	for _, row := range coins {
		resp.Coins = append(resp.Coins, gameBoardRowDTO{UserID: row.UserID, Name: row.Name, AvatarURL: row.AvatarURL, Coins: row.Coins})
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
```

Add the `gameJTI` field to `Deps` in `middleware.go`:

```go
	gameJTI *seenJTI
```

- [ ] **Step 5: Register routes**

In `backend/internal/httpapi/router.go`, inside the `priv` group (alongside the other authenticated routes):

```go
			priv.Get("/game/leaderboard", d.GetGameLeaderboard)
			priv.Post("/game/runs", d.PostGameRun)
```

- [ ] **Step 6: Run test to verify it passes**

Run: `cd backend && go test ./internal/httpapi/ -run TestPostGameRun -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/httpapi/game_handler.go backend/internal/httpapi/game_handler_test.go backend/internal/httpapi/middleware.go backend/internal/httpapi/router.go
git commit -m "feat(game): leaderboard + run-submit handlers with token+pacing validation (§11/§18)"
```

---

### Task 6: Server wiring + config (`GAME_*` env)

Load the `GAME_*` knobs (§14), build the `TokenManager` from `SESSION_SECRET`, and wire the new `Deps` fields in `cmd/server/main.go`.

**Files:**
- Modify: `backend/internal/config/config.go` (add `GAME_*` fields + parsing with defaults)
- Modify: `backend/cmd/server/main.go` (construct `game.TokenManager`, `game.Limits`, set `Deps.Game/GameTokens/GameLimits`, call `deps.initGameJTISet()`)
- Test: `backend/internal/config/config_test.go` (defaults applied when env unset)

**Interfaces:**
- Consumes: `game.NewTokenManager`, `game.Limits` (Tasks 2–3); existing `config.Config`, `store.New(db)`.

- [ ] **Step 1: Write the failing config test** (follow the existing config-test style; this shows intent)

```go
func TestConfig_GameDefaults(t *testing.T) {
	// With no GAME_* env set, defaults from §14 apply.
	cfg := Load() // or whatever the existing loader entry point is
	if cfg.GameTokenTTL != 10*time.Minute {
		t.Errorf("GameTokenTTL default = %v, want 10m", cfg.GameTokenTTL)
	}
	if cfg.GameDistEpsM != 25 || cfg.GameDistEpsFrac != 0.02 {
		t.Errorf("dist eps defaults wrong: %v / %v", cfg.GameDistEpsM, cfg.GameDistEpsFrac)
	}
	if cfg.GameCoinMinSpacingM != 300 || cfg.GameCoinSlack != 3 {
		t.Errorf("coin defaults wrong")
	}
}
```

> **Implementer note:** match the existing config loader's shape (struct + `Load`/`FromEnv`). Add fields `GameTokenTTL time.Duration`, `GameDurationSlackMs float64`, `GameDistEpsM float64`, `GameDistEpsFrac float64`, `GameCoinMinSpacingM int`, `GameCoinSlack int`, `GameMaxDistance int`, parsing `GAME_TOKEN_TTL` (a Go duration string), the numeric vars, with the §14 defaults.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/config/ -run TestConfig_GameDefaults -v`
Expected: FAIL — fields undefined.

- [ ] **Step 3: Add config fields + parsing**

Add the fields and env parsing to `config.go` with the §14 defaults (`GAME_TOKEN_TTL=10m`, `GAME_DURATION_SLACK_MS=1500`, `GAME_DIST_EPS_M=25`, `GAME_DIST_EPS_FRAC=0.02`, `GAME_COIN_MIN_SPACING_M=300`, `GAME_COIN_SLACK=3`, `GAME_MAX_DISTANCE=0`).

- [ ] **Step 4: Wire `Deps` in `cmd/server/main.go`**

```go
	deps := &httpapi.Deps{
		// ...existing fields...
		Game:       st,
		GameTokens: game.NewTokenManager(cfg.SessionSecret, cfg.GameTokenTTL, func() time.Time { return time.Now().UTC() }),
		GameLimits: game.Limits{
			DurationSlackMs: cfg.GameDurationSlackMs,
			DistEpsM:        cfg.GameDistEpsM,
			DistEpsFrac:     cfg.GameDistEpsFrac,
			CoinMinSpacingM: cfg.GameCoinMinSpacingM,
			CoinSlack:       cfg.GameCoinSlack,
			MaxDistance:     cfg.GameMaxDistance,
		},
	}
	deps.initGameJTISet()
```

- [ ] **Step 5: Run config test + full backend build**

Run: `cd backend && go test ./internal/config/ -v && go build ./...`
Expected: PASS + clean build.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/config backend/cmd/server/main.go
git commit -m "feat(game): wire GAME_* config + token manager into the server (§14)"
```

---

### Task 7: Frontend API client + hooks (`lib/game.ts`)

Typed client + TanStack Query hooks mirroring `lib/leaderboard.ts`, including the token-rotation save mutation.

**Files:**
- Create: `frontend/src/lib/game.ts`
- Test: `frontend/src/lib/game.test.ts`
- Modify: `frontend/package.json` (add the bundle dependency)

**Interfaces:**
- Produces:
  - `type GameBoardRow = { user_id: number; name: string; avatar_url: string; distance?: number; coins?: number }`
  - `type GameLeaderboard = { distance: GameBoardRow[]; coins: GameBoardRow[]; me: { best_distance: number; coin_pool: number }; run_token: string }`
  - `type SaveRunInput = { run_token: string; distance: number; coins: number; duration_ms: number }`
  - `type SaveRunResult = { best_distance: number; coin_pool: number; run_token: string }`
  - `getGameLeaderboard(): Promise<GameLeaderboard>`
  - `saveGameRun(input: SaveRunInput): Promise<SaveRunResult>`
  - `useGameLeaderboard()` (query key `["game-leaderboard"]`)

- [ ] **Step 1: Install the bundle**

Run: `cd frontend && pnpm add ./goat-game/chased-by-the-goat-0.6.0.tgz`
Expected: `chased-by-the-goat` appears in `package.json#dependencies`.

- [ ] **Step 2: Write the failing test** (mock `fetch`)

```ts
import { describe, it, expect, vi, beforeEach } from "vitest";
import { getGameLeaderboard, saveGameRun } from "./game";

beforeEach(() => vi.restoreAllMocks());

describe("game api", () => {
  it("fetches the leaderboard with credentials", async () => {
    const board = { distance: [], coins: [], me: { best_distance: 0, coin_pool: 0 }, run_token: "t" };
    const spy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify(board), { status: 200 }),
    );
    const got = await getGameLeaderboard();
    expect(got.run_token).toBe("t");
    expect(spy).toHaveBeenCalledWith(expect.stringContaining("/game/leaderboard"), expect.objectContaining({ credentials: "include" }));
  });

  it("POSTs a run and returns the next token", async () => {
    const res = { best_distance: 100, coin_pool: 12, run_token: "next" };
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify(res), { status: 200 }));
    const got = await saveGameRun({ run_token: "t", distance: 100, coins: 12, duration_ms: 5000 });
    expect(got.run_token).toBe("next");
  });

  it("throws on 4xx", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("{}", { status: 422 }));
    await expect(saveGameRun({ run_token: "t", distance: 9, coins: 0, duration_ms: 1 })).rejects.toThrow();
  });
});
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd frontend && pnpm vitest run src/lib/game.test.ts`
Expected: FAIL — module `./game` not found.

- [ ] **Step 4: Write the client + hooks**

```ts
import { useQuery } from "@tanstack/react-query";

const BASE = import.meta.env.VITE_API_BASE_URL ?? "/api";

export type GameBoardRow = {
  user_id: number;
  name: string;
  avatar_url: string;
  distance?: number;
  coins?: number;
};

export type GameLeaderboard = {
  distance: GameBoardRow[];
  coins: GameBoardRow[];
  me: { best_distance: number; coin_pool: number };
  run_token: string;
};

export type SaveRunInput = { run_token: string; distance: number; coins: number; duration_ms: number };
export type SaveRunResult = { best_distance: number; coin_pool: number; run_token: string };

export async function getGameLeaderboard(): Promise<GameLeaderboard> {
  const res = await fetch(`${BASE}/game/leaderboard`, { credentials: "include" });
  if (!res.ok) throw new Error(`game leaderboard failed: ${res.status}`);
  return res.json() as Promise<GameLeaderboard>;
}

export async function saveGameRun(input: SaveRunInput): Promise<SaveRunResult> {
  const res = await fetch(`${BASE}/game/runs`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!res.ok) throw new Error(`save run failed: ${res.status}`);
  return res.json() as Promise<SaveRunResult>;
}

export function useGameLeaderboard() {
  return useQuery({ queryKey: ["game-leaderboard"], queryFn: getGameLeaderboard });
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd frontend && pnpm vitest run src/lib/game.test.ts`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add frontend/package.json frontend/pnpm-lock.yaml frontend/src/lib/game.ts frontend/src/lib/game.test.ts
git commit -m "feat(game): frontend api client + leaderboard hook; vendor the bundle"
```

---

### Task 8: `GoatGame` component (mount + token rotation + refresh loop)

Wraps `mountGoatGame`, maps SayScore data → bundle config, and runs the save → refetch → `setLeaderboard`/`setCoinLeaderboard`/`setRunToken` loop (bundle `INTEGRATION.md` §3/§5/§8).

**Files:**
- Create: `frontend/src/components/GoatGame.tsx`
- Test: `frontend/src/components/GoatGame.test.tsx` (mock the bundle module)

**Interfaces:**
- Consumes: `useGameLeaderboard`, `saveGameRun` (Task 7); `useMe` (existing); `mountGoatGame` from `chased-by-the-goat`.
- Produces: `export function GoatGame(): JSX.Element`.

- [ ] **Step 1: Write the failing test** (mock the ESM bundle so jsdom needs no canvas)

```tsx
import { describe, it, expect, vi } from "vitest";
import { render, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mount = vi.fn(() => ({ setLeaderboard: vi.fn(), setCoinLeaderboard: vi.fn(), setPlayer: vi.fn(), setRunToken: vi.fn(), update: vi.fn(), destroy: vi.fn() }));
vi.mock("chased-by-the-goat", () => ({ mountGoatGame: mount }));
vi.mock("../lib/auth", () => ({ useMe: () => ({ data: { id: 7, name: "Renjith", avatar_url: "" } }) }));
vi.mock("../lib/game", () => ({
  useGameLeaderboard: () => ({ data: { distance: [], coins: [], me: { best_distance: 0, coin_pool: 0 }, run_token: "tok-1" } }),
  saveGameRun: vi.fn(async () => ({ best_distance: 1, coin_pool: 1, run_token: "tok-2" })),
}));

import { GoatGame } from "./GoatGame";

function wrap(ui: React.ReactNode) {
  return <QueryClientProvider client={new QueryClient()}>{ui}</QueryClientProvider>;
}

describe("GoatGame", () => {
  it("mounts the bundle once with the player + run token", async () => {
    render(wrap(<GoatGame />));
    await waitFor(() => expect(mount).toHaveBeenCalledTimes(1));
    const cfg = mount.mock.calls[0][1];
    expect(cfg.player).toMatchObject({ id: "7", name: "Renjith" });
    expect(cfg.runToken).toBe("tok-1");
    expect(typeof cfg.onGameEnd).toBe("function");
  });

  it("destroys the instance on unmount", async () => {
    const { unmount } = render(wrap(<GoatGame />));
    await waitFor(() => expect(mount).toHaveBeenCalled());
    const handle = mount.mock.results[0].value;
    unmount();
    expect(handle.destroy).toHaveBeenCalled();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && pnpm vitest run src/components/GoatGame.test.tsx`
Expected: FAIL — `./GoatGame` not found.

- [ ] **Step 3: Write the component**

```tsx
import { useEffect, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { mountGoatGame, type GoatGameHandle, type GoatResult } from "chased-by-the-goat";
import { useMe } from "../lib/auth";
import { useGameLeaderboard, saveGameRun, type GameLeaderboard } from "../lib/game";

export function GoatGame() {
  const { data: me } = useMe();
  const { data: board } = useGameLeaderboard();
  const qc = useQueryClient();
  const hostRef = useRef<HTMLDivElement>(null);
  const handleRef = useRef<GoatGameHandle | null>(null);
  // Keep the freshest token/board in refs so onGameEnd (captured once at mount) reads current values.
  const tokenRef = useRef<string | undefined>(board?.run_token);

  // Mount once, after we have both the player and the first board+token.
  useEffect(() => {
    if (!hostRef.current || !me || !board || handleRef.current) return;
    tokenRef.current = board.run_token;
    handleRef.current = mountGoatGame(hostRef.current, {
      player: { id: String(me.id), name: me.name || me.email, coins: board.me.coin_pool },
      leaderboard: board.distance.map((r) => ({ name: r.name, team: "", distance: r.distance ?? 0 })),
      coinLeaderboard: board.coins.map((r) => ({ name: r.name, team: "", coins: r.coins ?? 0 })),
      runToken: board.run_token,
      async onGameEnd(result: GoatResult) {
        try {
          const res = await saveGameRun({
            run_token: result.runToken ?? tokenRef.current ?? "",
            distance: result.distance,
            coins: result.coins,
            duration_ms: result.durationMs,
          });
          tokenRef.current = res.run_token;
          handleRef.current?.setRunToken(res.run_token); // arm next run
          await qc.invalidateQueries({ queryKey: ["game-leaderboard"] }); // refetch → effect below pushes boards
        } catch {
          // Save failed (e.g. token race / rejected run) — leave boards as-is; the player can run again.
        }
      },
    });
    return () => {
      handleRef.current?.destroy();
      handleRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [me, board]);

  // Push refreshed boards + token in place whenever the query data changes (no remount).
  useEffect(() => {
    if (!handleRef.current || !board) return;
    handleRef.current.setLeaderboard(board.distance.map((r) => ({ name: r.name, team: "", distance: r.distance ?? 0 })));
    handleRef.current.setCoinLeaderboard(board.coins.map((r) => ({ name: r.name, team: "", coins: r.coins ?? 0 })));
    if (board.run_token && board.run_token !== tokenRef.current) {
      tokenRef.current = board.run_token;
      handleRef.current.setRunToken(board.run_token);
    }
  }, [board]);

  return <div className="goat-host" ref={hostRef} style={{ width: "100%" }} />;
}

// avoid unused type import error if GameLeaderboard isn't referenced
export type { GameLeaderboard };
```

> **Implementer note (§7 + bundle caveats):** add a `.goat-host { width: 100%; max-width: 1100px; margin: 0 auto; }` rule. The bundle's full-viewport takeover breaks under a transformed ancestor — mount this **outside** any `transform`/`filter`/`contain` wrapper (watch the `.shimmer-stage` / app-shell layers per CLAUDE.md). `team: ""` renders the neutral flag (no per-user country in SayScore).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend && pnpm vitest run src/components/GoatGame.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/GoatGame.tsx frontend/src/components/GoatGame.test.tsx
git commit -m "feat(game): GoatGame component — mount, token rotation, refresh loop (INTEGRATION §3/§8)"
```

---

### Task 9: Launch surface — split the promo banner + game overlay

The game's entry point is the **top promo strip on the Predictions screen** ([Home.tsx](../../../frontend/src/routes/Home.tsx)): today a single full-width `.promo-banner-wrap` linking out to the external Penalty Shootout game. Split it into **two side-by-side cards** — Penalty Shootout (unchanged, external `<a>`) and **Chased by the GOAT** (a `<button>` that opens a full-screen in-app overlay hosting `<GoatGame>`). No nav tab — the overlay mounts at the app root (sibling to `.app`, like `HowToPlayModal`), which also avoids the bundle's transformed-ancestor caveat.

**Files:**
- Create: `frontend/src/components/GameOverlay.tsx` (full-screen modal hosting `<GoatGame>` + close button; mirror `HowToPlayModal`)
- Create: `frontend/src/components/GameOverlay.test.tsx`
- Modify: `frontend/src/routes/Home.tsx` (split the banner; accept an `onOpenGame` prop)
- Modify: `frontend/src/App.tsx` (lift `gameOpen` state, pass `onOpenGame` to `<Home>`, render the overlay at root)
- Modify: `frontend/src/styles/v2-components.css` (2-up `.promo-banner-wrap`; `.promo-banner--goat`; `.game-overlay`)
- Test: `frontend/src/routes/Home.test.tsx` (or extend) — the GOAT card calls `onOpenGame`

**Interfaces:**
- Consumes: `GoatGame` (Task 8); existing `HowToPlayModal` overlay conventions (focus trap, Escape, backdrop, `role="dialog"`, body-scroll lock).
- Produces:
  - `GameOverlay({ onClose }: { onClose: () => void }): JSX.Element`
  - `Home` gains prop `onOpenGame: () => void`.

- [ ] **Step 1: Write the failing test** (the GOAT card invokes `onOpenGame`; the external one stays a link)

```tsx
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Home } from "./Home";

// Stub the data panels so this test targets only the banner wiring.
vi.mock("../components/BonusPanel", () => ({ BonusPanel: () => null }));
vi.mock("../components/MatchesColumn", () => ({ MatchesColumn: () => null }));
vi.mock("../components/StandingCard", () => ({ StandingCard: () => null }));
vi.mock("../components/LeaderboardPanel", () => ({ LeaderboardPanel: () => null }));
vi.mock("../components/HallOfFame", () => ({ HallOfFame: () => null }));

function wrap(ui: React.ReactNode) {
  return <QueryClientProvider client={new QueryClient()}>{ui}</QueryClientProvider>;
}

describe("Home promo banners", () => {
  it("invokes onOpenGame when the GOAT card is clicked", async () => {
    const onOpenGame = vi.fn();
    render(wrap(<Home onOpenGame={onOpenGame} />));
    await userEvent.click(screen.getByRole("button", { name: /chased by the goat/i }));
    expect(onOpenGame).toHaveBeenCalledTimes(1);
  });

  it("keeps Penalty Shootout an external link", () => {
    render(wrap(<Home onOpenGame={() => {}} />));
    const link = screen.getByRole("link", { name: /penalty shootout/i });
    expect(link).toHaveAttribute("target", "_blank");
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && pnpm vitest run src/routes/Home.test.tsx`
Expected: FAIL — `Home` has no `onOpenGame` prop / no GOAT button.

- [ ] **Step 3: Split the banner + thread the prop in `Home.tsx`**

Change the `Home` signature to accept `onOpenGame` (add to `HomeProps`):

```tsx
type HomeProps = {
  mobileView?: HomeMobileView;
  onOpenGame: () => void;
};

export function Home({ mobileView, onOpenGame }: HomeProps) {
```

Replace the single `.promo-banner-wrap` section with the 2-up split:

```tsx
<section className="promo-banner-wrap promo-banner-wrap--split" aria-label="Mini-games">
  <a
    className="promo-banner"
    href="https://d23okley85vr35.cloudfront.net/"
    target="_blank"
    rel="noreferrer"
    aria-label="Open Penalty Shootout in a new tab"
  >
    <img
      className="promo-banner__image"
      src="/penalty-shootout-banner.png"
      alt="Penalty Shootout. One kick. One chance. Be the hero."
    />
  </a>

  <button
    type="button"
    className="promo-banner promo-banner--goat"
    onClick={onOpenGame}
    aria-label="Play Chased by the GOAT"
  >
    {/* When the banner art exists, swap this block for:
        <img className="promo-banner__image" src="/goat-game-banner.png"
             alt="Chased by the GOAT. Outrun the legend." /> */}
    <span className="promo-banner__fallback">
      <span className="promo-banner__fallback-title">Chased by the GOAT</span>
      <span className="promo-banner__fallback-sub">Outrun the legend ▶</span>
    </span>
  </button>
</section>
```

- [ ] **Step 4: Write the `GameOverlay` component**

`frontend/src/components/GameOverlay.tsx` (mirror `HowToPlayModal`: focus the close button on mount, restore on unmount, Escape + backdrop close, body-scroll lock, `role="dialog"` + `aria-modal`):

```tsx
import { useEffect, useId, useRef } from "react";
import { GoatGame } from "./GoatGame";

export function GameOverlay({ onClose }: { onClose: () => void }) {
  const titleId = useId();
  const closeRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    const prev = document.activeElement as HTMLElement | null;
    closeRef.current?.focus();
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    document.addEventListener("keydown", onKey);
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = prevOverflow;
      prev?.focus();
    };
  }, [onClose]);

  return (
    <div className="game-overlay" role="dialog" aria-modal="true" aria-labelledby={titleId}
         onPointerDown={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="game-overlay__panel">
        <div className="game-overlay__head">
          <h2 id={titleId} className="game-overlay__title">Chased by the GOAT</h2>
          <button ref={closeRef} type="button" className="game-overlay__close" aria-label="Close game" onClick={onClose}>✕</button>
        </div>
        <GoatGame />
      </div>
    </div>
  );
}
```

- [ ] **Step 5: Write the GameOverlay test**

`frontend/src/components/GameOverlay.test.tsx` (mock `GoatGame` so jsdom needs no canvas):

```tsx
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("./GoatGame", () => ({ GoatGame: () => <div data-testid="goat-host" /> }));
import { GameOverlay } from "./GameOverlay";

describe("GameOverlay", () => {
  it("renders the game host and closes on the ✕ button", async () => {
    const onClose = vi.fn();
    render(<GameOverlay onClose={onClose} />);
    expect(screen.getByTestId("goat-host")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /close game/i }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
```

- [ ] **Step 6: Wire the overlay state in `App.tsx`**

Add state alongside `helpOpen`:

```tsx
const [gameOpen, setGameOpen] = useState(false);
```

Pass the opener to `Home` (the predictions branch):

```tsx
{activeView === "predictions" ? (
  <Home mobileView={homeMobileView} onOpenGame={() => setGameOpen(true)} />
) : (
  <Admin />
)}
```

Render the overlay at root, next to the other modals (after `</div>` of `.app`, beside `{helpOpen && <HowToPlayModal …/>}`):

```tsx
{gameOpen && <GameOverlay onClose={() => setGameOpen(false)} />}
```

Import `GameOverlay`.

- [ ] **Step 7: Add the CSS** (`frontend/src/styles/v2-components.css`)

Make `.promo-banner-wrap--split` a 2-up grid that stacks on narrow widths, give `.promo-banner` a consistent height as a `<button>` (reset button chrome), style the `--goat` fallback card, and add a full-screen `.game-overlay` (fixed, high z-index, centered `.game-overlay__panel` with `width: min(1100px, 96vw)`). Reuse the existing `.promo-banner` hover/focus states (they already exist, §7). Honor `prefers-reduced-motion` for the sheen.

```css
.promo-banner-wrap--split { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
@media (max-width: 560px) { .promo-banner-wrap--split { grid-template-columns: 1fr; } }
button.promo-banner { border: 0; padding: 0; cursor: pointer; background: transparent; }
.promo-banner--goat .promo-banner__fallback { display: flex; flex-direction: column; align-items: center; justify-content: center; width: 100%; height: 100%; gap: 4px; }
/* …title/sub typography + .game-overlay / .game-overlay__panel / __head / __close per §7 tokens… */
```

> **Implementer note (§7 design contract):** match the existing `.promo-banner` radius/elevation; the GOAT card must define default/hover/focus(visible ring)/active states and a ≥44px target; the overlay needs a visible focus ring on close, ≥4.5:1 contrast, and a `prefers-reduced-motion` path. Use `sayscore-frontend-engineer` + the `impeccable` skill. **Asset:** drop `/goat-game-banner.png` into `frontend/public/` and swap the fallback `<span>` for the `<img>` (comment in Step 3) once art is available.

- [ ] **Step 8: Run tests + type-check**

Run: `cd frontend && pnpm vitest run src/routes/Home.test.tsx src/components/GameOverlay.test.tsx && pnpm tsc --noEmit`
Expected: PASS + clean types.

- [ ] **Step 9: Commit**

```bash
git add frontend/src/routes/Home.tsx frontend/src/components/GameOverlay.tsx frontend/src/components/GameOverlay.test.tsx frontend/src/routes/Home.test.tsx frontend/src/App.tsx frontend/src/styles/v2-components.css
git commit -m "feat(game): split promo strip + GOAT game overlay launcher (§3.10)"
```

---

### Task 10: Full verification + DoD

**Files:** none (verification only).

- [ ] **Step 1: Backend suite**

Run: `cd backend && go vet ./... && go test ./... && make sqlc && git diff --exit-code internal/store/sqlc`
Expected: vet clean; all tests pass; `sqlc diff` shows no drift.

- [ ] **Step 2: Frontend suite**

Run: `cd frontend && pnpm tsc --noEmit && pnpm vitest run && pnpm build`
Expected: clean types; all tests pass; production build succeeds.

- [ ] **Step 3: Dispatch the `sayscore-verifier` agent**

Confirm build/vet/lint/test across both stacks and the §3.10/§18 Definition of Done. Address any findings.

- [ ] **Step 4: Architecture + security review**

Dispatch `sayscore-architecture-reviewer` (pure `internal/game` core has zero I/O; handlers depend on `store.GameStore`; off the prize path) and `sayscore-security-reviewer` (token signing, single-use jti, server-authoritative validation, rate-limit inheritance). Address findings.

- [ ] **Step 5: Final commit (if review fixes were needed)**

```bash
git add -A
git commit -m "chore(game): address verification + review findings"
```

---

## Self-Review

**Spec coverage:**
- §3.10 (feature, two boards, append-only, off-prize) → Tasks 4 (table/aggregates), 5 (handlers), 8–9 (UI). ✅
- §10 `game_runs` → Task 4. ✅
- §11 `GET /api/game/leaderboard`, `POST /api/game/runs` (status codes 400/403/422) → Task 5. ✅
- §12 game-run validation bullet → Tasks 2,3,5 (token + validation + rate-limit inheritance via the `priv` group). ✅
- §14 `GAME_*` env + `SESSION_SECRET` signing → Task 6. ✅
- §18.1 lifecycle (host-driven, token rotation) → Tasks 5 (issue/rotate), 8 (client rotation). ✅
- §18.2 token (signature, TTL, single-use jti, user match) → Tasks 2 (sign/verify) + 5 (jti set + user match). ✅
- §18.3 pacing model + worked table → Task 1. ✅
- §18.4 coin band → Task 3. ✅
- §18.5 residual/tuning (`GAME_MAX_DISTANCE`, TTL) → Tasks 3 (ceiling) + 6 (config). ✅

**Placeholder scan:** core-logic tasks (1–3) carry complete code + tests; wiring tasks (4–9) carry complete code with explicit "adapt to generated sqlc / existing test helper" notes where the repo's generated names or test conventions are authoritative. No `TBD`/`handle edge cases`.

**Type consistency:** `run_token`/`distance`/`coins`/`duration_ms` (snake_case wire) ↔ `SaveRunInput` (TS) ↔ `postGameRunRequest` (Go) align; `GameMe{BestDistance,CoinPool}` ↔ `me.best_distance/coin_pool` JSON ↔ TS `me.best_distance/coin_pool` align; `Limits` fields match across Tasks 3/5/6; `PaceDistance` signature consistent across Tasks 1/3/handler test.

---

## Execution Handoff

Two execution options:

1. **Subagent-Driven (recommended)** — dispatch a fresh subagent per task with review between tasks; for SayScore use the specialist agents (`sayscore-test-engineer` for Tasks 1–3, `sayscore-db-engineer` for Task 4, `sayscore-frontend-engineer` for Tasks 7–9), then `sayscore-verifier` + reviewers for Task 10.
2. **Inline Execution** — execute tasks in this session with checkpoints.
