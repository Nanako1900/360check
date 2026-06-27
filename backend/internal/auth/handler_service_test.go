package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gendb "github.com/nnkglobal/c5-backend/internal/gen/db"
	"github.com/nnkglobal/c5-backend/internal/rbac"
)

// reqCtx builds a gin context for a request with an optional JSON body and an
// optional authenticated user id (mirrors what the auth middleware would set).
func reqCtx(method, body string, userID int64) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var rdr *strings.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	} else {
		rdr = strings.NewReader("")
	}
	c.Request = httptest.NewRequest(method, "/", rdr)
	c.Request.Header.Set("Content-Type", "application/json")
	if userID != 0 {
		c.Set(rbac.CtxUserID, userID)
	}
	return c, w
}

func envCode(t *testing.T, w *httptest.ResponseRecorder) string {
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

func newTestHandler(t *testing.T, q gendb.Querier) (*Handler, *rbac.Enforcer) {
	t.Helper()
	enf, err := rbac.NewInMemoryEnforcer()
	require.NoError(t, err)
	store, _ := newTestStore(t, time.Hour)
	return NewHandler(NewService(q, fakeJWT{}, store, enf)), enf
}

// Login handler: bad credentials -> 401 UNAUTHENTICATED (not 500), to avoid
// distinguishing "wrong password" from "internal error" to the client.
func TestHandlerLogin_BadCredentials(t *testing.T) {
	q := &fakeQ{getByUsername: func(context.Context, string) (gendb.GetUserByUsernameRow, error) {
		return gendb.GetUserByUsernameRow{}, pgx.ErrNoRows
	}}
	h, _ := newTestHandler(t, q)
	c, w := reqCtx(http.MethodPost, `{"username":"u","password":"whatever"}`, 0)
	h.Login(c)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "UNAUTHENTICATED", envCode(t, w))
}

// Login handler: a genuine DB error -> 500 INTERNAL.
func TestHandlerLogin_InternalError(t *testing.T) {
	q := &fakeQ{getByUsername: func(context.Context, string) (gendb.GetUserByUsernameRow, error) {
		return gendb.GetUserByUsernameRow{}, errBoom
	}}
	h, _ := newTestHandler(t, q)
	c, w := reqCtx(http.MethodPost, `{"username":"u","password":"whatever"}`, 0)
	h.Login(c)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "INTERNAL", envCode(t, w))
}

func TestHandlerLogin_Success(t *testing.T) {
	q := &fakeQ{
		getByUsername: func(context.Context, string) (gendb.GetUserByUsernameRow, error) {
			return gendb.GetUserByUsernameRow{
				ID: 1, Username: "u", PasswordHash: mustHash(t, "correctpass"), IsActive: true,
			}, nil
		},
		touchLogin: func(context.Context, int64) error { return nil },
	}
	h, _ := newTestHandler(t, q)
	c, w := reqCtx(http.MethodPost, `{"username":"u","password":"correctpass"}`, 0)
	h.Login(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

// Refresh handler: unknown token -> 401 UNAUTHENTICATED.
func TestHandlerRefresh_NotFound(t *testing.T) {
	h, _ := newTestHandler(t, &fakeQ{})
	c, w := reqCtx(http.MethodPost, `{"refresh_token":"nope"}`, 0)
	h.Refresh(c)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "UNAUTHENTICATED", envCode(t, w))
}

// Refresh handler: a genuine DB error during user load -> 500 INTERNAL.
func TestHandlerRefresh_InternalError(t *testing.T) {
	q := &fakeQ{getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
		return gendb.GetUserByIDRow{}, errBoom
	}}
	// Seed a valid refresh token so Rotate succeeds and the user-load error fires.
	store, _ := newTestStore(t, time.Hour)
	enf, _ := rbac.NewInMemoryEnforcer()
	svc := NewService(q, fakeJWT{}, store, enf)
	h := NewHandler(svc)
	tok, err := store.Create(context.Background(), 1)
	require.NoError(t, err)
	c, w := reqCtx(http.MethodPost, `{"refresh_token":"`+tok+`"}`, 0)
	h.Refresh(c)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "INTERNAL", envCode(t, w))
}

// Logout handler: always 200 (idempotent revoke).
func TestHandlerLogout(t *testing.T) {
	h, _ := newTestHandler(t, &fakeQ{})
	c, w := reqCtx(http.MethodPost, `{"refresh_token":"anything"}`, 0)
	h.Logout(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

// Me handler: user not found -> 401 UNAUTHENTICATED.
func TestHandlerMe_NotFound(t *testing.T) {
	q := &fakeQ{getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
		return gendb.GetUserByIDRow{}, pgx.ErrNoRows
	}}
	h, _ := newTestHandler(t, q)
	c, w := reqCtx(http.MethodGet, "", 5)
	h.Me(c)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "UNAUTHENTICATED", envCode(t, w))
}

// Me handler: a genuine DB error -> 500 INTERNAL.
func TestHandlerMe_InternalError(t *testing.T) {
	q := &fakeQ{getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
		return gendb.GetUserByIDRow{}, errBoom
	}}
	h, _ := newTestHandler(t, q)
	c, w := reqCtx(http.MethodGet, "", 5)
	h.Me(c)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "INTERNAL", envCode(t, w))
}

// Me handler success exercises the role-loop (enforcer grants a role; the matching
// catalog permission expands) and the GetRoleByCode lookup.
func TestHandlerMe_SuccessWithRole(t *testing.T) {
	q := &fakeQ{
		getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
			return gendb.GetUserByIDRow{ID: 1, Username: "alice", IsActive: true}, nil
		},
		getRoleByCode: func(_ context.Context, code string) (gendb.GetRoleByCodeRow, error) {
			return gendb.GetRoleByCodeRow{ID: 10, Code: code, Name: "Admin"}, nil
		},
		listPerms: func(context.Context) ([]gendb.ListPermissionsRow, error) {
			return []gendb.ListPermissionsRow{
				{ID: 1, Code: "user:read", Object: "/api/v1/users", Action: "GET"},
				{ID: 2, Code: "secret:write", Object: "/api/v1/secret", Action: "POST"},
			}, nil
		},
	}
	enf, err := rbac.NewInMemoryEnforcer()
	require.NoError(t, err)
	// Grant alice -> admin, admin -> (/api/v1/users, GET) so exactly one permission expands.
	require.NoError(t, enf.SetPermissionsForRole("admin", [][2]string{{"/api/v1/users", "GET"}}))
	require.NoError(t, enf.SetRolesForUser("alice", []string{"admin"}))
	store, _ := newTestStore(t, time.Hour)
	h := NewHandler(NewService(q, fakeJWT{}, store, enf))

	c, w := reqCtx(http.MethodGet, "", 1)
	h.Me(c)
	require.Equal(t, http.StatusOK, w.Code)

	var env struct {
		Data struct {
			Roles       []map[string]any `json:"roles"`
			Permissions []string         `json:"permissions"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Len(t, env.Data.Roles, 1)
	assert.Equal(t, "admin", env.Data.Roles[0]["code"])
	assert.Equal(t, []string{"user:read"}, env.Data.Permissions, "only the granted permission expands")
}

// Me role-loop: GetRoleByCode pgx.ErrNoRows for a granted role is skipped (the
// role row was deleted but the casbin grant lingers) without failing the request.
func TestHandlerMe_RoleRowMissingIsSkipped(t *testing.T) {
	q := &fakeQ{
		getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
			return gendb.GetUserByIDRow{ID: 1, Username: "alice", IsActive: true}, nil
		},
		getRoleByCode: func(context.Context, string) (gendb.GetRoleByCodeRow, error) {
			return gendb.GetRoleByCodeRow{}, pgx.ErrNoRows
		},
		listPerms: func(context.Context) ([]gendb.ListPermissionsRow, error) { return nil, nil },
	}
	enf, err := rbac.NewInMemoryEnforcer()
	require.NoError(t, err)
	require.NoError(t, enf.SetRolesForUser("alice", []string{"ghost"}))
	store, _ := newTestStore(t, time.Hour)
	svc := NewService(q, fakeJWT{}, store, enf)

	me, err := svc.Me(context.Background(), 1)
	require.NoError(t, err)
	assert.Empty(t, me.Roles, "a granted role with no row is skipped, not fatal")
}

// Me role-loop: a genuine GetRoleByCode DB error is wrapped and returned.
func TestMe_GetRoleByCodeDBError_Wrapped(t *testing.T) {
	q := &fakeQ{
		getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
			return gendb.GetUserByIDRow{ID: 1, Username: "alice", IsActive: true}, nil
		},
		getRoleByCode: func(context.Context, string) (gendb.GetRoleByCodeRow, error) {
			return gendb.GetRoleByCodeRow{}, errBoom
		},
	}
	enf, err := rbac.NewInMemoryEnforcer()
	require.NoError(t, err)
	require.NoError(t, enf.SetRolesForUser("alice", []string{"admin"}))
	store, _ := newTestStore(t, time.Hour)
	svc := NewService(q, fakeJWT{}, store, enf)

	_, err = svc.Me(context.Background(), 1)
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "auth: load role")
}

// ChangePassword handler: wrong old password -> 422 VALIDATION_FAILED.
func TestHandlerChangePassword_WrongOld(t *testing.T) {
	q := &fakeQ{getPwHash: func(context.Context, int64) (string, error) {
		return mustHash(t, "correctold"), nil
	}}
	h, _ := newTestHandler(t, q)
	c, w := reqCtx(http.MethodPut, `{"old_password":"wrong","new_password":"newpassword123"}`, 1)
	h.ChangePassword(c)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.Equal(t, "VALIDATION_FAILED", envCode(t, w))
}

// ChangePassword handler: user not found -> 401 UNAUTHENTICATED.
func TestHandlerChangePassword_UserGone(t *testing.T) {
	q := &fakeQ{getPwHash: func(context.Context, int64) (string, error) {
		return "", pgx.ErrNoRows
	}}
	h, _ := newTestHandler(t, q)
	c, w := reqCtx(http.MethodPut, `{"old_password":"x","new_password":"newpassword123"}`, 1)
	h.ChangePassword(c)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "UNAUTHENTICATED", envCode(t, w))
}

// ChangePassword handler: a genuine DB error -> 500 INTERNAL.
func TestHandlerChangePassword_InternalError(t *testing.T) {
	q := &fakeQ{getPwHash: func(context.Context, int64) (string, error) {
		return "", errBoom
	}}
	h, _ := newTestHandler(t, q)
	c, w := reqCtx(http.MethodPut, `{"old_password":"x","new_password":"newpassword123"}`, 1)
	h.ChangePassword(c)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "INTERNAL", envCode(t, w))
}

// ChangePassword handler success path.
func TestHandlerChangePassword_Success(t *testing.T) {
	q := &fakeQ{
		getPwHash: func(context.Context, int64) (string, error) { return mustHash(t, "correctold"), nil },
		updatePw:  func(context.Context, gendb.UpdateUserPasswordParams) (int64, error) { return 1, nil },
	}
	h, _ := newTestHandler(t, q)
	c, w := reqCtx(http.MethodPut, `{"old_password":"correctold","new_password":"newpassword123"}`, 1)
	h.ChangePassword(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

// RegisterPublic / RegisterProtected mount the routes (smoke coverage).
func TestRegisterRoutes(t *testing.T) {
	h, _ := newTestHandler(t, &fakeQ{})
	r := gin.New()
	h.RegisterPublic(r)
	h.RegisterProtected(r)
	routes := r.Routes()
	paths := make(map[string]bool, len(routes))
	for _, rt := range routes {
		paths[rt.Method+" "+rt.Path] = true
	}
	assert.True(t, paths["POST /auth/login"])
	assert.True(t, paths["POST /auth/refresh"])
	assert.True(t, paths["POST /auth/logout"])
	assert.True(t, paths["GET /auth/me"])
	assert.True(t, paths["PUT /auth/password"])
}
