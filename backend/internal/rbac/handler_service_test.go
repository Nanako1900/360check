package rbac

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gendb "github.com/nnkglobal/c5-backend/internal/gen/db"
)

func hCtx(method, body string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
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
	c.Params = params
	return c, w
}

func hnd(t *testing.T, q gendb.Querier) (*Handler, *Enforcer) {
	t.Helper()
	enf, err := NewInMemoryEnforcer()
	require.NoError(t, err)
	return NewHandler(NewService(q, enf)), enf
}

var id1 = gin.Params{{Key: "id", Value: "1"}}

// --- ListRoles handler -----------------------------------------------------

func TestHandlerListRoles_InternalError(t *testing.T) {
	q := &fakeQ{listRoles: func(context.Context) ([]gendb.ListRolesRow, error) { return nil, errBoom }}
	h, _ := hnd(t, q)
	c, w := hCtx(http.MethodGet, "", nil)
	h.ListRoles(c)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "INTERNAL", errCode(t, w))
}

func TestHandlerListRoles_Success(t *testing.T) {
	q := &fakeQ{listRoles: func(context.Context) ([]gendb.ListRolesRow, error) {
		return []gendb.ListRolesRow{{ID: 1, Code: "admin", Name: "Admin"}}, nil
	}}
	h, _ := hnd(t, q)
	c, w := hCtx(http.MethodGet, "", nil)
	h.ListRoles(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

// --- CreateRole handler ----------------------------------------------------

func TestHandlerCreateRole_Conflict(t *testing.T) {
	q := &fakeQ{roleCodeEx: func(context.Context, string) (bool, error) { return true, nil }}
	h, _ := hnd(t, q)
	c, w := hCtx(http.MethodPost, `{"code":"ops","name":"Ops"}`, nil)
	h.CreateRole(c)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, "CONFLICT", errCode(t, w))
}

func TestHandlerCreateRole_InternalError(t *testing.T) {
	q := &fakeQ{roleCodeEx: func(context.Context, string) (bool, error) { return false, errBoom }}
	h, _ := hnd(t, q)
	c, w := hCtx(http.MethodPost, `{"code":"ops","name":"Ops"}`, nil)
	h.CreateRole(c)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandlerCreateRole_Success(t *testing.T) {
	q := &fakeQ{
		roleCodeEx: func(context.Context, string) (bool, error) { return false, nil },
		createRole: func(_ context.Context, arg gendb.CreateRoleParams) (gendb.CreateRoleRow, error) {
			return gendb.CreateRoleRow{ID: 3, Code: arg.Code, Name: arg.Name}, nil
		},
	}
	h, _ := hnd(t, q)
	c, w := hCtx(http.MethodPost, `{"code":"ops","name":"Ops"}`, nil)
	h.CreateRole(c)
	assert.Equal(t, http.StatusCreated, w.Code)
}

// --- UpdateRole handler ----------------------------------------------------

func TestHandlerUpdateRole_NotFound(t *testing.T) {
	q := &fakeQ{updateRole: func(context.Context, gendb.UpdateRoleParams) (gendb.UpdateRoleRow, error) {
		return gendb.UpdateRoleRow{}, pgx.ErrNoRows
	}}
	h, _ := hnd(t, q)
	c, w := hCtx(http.MethodPut, `{"name":"X"}`, id1)
	h.UpdateRole(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "NOT_FOUND", errCode(t, w))
}

func TestHandlerUpdateRole_Success(t *testing.T) {
	q := &fakeQ{updateRole: func(context.Context, gendb.UpdateRoleParams) (gendb.UpdateRoleRow, error) {
		return gendb.UpdateRoleRow{ID: 1, Code: "ops", Name: "X"}, nil
	}}
	h, _ := hnd(t, q)
	c, w := hCtx(http.MethodPut, `{"name":"X"}`, id1)
	h.UpdateRole(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

// --- DeleteRole handler ----------------------------------------------------

func TestHandlerDeleteRole_SystemRole_Conflict(t *testing.T) {
	q := &fakeQ{getRole: func(context.Context, int64) (gendb.GetRoleRow, error) {
		return gendb.GetRoleRow{ID: 1, Code: "admin", IsSystem: true}, nil
	}}
	h, _ := hnd(t, q)
	c, w := hCtx(http.MethodDelete, "", id1)
	h.DeleteRole(c)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, "CONFLICT", errCode(t, w))
}

func TestHandlerDeleteRole_NotFound(t *testing.T) {
	q := &fakeQ{getRole: func(context.Context, int64) (gendb.GetRoleRow, error) {
		return gendb.GetRoleRow{}, pgx.ErrNoRows
	}}
	h, _ := hnd(t, q)
	c, w := hCtx(http.MethodDelete, "", id1)
	h.DeleteRole(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandlerDeleteRole_Success(t *testing.T) {
	q := &fakeQ{
		getRole: func(context.Context, int64) (gendb.GetRoleRow, error) {
			return gendb.GetRoleRow{ID: 2, Code: "ops", IsSystem: false}, nil
		},
		softDeleteRl: func(context.Context, int64) (int64, error) { return 1, nil },
	}
	h, _ := hnd(t, q)
	c, w := hCtx(http.MethodDelete, "", id1)
	h.DeleteRole(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandlerDeleteRole_InvalidID(t *testing.T) {
	h, _ := hnd(t, &fakeQ{})
	c, w := hCtx(http.MethodDelete, "", gin.Params{{Key: "id", Value: "abc"}})
	h.DeleteRole(c)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

// --- GetRolePermissions handler --------------------------------------------

func TestHandlerGetRolePermissions_NotFound(t *testing.T) {
	q := &fakeQ{getRole: func(context.Context, int64) (gendb.GetRoleRow, error) {
		return gendb.GetRoleRow{}, pgx.ErrNoRows
	}}
	h, _ := hnd(t, q)
	c, w := hCtx(http.MethodGet, "", id1)
	h.GetRolePermissions(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandlerGetRolePermissions_Success(t *testing.T) {
	q := &fakeQ{
		getRole: func(context.Context, int64) (gendb.GetRoleRow, error) {
			return gendb.GetRoleRow{ID: 1, Code: "ops"}, nil
		},
		listPerms: func(context.Context) ([]gendb.ListPermissionsRow, error) { return nil, nil },
	}
	h, _ := hnd(t, q)
	c, w := hCtx(http.MethodGet, "", id1)
	h.GetRolePermissions(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

// --- SetRolePermissions handler --------------------------------------------

func TestHandlerSetRolePermissions_NotFound(t *testing.T) {
	q := &fakeQ{getRole: func(context.Context, int64) (gendb.GetRoleRow, error) {
		return gendb.GetRoleRow{}, pgx.ErrNoRows
	}}
	h, _ := hnd(t, q)
	c, w := hCtx(http.MethodPut, `{"permission_ids":[1]}`, id1)
	h.SetRolePermissions(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandlerSetRolePermissions_Success(t *testing.T) {
	q := &fakeQ{
		getRole: func(context.Context, int64) (gendb.GetRoleRow, error) {
			return gendb.GetRoleRow{ID: 1, Code: "ops"}, nil
		},
		permsByIDs: func(context.Context, []int64) ([]gendb.PermissionsByIDsRow, error) {
			return []gendb.PermissionsByIDsRow{{ID: 1, Code: "p:read", Object: "/x", Action: "GET"}}, nil
		},
		listPerms: func(context.Context) ([]gendb.ListPermissionsRow, error) {
			return []gendb.ListPermissionsRow{{ID: 1, Code: "p:read", Object: "/x", Action: "GET"}}, nil
		},
	}
	h, _ := hnd(t, q)
	c, w := hCtx(http.MethodPut, `{"permission_ids":[1]}`, id1)
	h.SetRolePermissions(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

// --- ListPermissions handler -----------------------------------------------

func TestHandlerListPermissions_InternalError(t *testing.T) {
	q := &fakeQ{listPerms: func(context.Context) ([]gendb.ListPermissionsRow, error) { return nil, errBoom }}
	h, _ := hnd(t, q)
	c, w := hCtx(http.MethodGet, "", nil)
	h.ListPermissions(c)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandlerListPermissions_Success(t *testing.T) {
	q := &fakeQ{listPerms: func(context.Context) ([]gendb.ListPermissionsRow, error) {
		return []gendb.ListPermissionsRow{{ID: 1, Code: "p:read", Object: "/x", Action: "GET"}}, nil
	}}
	h, _ := hnd(t, q)
	c, w := hCtx(http.MethodGet, "", nil)
	h.ListPermissions(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

// --- GetUserRoles handler --------------------------------------------------

func TestHandlerGetUserRoles_UserNotFound(t *testing.T) {
	q := &fakeQ{getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
		return gendb.GetUserByIDRow{}, pgx.ErrNoRows
	}}
	h, _ := hnd(t, q)
	c, w := hCtx(http.MethodGet, "", id1)
	h.GetUserRoles(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandlerGetUserRoles_Success(t *testing.T) {
	q := &fakeQ{getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
		return gendb.GetUserByIDRow{ID: 1, Username: "alice"}, nil
	}}
	h, _ := hnd(t, q)
	c, w := hCtx(http.MethodGet, "", id1)
	h.GetUserRoles(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

// --- SetUserRoles handler --------------------------------------------------

func TestHandlerSetUserRoles_UserNotFound(t *testing.T) {
	q := &fakeQ{getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
		return gendb.GetUserByIDRow{}, pgx.ErrNoRows
	}}
	h, _ := hnd(t, q)
	c, w := hCtx(http.MethodPut, `{"role_ids":[1]}`, id1)
	h.SetUserRoles(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandlerSetUserRoles_Success(t *testing.T) {
	q := &fakeQ{
		getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
			return gendb.GetUserByIDRow{ID: 1, Username: "alice"}, nil
		},
		rolesByIDs: func(context.Context, []int64) ([]gendb.RolesByIDsRow, error) {
			return []gendb.RolesByIDsRow{{ID: 7, Code: "ops", Name: "Ops"}}, nil
		},
		getRoleByCode: func(_ context.Context, code string) (gendb.GetRoleByCodeRow, error) {
			return gendb.GetRoleByCodeRow{ID: 7, Code: code, Name: "Ops"}, nil
		},
	}
	h, _ := hnd(t, q)
	c, w := hCtx(http.MethodPut, `{"role_ids":[7]}`, id1)
	h.SetUserRoles(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

// --- RegisterRoutes --------------------------------------------------------

func TestHandlerRegisterRoutes(t *testing.T) {
	h, _ := hnd(t, &fakeQ{})
	r := gin.New()
	h.RegisterRoutes(r)
	got := make(map[string]bool)
	for _, rt := range r.Routes() {
		got[rt.Method+" "+rt.Path] = true
	}
	for _, want := range []string{
		"GET /roles", "POST /roles", "PUT /roles/:id", "DELETE /roles/:id",
		"GET /roles/:id/permissions", "PUT /roles/:id/permissions",
		"GET /permissions", "GET /users/:id/roles", "PUT /users/:id/roles",
	} {
		assert.True(t, got[want], want)
	}
}

// --- Authz middleware ------------------------------------------------------

// Authz with no principal in context -> 401 (must run after Authn).
func TestAuthz_MissingPrincipal(t *testing.T) {
	enf, err := NewInMemoryEnforcer()
	require.NoError(t, err)
	c, w := hCtx(http.MethodGet, "", nil)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/roles", nil)
	Authz(enf)(c)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.True(t, c.IsAborted())
}

// Authz denies a subject without a matching policy -> 403 FORBIDDEN.
func TestAuthz_Deny(t *testing.T) {
	enf, err := NewInMemoryEnforcer()
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/roles/1", nil)
	c.Set(CtxUsername, "bob")
	Authz(enf)(c)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "FORBIDDEN", errCode(t, w))
	assert.True(t, c.IsAborted())
}

// Authz allows a subject whose role grants the (path, method).
func TestAuthz_Allow(t *testing.T) {
	enf, err := NewInMemoryEnforcer()
	require.NoError(t, err)
	require.NoError(t, enf.SetPermissionsForRole("admin", [][2]string{{"/api/v1/*", "*"}}))
	require.NoError(t, enf.SetRolesForUser("alice", []string{"admin"}))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/roles/1", nil)
	c.Set(CtxUsername, "alice")
	Authz(enf)(c)
	assert.False(t, c.IsAborted())
}
