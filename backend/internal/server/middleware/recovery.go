package middleware

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/httpx"
)

// Recovery converts any panic into a 500 INTERNAL envelope, logging full context
// server-side while never leaking stack traces or SQL to the client.
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic recovered",
					slog.Any("panic", r),
					slog.String("request_id", RequestIDFromContext(c)),
					slog.String("method", c.Request.Method),
					slog.String("path", c.Request.URL.Path),
				)
				if !c.Writer.Written() {
					httpx.Fail(c, httpx.NewError(oapi.INTERNAL, "internal server error"))
				}
				c.Abort()
			}
		}()
		c.Next()
	}
}
