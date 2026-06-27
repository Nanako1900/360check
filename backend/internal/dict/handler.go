package dict

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/httpx"
	"github.com/nnkglobal/c5-backend/internal/rbac"
)

// Handler exposes the dictionary + config endpoints as gin handlers.
type Handler struct {
	svc *Service
}

// NewHandler builds the dict HTTP handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts the dict + config routes (caller wraps them in
// Authn+Authz). Paths mirror the frozen API contract under /api/v1.
func (h *Handler) RegisterRoutes(rg gin.IRouter) {
	// dict_type
	rg.GET("/dict/types", h.ListDictTypes)
	rg.POST("/dict/types", h.CreateDictType)
	rg.PUT("/dict/types/:id", h.UpdateDictType)
	rg.DELETE("/dict/types/:id", h.DeleteDictType)

	// dict_item pull-with-version (ETag); items addressed by the type's code.
	rg.GET("/dict/types/:id/items", h.ListItems)

	// dict_item mutations
	rg.POST("/dict/items", h.CreateItem)
	rg.PUT("/dict/items/:id", h.UpdateItem)
	rg.DELETE("/dict/items/:id", h.DeleteItem)

	// app_config
	rg.GET("/config/:key", h.GetConfig)
	rg.PUT("/config/:key", h.PutConfig)
	rg.GET("/config/:key/history", h.GetConfigHistory)
}

// --- dict_type --------------------------------------------------------------

// ListDictTypes handles GET /dict/types (?scope=&is_active=).
func (h *Handler) ListDictTypes(c *gin.Context) {
	var scope *string
	if v := c.Query("scope"); v != "" {
		scope = &v
	}
	var isActive *bool
	if v := c.Query("is_active"); v != "" {
		b := v == "true" || v == "1"
		isActive = &b
	}
	items, err := h.svc.ListDictTypes(c.Request.Context(), scope, isActive)
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal("failed to list dict types"))
		return
	}
	// Unpaginated admin catalog; emit empty meta for envelope shape consistency.
	httpx.List(c, items, oapi.Meta{})
}

// CreateDictType handles POST /dict/types.
func (h *Handler) CreateDictType(c *gin.Context) {
	var in oapi.DictTypeCreate
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}
	dt, err := h.svc.CreateDictType(c.Request.Context(), in, rbac.UserIDFromContext(c))
	if err != nil {
		failErr(c, err, "dict type")
		return
	}
	httpx.Created(c, dt)
}

// UpdateDictType handles PUT /dict/types/{id}.
func (h *Handler) UpdateDictType(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in oapi.DictTypeUpdate
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}
	dt, err := h.svc.UpdateDictType(c.Request.Context(), id, in, rbac.UserIDFromContext(c))
	if err != nil {
		failErr(c, err, "dict type")
		return
	}
	httpx.OK(c, dt)
}

// DeleteDictType handles DELETE /dict/types/{id} (soft delete).
func (h *Handler) DeleteDictType(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteDictType(c.Request.Context(), id, rbac.UserIDFromContext(c)); err != nil {
		failErr(c, err, "dict type")
		return
	}
	httpx.OKEmpty(c)
}

// --- dict_item pull-with-version --------------------------------------------

// ListItems handles GET /dict/types/{code}/items. It sets ETag to the stored
// content_hash and returns 304 when the client's If-None-Match matches. The
// payload includes retired (is_active=false) items by default so clients can
// render historical references; ?include_inactive=false drops them.
func (h *Handler) ListItems(c *gin.Context) {
	code := c.Param("id") // the :id segment carries the dict_type code here
	if code == "" {
		httpx.Fail(c, httpx.ErrValidation("invalid dict type code"))
		return
	}
	includeInactive := true
	if v := c.Query("include_inactive"); v != "" {
		includeInactive = !(v == "false" || v == "0")
	}
	ifNoneMatch := c.GetHeader("If-None-Match")

	res, err := h.svc.GetItemsByTypeCode(c.Request.Context(), code, includeInactive, ifNoneMatch)
	if err != nil {
		failErr(c, err, "dict type")
		return
	}
	// Always advertise the current ETag (even on 304) so caches refresh it.
	c.Header("ETag", quoteETag(res.ETag))
	if res.NotModified {
		c.Status(http.StatusNotModified)
		return
	}
	httpx.OK(c, res.Payload)
}

// --- dict_item mutations ----------------------------------------------------

// CreateItem handles POST /dict/items.
func (h *Handler) CreateItem(c *gin.Context) {
	var in oapi.DictItemCreate
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}
	item, err := h.svc.CreateItem(c.Request.Context(), in, rbac.UserIDFromContext(c))
	if err != nil {
		failErr(c, err, "dict item")
		return
	}
	httpx.Created(c, item)
}

// UpdateItem handles PUT /dict/items/{id} (retire via is_active=false).
func (h *Handler) UpdateItem(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in oapi.DictItemUpdate
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}
	item, err := h.svc.UpdateItem(c.Request.Context(), id, in, rbac.UserIDFromContext(c))
	if err != nil {
		failErr(c, err, "dict item")
		return
	}
	httpx.OK(c, item)
}

// DeleteItem handles DELETE /dict/items/{id}. A referenced item is retired
// (is_active=false), never hard-deleted; an unreferenced item is soft-deleted.
func (h *Handler) DeleteItem(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if _, err := h.svc.DeleteItem(c.Request.Context(), id, rbac.UserIDFromContext(c)); err != nil {
		failErr(c, err, "dict item")
		return
	}
	httpx.OKEmpty(c)
}

// --- app_config -------------------------------------------------------------

// GetConfig handles GET /config/{key}. ETag-on-content_hash + 304 like items.
func (h *Handler) GetConfig(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		httpx.Fail(c, httpx.ErrValidation("invalid config key"))
		return
	}
	ifNoneMatch := c.GetHeader("If-None-Match")
	res, err := h.svc.GetConfig(c.Request.Context(), key, ifNoneMatch)
	if err != nil {
		failErr(c, err, "config")
		return
	}
	c.Header("ETag", quoteETag(res.ETag))
	if res.NotModified {
		c.Status(http.StatusNotModified)
		return
	}
	httpx.OK(c, res.Config)
}

// PutConfig handles PUT /config/{key} (versioned write in a tx).
func (h *Handler) PutConfig(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		httpx.Fail(c, httpx.ErrValidation("invalid config key"))
		return
	}
	var in oapi.AppConfigUpdate
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}
	cfg, err := h.svc.PutConfig(c.Request.Context(), key, in, rbac.UserIDFromContext(c))
	if err != nil {
		failErr(c, err, "config")
		return
	}
	c.Header("ETag", quoteETag(cfg.ContentHash))
	httpx.OK(c, cfg)
}

// GetConfigHistory handles GET /config/{key}/history (all versions, newest first).
func (h *Handler) GetConfigHistory(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		httpx.Fail(c, httpx.ErrValidation("invalid config key"))
		return
	}
	history, err := h.svc.GetConfigHistory(c.Request.Context(), key)
	if err != nil {
		failErr(c, err, "config")
		return
	}
	httpx.List(c, history, oapi.Meta{})
}

// --- helpers ----------------------------------------------------------------

// pathID parses the :id path param; a non-numeric id -> 422 VALIDATION_FAILED.
func pathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, httpx.ErrValidation("invalid id"))
		return 0, false
	}
	return id, true
}

// quoteETag wraps a content_hash in the strong-validator double-quote syntax
// (`"<hash>"`) required by the HTTP ETag header grammar.
func quoteETag(hash string) string { return `"` + hash + `"` }

// failErr maps a service error to the canonical envelope code. The resource name
// personalizes the not-found message.
func failErr(c *gin.Context, err error, resource string) {
	var ve *ErrValidation
	if errors.As(err, &ve) {
		httpx.Fail(c, httpx.ErrValidation(ve.Message).WithDetails(ve.Details...))
		return
	}
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Fail(c, httpx.ErrNotFound(resource+" not found"))
	case errors.Is(err, ErrCodeTaken):
		httpx.Fail(c, httpx.ErrConflict("dict type code already taken"))
	case errors.Is(err, ErrItemCodeTaken):
		httpx.Fail(c, httpx.ErrConflict("dict item code already taken within type"))
	case errors.Is(err, ErrConfigConflict):
		httpx.Fail(c, httpx.ErrConflict("config version conflict; retry"))
	default:
		httpx.Fail(c, httpx.ErrInternal(resource+" operation failed"))
	}
}
