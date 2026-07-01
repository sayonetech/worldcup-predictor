package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestKeyedLimiter_AllowsThenBlocks(t *testing.T) {
	kl := newKeyedLimiter(rate.Limit(0.0001), 2) // ~never refills; burst 2
	first := kl.Allow("a")
	second := kl.Allow("a")
	if !first || !second {
		t.Fatal("first 2 within burst should pass")
	}
	if kl.Allow("a") {
		t.Fatal("3rd should be blocked")
	}
	if !kl.Allow("b") {
		t.Fatal("different key must be isolated")
	}
}

func TestRateLimitIP_429WithRetryAfter(t *testing.T) {
	kl := newKeyedLimiter(rate.Limit(0.0001), 1)
	h := rateLimitIP(kl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	call := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/google", nil)
		req.RemoteAddr = "1.2.3.4:5555"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if call().Code != 200 {
		t.Fatal("first should pass")
	}
	rec := call()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("2nd should be 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 must set Retry-After")
	}
}

func TestRateLimitWrites_OnlyMutating_PerUser(t *testing.T) {
	kl := newKeyedLimiter(rate.Limit(0.0001), 1)
	h := rateLimitWrites(kl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	withUser := func(method string, id int64) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/x", nil)
		req = ctxUser(req, id) // helper from existing tests: injects store.User{ID:id}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	// GET never limited
	for i := 0; i < 5; i++ {
		if withUser(http.MethodGet, 1).Code != 200 {
			t.Fatal("GET must never be limited")
		}
	}
	// writes limited per user
	if withUser(http.MethodPut, 1).Code != 200 {
		t.Fatal("1st write within burst")
	}
	if withUser(http.MethodPut, 1).Code != http.StatusTooManyRequests {
		t.Fatal("2nd write same user → 429")
	}
	if withUser(http.MethodPut, 2).Code != 200 {
		t.Fatal("different user isolated")
	}
}

// TestRateLimitIP_AuthRetryAfter asserts the auth limiter's 429 advertises a
// wait > 1s (specifically 6s for 10/min), not the old hardcoded "1".
func TestRateLimitIP_AuthRetryAfter(t *testing.T) {
	kl := newKeyedLimiter(authRate, authBurst) // same params as production auth limiter
	h := rateLimitIP(kl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	call := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/google", nil)
		req.RemoteAddr = "5.6.7.8:1234"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	// Exhaust burst to guarantee a 429.
	for i := 0; i < authBurst; i++ {
		call()
	}
	rec := call()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after burst exhausted, got %d", rec.Code)
	}
	ra := rec.Header().Get("Retry-After")
	if ra == "" {
		t.Fatal("429 must set Retry-After")
	}
	if ra == "1" {
		t.Fatalf("Retry-After must reflect real refill time for auth limiter (10/min → 6s), got %q", ra)
	}
	// ceil(1 / (10/60)) = ceil(6) = 6
	if ra != "6" {
		t.Fatalf("expected Retry-After: 6 for auth limiter (10/min), got %q", ra)
	}
}

// TestRateLimitUser_LimitsGETPerUser asserts that rateLimitUser (used on
// GET /api/game/leaderboard) 429s the same user after their burst is exhausted,
// while a different user passes — and that GET requests ARE counted (unlike
// rateLimitWrites which skips them).
func TestRateLimitUser_LimitsGETPerUser(t *testing.T) {
	kl := newKeyedLimiter(rate.Limit(0.0001), 2) // ~never refills; burst 2
	h := rateLimitUser(kl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	do := func(method string, id int64) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/game/leaderboard", nil)
		req = ctxUser(req, id) // inject store.User{ID: id}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	// Exhaust burst for user 1 with GET requests.
	if do(http.MethodGet, 1).Code != 200 {
		t.Fatal("1st GET for user 1 must pass")
	}
	if do(http.MethodGet, 1).Code != 200 {
		t.Fatal("2nd GET for user 1 (within burst) must pass")
	}
	rec := do(http.MethodGet, 1)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("3rd GET same user → want 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 must carry Retry-After header")
	}
	// Different user is isolated.
	if do(http.MethodGet, 2).Code != 200 {
		t.Fatal("first GET for user 2 must pass (isolated bucket)")
	}
}

// TestKeyedLimiter_IdleSweep verifies the unbounded-growth guard: entries idle
// for longer than limiterIdleTTL are removed during the next Allow call.
func TestKeyedLimiter_IdleSweep(t *testing.T) {
	kl := newKeyedLimiter(rate.Limit(1), 1)

	// Seed key "a" so it has an entry in the map.
	kl.Allow("a")

	// Back-date "a"'s lastSeen past the idle TTL so the sweep will evict it.
	kl.mu.Lock()
	kl.keys["a"].lastSeen = time.Now().Add(-(limiterIdleTTL + time.Second))
	kl.mu.Unlock()

	// Allow("b") triggers the opportunistic sweep inside Allow.
	kl.Allow("b")

	kl.mu.Lock()
	_, aPresent := kl.keys["a"]
	_, bPresent := kl.keys["b"]
	kl.mu.Unlock()

	if aPresent {
		t.Error("idle key 'a' should have been swept from the map")
	}
	if !bPresent {
		t.Error("active key 'b' should be present in the map")
	}
}
