package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS allows cross-origin requests from an exact allow-list of browser Origins,
// echoing the matched Origin (never "*"). Auth is via the Authorization header
// (no cookies), so credentials are deliberately NOT enabled — that lets the
// allow-list stay an exact echo without the security pitfalls of credentialed
// wildcard CORS.
//
// Two correctness notes baked in:
//   - Headers are written in the REQUEST phase (before c.Next): a streaming SSE
//     handler flushes response headers on its first write, so a header set after
//     c.Next would be lost.
//   - Preflight OPTIONS is short-circuited with 204. The router registers no
//     OPTIONS routes and leaves HandleMethodNotAllowed off, so an un-aborted
//     OPTIONS would fall through to NoRoute and return a 404 envelope.
func CORS(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = struct{}{}
	}
	return func(c *gin.Context) {
		h := c.Writer.Header()
		// Always vary on Origin so a cached response for one origin is never reused
		// for another.
		h.Add("Vary", "Origin")

		origin := c.GetHeader("Origin")
		if _, ok := allowed[origin]; ok && origin != "" {
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Skip-Refresh, If-None-Match")
			h.Set("Access-Control-Expose-Headers", "ETag")
			h.Set("Access-Control-Max-Age", "600")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
