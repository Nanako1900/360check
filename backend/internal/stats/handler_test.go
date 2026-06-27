package stats

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
)

func init() { gin.SetMode(gin.TestMode) }

// getCtx builds a GET gin context for /stats/overview?<rawQuery>. The parse paths
// under test run BEFORE the handler touches the service, so a nil service is safe
// for every case that returns at parseFilter (bad timestamps).
func getCtx(rawQuery string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/stats/overview?"+rawQuery, nil)
	return c, w
}

// decodeEnvelope unmarshals the standard {success,data,error,meta} wrapper.
func decodeEnvelope(t *testing.T, body []byte) struct {
	Success bool              `json:"success"`
	Data    json.RawMessage   `json:"data"`
	Error   *oapi.ErrorObject `json:"error"`
	Meta    *oapi.Meta        `json:"meta"`
} {
	t.Helper()
	var env struct {
		Success bool              `json:"success"`
		Data    json.RawMessage   `json:"data"`
		Error   *oapi.ErrorObject `json:"error"`
		Meta    *oapi.Meta        `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(body, &env))
	return env
}

// TestParseFilter_BadFromTimestamp asserts a malformed `from` short-circuits to a
// 422 VALIDATION_FAILED envelope before the service is ever consulted (nil svc).
func TestParseFilter_BadFromTimestamp(t *testing.T) {
	h := NewHandler(nil)
	c, w := getCtx("from=not-a-time")
	h.Overview(c)

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	env := decodeEnvelope(t, w.Body.Bytes())
	assert.False(t, env.Success)
	require.NotNil(t, env.Error)
	assert.Equal(t, oapi.VALIDATIONFAILED, env.Error.Code)
	assert.JSONEq(t, "null", string(env.Data), "error envelope carries JSON-null data")
}

// TestParseFilter_BadToTimestamp covers the second timestamp branch (to). A valid
// `from` is supplied so parsing reaches the `to` check, proving each bound is
// validated independently.
func TestParseFilter_BadToTimestamp(t *testing.T) {
	h := NewHandler(nil)
	c, w := getCtx("from=2026-06-01T00:00:00Z&to=garbage")
	h.Overview(c)

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	env := decodeEnvelope(t, w.Body.Bytes())
	assert.False(t, env.Success)
	require.NotNil(t, env.Error)
	assert.Equal(t, oapi.VALIDATIONFAILED, env.Error.Code)
}

// TestParseFilter_Valid exercises every accepted query param and asserts the
// parsed Filter mirrors the input (including UTC normalization of the bounds).
func TestParseFilter_Valid(t *testing.T) {
	c, _ := getCtx("project_id=7&inspector_id=42&from=2026-06-01T00:00:00Z&to=2026-06-30T00:00:00Z")
	f, ok := parseFilter(c)
	require.True(t, ok)

	require.NotNil(t, f.ProjectID)
	assert.EqualValues(t, 7, *f.ProjectID)
	require.NotNil(t, f.InspectorID)
	assert.EqualValues(t, 42, *f.InspectorID)
	require.NotNil(t, f.From)
	assert.Equal(t, 2026, f.From.Year())
	assert.Equal(t, "UTC", f.From.Location().String(), "bounds are normalized to UTC")
	require.NotNil(t, f.To)
	assert.Equal(t, 30, f.To.Day())
}

// TestParseFilter_NonUTCOffsetNormalized confirms an RFC3339 value with a zone
// offset is converted to UTC (the service binds timestamptz; normalization keeps
// the two roll-ups comparable).
func TestParseFilter_NonUTCOffsetNormalized(t *testing.T) {
	// 2026-06-01T08:00:00+08:00 == 2026-06-01T00:00:00Z.
	c, _ := getCtx("from=2026-06-01T08:00:00%2B08:00")
	f, ok := parseFilter(c)
	require.True(t, ok)
	require.NotNil(t, f.From)
	assert.Equal(t, "UTC", f.From.Location().String())
	assert.Equal(t, 0, f.From.Hour(), "08:00+08:00 normalizes to 00:00Z")
	assert.Equal(t, 1, f.From.Day())
}

// TestParseFilter_NoFilters returns an all-nil Filter (whole-dataset default).
func TestParseFilter_NoFilters(t *testing.T) {
	c, _ := getCtx("")
	f, ok := parseFilter(c)
	require.True(t, ok)
	assert.Nil(t, f.ProjectID)
	assert.Nil(t, f.InspectorID)
	assert.Nil(t, f.From)
	assert.Nil(t, f.To)
}

// TestParseFilter_UnparsableIDsIgnored asserts a non-numeric id is silently
// dropped (queryInt64 returns false) rather than rejected — the filter falls back
// to the unfiltered dimension, matching the handler's lenient id contract.
func TestParseFilter_UnparsableIDsIgnored(t *testing.T) {
	c, _ := getCtx("project_id=abc&inspector_id=")
	f, ok := parseFilter(c)
	require.True(t, ok)
	assert.Nil(t, f.ProjectID, "non-numeric project_id is ignored, not an error")
	assert.Nil(t, f.InspectorID, "empty inspector_id is ignored")
}

// TestQueryInt64 table-drives the int64 query parser directly.
func TestQueryInt64(t *testing.T) {
	cases := []struct {
		raw      string
		wantN    int64
		wantOK   bool
		describe string
	}{
		{"id=15", 15, true, "valid positive"},
		{"id=-3", -3, true, "valid negative"},
		{"id=", 0, false, "empty -> absent"},
		{"id=9.5", 0, false, "float -> unparsable"},
		{"id=notanumber", 0, false, "text -> unparsable"},
	}
	for _, tc := range cases {
		c, _ := getCtx(tc.raw)
		n, ok := queryInt64(c, "id")
		assert.Equal(t, tc.wantOK, ok, tc.describe)
		assert.Equal(t, tc.wantN, n, tc.describe)
	}
}

// TestQueryTime table-drives the RFC3339 timestamp parser, including the (value,
// present, bad) tri-state contract.
func TestQueryTime(t *testing.T) {
	cases := []struct {
		raw         string
		wantPresent bool
		wantBad     bool
		describe    string
	}{
		{"t=2026-06-01T00:00:00Z", true, false, "valid RFC3339"},
		{"t=", false, false, "empty -> absent, not bad"},
		{"t=2026-06-01", false, true, "date-only is not RFC3339 -> bad"},
		{"t=garbage", false, true, "garbage -> bad"},
	}
	for _, tc := range cases {
		c, _ := getCtx(tc.raw)
		v, present, bad := queryTime(c, "t")
		assert.Equal(t, tc.wantPresent, present, tc.describe)
		assert.Equal(t, tc.wantBad, bad, tc.describe)
		if present {
			assert.Equal(t, "UTC", v.Location().String(), tc.describe)
		}
	}
}

// TestNewHandler_RegisterRoutes wires the handler onto a gin engine and asserts
// the route is mounted (a request reaches the handler rather than 404). The route
// is exercised with a malformed timestamp so it returns 422 at parseFilter without
// needing a live service.
func TestNewHandler_RegisterRoutes(t *testing.T) {
	h := NewHandler(nil)
	r := gin.New()
	h.RegisterRoutes(r)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stats/overview?from=bad", nil)
	r.ServeHTTP(w, req)

	// Mounted (not 404) and short-circuits to 422 at parse time.
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	env := decodeEnvelope(t, w.Body.Bytes())
	assert.Equal(t, oapi.VALIDATIONFAILED, env.Error.Code)
}
