package obs

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func doGet(t *testing.T, h http.Handler, path string) (*http.Response, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()
	var body map[string]any
	if res.Body != nil {
		_ = json.NewDecoder(res.Body).Decode(&body)
	}
	return res, body
}

func TestHealthz_AlwaysOK(t *testing.T) {
	// Liveness must succeed even when a readiness dependency is down.
	mux := NewHealthMux(nil, ReadyCheck{Name: "db", Check: func(context.Context) error {
		return errors.New("db down")
	}})
	res, body := doGet(t, mux, "/healthz")
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "ok", body["status"])
}

func TestReadyz_AllPass(t *testing.T) {
	mux := NewHealthMux(nil,
		ReadyCheck{Name: "db", Check: func(context.Context) error { return nil }},
		ReadyCheck{Name: "redis", Check: func(context.Context) error { return nil }},
	)
	res, body := doGet(t, mux, "/readyz")
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "ready", body["status"])
}

func TestReadyz_NoChecksIsReady(t *testing.T) {
	res, body := doGet(t, NewHealthMux(nil), "/readyz")
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "ready", body["status"])
}

func TestReadyz_OneFails_Returns503AndNamesIt(t *testing.T) {
	mux := NewHealthMux(nil,
		ReadyCheck{Name: "db", Check: func(context.Context) error { return nil }},
		ReadyCheck{Name: "redis", Check: func(context.Context) error { return errors.New("dial timeout") }},
	)
	res, body := doGet(t, mux, "/readyz")
	require.Equal(t, http.StatusServiceUnavailable, res.StatusCode)
	assert.Equal(t, "unready", body["status"])
	failed, ok := body["failed"].(map[string]any)
	require.True(t, ok, "failed map present")
	assert.Equal(t, "dial timeout", failed["redis"])
	assert.NotContains(t, failed, "db")
}

func TestReadyzHandler_RespectsRequestContext(t *testing.T) {
	// A check that observes ctx cancellation reports the dependency as failed
	// rather than hanging the probe.
	mux := NewHealthMux(nil, ReadyCheck{Name: "slow", Check: func(ctx context.Context) error {
		return ctx.Err()
	}})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	ctx, cancel := context.WithCancel(req.Context())
	cancel() // already-cancelled
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Result().StatusCode)
}

func TestNewHealthServer_BindsAddrAndServesMux(t *testing.T) {
	srv := NewHealthServer(":9091", nil)
	assert.Equal(t, ":9091", srv.Addr)
	require.NotNil(t, srv.Handler)
	// Drive the configured handler directly (no real listener).
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	assert.Equal(t, http.StatusOK, rec.Result().StatusCode)
}

func TestMetricsEndpoint_Served(t *testing.T) {
	res, _ := doGet(t, NewHealthMux(nil), "/metrics")
	assert.Equal(t, http.StatusOK, res.StatusCode)
}
