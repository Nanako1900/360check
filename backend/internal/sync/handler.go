package sync

import (
	"github.com/gin-gonic/gin"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/httpx"
	"github.com/nnkglobal/c5-backend/internal/rbac"
)

// Handler exposes POST /sync/batch as a gin handler.
type Handler struct {
	svc *Service
}

// NewHandler builds the sync HTTP handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts the sync route (the caller wraps it in Authn+Authz). The
// batch envelope is the only endpoint; per-item results live in the response body.
func (h *Handler) RegisterRoutes(rg gin.IRouter) {
	rg.POST("/sync/batch", h.Batch)
}

// Batch handles POST /sync/batch. The body is a SyncBatchRequest carrying any mix
// of inspections / trajectory points / problems / media references created
// offline. The whole batch is processed in one transaction with per-item
// savepoints, so a single bad item is rejected without aborting the batch. The
// HTTP status is always 200 on a well-formed envelope; per-item accept/reject is
// in the body. A malformed JSON body is the only 422 path. The acting user
// (rbac) supplies the default inspector/owner/creator for items that omit one.
func (h *Handler) Batch(c *gin.Context) {
	var req oapi.SyncBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}

	result, err := h.svc.Process(c.Request.Context(), req, rbac.UserIDFromContext(c))
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal("failed to process sync batch"))
		return
	}
	httpx.OK(c, result)
}
