package middleware

import "github.com/gin-gonic/gin"

// Security header values. This is a JSON API (no HTML/script is ever served), so
// the XSS-surface headers matter less than transport hardening; HSTS is the
// highest-value control. Set early so they apply to every response, including
// error envelopes and 404s.
const (
	// hstsValue: 1 year, include subdomains. "preload" is intentionally omitted —
	// it is an irreversible commitment to the browser preload list and should be a
	// deliberate ops decision, not a code default.
	hstsValue           = "max-age=31536000; includeSubDomains"
	contentTypeOptions  = "nosniff"
	frameOptions        = "DENY"
	referrerPolicy      = "strict-origin-when-cross-origin"
	permissionsPolicy   = "camera=(), microphone=(), geolocation=()"
	crossOriginResource = "same-site"
)

// SecurityHeaders sets transport/clickjacking/sniffing hardening headers on every
// response. HSTS is honored by browsers only over HTTPS (TLS terminates at the
// ingress), so it is safe to send unconditionally.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("Strict-Transport-Security", hstsValue)
		h.Set("X-Content-Type-Options", contentTypeOptions)
		h.Set("X-Frame-Options", frameOptions)
		h.Set("Referrer-Policy", referrerPolicy)
		h.Set("Permissions-Policy", permissionsPolicy)
		h.Set("Cross-Origin-Resource-Policy", crossOriginResource)
		c.Next()
	}
}
