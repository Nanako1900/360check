// Package middleware holds the gin middleware chain: request id, recovery,
// access logging and Prometheus metrics. Auth/casbin middleware are added in P2.
package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// HeaderRequestID is the inbound/outbound correlation header.
	HeaderRequestID = "X-Request-Id"
	// ContextRequestID is the gin context key for the resolved request id.
	ContextRequestID = "request_id"
)

// RequestID ensures every request carries an X-Request-Id, generating one when
// absent, storing it in the context and echoing it on the response.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(HeaderRequestID)
		if rid == "" {
			rid = uuid.NewString()
		}
		c.Set(ContextRequestID, rid)
		c.Writer.Header().Set(HeaderRequestID, rid)
		c.Next()
	}
}

// RequestIDFromContext returns the request id stored by the RequestID middleware,
// or "" when absent.
func RequestIDFromContext(c *gin.Context) string {
	if v, ok := c.Get(ContextRequestID); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
