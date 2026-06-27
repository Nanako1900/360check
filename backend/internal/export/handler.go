package export

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/httpx"
	"github.com/nnkglobal/c5-backend/internal/platform/cos"
	"github.com/nnkglobal/c5-backend/internal/rbac"
)

// Handler exposes the export create/poll/SSE endpoints as gin handlers.
type Handler struct {
	svc    *Service
	signer cos.Client // for SignedURL on SUCCEEDED (poll + SSE done)
}

// NewHandler builds the export HTTP handler. signer is used to produce the
// result_url for finished jobs; it may be nil (result_url then stays absent).
func NewHandler(svc *Service, signer cos.Client) *Handler {
	return &Handler{svc: svc, signer: signer}
}

// RegisterRoutes mounts the export routes (caller wraps them in Authn+Authz).
// The literal /exports/:job_uuid/events nests under the :job_uuid param.
func (h *Handler) RegisterRoutes(rg gin.IRouter) {
	rg.POST("/exports", h.Create)
	rg.GET("/exports/:job_uuid", h.Get)
	rg.GET("/exports/:job_uuid/events", h.Events)
}

// Create handles POST /exports. Body {type, params} -> a PENDING job + enqueue.
// 201 with the job (job_uuid, status PENDING).
func (h *Handler) Create(c *gin.Context) {
	var req oapi.ExportJobCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}
	if !validType(req.Type) {
		httpx.Fail(c, httpx.ErrValidation("type must be INSPECTION_RECORDS, PROBLEM_LIST or PROJECT_STATS"))
		return
	}

	var params map[string]any
	if req.Params != nil {
		params = *req.Params
	}

	job, err := h.svc.Create(c.Request.Context(), CreateInput{
		Type:        req.Type,
		Params:      params,
		RequestedBy: rbac.UserIDFromContext(c),
	})
	if err != nil {
		if errors.Is(err, ErrInvalidType) {
			httpx.Fail(c, httpx.ErrValidation("invalid export type"))
			return
		}
		if errors.Is(err, ErrInvalidStatus) {
			httpx.Fail(c, httpx.ErrValidation("status must be IN_PROGRESS, FINISHED or ABANDONED"))
			return
		}
		httpx.Fail(c, httpx.ErrInternal("failed to create export job"))
		return
	}
	httpx.Created(c, toAPI(c.Request.Context(), job, h.signer))
}

// Get handles GET /exports/{job_uuid} (poll fallback). Returns the job; on
// SUCCEEDED result_url is a signed CDN URL to the produced .xlsx.
func (h *Handler) Get(c *gin.Context) {
	jobUUID := c.Param("job_uuid")
	job, err := h.svc.GetByUUID(c.Request.Context(), jobUUID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Fail(c, httpx.ErrNotFound("export job not found"))
			return
		}
		httpx.Fail(c, httpx.ErrInternal("failed to load export job"))
		return
	}
	httpx.OK(c, toAPI(c.Request.Context(), job, h.signer))
}

// Events handles GET /exports/{job_uuid}/events (hand-written SSE). It streams
// `event: progress` data every ~1s and a terminal `event: done`, with heartbeat
// comment lines, setting the no-buffer headers Tencent CLB/CDN respect. Clients
// that lose the stream fall back to polling GET /exports/{job_uuid}.
func (h *Handler) Events(c *gin.Context) {
	jobUUID := c.Param("job_uuid")
	// Confirm the job exists before committing to a streaming response (so a 404
	// is a normal envelope, not an SSE error frame).
	if _, err := h.svc.GetByUUID(c.Request.Context(), jobUUID); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Fail(c, httpx.ErrNotFound("export job not found"))
			return
		}
		httpx.Fail(c, httpx.ErrInternal("failed to load export job"))
		return
	}

	setSSEHeaders(c.Writer)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		httpx.Fail(c, httpx.ErrInternal("streaming unsupported"))
		return
	}

	streamJobEvents(c.Request.Context(), sseSink{w: c.Writer, f: flusher},
		h.svc, h.signer, jobUUID, pollInterval)
}

// setSSEHeaders writes the SSE + anti-buffering headers (Tencent CLB/CDN respect
// X-Accel-Buffering: no). Connection: keep-alive keeps the stream open.
func setSSEHeaders(w gin.ResponseWriter) {
	hdr := w.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache")
	hdr.Set("Connection", "keep-alive")
	hdr.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
}
