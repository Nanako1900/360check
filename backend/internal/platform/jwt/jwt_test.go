package jwt

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueAndParse(t *testing.T) {
	m := NewManager("test-secret", "c5-api", 15*time.Minute)
	now := time.Now()

	tok, expiresIn, err := m.Issue(7, "zhangsan", []string{"inspector"}, now)
	require.NoError(t, err)
	assert.Equal(t, 900, expiresIn)

	claims, err := m.Parse(tok)
	require.NoError(t, err)
	assert.Equal(t, int64(7), claims.UserID)
	assert.Equal(t, "zhangsan", claims.Username)
	assert.Equal(t, []string{"inspector"}, claims.Roles)
	assert.Equal(t, "7", claims.Subject)
	assert.Equal(t, "c5-api", claims.Issuer)
	assert.NotEmpty(t, claims.ID) // jti present
}

func TestParse_Expired(t *testing.T) {
	m := NewManager("test-secret", "c5-api", time.Minute)
	// Issue a token whose window is entirely in the past.
	past := time.Now().Add(-2 * time.Hour)
	tok, _, err := m.Issue(1, "admin", []string{"admin"}, past)
	require.NoError(t, err)

	_, err = m.Parse(tok)
	require.ErrorIs(t, err, ErrExpired)
}

func TestParse_WrongSecret(t *testing.T) {
	signer := NewManager("secret-A", "c5-api", time.Minute)
	verifier := NewManager("secret-B", "c5-api", time.Minute)
	tok, _, err := signer.Issue(1, "admin", nil, time.Now())
	require.NoError(t, err)

	_, err = verifier.Parse(tok)
	require.ErrorIs(t, err, ErrInvalid)
}

func TestParse_WrongIssuer(t *testing.T) {
	signer := NewManager("s", "other-issuer", time.Minute)
	verifier := NewManager("s", "c5-api", time.Minute)
	tok, _, err := signer.Issue(1, "admin", nil, time.Now())
	require.NoError(t, err)

	_, err = verifier.Parse(tok)
	require.ErrorIs(t, err, ErrInvalid)
}

func TestParse_Garbage(t *testing.T) {
	m := NewManager("s", "c5-api", time.Minute)
	_, err := m.Parse("not-a-jwt")
	require.ErrorIs(t, err, ErrInvalid)
}

func TestIssue_NilRolesBecomesEmpty(t *testing.T) {
	m := NewManager("s", "c5-api", time.Minute)
	tok, _, err := m.Issue(1, "admin", nil, time.Now())
	require.NoError(t, err)
	claims, err := m.Parse(tok)
	require.NoError(t, err)
	assert.NotNil(t, claims.Roles)
	assert.Empty(t, claims.Roles)
}

// alg=none must be rejected (algorithm-confusion guard).
func TestParse_RejectsNoneAlg(t *testing.T) {
	m := NewManager("s", "c5-api", time.Minute)
	// A hand-built unsigned token with alg=none.
	const noneToken = "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJ1aWQiOjEsImlzcyI6ImM1LWFwaSJ9."
	_, err := m.Parse(noneToken)
	require.ErrorIs(t, err, ErrInvalid)
}

// An RS256-signed token must be rejected by the HS256-only manager
// (RS/HMAC algorithm-confusion guard).
func TestParse_RejectsRS256(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tok := jwtv5.NewWithClaims(jwtv5.SigningMethodRS256, &Claims{
		RegisteredClaims: jwtv5.RegisteredClaims{
			Issuer:    "c5-api",
			ExpiresAt: jwtv5.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	signed, err := tok.SignedString(key)
	require.NoError(t, err)

	m := NewManager("test-secret", "c5-api", time.Minute)
	_, err = m.Parse(signed)
	require.ErrorIs(t, err, ErrInvalid)
}

func TestAccessTTL(t *testing.T) {
	m := NewManager("s", "c5-api", 42*time.Minute)
	assert.Equal(t, 42*time.Minute, m.AccessTTL())
}
