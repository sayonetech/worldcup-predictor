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
