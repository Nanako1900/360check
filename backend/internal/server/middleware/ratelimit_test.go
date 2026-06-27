package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newLimitEngine(t *testing.T, cfg RateLimitConfig) (*gin.Engine, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rc := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/auth/login", AuthRateLimit(rc, cfg), func(c *gin.Context) {
		// proves the body peek restored the body for the handler:
		var b struct {
			Username string `json:"username"`
		}
		_ = c.ShouldBindJSON(&b)
		c.JSON(http.StatusOK, gin.H{"u": b.Username})
	})
	return r, mr
}

func post(r *gin.Engine, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.7:5555"
	r.ServeHTTP(w, req)
	return w
}

func TestAuthRateLimit_PerIP(t *testing.T) {
	r, _ := newLimitEngine(t, RateLimitConfig{Window: time.Minute, MaxPerIP: 3})
	for i := 0; i < 3; i++ {
		assert.Equal(t, http.StatusOK, post(r, `{"username":"a"}`).Code, "req %d under limit", i)
	}
	w := post(r, `{"username":"a"}`) // 4th over MaxPerIP
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Body.String(), "RATE_LIMITED")
}

func TestAuthRateLimit_PerUsername(t *testing.T) {
	// High IP limit, low per-user limit: same user trips the user dimension.
	r, _ := newLimitEngine(t, RateLimitConfig{Window: time.Minute, MaxPerIP: 100, MaxPerUser: 2})
	assert.Equal(t, http.StatusOK, post(r, `{"username":"bob"}`).Code)
	assert.Equal(t, http.StatusOK, post(r, `{"username":"bob"}`).Code)
	assert.Equal(t, http.StatusTooManyRequests, post(r, `{"username":"bob"}`).Code)
	// A different user is unaffected (separate key).
	assert.Equal(t, http.StatusOK, post(r, `{"username":"carol"}`).Code)
}

func TestAuthRateLimit_BodyRestoredForHandler(t *testing.T) {
	r, _ := newLimitEngine(t, RateLimitConfig{Window: time.Minute, MaxPerIP: 10, MaxPerUser: 10})
	w := post(r, `{"username":"echoed"}`)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "echoed") // handler still read the body
}

func TestAuthRateLimit_FirstHitArmsTTL(t *testing.T) {
	// The atomic INCR+PEXPIRE must arm a TTL on the very first hit, so a transient
	// Expire failure can never leave a counter that 429s an IP/user permanently.
	r, mr := newLimitEngine(t, RateLimitConfig{Window: time.Minute, MaxPerIP: 5})
	require.Equal(t, http.StatusOK, post(r, `{"username":"a"}`).Code)
	ttl := mr.TTL("rl:ip:/api/v1/auth/login:203.0.113.7")
	assert.Greater(t, ttl, time.Duration(0), "window key must carry a TTL")
	assert.LessOrEqual(t, ttl, time.Minute)
}

func TestAuthRateLimit_FailsOpenWhenRedisDown(t *testing.T) {
	r, mr := newLimitEngine(t, RateLimitConfig{Window: time.Minute, MaxPerIP: 1})
	mr.Close() // redis now unreachable → must fail open, not 500/lockout
	for i := 0; i < 5; i++ {
		assert.Equal(t, http.StatusOK, post(r, `{"username":"a"}`).Code, "fail-open req %d", i)
	}
}
