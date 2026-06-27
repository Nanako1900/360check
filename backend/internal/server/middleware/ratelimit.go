package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"

	"github.com/nnkglobal/c5-backend/internal/httpx"
)

// RateLimitConfig bounds a Redis fixed-window limiter.
type RateLimitConfig struct {
	Window     time.Duration
	MaxPerIP   int // attempts per window per client IP (0 disables)
	MaxPerUser int // attempts per window per login username (0 disables)
}

// DefaultAuthRateLimit: 20 attempts / 5 min per IP, 5 / 5 min per username — slows
// online brute-force / credential-stuffing on /auth/login and /auth/refresh.
func DefaultAuthRateLimit() RateLimitConfig {
	return RateLimitConfig{Window: 5 * time.Minute, MaxPerIP: 20, MaxPerUser: 5}
}

// AuthRateLimit is a Redis fixed-window limiter for the public auth group. It keys
// on client IP always, and additionally on the login username (read via a body
// peek that is restored for the handler) when present. On breach it aborts with
// 429 RATE_LIMITED. It FAILS OPEN on a Redis error so a Redis blip can never lock
// every user out of logging in.
func AuthRateLimit(rc goredis.Cmdable, cfg RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		path := c.FullPath()

		if over(ctx, rc, "rl:ip:"+path+":"+c.ClientIP(), cfg.MaxPerIP, cfg.Window) {
			httpx.FailAbort(c, httpx.ErrRateLimited("too many requests, please retry later"))
			return
		}

		if cfg.MaxPerUser > 0 {
			if user := peekUsername(c); user != "" {
				if over(ctx, rc, "rl:user:"+path+":"+user, cfg.MaxPerUser, cfg.Window) {
					httpx.FailAbort(c, httpx.ErrRateLimited("too many attempts for this account, please retry later"))
					return
				}
			}
		}
		c.Next()
	}
}

// fixedWindowScript atomically increments the window counter and sets its TTL on
// the first increment, in a single round-trip. Doing INCR and (P)EXPIRE as separate
// commands risks a counter that survives a failed EXPIRE with no TTL — a key that
// never resets and permanently 429s an IP/username, which would defeat the fail-open
// guarantee. The script is all-or-nothing: either both run server-side, or the call
// errors and we fail open. Portable to Redis 6/7 (EVAL since 2.6).
var fixedWindowScript = goredis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return n
`)

// over atomically increments the fixed-window counter for key (arming its TTL on the
// first hit) and reports whether it now exceeds max. Fails open (returns false) on
// any Redis error — availability over strictness for the auth path.
func over(ctx context.Context, rc goredis.Cmdable, key string, max int, window time.Duration) bool {
	if max <= 0 {
		return false
	}
	n, err := fixedWindowScript.Run(ctx, rc, []string{key}, window.Milliseconds()).Int64()
	if err != nil {
		return false // fail open — availability over strictness for auth
	}
	return n > int64(max)
}

// maxBodyPeek caps the body buffered for the username peek. Legitimate login /
// refresh bodies are well under 1 KiB; bounding the read stops a giant body on
// these unauthenticated endpoints from being buffered in full before auth.
const maxBodyPeek = 64 << 10 // 64 KiB

// peekUsername reads the JSON body's "username" without consuming it (the body is
// restored so the handler's ShouldBindJSON still works). Returns "" when absent
// (e.g. /auth/refresh) or unparseable.
func peekUsername(c *gin.Context) string {
	if c.Request.Body == nil {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBodyPeek))
	if err != nil {
		return ""
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(data)) // restore for the handler
	var probe struct {
		Username string `json:"username"`
	}
	if json.Unmarshal(data, &probe) != nil {
		return ""
	}
	return probe.Username
}
