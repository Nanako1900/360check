package rbac

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/httpx"
)

// Handler exposes role/permission management + user-role assignment as gin handlers.
type Handler struct {
	svc *Service
}

// NewHandler builds the rbac HTTP handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts roles/permissions + /users/{id}/roles (caller wraps in Authn+Authz).
func (h *Handler) RegisterRoutes(rg gin.IRouter) {
	rg.GET("/roles", h.ListRoles)
	rg.POST("/roles", h.CreateRole)
	rg.PUT("/roles/:id", h.UpdateRole)
	rg.DELETE("/roles/:id", h.DeleteRole)
	rg.GET("/roles/:id/permissions", h.GetRolePermissions)
	rg.PUT("/roles/:id/permissions", h.SetRolePermissions)
	rg.GET("/permissions", h.ListPermissions)
	rg.GET("/users/:id/roles", h.GetUserRoles)
	rg.PUT("/users/:id/roles", h.SetUserRoles)
}

func (h *Handler) ListRoles(c *gin.Context) {
	roles, err := h.svc.ListRoles(c.Request.Context())
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal("failed to list roles"))
		return
	}
	httpx.OK(c, roles)
}

func (h *Handler) CreateRole(c *gin.Context) {
	var in oapi.RoleCreate
	if err := c.ShouldBindJSON(&in); err != nil || in.Code == "" || in.Name == "" {
		httpx.Fail(c, httpx.ErrValidation("code and name are required"))
		return
	}
	r, err := h.svc.CreateRole(c.Request.Context(), in, currentUserID(c))
	if err != nil {
		if errors.Is(err, ErrRoleCodeTaken) {
			httpx.Fail(c, httpx.ErrConflict("role code already taken"))
			return
		}
		httpx.Fail(c, httpx.ErrInternal("failed to create role"))
		return
	}
	httpx.Created(c, r)
}

func (h *Handler) UpdateRole(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	var in oapi.RoleUpdate
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}
	r, err := h.svc.UpdateRole(c.Request.Context(), id, in, currentUserID(c))
	if err != nil {
		failRBACErr(c, err)
		return
	}
	httpx.OK(c, r)
}

func (h *Handler) DeleteRole(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteRole(c.Request.Context(), id); err != nil {
		if errors.Is(err, ErrSystemRole) {
			httpx.Fail(c, httpx.ErrConflict("system role cannot be deleted"))
			return
		}
		failRBACErr(c, err)
		return
	}
	httpx.OKEmpty(c)
}

func (h *Handler) GetRolePermissions(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	perms, err := h.svc.GetRolePermissions(c.Request.Context(), id)
	if err != nil {
		failRBACErr(c, err)
		return
	}
	httpx.OK(c, perms)
}

func (h *Handler) SetRolePermissions(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	var in oapi.SetRolePermissionsRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, httpx.ErrValidation("permission_ids is required"))
		return
	}
	perms, err := h.svc.SetRolePermissions(c.Request.Context(), id, in.PermissionIds)
	if err != nil {
		failRBACErr(c, err)
		return
	}
	httpx.OK(c, perms)
}

func (h *Handler) ListPermissions(c *gin.Context) {
	perms, err := h.svc.ListPermissions(c.Request.Context())
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal("failed to list permissions"))
		return
	}
	httpx.OK(c, perms)
}

func (h *Handler) GetUserRoles(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	roles, err := h.svc.GetUserRoles(c.Request.Context(), id)
	if err != nil {
		failRBACErr(c, err)
		return
	}
	httpx.OK(c, roles)
}

func (h *Handler) SetUserRoles(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	var in oapi.SetUserRolesRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, httpx.ErrValidation("role_ids is required"))
		return
	}
	roles, err := h.svc.SetUserRoles(c.Request.Context(), id, in.RoleIds)
	if err != nil {
		failRBACErr(c, err)
		return
	}
	httpx.OK(c, roles)
}

func parsePathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid id"))
		return 0, false
	}
	return id, true
}

func currentUserID(c *gin.Context) int64 { return UserIDFromContext(c) }

func failRBACErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrRoleNotFound):
		httpx.Fail(c, httpx.ErrNotFound("role not found"))
	case errors.Is(err, ErrUserNotFound):
		httpx.Fail(c, httpx.ErrNotFound("user not found"))
	case errors.Is(err, ErrRoleCodeTaken):
		httpx.Fail(c, httpx.ErrConflict("role code already taken"))
	default:
		httpx.Fail(c, httpx.ErrInternal("rbac operation failed"))
	}
}
