package httpapi

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// rate-limit tuning (per the M9 design).
const (
	authRate  = rate.Limit(10.0 / 60.0) // ~10/min per IP
	authBurst = 5

	writeRate  = rate.Limit(1.0) // ~60/min per user (1 req/sec sustained)
	writeBurst = 20

	chatRate  = rate.Limit(20.0 / 60.0) // ~20/min per user (bounds OpenAI spend)
	chatBurst = 5

	// gameReadRate / gameReadBurst throttle GET /api/game/leaderboard per user.
	// The GET mints a single-use run token on every call, so it needs its own
	// limiter (rateLimitWrites skips GET). ~30/min sustained, burst 10 — generous
	// for real play, bounds token hoarding and seenJTI growth (§3.10 / §12).
	gameReadRate  = rate.Limit(0.5) // 30/min per user
	gameReadBurst = 10

	limiterIdleTTL = 15 * time.Minute
)

type limiterEntry struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

// keyedLimiter is an in-memory per-key token-bucket limiter (single-instance app).
type keyedLimiter struct {
	limit rate.Limit
	burst int
	mu    sync.Mutex
	keys  map[string]*limiterEntry
}

func newKeyedLimiter(limit rate.Limit, burst int) *keyedLimiter {
	return &keyedLimiter{limit: limit, burst: burst, keys: map[string]*limiterEntry{}}
}

func (k *keyedLimiter) Allow(key string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	ts := time.Now()
	e, ok := k.keys[key]
	if !ok {
		e = &limiterEntry{lim: rate.NewLimiter(k.limit, k.burst)}
		k.keys[key] = e
	}
	e.lastSeen = ts
	// opportunistic sweep of idle keys (bounded work per call)
	for kk, ee := range k.keys {
		if ts.Sub(ee.lastSeen) > limiterIdleTTL {
			delete(k.keys, kk)
		}
	}
	return e.lim.Allow()
}

// clientIP extracts the client IP from r.RemoteAddr (which middleware.RealIP
// has already normalised to "ip:port" or bare "ip").
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr has no port (unusual after RealIP, but safe fallback)
		return r.RemoteAddr
	}
	return host
}

// retryAfterSecs returns the number of seconds a client should wait before
// retrying after a 429: ceil(1 / rate), minimum 1.
// auth bucket (10/min = 1/6s) → 6s; writes bucket (60/min = 1s) → 1s.
func retryAfterSecs(kl *keyedLimiter) int {
	secs := int(math.Ceil(1.0 / float64(kl.limit)))
	if secs < 1 {
		secs = 1
	}
	return secs
}

func tooMany(w http.ResponseWriter, kl *keyedLimiter) {
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSecs(kl)))
	writeError(w, http.StatusTooManyRequests, "rate limited")
}

// rateLimitIP throttles by client IP (intended for the unauthenticated auth endpoint).
func rateLimitIP(kl *keyedLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !kl.Allow(clientIP(r)) {
				tooMany(w, kl)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// rateLimitUser throttles ALL methods (including GET) by session user ID.
// Use this for endpoints that issue tokens or incur server-side cost on every GET,
// where rateLimitWrites (which skips GET/HEAD/OPTIONS) is insufficient.
// Must run AFTER RequireAuth (needs the user in context).
func rateLimitUser(kl *keyedLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := "anon"
			if u, ok := userFromContext(r.Context()); ok {
				key = strconv.FormatInt(u.ID, 10)
			}
			if !kl.Allow(key) {
				tooMany(w, kl)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// rateLimitWrites throttles mutating methods by session user ID; reads pass through.
// Must run AFTER RequireAuth (needs the user in context).
// The "anon" fallback key is defensive — in normal wiring RequireAuth always sets a user.
func rateLimitWrites(kl *keyedLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}
			key := "anon"
			if u, ok := userFromContext(r.Context()); ok {
				key = strconv.FormatInt(u.ID, 10)
			}
			if !kl.Allow(key) {
				tooMany(w, kl)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
