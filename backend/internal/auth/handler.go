package auth

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/httpx"
	"github.com/nnkglobal/c5-backend/internal/rbac"
)

// Password policy: bcrypt silently truncates at 72 bytes, so we cap length and
// require a sensible minimum.
const (
	minPasswordLen = 8
	maxPasswordLen = 72
)

// Handler exposes the auth endpoints as gin handlers.
type Handler struct {
	svc *Service
}

// NewHandler builds the auth HTTP handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterPublic mounts login + refresh (security: []).
func (h *Handler) RegisterPublic(rg gin.IRouter) {
	rg.POST("/auth/login", h.Login)
	rg.POST("/auth/refresh", h.Refresh)
}

// RegisterProtected mounts logout/me/password (require a valid access token).
func (h *Handler) RegisterProtected(rg gin.IRouter) {
	rg.POST("/auth/logout", h.Logout)
	rg.GET("/auth/me", h.Me)
	rg.PUT("/auth/password", h.ChangePassword)
}

// Login handles POST /auth/login.
func (h *Handler) Login(c *gin.Context) {
	var req oapi.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}
	if req.Username == "" || req.Password == "" {
		httpx.Fail(c, httpx.ErrValidation("username and password are required"))
		return
	}
	tokens, err := h.svc.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			httpx.Fail(c, httpx.ErrUnauthenticated("invalid username or password"))
			return
		}
		httpx.Fail(c, httpx.ErrInternal("login failed"))
		return
	}
	httpx.OK(c, tokens)
}

// Refresh handles POST /auth/refresh.
func (h *Handler) Refresh(c *gin.Context) {
	var req oapi.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.RefreshToken == "" {
		httpx.Fail(c, httpx.ErrValidation("refresh_token is required"))
		return
	}
	tokens, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, ErrRefreshNotFound) {
			httpx.Fail(c, httpx.ErrUnauthenticated("invalid or expired refresh token"))
			return
		}
		httpx.Fail(c, httpx.ErrInternal("refresh failed"))
		return
	}
	httpx.OK(c, tokens)
}

// Logout handles POST /auth/logout (revokes the supplied refresh token).
func (h *Handler) Logout(c *gin.Context) {
	var req oapi.RefreshRequest
	_ = c.ShouldBindJSON(&req) // body optional
	if err := h.svc.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		httpx.Fail(c, httpx.ErrInternal("logout failed"))
		return
	}
	httpx.OKEmpty(c)
}

// Me handles GET /auth/me.
func (h *Handler) Me(c *gin.Context) {
	uid := rbac.UserIDFromContext(c)
	me, err := h.svc.Me(c.Request.Context(), uid)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			httpx.Fail(c, httpx.ErrUnauthenticated("user not found"))
			return
		}
		httpx.Fail(c, httpx.ErrInternal("failed to load current user"))
		return
	}
	httpx.OK(c, me)
}

// ChangePassword handles PUT /auth/password.
func (h *Handler) ChangePassword(c *gin.Context) {
	var req oapi.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}
	if err := validatePassword(req.NewPassword); err != nil {
		httpx.Fail(c, err)
		return
	}
	uid := rbac.UserIDFromContext(c)
	if err := h.svc.ChangePassword(c.Request.Context(), uid, req.OldPassword, req.NewPassword); err != nil {
		switch {
		case errors.Is(err, ErrOldPasswordWrong):
			httpx.Fail(c, httpx.ErrValidation("old password is incorrect").
				WithDetails(oapi.ErrorDetail{Field: "old_password", Code: "incorrect", Message: "old password is incorrect"}))
		case errors.Is(err, ErrInvalidCredentials):
			httpx.Fail(c, httpx.ErrUnauthenticated("user not found"))
		default:
			httpx.Fail(c, httpx.ErrInternal("failed to change password"))
		}
		return
	}
	httpx.OKEmpty(c)
}

func validatePassword(pw string) *httpx.APIError {
	if len(pw) < minPasswordLen || len(pw) > maxPasswordLen {
		return httpx.ErrValidation("password length invalid").
			WithDetails(oapi.ErrorDetail{
				Field: "new_password", Code: "length",
				Message: "password must be 8-72 characters",
			})
	}
	return nil
}
