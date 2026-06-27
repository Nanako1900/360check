package project

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/httpx"
	"github.com/nnkglobal/c5-backend/internal/rbac"
)

// Handler exposes project and inspection-task endpoints as gin handlers.
type Handler struct {
	svc *Service
}

// NewHandler builds the project HTTP handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts the project + task CRUD routes (caller wraps them in
// Authn+Authz).
func (h *Handler) RegisterRoutes(rg gin.IRouter) {
	rg.GET("/projects", h.ListProjects)
	rg.POST("/projects", h.CreateProject)
	rg.GET("/projects/:id", h.GetProject)
	rg.PUT("/projects/:id", h.UpdateProject)
	rg.DELETE("/projects/:id", h.DeleteProject)

	rg.GET("/tasks", h.ListTasks)
	rg.POST("/tasks", h.CreateTask)
	rg.GET("/tasks/:id", h.GetTask)
	rg.PUT("/tasks/:id", h.UpdateTask)
	rg.DELETE("/tasks/:id", h.DeleteTask)
}

// --- projects ---------------------------------------------------------------

// ListProjects handles GET /projects.
func (h *Handler) ListProjects(c *gin.Context) {
	page := httpx.ParsePage(c)
	var q *string
	if v := c.Query("q"); v != "" {
		q = &v
	}
	var status *string
	if v := c.Query("status"); v != "" {
		status = &v
	}
	items, total, err := h.svc.ListProjects(c.Request.Context(), q, status, page.Limit(), page.Offset())
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal("failed to list projects"))
		return
	}
	httpx.List(c, items, httpx.MetaFor(total, page))
}

// CreateProject handles POST /projects.
func (h *Handler) CreateProject(c *gin.Context) {
	var in oapi.ProjectCreate
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}
	p, err := h.svc.CreateProject(c.Request.Context(), in, rbac.UserIDFromContext(c))
	if err != nil {
		failErr(c, err, "project")
		return
	}
	httpx.Created(c, p)
}

// GetProject handles GET /projects/{id}.
func (h *Handler) GetProject(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	p, err := h.svc.GetProject(c.Request.Context(), id)
	if err != nil {
		failErr(c, err, "project")
		return
	}
	httpx.OK(c, p)
}

// UpdateProject handles PUT /projects/{id}.
func (h *Handler) UpdateProject(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in oapi.ProjectUpdate
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}
	p, err := h.svc.UpdateProject(c.Request.Context(), id, in, rbac.UserIDFromContext(c))
	if err != nil {
		failErr(c, err, "project")
		return
	}
	httpx.OK(c, p)
}

// DeleteProject handles DELETE /projects/{id} (soft delete; guarded).
func (h *Handler) DeleteProject(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteProject(c.Request.Context(), id, rbac.UserIDFromContext(c)); err != nil {
		failErr(c, err, "project")
		return
	}
	httpx.OKEmpty(c)
}

// --- inspection_tasks -------------------------------------------------------

// ListTasks handles GET /tasks.
func (h *Handler) ListTasks(c *gin.Context) {
	page := httpx.ParsePage(c)
	projectID := queryInt64(c, "project_id")
	assigneeID := queryInt64(c, "assignee_id")
	var status *string
	if v := c.Query("status"); v != "" {
		status = &v
	}
	items, total, err := h.svc.ListTasks(c.Request.Context(), projectID, status, assigneeID, page.Limit(), page.Offset())
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal("failed to list tasks"))
		return
	}
	httpx.List(c, items, httpx.MetaFor(total, page))
}

// CreateTask handles POST /tasks.
func (h *Handler) CreateTask(c *gin.Context) {
	var in oapi.InspectionTaskCreate
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}
	t, err := h.svc.CreateTask(c.Request.Context(), in, rbac.UserIDFromContext(c))
	if err != nil {
		failErr(c, err, "task")
		return
	}
	httpx.Created(c, t)
}

// GetTask handles GET /tasks/{id}.
func (h *Handler) GetTask(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	t, err := h.svc.GetTask(c.Request.Context(), id)
	if err != nil {
		failErr(c, err, "task")
		return
	}
	httpx.OK(c, t)
}

// UpdateTask handles PUT /tasks/{id}.
func (h *Handler) UpdateTask(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in oapi.InspectionTaskUpdate
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, httpx.ErrValidation("invalid request body"))
		return
	}
	t, err := h.svc.UpdateTask(c.Request.Context(), id, in, rbac.UserIDFromContext(c))
	if err != nil {
		failErr(c, err, "task")
		return
	}
	httpx.OK(c, t)
}

// DeleteTask handles DELETE /tasks/{id} (soft delete).
func (h *Handler) DeleteTask(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteTask(c.Request.Context(), id, rbac.UserIDFromContext(c)); err != nil {
		failErr(c, err, "task")
		return
	}
	httpx.OKEmpty(c)
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

// queryInt64 parses an optional int64 query param, returning nil when absent or
// unparseable so a malformed filter is ignored rather than erroring the list.
func queryInt64(c *gin.Context, key string) *int64 {
	v := c.Query(key)
	if v == "" {
		return nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

// failErr maps a service error to the canonical envelope code. The resource name
// ("project"/"task") personalizes the not-found message.
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
		httpx.Fail(c, httpx.ErrConflict("project code already taken"))
	case errors.Is(err, ErrProjectInUse):
		httpx.Fail(c, httpx.ErrConflict("project has active inspections or tasks"))
	default:
		httpx.Fail(c, httpx.ErrInternal(resource+" operation failed"))
	}
}
