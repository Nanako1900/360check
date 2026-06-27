// Package jwt issues and validates short-lived HS256 access tokens. Refresh
// tokens are opaque and live in Redis (see internal/auth); access tokens are
// stateless and never checked against Redis, so revocation relies on the short
// access TTL plus refresh revocation.
package jwt

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Sentinel errors let the auth middleware distinguish 401 UNAUTHENTICATED
// (missing/invalid token) from 401 TOKEN_EXPIRED (so clients single-flight refresh).
var (
	ErrExpired = errors.New("token expired")
	ErrInvalid = errors.New("token invalid")
)

// Claims is the access-token payload. password_hash and other secrets never appear.
type Claims struct {
	UserID   int64    `json:"uid"`
	Username string   `json:"username"`
	Roles    []string `json:"roles"`
	jwtv5.RegisteredClaims
}

// Manager signs and verifies access tokens with a symmetric secret.
type Manager struct {
	secret    []byte
	issuer    string
	accessTTL time.Duration
}

// NewManager builds a Manager. secret must be non-empty (enforced by config fail-fast).
func NewManager(secret, issuer string, accessTTL time.Duration) *Manager {
	return &Manager{secret: []byte(secret), issuer: issuer, accessTTL: accessTTL}
}

// AccessTTL returns the configured access-token lifetime.
func (m *Manager) AccessTTL() time.Duration { return m.accessTTL }

// Issue mints a signed access token valid for accessTTL from now. It returns the
// token and its lifetime in seconds. now is explicit for testability; production
// passes time.Now().
func (m *Manager) Issue(userID int64, username string, roles []string, now time.Time) (string, int, error) {
	if roles == nil {
		roles = []string{}
	}
	claims := Claims{
		UserID:   userID,
		Username: username,
		Roles:    roles,
		RegisteredClaims: jwtv5.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   strconv.FormatInt(userID, 10),
			ID:        uuid.NewString(),
			IssuedAt:  jwtv5.NewNumericDate(now),
			ExpiresAt: jwtv5.NewNumericDate(now.Add(m.accessTTL)),
		},
	}
	signed, err := jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, claims).SignedString(m.secret)
	if err != nil {
		return "", 0, fmt.Errorf("sign access token: %w", err)
	}
	return signed, int(m.accessTTL.Seconds()), nil
}

// Parse validates a token's signature, method, issuer and expiry, returning its
// claims. It maps expiry to ErrExpired and any other failure to ErrInvalid.
func (m *Manager) Parse(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwtv5.ParseWithClaims(tokenStr, claims, func(t *jwtv5.Token) (any, error) {
		if _, ok := t.Method.(*jwtv5.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.secret, nil
	}, jwtv5.WithIssuer(m.issuer), jwtv5.WithValidMethods([]string{"HS256"}))
	if err != nil {
		if errors.Is(err, jwtv5.ErrTokenExpired) {
			return nil, ErrExpired
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return claims, nil
}
