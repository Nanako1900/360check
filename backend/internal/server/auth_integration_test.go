//go:build integration

package server

import (
	"bytes"
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
	"golang.org/x/crypto/bcrypt"

	migrations "github.com/nnkglobal/c5-backend/db/migrations"
	"github.com/nnkglobal/c5-backend/internal/auth"
	"github.com/nnkglobal/c5-backend/internal/config"
	"github.com/nnkglobal/c5-backend/internal/dict"
	gendb "github.com/nnkglobal/c5-backend/internal/gen/db"
	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/inspection"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
	"github.com/nnkglobal/c5-backend/internal/platform/jwt"
	"github.com/nnkglobal/c5-backend/internal/platform/redis"
	"github.com/nnkglobal/c5-backend/internal/problem"
	"github.com/nnkglobal/c5-backend/internal/project"
	"github.com/nnkglobal/c5-backend/internal/rbac"
	"github.com/nnkglobal/c5-backend/internal/user"
)

const bootstrapPassword = "ChangeMe@123" // test-only; written by setAdminPassword (seed ships admin LOCKED)

func newAuthTestServer(t *testing.T) http.Handler {
	t.Helper()
	ctx := context.Background()

	pg, err := tcpostgres.Run(ctx, "postgis/postgis:16-3.4",
		tcpostgres.WithDatabase("c5"), tcpostgres.WithUsername("c5"), tcpostgres.WithPassword("c5"),
		tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, db.RunMigrations(dsn, migrations.FS))

	rc, err := tcredis.Run(ctx, "redis:7")
	require.NoError(t, err)
	t.Cleanup(func() { _ = rc.Terminate(ctx) })
	redisAddr, err := rc.Endpoint(ctx, "")
	require.NoError(t, err)

	pool, err := db.New(ctx, dsn, 5, 1, 30*time.Second)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	rdb := redis.New(redisAddr, "", 0)
	t.Cleanup(func() { _ = rdb.Close() })

	enforcer, err := rbac.NewEnforcer(pool, redisAddr, "", 0)
	require.NoError(t, err)
	queries := gendb.New(pool.Pool)
	// The seed ships the bootstrap admin LOCKED (no default credential, BE-CRIT1);
	// set a known password here so these tests exercise the real admin login flow.
	setAdminPassword(t, ctx, queries, bootstrapPassword)
	jwtMgr := jwt.NewManager("test-secret", "c5-api", 15*time.Minute)
	refresh := auth.NewRefreshStore(rdb.Client, time.Hour)
	authH := auth.NewHandler(auth.NewService(queries, jwtMgr, refresh, enforcer))
	userH := user.NewHandler(user.NewService(queries))
	rbacH := rbac.NewHandler(rbac.NewService(queries, enforcer))
	projectH := project.NewHandler(project.NewService(pool))
	inspectionH := inspection.NewHandler(inspection.NewService(pool))
	dictH := dict.NewHandler(dict.NewService(pool))
	problemH := problem.NewHandler(problem.NewService(pool), pool)

	cfg := &config.Config{}
	cfg.Server.Mode = "test"
	cfg.Observ.ServiceName = "c5-api-it"
	return New(Deps{
		Cfg: cfg, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB: pool, Redis: rdb, Registry: prometheus.NewRegistry(),
		JWT: jwtMgr, Enforcer: enforcer,
		PublicRoutes: []RouteRegistrar{authH.RegisterPublic},
		ProtectedRoutes: []RouteRegistrar{
			authH.RegisterProtected, userH.RegisterRoutes, rbacH.RegisterRoutes,
			projectH.RegisterRoutes, inspectionH.RegisterRoutes,
			dictH.RegisterRoutes, problemH.RegisterRoutes,
		},
	})
}

// setAdminPassword gives the seeded (locked) admin a known password so the auth
// integration tests can log in. bcrypt.DefaultCost keeps the test fast; production
// uses cost 12.
func setAdminPassword(t *testing.T, ctx context.Context, q *gendb.Queries, pw string) {
	t.Helper()
	row, err := q.GetUserByUsername(ctx, "admin")
	require.NoError(t, err)
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	require.NoError(t, err)
	n, err := q.UpdateUserPassword(ctx, gendb.UpdateUserPasswordParams{
		ID: row.ID, PasswordHash: string(hash), UpdatedBy: &row.ID,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), n)
}

func doJSON(t *testing.T, srv http.Handler, method, path, bearer string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	var env map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	return w, env
}

func login(t *testing.T, srv http.Handler, user, pass string) (access, refresh string, code int) {
	w, env := doJSON(t, srv, http.MethodPost, "/api/v1/auth/login", "", oapi.LoginRequest{Username: user, Password: pass})
	if w.Code != http.StatusOK {
		return "", "", w.Code
	}
	data := env["data"].(map[string]any)
	return data["access_token"].(string), data["refresh_token"].(string), w.Code
}

func TestAuthFlow_Integration(t *testing.T) {
	srv := newAuthTestServer(t)

	// 1. Login as bootstrap admin.
	access, refresh, code := login(t, srv, "admin", bootstrapPassword)
	require.Equal(t, http.StatusOK, code)
	require.NotEmpty(t, access)
	require.NotEmpty(t, refresh)

	// 2. /auth/me with token -> admin + permission codes (admin wildcard expands to all).
	w, env := doJSON(t, srv, http.MethodGet, "/api/v1/auth/me", access, nil)
	require.Equal(t, http.StatusOK, w.Code)
	data := env["data"].(map[string]any)
	assert.Equal(t, "admin", data["user"].(map[string]any)["username"])
	perms := data["permissions"].([]any)
	assert.NotEmpty(t, perms, "admin should have effective permission codes")
	assert.Contains(t, toStrings(perms), "user:read")

	// 3. /auth/me without token -> 401 UNAUTHENTICATED.
	w, env = doJSON(t, srv, http.MethodGet, "/api/v1/auth/me", "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "UNAUTHENTICATED", env["error"].(map[string]any)["code"])

	// 4. Wrong password -> 401.
	_, _, code = login(t, srv, "admin", "wrong")
	assert.Equal(t, http.StatusUnauthorized, code)

	// 5. Refresh rotation: new tokens; old refresh is single-use.
	w, env = doJSON(t, srv, http.MethodPost, "/api/v1/auth/refresh", "", oapi.RefreshRequest{RefreshToken: refresh})
	require.Equal(t, http.StatusOK, w.Code)
	newRefresh := env["data"].(map[string]any)["refresh_token"].(string)
	assert.NotEqual(t, refresh, newRefresh)
	w, _ = doJSON(t, srv, http.MethodPost, "/api/v1/auth/refresh", "", oapi.RefreshRequest{RefreshToken: refresh})
	assert.Equal(t, http.StatusUnauthorized, w.Code, "rotated refresh token must be rejected")

	// 6. Logout revokes the (new) refresh token.
	w, _ = doJSON(t, srv, http.MethodPost, "/api/v1/auth/logout", access, oapi.RefreshRequest{RefreshToken: newRefresh})
	require.Equal(t, http.StatusOK, w.Code)
	w, _ = doJSON(t, srv, http.MethodPost, "/api/v1/auth/refresh", "", oapi.RefreshRequest{RefreshToken: newRefresh})
	assert.Equal(t, http.StatusUnauthorized, w.Code, "revoked refresh token must be rejected")
}

func TestAuthChangePassword_Integration(t *testing.T) {
	srv := newAuthTestServer(t)
	access, _, code := login(t, srv, "admin", bootstrapPassword)
	require.Equal(t, http.StatusOK, code)

	const newPass = "NewSecret@123"
	// Wrong old password -> 422.
	w, env := doJSON(t, srv, http.MethodPut, "/api/v1/auth/password", access,
		oapi.ChangePasswordRequest{OldPassword: "wrong", NewPassword: newPass})
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.Equal(t, "VALIDATION_FAILED", env["error"].(map[string]any)["code"])

	// Correct change.
	w, _ = doJSON(t, srv, http.MethodPut, "/api/v1/auth/password", access,
		oapi.ChangePasswordRequest{OldPassword: bootstrapPassword, NewPassword: newPass})
	require.Equal(t, http.StatusOK, w.Code)

	// Old password no longer works; new one does.
	_, _, code = login(t, srv, "admin", bootstrapPassword)
	assert.Equal(t, http.StatusUnauthorized, code)
	_, _, code = login(t, srv, "admin", newPass)
	assert.Equal(t, http.StatusOK, code)
}

func toStrings(in []any) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
