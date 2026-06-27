package rbac

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/nnkglobal/c5-backend/internal/httpx"
	"github.com/nnkglobal/c5-backend/internal/platform/jwt"
)

// Context keys for the authenticated principal, set by Authn.
const (
	CtxUserID   = "user_id"
	CtxUsername = "username"
	CtxRoles    = "roles"
)

// Authn validates the bearer access token and populates the principal in the gin
// context. Missing/invalid token -> 401 UNAUTHENTICATED; expired -> 401
// TOKEN_EXPIRED (so the client single-flights /auth/refresh).
func Authn(mgr *jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if h == "" || !strings.HasPrefix(h, "Bearer ") {
			httpx.FailAbort(c, httpx.ErrUnauthenticated("missing bearer token"))
			return
		}
		claims, err := mgr.Parse(strings.TrimSpace(strings.TrimPrefix(h, "Bearer ")))
		if err != nil {
			if errors.Is(err, jwt.ErrExpired) {
				httpx.FailAbort(c, httpx.ErrTokenExpired("access token expired"))
				return
			}
			httpx.FailAbort(c, httpx.ErrUnauthenticated("invalid access token"))
			return
		}
		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxUsername, claims.Username)
		c.Set(CtxRoles, claims.Roles)
		c.Next()
	}
}

// Authz enforces casbin on (sub=username, obj=request path, act=HTTP method).
// Deny -> 403 FORBIDDEN. Must run after Authn.
func Authz(en *Enforcer) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := UsernameFromContext(c)
		if username == "" {
			httpx.FailAbort(c, httpx.ErrUnauthenticated("missing principal"))
			return
		}
		ok, err := en.Enforce(username, c.Request.URL.Path, c.Request.Method)
		if err != nil {
			httpx.FailAbort(c, httpx.ErrInternal("authorization error"))
			return
		}
		if !ok {
			httpx.FailAbort(c, httpx.ErrForbidden("forbidden"))
			return
		}
		c.Next()
	}
}

// UserIDFromContext returns the authenticated user id (0 if absent).
func UserIDFromContext(c *gin.Context) int64 {
	if v, ok := c.Get(CtxUserID); ok {
		if id, ok := v.(int64); ok {
			return id
		}
	}
	return 0
}

// UsernameFromContext returns the authenticated username ("" if absent).
func UsernameFromContext(c *gin.Context) string {
	if v, ok := c.Get(CtxUsername); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
