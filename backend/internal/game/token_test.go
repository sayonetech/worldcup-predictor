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
