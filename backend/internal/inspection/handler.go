package inspection

import (
	"errors"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/httpx"
	"github.com/nnkglobal/c5-backend/internal/rbac"
)

// Handler exposes the inspection-session endpoints as gin handlers.
type Handler struct {
	svc *Service
}

// NewHandler builds the inspection HTTP handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts the inspection routes (caller wraps them in Authn+Authz).
// Order matters: the literal /inspections/start must register before the
// /inspections/:id wildcard so gin does not treat "start" as an id.
func (h *Handler) RegisterRoutes(rg gin.IRouter) {
	rg.GET("/inspections", h.List)
	rg.POST("/inspections/start", h.Start)
	rg.GET("/inspections/:id", h.Get)
	rg.POST("/inspections/:id/finish", h.Finish)
	rg.GET("/inspections/:id/trajectory", h.Trajectory)
}

// List handles GET /inspections (filters: project_id, inspector_id, status,
// from, to; paginated with meta).
func (h *Handler) List(c *gin.Context) {
	page := httpx.ParsePage(c)

	var f ListFilter
	if v, ok := queryInt64(c, "project_id"); ok {
		f.ProjectID = &v
	}
	if v, ok := queryInt64(c, "inspector_id"); ok {
		f.InspectorID = &v
	}
	if v := c.Query("status"); v != "" {
		if !validStatus(v) {
			httpx.Fail(c, httpx.ErrValidation("invalid status"))
			return
		}
		f.Status = &v
	}
	if v, ok, bad := queryTime(c, "from"); bad {
		httpx.Fail(c, httpx.ErrValidation("invalid from timestamp"))
		return
	} else if ok {
		f.From = &v
	}
	if v, ok, bad := queryTime(c, "to"); bad {
		httpx.Fail(c, httpx.ErrValidation("invalid to timestamp"))
		return
	} else if ok {
		f.To = &v
	}

	items, total, err := h.svc.List(c.Request.Context(), f, page.Limit(), page.Offset())
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal("failed to list inspections"))
		return
	}
	httpx.List(c, items, httpx.MetaFor(total, page))
}

// Get handles GET /inspections/{id}.
func (h *Handler) Get(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	insp, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, insp)
}

// Start handles POST /inspections/start. inspector_id defaults to the
// authenticated user when omitted; client_uuid makes the call idempotent.
func (h *Handler) Start(c *gin.Context) {
	var req oapi.InspectionStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}
	if req.ProjectId == 0 {
		httpx.Fail(c, httpx.ErrValidation("project_id is required"))
		return
	}
	if req.StartedAt.IsZero() {
		httpx.Fail(c, httpx.ErrValidation("started_at is required"))
		return
	}

	inspectorID := rbac.UserIDFromContext(c)
	if req.InspectorId != nil && *req.InspectorId != 0 {
		inspectorID = *req.InspectorId
	}

	in := StartInput{
		ProjectID:   req.ProjectId,
		StartedAt:   req.StartedAt.UTC(),
		InspectorID: inspectorID,
		TaskID:      req.TaskId,
		DeviceInfo:  req.DeviceInfo,
		Note:        req.Note,
	}
	if req.ClientUuid != nil {
		s := req.ClientUuid.String()
		in.ClientUUID = &s
	}

	insp, err := h.svc.Start(c.Request.Context(), in, rbac.UserIDFromContext(c))
	if err != nil {
		switch {
		case errors.Is(err, ErrProjectMissing):
			httpx.Fail(c, httpx.ErrValidation("project_id does not reference a live project").
				WithDetails(oapi.ErrorDetail{Field: "project_id", Code: "not_found", Message: "project_id does not reference a live project"}))
		case errors.Is(err, ErrTaskMissing):
			httpx.Fail(c, httpx.ErrValidation("task_id does not reference a live task").
				WithDetails(oapi.ErrorDetail{Field: "task_id", Code: "not_found", Message: "task_id does not reference a live task"}))
		case errors.Is(err, ErrInspectorMissing):
			httpx.Fail(c, httpx.ErrValidation("inspector_id does not reference a known user").
				WithDetails(oapi.ErrorDetail{Field: "inspector_id", Code: "not_found", Message: "inspector_id does not reference a known user"}))
		case errors.Is(err, ErrForeignKeyViolated):
			httpx.Fail(c, httpx.ErrValidation("a referenced entity does not exist"))
		default:
			httpx.Fail(c, httpx.ErrInternal("failed to start inspection"))
		}
		return
	}
	httpx.Created(c, insp)
}

// Finish handles POST /inspections/{id}/finish. Only an IN_PROGRESS session may
// be finished; Status=ABANDONED discards it instead of marking it FINISHED.
func (h *Handler) Finish(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req oapi.InspectionFinishRequest
	// Body is optional; ignore a decode error on an empty/missing body but reject
	// a malformed non-empty body.
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Fail(c, httpx.ErrValidation("invalid request body"))
			return
		}
	}

	in := FinishInput{Note: req.Note}
	if req.EndedAt != nil {
		t := req.EndedAt.UTC()
		in.EndedAt = &t
	}
	if req.Status != nil {
		switch *req.Status {
		case oapi.InspectionStatusABANDONED:
			in.Discard = true
		case oapi.InspectionStatusFINISHED:
			// explicit normal finish
		default:
			httpx.Fail(c, httpx.ErrValidation("status must be FINISHED or ABANDONED"))
			return
		}
	}

	insp, err := h.svc.Finish(c.Request.Context(), id, in, rbac.UserIDFromContext(c))
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, insp)
}

// Trajectory handles GET /inspections/{id}/trajectory (route + ordered points, WGS84).
func (h *Handler) Trajectory(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	traj, err := h.svc.Trajectory(c.Request.Context(), id)
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, traj)
}

// failErr maps service sentinel errors to the canonical envelope codes.
func failErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Fail(c, httpx.ErrNotFound("inspection not found"))
	case errors.Is(err, ErrNotInProgress):
		httpx.Fail(c, httpx.ErrConflict("inspection is not in progress"))
	case errors.Is(err, ErrEndedBeforeStarted):
		httpx.Fail(c, httpx.ErrValidation("ended_at must be on or after started_at").
			WithDetails(oapi.ErrorDetail{Field: "ended_at", Code: "out_of_range", Message: "ended_at must be >= started_at"}))
	case errors.Is(err, ErrProjectMissing):
		httpx.Fail(c, httpx.ErrValidation("project not found"))
	default:
		httpx.Fail(c, httpx.ErrInternal("inspection operation failed"))
	}
}

// pathID parses the :id path param; invalid -> 422 VALIDATION_FAILED.
func pathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, httpx.ErrValidation("invalid id"))
		return 0, false
	}
	return id, true
}

// queryInt64 reads an int64 query param; absent or unparsable -> (0, false).
func queryInt64(c *gin.Context, key string) (int64, bool) {
	v := c.Query(key)
	if v == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// queryTime reads an RFC3339 timestamp query param. Returns (value, present, bad).
func queryTime(c *gin.Context, key string) (time.Time, bool, bool) {
	v := c.Query(key)
	if v == "" {
		return time.Time{}, false, false
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, false, true
	}
	return t.UTC(), true, false
}

// validStatus reports whether s is a frozen inspection_status literal.
func validStatus(s string) bool {
	return s == statusInProgress || s == statusFinished || s == statusAbandoned
}
