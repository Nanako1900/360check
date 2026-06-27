package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnkglobal/c5-backend/internal/config"
	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
)

// fakePinger is a test double for a health dependency.
type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func newTestServer(t *testing.T, dbErr, redisErr error) http.Handler {
	t.Helper()
	cfg := &config.Config{}
	cfg.Server.Mode = "test"
	cfg.Observ.ServiceName = "c5-api-test"
	return New(Deps{
		Cfg:      cfg,
		Logger:   slog.New(slog.NewTextHandler(testWriter{t}, nil)),
		DB:       fakePinger{err: dbErr},
		Redis:    fakePinger{err: redisErr},
		Registry: prometheus.NewRegistry(),
	})
}

type testWriter struct{ t *testing.T }

func (testWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestHealthz_Healthy(t *testing.T) {
	srv := newTestServer(t, nil, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp oapi.HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, "ok", resp.Data.Status)
	require.NotNil(t, resp.Data.Db)
	require.NotNil(t, resp.Data.Redis)
	assert.True(t, *resp.Data.Db)
	assert.True(t, *resp.Data.Redis)
}

func TestHealthz_DBDown_503(t *testing.T) {
	srv := newTestServer(t, errors.New("db unreachable"), nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	var resp oapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.Success)
	assert.Equal(t, oapi.INTERNAL, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "db")
	assert.NotContains(t, resp.Error.Message, "redis")
}

func TestHealthz_RedisDown_503(t *testing.T) {
	srv := newTestServer(t, nil, errors.New("redis unreachable"))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	var resp oapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp.Error.Message, "redis")
}

func TestLivez_AlwaysOK_EvenWhenDependenciesDown(t *testing.T) {
	// Liveness is process-only: a dead DB+Redis must NOT fail it (else k8s would
	// restart-loop the pod on a transient dependency blip).
	srv := newTestServer(t, errors.New("db down"), errors.New("redis down"))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/livez", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Success bool              `json:"success"`
		Data    map[string]string `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, "alive", resp.Data["status"])
}

func TestReadyz_Ready(t *testing.T) {
	srv := newTestServer(t, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	var resp oapi.HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, "ready", resp.Data.Status)
}

func TestReadyz_DBDown_503(t *testing.T) {
	srv := newTestServer(t, errors.New("db unreachable"), nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	var resp oapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.Success)
	assert.Equal(t, oapi.INTERNAL, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "db")
}

func TestMetricsEndpoint(t *testing.T) {
	srv := newTestServer(t, nil, nil)
	// drive one request so the histogram has a sample
	srv.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil))

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "c5_http_requests_total")
}

func TestUnknownRoute_NotFoundEnvelope(t *testing.T) {
	srv := newTestServer(t, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/does-not-exist", nil))

	assert.Equal(t, http.StatusNotFound, w.Code)
	var resp oapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.Success)
	assert.Equal(t, oapi.NOTFOUND, resp.Error.Code)
}
