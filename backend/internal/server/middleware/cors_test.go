package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func corsRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS([]string{"https://admin.x.com"}))
	r.GET("/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	return r
}

func TestCORS_AllowedOrigin_EchoesAndExposes(t *testing.T) {
	r := corsRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "https://admin.x.com")
	r.ServeHTTP(w, req)

	h := w.Header()
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "https://admin.x.com", h.Get("Access-Control-Allow-Origin"))
	assert.Contains(t, h.Values("Vary"), "Origin")
	assert.Equal(t, "ETag", h.Get("Access-Control-Expose-Headers"))
	// No cookies anywhere in the app → credentials must NOT be enabled.
	assert.Empty(t, h.Get("Access-Control-Allow-Credentials"))
}

func TestCORS_Preflight_NoContentWithHeaders(t *testing.T) {
	r := corsRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/x", nil)
	req.Header.Set("Origin", "https://admin.x.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	allow := w.Header().Get("Access-Control-Allow-Headers")
	for _, want := range []string{"Authorization", "Content-Type", "If-None-Match", "X-Skip-Refresh"} {
		assert.Truef(t, strings.Contains(allow, want), "Allow-Headers %q missing %q", allow, want)
	}
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "OPTIONS")
}

func TestCORS_DisallowedOrigin_NoAllowOrigin(t *testing.T) {
	r := corsRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_NoOrigin_Passthrough(t *testing.T) {
	r := corsRouter()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ok", w.Body.String())
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}
