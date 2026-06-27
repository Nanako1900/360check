package problem

import (
	"errors"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/httpx"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
	"github.com/nnkglobal/c5-backend/internal/rbac"
)

// Handler exposes the problem + processing-log + map endpoints as gin handlers.
type Handler struct {
	svc  *Service
	pool *db.Pool // for ST_IsValid geometry validation on the write path
}

// NewHandler builds the problem HTTP handler. The pool is used only for
// server-side geometry validation (db.ValidateGeomWGS84) before create.
func NewHandler(svc *Service, pool *db.Pool) *Handler {
	return &Handler{svc: svc, pool: pool}
}

// RegisterRoutes mounts the problem routes (caller wraps them in Authn+Authz).
// Order matters: the literal /problems/map must register before the
// /problems/:id wildcard so gin does not treat "map" as an id.
func (h *Handler) RegisterRoutes(rg gin.IRouter) {
	rg.GET("/problems", h.List)
	rg.POST("/problems", h.Create)
	rg.GET("/problems/map", h.Map)
	rg.GET("/problems/:id", h.Get)
	rg.PUT("/problems/:id", h.Update)
	rg.DELETE("/problems/:id", h.Delete)
	rg.GET("/problems/:id/logs", h.ListLogs)
	rg.POST("/problems/:id/logs", h.CreateLog)
}

// List handles GET /problems (D1 filters: project_id, type, status, category,
// from, to, inspector_id, inspection_id; paginated with meta).
func (h *Handler) List(c *gin.Context) {
	page := httpx.ParsePage(c)
	f, ok := h.parseFilter(c)
	if !ok {
		return
	}
	items, total, err := h.svc.List(c.Request.Context(), f, page.Limit(), page.Offset())
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal("failed to list problems"))
		return
	}
	httpx.List(c, items, httpx.MetaFor(total, page))
}

// Create handles POST /problems. geom must be a WGS84 Point; inspector_id
// defaults to the authenticated user; client_uuid makes the call idempotent.
func (h *Handler) Create(c *gin.Context) {
	var req oapi.ProblemCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}
	if req.ProjectId == 0 {
		httpx.Fail(c, httpx.ErrValidation("project_id is required"))
		return
	}
	if req.CapturedAt.IsZero() {
		httpx.Fail(c, httpx.ErrValidation("captured_at is required"))
		return
	}
	if len(req.Geom.Coordinates) < 2 {
		httpx.Fail(c, httpx.ErrValidation("geom must be a Point with [lon, lat] coordinates"))
		return
	}

	geomEWKB, err := geoJSONToPointEWKB(req.Geom)
	if err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid geometry"))
		return
	}
	// Server-side ST_IsValid + SRID check before the DB CHECK fires (clearer 422).
	valid, reason, err := db.ValidateGeomWGS84(c.Request.Context(), h.pool, geomEWKB)
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal("failed to validate geometry"))
		return
	}
	if !valid {
		httpx.Fail(c, httpx.ErrValidation("invalid geometry: "+reason).
			WithDetails(oapi.ErrorDetail{Field: "geom", Code: "invalid", Message: reason}))
		return
	}

	inspectorID := rbac.UserIDFromContext(c)
	if req.InspectorId != nil && *req.InspectorId != 0 {
		inspectorID = *req.InspectorId
	}

	in := CreateInput{
		ProjectID:       req.ProjectId,
		GeomEWKB:        geomEWKB,
		CapturedAt:      req.CapturedAt.UTC(),
		InspectorID:     inspectorID,
		InspectionID:    req.InspectionId,
		TypeItemID:      req.TypeItemId,
		StatusItemID:    req.StatusItemId,
		CategoryItemID:  req.CategoryItemId,
		DictVersionUsed: req.DictVersionUsed,
		CoverMediaID:    req.CoverMediaId,
		Title:           req.Title,
		Description:     req.Description,
		Note:            req.Note,
	}
	if req.ClientUuid != nil {
		s := req.ClientUuid.String()
		in.ClientUUID = &s
	}

	p, err := h.svc.Create(c.Request.Context(), in, rbac.UserIDFromContext(c))
	if err != nil {
		failCreateErr(c, err)
		return
	}
	httpx.Created(c, p)
}

// Get handles GET /problems/{id}.
func (h *Handler) Get(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	p, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, p)
}

// Update handles PUT /problems/{id}. D3: changing status_item_id appends a
// STATUS_CHANGE log row in the same transaction (handled in the service).
func (h *Handler) Update(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req oapi.ProblemUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}
	in := UpdateInput{
		StatusItemID:   req.StatusItemId,
		CategoryItemID: req.CategoryItemId,
		TypeItemID:     req.TypeItemId,
		InspectionID:   req.InspectionId,
		CoverMediaID:   req.CoverMediaId,
		Title:          req.Title,
		Description:    req.Description,
		Note:           req.Note,
	}
	p, err := h.svc.Update(c.Request.Context(), id, in, rbac.UserIDFromContext(c))
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, p)
}

// Delete handles DELETE /problems/{id} (soft delete).
func (h *Handler) Delete(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id, rbac.UserIDFromContext(c)); err != nil {
		failErr(c, err)
		return
	}
	httpx.OKEmpty(c)
}

// ListLogs handles GET /problems/{id}/logs (chronological).
func (h *Handler) ListLogs(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	logs, err := h.svc.ListLogs(c.Request.Context(), id)
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, logs)
}

// CreateLog handles POST /problems/{id}/logs. Only COMMENT or REASSIGN are
// accepted; STATUS_CHANGE is backend-only (D3) and is rejected with 422.
func (h *Handler) CreateLog(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req oapi.ProblemLogCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}
	action := string(req.Action)
	if !validClientLogAction(action) {
		httpx.Fail(c, httpx.ErrValidation("action must be COMMENT or REASSIGN").
			WithDetails(oapi.ErrorDetail{Field: "action", Code: "invalid", Message: "STATUS_CHANGE is backend-only"}))
		return
	}
	in := CreateLogInput{Action: action, Note: req.Note, OperatorID: req.OperatorId}
	log, err := h.svc.CreateLog(c.Request.Context(), id, in, rbac.UserIDFromContext(c))
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.Created(c, log)
}

// Map handles GET /problems/map (GeoJSON FeatureCollection, WGS84, capped).
func (h *Handler) Map(c *gin.Context) {
	f, ok := h.parseFilter(c)
	if !ok {
		return
	}
	limit := MapLimitDefault
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	fc, err := h.svc.Map(c.Request.Context(), f, limit)
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal("failed to build problem map"))
		return
	}
	httpx.OK(c, fc)
}

// parseFilter reads the shared D1 filter set from the query string, writing a 422
// and returning ok=false on a malformed timestamp.
func (h *Handler) parseFilter(c *gin.Context) (ListFilter, bool) {
	var f ListFilter
	if v, ok := queryInt64(c, "project_id"); ok {
		f.ProjectID = &v
	}
	if v, ok := queryInt64(c, "type"); ok {
		f.Type = &v
	}
	if v, ok := queryInt64(c, "status"); ok {
		f.Status = &v
	}
	if v, ok := queryInt64(c, "category"); ok {
		f.Category = &v
	}
	if v, ok := queryInt64(c, "inspector_id"); ok {
		f.InspectorID = &v
	}
	if v, ok := queryInt64(c, "inspection_id"); ok {
		f.InspectionID = &v
	}
	if v, ok, bad := queryTime(c, "from"); bad {
		httpx.Fail(c, httpx.ErrValidation("invalid from timestamp"))
		return f, false
	} else if ok {
		f.From = &v
	}
	if v, ok, bad := queryTime(c, "to"); bad {
		httpx.Fail(c, httpx.ErrValidation("invalid to timestamp"))
		return f, false
	} else if ok {
		f.To = &v
	}
	return f, true
}

// failCreateErr maps create-time sentinels (FK violations) to the envelope codes.
func failCreateErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrProjectMissing):
		httpx.Fail(c, httpx.ErrValidation("project not found"))
	case errors.Is(err, ErrInspectorMissing):
		httpx.Fail(c, httpx.ErrValidation("inspector not found"))
	case errors.Is(err, ErrNotFound):
		httpx.Fail(c, httpx.ErrValidation("referenced inspection not found"))
	default:
		httpx.Fail(c, httpx.ErrInternal("failed to create problem"))
	}
}

// failErr maps service sentinel errors to the canonical envelope codes.
func failErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Fail(c, httpx.ErrNotFound("problem not found"))
	case errors.Is(err, ErrInvalidLogAction):
		httpx.Fail(c, httpx.ErrValidation("action must be COMMENT or REASSIGN"))
	case errors.Is(err, ErrProjectMissing):
		httpx.Fail(c, httpx.ErrValidation("project not found"))
	case errors.Is(err, ErrInspectorMissing):
		httpx.Fail(c, httpx.ErrValidation("inspector not found"))
	default:
		httpx.Fail(c, httpx.ErrInternal("problem operation failed"))
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
