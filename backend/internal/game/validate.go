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
