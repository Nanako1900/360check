//go:build integration

package export

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/rbac"
)

// MEDIUM fix: the export:run task is bounded by a Timeout so a runaway build
// cannot hold an asynq worker slot indefinitely.
//
// asynq stores NewTask opts on the (unexported) Task.opts field and merges them
// into the real Client.Enqueue; they are not surfaced through the Enqueuer's
// variadic, so a fake enqueuer cannot observe them. We instead pin the bound at
// the construction boundary the production code uses: the option produced by
// asynq.Timeout(exportRunTimeout) carries OptionType TimeoutOpt with a 10m value.
func TestExportRunTimeoutBound(t *testing.T) {
	assert.Equal(t, 10*time.Minute, exportRunTimeout, "export:run timeout bound is 10m")

	opt := asynq.Timeout(exportRunTimeout)
	assert.Equal(t, asynq.TimeoutOpt, opt.Type(), "wired as a Timeout option")
	assert.Equal(t, exportRunTimeout, opt.Value(), "carries the 10m bound")

	// The Create path still enqueues a single task (the opt rides on it).
	e := newExpEnv(t)
	job, err := e.svc.Create(context.Background(),
		CreateInput{Type: oapi.PROJECTSTATS, RequestedBy: e.adminID})
	require.NoError(t, err)
	assert.Equal(t, oapi.PENDING, job.Status)
	require.Len(t, e.enq.tasks, 1)
	assert.Equal(t, TaskTypeRun, e.enq.tasks[0].Type())
}

// LOW fix: an INSPECTION_RECORDS export with a valid enum status is accepted.
func TestCreate_InspectionStatus_ValidAccepted(t *testing.T) {
	e := newExpEnv(t)
	ctx := context.Background()

	for _, st := range []string{"IN_PROGRESS", "FINISHED", "ABANDONED"} {
		job, err := e.svc.Create(ctx, CreateInput{
			Type:        oapi.INSPECTIONRECORDS,
			Params:      map[string]any{"status": st},
			RequestedBy: e.adminID,
		})
		require.NoErrorf(t, err, "status %q must be accepted", st)
		assert.Equal(t, oapi.PENDING, job.Status)
		e.enq.tasks = nil
	}

	// Absent status is also fine (no filter).
	job, err := e.svc.Create(ctx, CreateInput{Type: oapi.INSPECTIONRECORDS, RequestedBy: e.adminID})
	require.NoError(t, err)
	assert.Equal(t, oapi.PENDING, job.Status)
}

// LOW fix: an invalid enum status is rejected at Create with ErrInvalidStatus,
// and crucially does NOT insert an orphaned export_jobs row.
func TestCreate_InspectionStatus_InvalidRejected(t *testing.T) {
	e := newExpEnv(t)
	ctx := context.Background()

	before := countJobs(t, ctx, e)

	_, err := e.svc.Create(ctx, CreateInput{
		Type:        oapi.INSPECTIONRECORDS,
		Params:      map[string]any{"status": "BOGUS"},
		RequestedBy: e.adminID,
	})
	require.ErrorIs(t, err, ErrInvalidStatus)
	assert.Empty(t, e.enq.tasks, "rejected export never enqueues")
	assert.Equal(t, before, countJobs(t, ctx, e), "no orphaned PENDING row inserted")
}

// A numeric status on PROBLEM_LIST (a dict id, not an enum) must not be
// enum-validated; it is accepted and decoded as an id by that builder.
func TestCreate_ProblemListNumericStatus_NotEnumValidated(t *testing.T) {
	e := newExpEnv(t)
	ctx := context.Background()

	job, err := e.svc.Create(ctx, CreateInput{
		Type:        oapi.PROBLEMLIST,
		Params:      map[string]any{"status": e.statusOK}, // numeric dict_item id
		RequestedBy: e.adminID,
	})
	require.NoError(t, err, "numeric PROBLEM_LIST status is not enum-checked")
	assert.Equal(t, oapi.PENDING, job.Status)
}

// End-to-end through the HTTP handler: an invalid INSPECTION_RECORDS status maps
// to 422 VALIDATION_FAILED (not 500, not a mid-build FAILED job).
func TestHandler_Create_InvalidInspectionStatusIs422(t *testing.T) {
	e := newExpEnv(t)
	gin.SetMode(gin.TestMode)

	h := NewHandler(e.svc, e.mockCOS)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(rbac.CtxUserID, e.adminID); c.Next() })
	h.RegisterRoutes(r)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/exports",
		strings.NewReader(`{"type":"INSPECTION_RECORDS","params":{"status":"BOGUS"}}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	assert.Equal(t, "VALIDATION_FAILED", envBody(t, w).Error.Code)
	assert.Empty(t, e.enq.tasks, "rejected request never enqueues")
}

// countJobs returns the current export_jobs row count (orphan-insert guard).
func countJobs(t *testing.T, ctx context.Context, e *expEnv) int {
	t.Helper()
	var n int
	require.NoError(t, e.pool.QueryRow(ctx, `SELECT count(*) FROM export_jobs`).Scan(&n))
	return n
}
