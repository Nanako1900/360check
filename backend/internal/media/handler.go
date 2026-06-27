package media

import (
	"errors"
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/httpx"
	"github.com/nnkglobal/c5-backend/internal/rbac"
)

// Enqueuer is the minimal asynq enqueue surface the handler needs (satisfied by
// *asynq.Client). Kept as an interface so the handler can be unit-tested without
// a Redis-backed client.
type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Handler exposes the media endpoints as gin handlers.
type Handler struct {
	svc      *Service
	enqueuer Enqueuer
}

// NewHandler builds the media HTTP handler. enqueuer may be nil (e.g. in tests
// that do not assert enqueue); when nil, confirm simply skips enqueueing.
func NewHandler(svc *Service, enqueuer Enqueuer) *Handler {
	return &Handler{svc: svc, enqueuer: enqueuer}
}

// RegisterRoutes mounts the media routes (caller wraps them in Authn+Authz).
func (h *Handler) RegisterRoutes(rg gin.IRouter) {
	rg.POST("/media/upload-credentials", h.UploadCredentials)
	rg.POST("/media/confirm", h.Confirm)
	rg.GET("/media/:id", h.Get)
}

// UploadCredentials handles POST /media/upload-credentials. Per D4 only
// tier=original is accepted; web/thumb → 422.
func (h *Handler) UploadCredentials(c *gin.Context) {
	var req oapi.MediaUploadCredentialsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}
	if req.OwnerId == 0 {
		httpx.Fail(c, httpx.ErrValidation("owner_id is required"))
		return
	}
	if req.ClientUuid == uuid.Nil {
		httpx.Fail(c, httpx.ErrValidation("client_uuid is required"))
		return
	}

	in := CredentialsInput{
		OwnerType:   req.OwnerType,
		OwnerID:     req.OwnerId,
		Tier:        req.Tier,
		ContentType: req.ContentType,
		ByteSize:    req.ByteSize,
		ClientUUID:  req.ClientUuid,
	}
	creds, err := h.svc.IssueUploadCredentials(c.Request.Context(), in, rbac.UserIDFromContext(c))
	if err != nil {
		switch {
		case errors.Is(err, ErrTierNotOriginal):
			httpx.Fail(c, httpx.ErrValidation("only tier=original may be uploaded by the client").
				WithDetails(oapi.ErrorDetail{Field: "tier", Code: "unsupported", Message: "APP uploads only original; web/thumb are backend-derived"}))
		case errors.Is(err, ErrInvalidOwnerType):
			httpx.Fail(c, httpx.ErrValidation("invalid owner_type").
				WithDetails(oapi.ErrorDetail{Field: "owner_type", Code: "invalid", Message: "owner_type must be problem|inspection|project|user"}))
		default:
			httpx.Fail(c, httpx.ErrInternal("failed to issue upload credentials"))
		}
		return
	}
	// Frozen OpenAPI declares only 200 → MediaUploadCredentialsResponse for this
	// endpoint (no 201); the resource is the STS grant, not a created entity.
	httpx.OK(c, creds)
}

// Confirm handles POST /media/confirm. On a verified original transitioning to
// CONFIRMED it enqueues the derive-media-tiers task.
func (h *Handler) Confirm(c *gin.Context) {
	var req oapi.MediaConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}

	in := ConfirmInput{
		ClientUUID: req.ClientUuid,
		Key:        req.Key,
		Etag:       req.Etag,
		ByteSize:   req.ByteSize,
		Width:      req.Width,
		Height:     req.Height,
	}
	asset, enqueue, err := h.svc.Confirm(c.Request.Context(), in, rbac.UserIDFromContext(c))
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			httpx.Fail(c, httpx.ErrNotFound("media asset not found"))
		case errors.Is(err, ErrVerifyFailed):
			httpx.Fail(c, httpx.ErrMediaVerifyFailed("media object verification failed; key missing or etag/size mismatch"))
		default:
			httpx.Fail(c, httpx.ErrInternal("failed to confirm media"))
		}
		return
	}

	if enqueue {
		h.enqueueDerive(asset.Id)
	}
	httpx.OK(c, asset)
}

// enqueueDerive fires the derive-media-tiers task for an id. An enqueue failure
// does NOT fail the client's confirm (the confirm already succeeded and is the
// source of truth; the reaper/derive can be re-driven), but it MUST be observable
// so a Redis-down derive miss is not silent — log a WARN with the original id.
func (h *Handler) enqueueDerive(originalID int64) {
	if h.enqueuer == nil {
		return
	}
	task, err := NewDeriveTask(originalID)
	if err != nil {
		slog.Warn("media: build derive task failed", "original_id", originalID, "error", err)
		return
	}
	if _, err := h.enqueuer.Enqueue(task); err != nil {
		slog.Warn("media: enqueue derive task failed; derive must be re-driven",
			"original_id", originalID, "error", err)
	}
}

// Get handles GET /media/{id} — returns the asset with a signed CDN URL.
func (h *Handler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, httpx.ErrValidation("invalid id"))
		return
	}
	asset, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Fail(c, httpx.ErrNotFound("media asset not found"))
			return
		}
		httpx.Fail(c, httpx.ErrInternal("failed to load media"))
		return
	}
	httpx.OK(c, asset)
}
