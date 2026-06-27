package stats

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nnkglobal/c5-backend/internal/httpx"
)

// Handler exposes the D2 /stats/overview endpoint as a gin handler.
type Handler struct {
	svc *Service
}

// NewHandler builds the stats HTTP handler over the aggregation service.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts the stats routes (caller wraps them in Authn+Authz).
func (h *Handler) RegisterRoutes(rg gin.IRouter) {
	rg.GET("/stats/overview", h.Overview)
}

// Overview handles GET /stats/overview (D2 extended shape). Filters: project_id,
// from, to, inspector_id. A malformed timestamp -> 422.
func (h *Handler) Overview(c *gin.Context) {
	f, ok := parseFilter(c)
	if !ok {
		return
	}
	ov, err := h.svc.Overview(c.Request.Context(), f)
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal("failed to compute stats overview"))
		return
	}
	httpx.OK(c, ov)
}

// parseFilter reads the D2 filter set (project_id, inspector_id, from, to) from
// the query string (RFC3339 time bounds). A bad timestamp writes a 422.
func parseFilter(c *gin.Context) (Filter, bool) {
	var f Filter
	if v, ok := queryInt64(c, "project_id"); ok {
		f.ProjectID = &v
	}
	if v, ok := queryInt64(c, "inspector_id"); ok {
		f.InspectorID = &v
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
