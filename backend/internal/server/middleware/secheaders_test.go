package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestSecurityHeaders_SetOnEveryResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SecurityHeaders())
	r.GET("/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	// Also assert on an unmatched route (404) — headers must still be present.
	r.NoRoute(func(c *gin.Context) { c.String(http.StatusNotFound, "nf") })

	for _, path := range []string{"/x", "/missing"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		h := w.Header()
		assert.Equal(t, "max-age=31536000; includeSubDomains", h.Get("Strict-Transport-Security"), path)
		assert.Equal(t, "nosniff", h.Get("X-Content-Type-Options"), path)
		assert.Equal(t, "DENY", h.Get("X-Frame-Options"), path)
		assert.Equal(t, "strict-origin-when-cross-origin", h.Get("Referrer-Policy"), path)
		assert.NotEmpty(t, h.Get("Permissions-Policy"), path)
	}
}
