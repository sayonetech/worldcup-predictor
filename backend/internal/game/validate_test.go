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
