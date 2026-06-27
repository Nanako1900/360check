//go:build integration

package export

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func init() { gin.SetMode(gin.TestMode) }

// newRouter mounts the export routes on a fresh engine, injecting the given user
// id into the gin context (the way Authn would) so RequestedBy is populated.
func (e *expEnv) newRouter(userID int64) *gin.Engine {
	h := NewHandler(e.svc, e.mockCOS)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if userID != 0 {
			c.Set(rbac.CtxUserID, userID)
		}
		c.Next()
	})
	h.RegisterRoutes(r)
	return r
}

// envBody decodes the standard {success,data,error} envelope from a recorder.
func envBody(t *testing.T, w *httptest.ResponseRecorder) struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
} {
	t.Helper()
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	return env
}

func TestHandler_Create_Get_RoundTrip(t *testing.T) {
	e := newExpEnv(t)
	ctx := context.Background()
	r := e.newRouter(e.adminID)

	// POST /exports creates a PENDING job (201) and enqueues exactly one task.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/exports",
		strings.NewReader(`{"type":"PROBLEM_LIST","params":{"project_id":`+strconv.FormatInt(e.projectID, 10)+`}}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	env := envBody(t, w)
	require.True(t, env.Success)

	var created struct {
		JobUUID     string  `json:"job_uuid"`
		Status      string  `json:"status"`
		RequestedBy *int64  `json:"requested_by"`
		ResultUrl   *string `json:"result_url"`
	}
	require.NoError(t, json.Unmarshal(env.Data, &created))
	assert.Equal(t, "PENDING", created.Status)
	require.NotEmpty(t, created.JobUUID)
	require.NotNil(t, created.RequestedBy, "RequestedBy from the gin context")
	assert.Equal(t, e.adminID, *created.RequestedBy)
	assert.Nil(t, created.ResultUrl, "no signed url for a PENDING job")

	// Drive the worker on the enqueued task so the job becomes SUCCEEDED.
	require.Len(t, e.enq.tasks, 1)
	require.NoError(t, e.worker.handleRun(ctx, e.enq.tasks[0]))

	// GET /exports/{uuid} now returns SUCCEEDED + a signed result_url.
	wg := httptest.NewRecorder()
	r.ServeHTTP(wg, httptest.NewRequest(http.MethodGet, "/exports/"+created.JobUUID, nil))
	require.Equal(t, http.StatusOK, wg.Code, wg.Body.String())

	var got struct {
		Status    string  `json:"status"`
		ResultUrl *string `json:"result_url"`
	}
	require.NoError(t, json.Unmarshal(envBody(t, wg).Data, &got))
	assert.Equal(t, "SUCCEEDED", got.Status)
	require.NotNil(t, got.ResultUrl)
	assert.Contains(t, *got.ResultUrl, "mock-cdn.example.com")
}

func TestHandler_Create_BadTypeIs422(t *testing.T) {
	e := newExpEnv(t)
	r := e.newRouter(e.adminID)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/exports",
		strings.NewReader(`{"type":"NOPE"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	env := envBody(t, w)
	require.NotNil(t, env.Error)
	assert.Equal(t, "VALIDATION_FAILED", env.Error.Code)
	assert.Empty(t, e.enq.tasks, "rejected request never enqueues")
}

func TestHandler_Create_BadBodyIs422(t *testing.T) {
	e := newExpEnv(t)
	r := e.newRouter(e.adminID)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/exports", strings.NewReader(`{not json`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.Equal(t, "VALIDATION_FAILED", envBody(t, w).Error.Code)
}

func TestHandler_Create_ServiceErrorIs500(t *testing.T) {
	e := newExpEnv(t)

	// A handler whose service has a failing enqueuer: Create passes type
	// validation, inserts the PENDING row, then the enqueue fails -> the service
	// rolls back and returns a non-ErrInvalidType error, which the handler maps to
	// a 500 INTERNAL envelope.
	failSvc := NewService(e.pool, failEnq{})
	h := NewHandler(failSvc, e.mockCOS)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(rbac.CtxUserID, e.adminID); c.Next() })
	h.RegisterRoutes(r)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/exports", strings.NewReader(`{"type":"PROBLEM_LIST"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	assert.Equal(t, "INTERNAL", envBody(t, w).Error.Code)
}

func TestHandler_Get_NotFound(t *testing.T) {
	e := newExpEnv(t)
	r := e.newRouter(e.adminID)

	// A syntactically valid but unknown uuid -> 404 envelope.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet,
		"/exports/22222222-2222-2222-2222-222222222222", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "NOT_FOUND", envBody(t, w).Error.Code)
}

func TestHandler_Events_NotFound(t *testing.T) {
	e := newExpEnv(t)
	r := e.newRouter(e.adminID)

	// Events confirms existence before streaming; unknown uuid -> normal 404
	// envelope (not an SSE error frame).
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet,
		"/exports/22222222-2222-2222-2222-222222222222/events", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "NOT_FOUND", envBody(t, w).Error.Code)
}

func TestHandler_Events_StreamsTerminalJob(t *testing.T) {
	e := newExpEnv(t)
	ctx := context.Background()
	r := e.newRouter(e.adminID)

	// A finished job: the SSE loop emits the initial progress + a terminal done
	// immediately and returns (no real sleep, the job is already SUCCEEDED).
	job := e.runJob(t, ctx, oapi.INSPECTIONRECORDS, map[string]any{"project_id": e.projectID})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/exports/"+job.JobUUID+"/events", nil))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Equal(t, "no", w.Header().Get("X-Accel-Buffering"))

	evts := parseEvents(w.Body.String())
	require.NotEmpty(t, evts)
	assert.Equal(t, "progress", evts[0].Event)
	last := evts[len(evts)-1]
	assert.Equal(t, "done", last.Event)
	assert.Contains(t, last.Data, `"status":"SUCCEEDED"`)
	assert.Contains(t, last.Data, "mock-cdn.example.com")
}

func TestHandler_Events_ContextCancelStops(t *testing.T) {
	e := newExpEnv(t)
	ctx := context.Background()
	r := e.newRouter(e.adminID)

	// Create a PENDING (non-terminal) job by hand so the SSE loop would otherwise
	// poll forever; a cancelled request context must stop it promptly.
	job, err := e.svc.Create(ctx, CreateInput{Type: oapi.PROBLEMLIST, RequestedBy: e.adminID})
	require.NoError(t, err)
	e.enq.tasks = nil

	reqCtx, cancel := context.WithCancel(context.Background())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/exports/"+job.JobUUID+"/events", nil).WithContext(reqCtx)

	done := make(chan struct{})
	go func() {
		r.ServeHTTP(w, req)
		close(done)
	}()
	// Let the handler emit the initial progress event before cancelling (the loop
	// polls at pollInterval ~1s, so it is still blocked on the ticker, not the DB).
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Events handler did not return after context cancel")
	}

	// The initial progress (PENDING) was emitted; no terminal done.
	evts := parseEvents(w.Body.String())
	require.NotEmpty(t, evts)
	assert.Equal(t, "progress", evts[0].Event)
	for _, ev := range evts {
		assert.NotEqual(t, "done", ev.Event)
	}
}

// failEnq is an enqueuer whose Enqueue always fails, so Service.Create reaches
// its enqueue-error rollback path and returns a non-ErrInvalidType error.
type failEnq struct{}

func (failEnq) Enqueue(_ *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	return nil, errors.New("simulated enqueue failure")
}
