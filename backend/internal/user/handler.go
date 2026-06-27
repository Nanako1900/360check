package user

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/httpx"
	"github.com/nnkglobal/c5-backend/internal/rbac"
)

// Handler exposes user-management endpoints as gin handlers.
type Handler struct {
	svc *Service
}

// NewHandler builds the user HTTP handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts the user CRUD routes (caller wraps them in Authn+Authz).
func (h *Handler) RegisterRoutes(rg gin.IRouter) {
	rg.GET("/users", h.List)
	rg.POST("/users", h.Create)
	rg.GET("/users/:id", h.Get)
	rg.PUT("/users/:id", h.Update)
	rg.DELETE("/users/:id", h.Delete)
	rg.PUT("/users/:id/password", h.ResetPassword)
}

// List handles GET /users.
func (h *Handler) List(c *gin.Context) {
	page := httpx.ParsePage(c)
	var q *string
	if v := c.Query("q"); v != "" {
		q = &v
	}
	var isActive *bool
	if v := c.Query("is_active"); v != "" {
		b := v == "true" || v == "1"
		isActive = &b
	}
	users, total, err := h.svc.List(c.Request.Context(), q, isActive, page.Limit(), page.Offset())
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal("failed to list users"))
		return
	}
	httpx.List(c, users, httpx.MetaFor(total, page))
}

// Create handles POST /users.
func (h *Handler) Create(c *gin.Context) {
	var in oapi.UserCreate
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}
	if in.Username == "" || len(in.Password) < 8 || len(in.Password) > 72 {
		httpx.Fail(c, httpx.ErrValidation("username required and password must be 8-72 characters"))
		return
	}
	u, err := h.svc.Create(c.Request.Context(), in, rbac.UserIDFromContext(c))
	if err != nil {
		if errors.Is(err, ErrUsernameTaken) {
			httpx.Fail(c, httpx.ErrConflict("username already taken"))
			return
		}
		httpx.Fail(c, httpx.ErrInternal("failed to create user"))
		return
	}
	httpx.Created(c, u)
}

// Get handles GET /users/{id}.
func (h *Handler) Get(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	u, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		failGetErr(c, err)
		return
	}
	httpx.OK(c, u)
}

// Update handles PUT /users/{id}.
func (h *Handler) Update(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in oapi.UserUpdate
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}
	u, err := h.svc.Update(c.Request.Context(), id, in, rbac.UserIDFromContext(c))
	if err != nil {
		failGetErr(c, err)
		return
	}
	httpx.OK(c, u)
}

// Delete handles DELETE /users/{id} (soft delete).
func (h *Handler) Delete(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id, rbac.UserIDFromContext(c)); err != nil {
		failGetErr(c, err)
		return
	}
	httpx.OKEmpty(c)
}

// ResetPassword handles PUT /users/{id}/password (admin reset).
func (h *Handler) ResetPassword(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in oapi.ResetPasswordRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}
	if len(in.NewPassword) < 8 || len(in.NewPassword) > 72 {
		httpx.Fail(c, httpx.ErrValidation("password must be 8-72 characters"))
		return
	}
	if err := h.svc.ResetPassword(c.Request.Context(), id, in.NewPassword, rbac.UserIDFromContext(c)); err != nil {
		failGetErr(c, err)
		return
	}
	httpx.OKEmpty(c)
}

func pathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid id"))
		return 0, false
	}
	return id, true
}

func failGetErr(c *gin.Context, err error) {
	if errors.Is(err, ErrNotFound) {
		httpx.Fail(c, httpx.ErrNotFound("user not found"))
		return
	}
	if errors.Is(err, ErrUsernameTaken) {
		httpx.Fail(c, httpx.ErrConflict("username already taken"))
		return
	}
	httpx.Fail(c, httpx.ErrInternal("user operation failed"))
}
