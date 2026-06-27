//go:build integration

package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/nnkglobal/c5-backend/internal/config"
	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
	"github.com/nnkglobal/c5-backend/internal/platform/redis"
)

// TestHealthz_Integration boots real PostGIS + Redis containers and asserts the
// spec-conformant /api/v1/healthz returns a 200 envelope with db+redis healthy.
func TestHealthz_Integration(t *testing.T) {
	ctx := context.Background()

	pg, err := tcpostgres.Run(ctx, "postgis/postgis:16-3.4",
		tcpostgres.WithDatabase("c5"),
		tcpostgres.WithUsername("c5"),
		tcpostgres.WithPassword("c5"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	rc, err := tcredis.Run(ctx, "redis:7")
	require.NoError(t, err)
	t.Cleanup(func() { _ = rc.Terminate(ctx) })

	redisEndpoint, err := rc.Endpoint(ctx, "")
	require.NoError(t, err)

	pool, err := db.New(ctx, dsn, 5, 1, 30*time.Second)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, pool.Ping(ctx))

	rdb := redis.New(redisEndpoint, "", 0)
	t.Cleanup(func() { _ = rdb.Close() })
	require.NoError(t, rdb.Ping(ctx))

	cfg := &config.Config{}
	cfg.Server.Mode = "test"
	cfg.Observ.ServiceName = "c5-api-it"

	srv := New(Deps{
		Cfg:      cfg,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:       pool,
		Redis:    rdb,
		Registry: prometheus.NewRegistry(),
	})

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var resp oapi.HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, "ok", resp.Data.Status)
	require.NotNil(t, resp.Data.Db)
	require.NotNil(t, resp.Data.Redis)
	assert.True(t, *resp.Data.Db)
	assert.True(t, *resp.Data.Redis)
}
