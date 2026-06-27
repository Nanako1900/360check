package problem

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

func TestValidClientLogAction(t *testing.T) {
	cases := []struct {
		action string
		want   bool
	}{
		{"COMMENT", true},
		{"REASSIGN", true},
		{"STATUS_CHANGE", false}, // backend-only (D3)
		{"comment", false},       // case-sensitive
		{"", false},
		{"DELETE", false},
	}
	for _, tc := range cases {
		assert.Equalf(t, tc.want, validClientLogAction(tc.action), "action=%q", tc.action)
	}
}

func TestStatusChanged(t *testing.T) {
	i := func(v int64) *int64 { return &v }
	cases := []struct {
		name            string
		requested, curr *int64
		want            bool
	}{
		{"nil requested -> no change", nil, i(7), false},
		{"nil current, new value -> change", i(7), nil, true},
		{"same value -> no change", i(7), i(7), false},
		{"different value -> change", i(8), i(7), true},
		{"both nil -> no change", nil, nil, false},
	}
	for _, tc := range cases {
		assert.Equalf(t, tc.want, statusChanged(tc.requested, tc.curr), tc.name)
	}
}

func TestIsForeignKeyViolation(t *testing.T) {
	assert.True(t, isForeignKeyViolation(&pgconn.PgError{Code: "23503"}))
	assert.False(t, isForeignKeyViolation(&pgconn.PgError{Code: "23505"}))
	assert.False(t, isForeignKeyViolation(errors.New("plain error")))
	assert.False(t, isForeignKeyViolation(nil))
}

func TestClassifyFK(t *testing.T) {
	assert.ErrorIs(t, classifyFK(errors.New(`violates foreign key constraint "fk_inspector_id"`)), ErrInspectorMissing)
	assert.ErrorIs(t, classifyFK(errors.New(`violates fk on inspection_id`)), ErrNotFound)
	assert.ErrorIs(t, classifyFK(errors.New(`violates project_id restrict`)), ErrProjectMissing)
	// Unknown FK defaults to project-missing.
	assert.ErrorIs(t, classifyFK(errors.New(`some other constraint`)), ErrProjectMissing)
}

func errCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.NotNil(t, env.Error)
	return env.Error.Code
}

func bodyCtx(method, body string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	return c, w
}

// Handler validation branches that return before touching the service/pool, so a
// nil-dependency handler is safe to exercise.
func TestProblemHandler_Validation(t *testing.T) {
	h := NewHandler(nil, nil)

	// Create: malformed body, missing project_id, missing captured_at, bad geom.
	createBad := []string{
		`x`,
		`{"captured_at":"2026-01-01T00:00:00Z","geom":{"type":"Point","coordinates":[1,2]}}`,              // no project_id
		`{"project_id":1,"geom":{"type":"Point","coordinates":[1,2]}}`,                                    // no captured_at
		`{"project_id":1,"captured_at":"2026-01-01T00:00:00Z","geom":{"type":"Point","coordinates":[1]}}`, // bad coords
	}
	for _, b := range createBad {
		c, w := bodyCtx(http.MethodPost, b, nil)
		h.Create(c)
		assert.Equalf(t, http.StatusUnprocessableEntity, w.Code, "body=%s", b)
		assert.Equal(t, "VALIDATION_FAILED", errCode(t, w))
	}

	// CreateLog: STATUS_CHANGE is rejected before the service is reached (D3).
	c, w := bodyCtx(http.MethodPost, `{"action":"STATUS_CHANGE"}`, gin.Params{{Key: "id", Value: "5"}})
	h.CreateLog(c)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.Equal(t, "VALIDATION_FAILED", errCode(t, w))

	// CreateLog: unknown action rejected.
	c, w = bodyCtx(http.MethodPost, `{"action":"BOGUS"}`, gin.Params{{Key: "id", Value: "5"}})
	h.CreateLog(c)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

	// Invalid :id -> 422 on every id-bearing route.
	for _, fn := range []func(*gin.Context){h.Get, h.Update, h.Delete, h.ListLogs, h.CreateLog} {
		c, w := bodyCtx(http.MethodGet, `{}`, gin.Params{{Key: "id", Value: "abc"}})
		fn(c)
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	}
}

func TestProblemFailErr(t *testing.T) {
	cases := []struct {
		err  error
		code int
		body string
	}{
		{ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
		{ErrInvalidLogAction, http.StatusUnprocessableEntity, "VALIDATION_FAILED"},
		{ErrProjectMissing, http.StatusUnprocessableEntity, "VALIDATION_FAILED"},
		{ErrInspectorMissing, http.StatusUnprocessableEntity, "VALIDATION_FAILED"},
		{errors.New("boom"), http.StatusInternalServerError, "INTERNAL"},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		failErr(c, tc.err)
		assert.Equal(t, tc.code, w.Code)
		assert.Equal(t, tc.body, errCode(t, w))
	}
}
