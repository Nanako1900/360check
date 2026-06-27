package rbac

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/nnkglobal/c5-backend/internal/platform/jwt"
)

func TestAuthn_Rejections(t *testing.T) {
	mgr := jwt.NewManager("secret", "c5-api", 15*time.Minute)
	// Same secret + issuer so the signature validates but exp is in the past.
	expired, _, err := mgr.Issue(7, "alice", nil, time.Now().Add(-2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, header, wantCode string
	}{
		{"missing header", "", "UNAUTHENTICATED"},
		{"no bearer prefix", "Token abc", "UNAUTHENTICATED"},
		{"empty token after bearer", "Bearer ", "UNAUTHENTICATED"},
		{"garbage token", "Bearer not.a.jwt", "UNAUTHENTICATED"},
		{"expired token", "Bearer " + expired, "TOKEN_EXPIRED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.header != "" {
				c.Request.Header.Set("Authorization", tc.header)
			}
			Authn(mgr)(c)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.True(t, c.IsAborted())
			assert.Equal(t, tc.wantCode, errCode(t, w))
		})
	}
}

func TestAuthn_ValidPopulatesPrincipal(t *testing.T) {
	mgr := jwt.NewManager("secret", "c5-api", 15*time.Minute)
	tok, _, err := mgr.Issue(7, "alice", []string{"admin"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Authorization", "Bearer "+tok)

	Authn(mgr)(c)
	assert.False(t, c.IsAborted())
	assert.Equal(t, int64(7), UserIDFromContext(c))
	assert.Equal(t, "alice", UsernameFromContext(c))
}

func TestContextHelpers_Empty(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	assert.Equal(t, int64(0), UserIDFromContext(c))
	assert.Equal(t, "", UsernameFromContext(c))
}
