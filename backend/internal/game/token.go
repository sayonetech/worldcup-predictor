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
	h.Write([]byte("game-run:")) // domain separation from auth session cookies
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
