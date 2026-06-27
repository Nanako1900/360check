package rbac

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gendb "github.com/nnkglobal/c5-backend/internal/gen/db"
	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
)

var errBoom = errors.New("boom")

// fakeQ embeds gendb.Querier so only the overridden methods are implemented; an
// un-overridden call panics, keeping each test precise about which query it hits.
type fakeQ struct {
	gendb.Querier

	listRoles     func(ctx context.Context) ([]gendb.ListRolesRow, error)
	roleCodeEx    func(ctx context.Context, code string) (bool, error)
	createRole    func(ctx context.Context, arg gendb.CreateRoleParams) (gendb.CreateRoleRow, error)
	updateRole    func(ctx context.Context, arg gendb.UpdateRoleParams) (gendb.UpdateRoleRow, error)
	getRole       func(ctx context.Context, id int64) (gendb.GetRoleRow, error)
	softDeleteRl  func(ctx context.Context, id int64) (int64, error)
	listPerms     func(ctx context.Context) ([]gendb.ListPermissionsRow, error)
	permsByIDs    func(ctx context.Context, ids []int64) ([]gendb.PermissionsByIDsRow, error)
	getByID       func(ctx context.Context, id int64) (gendb.GetUserByIDRow, error)
	rolesByIDs    func(ctx context.Context, ids []int64) ([]gendb.RolesByIDsRow, error)
	getRoleByCode func(ctx context.Context, code string) (gendb.GetRoleByCodeRow, error)
}

func (f *fakeQ) ListRoles(ctx context.Context) ([]gendb.ListRolesRow, error) {
	return f.listRoles(ctx)
}
func (f *fakeQ) RoleCodeExists(ctx context.Context, code string) (bool, error) {
	return f.roleCodeEx(ctx, code)
}
func (f *fakeQ) CreateRole(ctx context.Context, arg gendb.CreateRoleParams) (gendb.CreateRoleRow, error) {
	return f.createRole(ctx, arg)
}
func (f *fakeQ) UpdateRole(ctx context.Context, arg gendb.UpdateRoleParams) (gendb.UpdateRoleRow, error) {
	return f.updateRole(ctx, arg)
}
func (f *fakeQ) GetRole(ctx context.Context, id int64) (gendb.GetRoleRow, error) {
	return f.getRole(ctx, id)
}
func (f *fakeQ) SoftDeleteRole(ctx context.Context, id int64) (int64, error) {
	return f.softDeleteRl(ctx, id)
}
func (f *fakeQ) ListPermissions(ctx context.Context) ([]gendb.ListPermissionsRow, error) {
	return f.listPerms(ctx)
}
func (f *fakeQ) PermissionsByIDs(ctx context.Context, ids []int64) ([]gendb.PermissionsByIDsRow, error) {
	return f.permsByIDs(ctx, ids)
}
func (f *fakeQ) GetUserByID(ctx context.Context, id int64) (gendb.GetUserByIDRow, error) {
	return f.getByID(ctx, id)
}
func (f *fakeQ) RolesByIDs(ctx context.Context, ids []int64) ([]gendb.RolesByIDsRow, error) {
	return f.rolesByIDs(ctx, ids)
}
func (f *fakeQ) GetRoleByCode(ctx context.Context, code string) (gendb.GetRoleByCodeRow, error) {
	return f.getRoleByCode(ctx, code)
}

// newSvc wires a Service with the fake querier and a fresh in-memory enforcer.
func newSvc(t *testing.T, q gendb.Querier) (*Service, *Enforcer) {
	t.Helper()
	enf, err := NewInMemoryEnforcer()
	require.NoError(t, err)
	return NewService(q, enf), enf
}

func intptr(i int) *int       { return &i }
func strptr(s string) *string { return &s }

// --- ListRoles -------------------------------------------------------------

func TestSvcListRoles_DBError_Wrapped(t *testing.T) {
	q := &fakeQ{listRoles: func(context.Context) ([]gendb.ListRolesRow, error) { return nil, errBoom }}
	s, _ := newSvc(t, q)
	_, err := s.ListRoles(context.Background())
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "rbac: list roles")
}

func TestSvcListRoles_Success(t *testing.T) {
	q := &fakeQ{listRoles: func(context.Context) ([]gendb.ListRolesRow, error) {
		return []gendb.ListRolesRow{{ID: 1, Code: "admin", Name: "Admin"}}, nil
	}}
	s, _ := newSvc(t, q)
	out, err := s.ListRoles(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "admin", out[0].Code)
}

// --- CreateRole ------------------------------------------------------------

func TestSvcCreateRole_CheckCodeError_Wrapped(t *testing.T) {
	q := &fakeQ{roleCodeEx: func(context.Context, string) (bool, error) { return false, errBoom }}
	s, _ := newSvc(t, q)
	_, err := s.CreateRole(context.Background(), oapi.RoleCreate{Code: "x", Name: "X"}, 1)
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "rbac: check role code")
}

func TestSvcCreateRole_CodeTaken(t *testing.T) {
	q := &fakeQ{roleCodeEx: func(context.Context, string) (bool, error) { return true, nil }}
	s, _ := newSvc(t, q)
	_, err := s.CreateRole(context.Background(), oapi.RoleCreate{Code: "x", Name: "X"}, 1)
	require.ErrorIs(t, err, ErrRoleCodeTaken)
}

func TestSvcCreateRole_InsertError_Wrapped(t *testing.T) {
	q := &fakeQ{
		roleCodeEx: func(context.Context, string) (bool, error) { return false, nil },
		createRole: func(context.Context, gendb.CreateRoleParams) (gendb.CreateRoleRow, error) {
			return gendb.CreateRoleRow{}, errBoom
		},
	}
	s, _ := newSvc(t, q)
	_, err := s.CreateRole(context.Background(), oapi.RoleCreate{Code: "x", Name: "X"}, 1)
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "rbac: create role")
}

func TestSvcCreateRole_Success(t *testing.T) {
	q := &fakeQ{
		roleCodeEx: func(context.Context, string) (bool, error) { return false, nil },
		createRole: func(_ context.Context, arg gendb.CreateRoleParams) (gendb.CreateRoleRow, error) {
			return gendb.CreateRoleRow{ID: 9, Code: arg.Code, Name: arg.Name}, nil
		},
	}
	s, _ := newSvc(t, q)
	r, err := s.CreateRole(context.Background(),
		oapi.RoleCreate{Code: "ops", Name: "Ops", Description: strptr("d"), SortOrder: intptr(3)}, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(9), r.Id)
	assert.Equal(t, "ops", r.Code)
}

// --- UpdateRole ------------------------------------------------------------

func TestSvcUpdateRole_NotFound(t *testing.T) {
	q := &fakeQ{updateRole: func(context.Context, gendb.UpdateRoleParams) (gendb.UpdateRoleRow, error) {
		return gendb.UpdateRoleRow{}, pgx.ErrNoRows
	}}
	s, _ := newSvc(t, q)
	_, err := s.UpdateRole(context.Background(), 1, oapi.RoleUpdate{}, 1)
	require.ErrorIs(t, err, ErrRoleNotFound)
}

func TestSvcUpdateRole_DBError_Wrapped(t *testing.T) {
	q := &fakeQ{updateRole: func(context.Context, gendb.UpdateRoleParams) (gendb.UpdateRoleRow, error) {
		return gendb.UpdateRoleRow{}, errBoom
	}}
	s, _ := newSvc(t, q)
	_, err := s.UpdateRole(context.Background(), 1, oapi.RoleUpdate{SortOrder: intptr(2)}, 1)
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "rbac: update role")
}

func TestSvcUpdateRole_Success(t *testing.T) {
	q := &fakeQ{updateRole: func(context.Context, gendb.UpdateRoleParams) (gendb.UpdateRoleRow, error) {
		return gendb.UpdateRoleRow{ID: 4, Code: "ops", Name: "Ops2"}, nil
	}}
	s, _ := newSvc(t, q)
	r, err := s.UpdateRole(context.Background(), 4, oapi.RoleUpdate{Name: strptr("Ops2")}, 1)
	require.NoError(t, err)
	assert.Equal(t, "Ops2", r.Name)
}

// --- DeleteRole ------------------------------------------------------------

func TestSvcDeleteRole_NotFound(t *testing.T) {
	q := &fakeQ{getRole: func(context.Context, int64) (gendb.GetRoleRow, error) {
		return gendb.GetRoleRow{}, pgx.ErrNoRows
	}}
	s, _ := newSvc(t, q)
	require.ErrorIs(t, s.DeleteRole(context.Background(), 1), ErrRoleNotFound)
}

func TestSvcDeleteRole_LoadError_Wrapped(t *testing.T) {
	q := &fakeQ{getRole: func(context.Context, int64) (gendb.GetRoleRow, error) {
		return gendb.GetRoleRow{}, errBoom
	}}
	s, _ := newSvc(t, q)
	err := s.DeleteRole(context.Background(), 1)
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "rbac: load role")
}

// A system role must never be deletable.
func TestSvcDeleteRole_SystemRole(t *testing.T) {
	q := &fakeQ{getRole: func(context.Context, int64) (gendb.GetRoleRow, error) {
		return gendb.GetRoleRow{ID: 1, Code: "admin", IsSystem: true}, nil
	}}
	s, _ := newSvc(t, q)
	require.ErrorIs(t, s.DeleteRole(context.Background(), 1), ErrSystemRole)
}

func TestSvcDeleteRole_SoftDeleteError_Wrapped(t *testing.T) {
	q := &fakeQ{
		getRole: func(context.Context, int64) (gendb.GetRoleRow, error) {
			return gendb.GetRoleRow{ID: 2, Code: "ops", IsSystem: false}, nil
		},
		softDeleteRl: func(context.Context, int64) (int64, error) { return 0, errBoom },
	}
	s, _ := newSvc(t, q)
	err := s.DeleteRole(context.Background(), 2)
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "rbac: delete role")
}

// Zero rows affected (a concurrent delete or it became a system row) -> ErrSystemRole.
func TestSvcDeleteRole_ZeroRows(t *testing.T) {
	q := &fakeQ{
		getRole: func(context.Context, int64) (gendb.GetRoleRow, error) {
			return gendb.GetRoleRow{ID: 2, Code: "ops", IsSystem: false}, nil
		},
		softDeleteRl: func(context.Context, int64) (int64, error) { return 0, nil },
	}
	s, _ := newSvc(t, q)
	require.ErrorIs(t, s.DeleteRole(context.Background(), 2), ErrSystemRole)
}

func TestSvcDeleteRole_Success_RemovesCasbinRules(t *testing.T) {
	q := &fakeQ{
		getRole: func(context.Context, int64) (gendb.GetRoleRow, error) {
			return gendb.GetRoleRow{ID: 2, Code: "ops", IsSystem: false}, nil
		},
		softDeleteRl: func(context.Context, int64) (int64, error) { return 1, nil },
	}
	s, enf := newSvc(t, q)
	require.NoError(t, enf.SetPermissionsForRole("ops", [][2]string{{"/x", "GET"}}))
	require.NoError(t, enf.SetRolesForUser("alice", []string{"ops"}))
	require.NoError(t, s.DeleteRole(context.Background(), 2))
	// Casbin p-rules and g-rules for the role are gone.
	perms, err := enf.PermissionsForRole("ops")
	require.NoError(t, err)
	assert.Empty(t, perms)
	roles, err := enf.RolesForUser("alice")
	require.NoError(t, err)
	assert.Empty(t, roles)
}

// --- GetRolePermissions ----------------------------------------------------

func TestSvcGetRolePermissions_NotFound(t *testing.T) {
	q := &fakeQ{getRole: func(context.Context, int64) (gendb.GetRoleRow, error) {
		return gendb.GetRoleRow{}, pgx.ErrNoRows
	}}
	s, _ := newSvc(t, q)
	_, err := s.GetRolePermissions(context.Background(), 1)
	require.ErrorIs(t, err, ErrRoleNotFound)
}

func TestSvcGetRolePermissions_LoadRoleError_Wrapped(t *testing.T) {
	q := &fakeQ{getRole: func(context.Context, int64) (gendb.GetRoleRow, error) {
		return gendb.GetRoleRow{}, errBoom
	}}
	s, _ := newSvc(t, q)
	_, err := s.GetRolePermissions(context.Background(), 1)
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "rbac: load role")
}

func TestSvcGetRolePermissions_ListPermsError_Wrapped(t *testing.T) {
	q := &fakeQ{
		getRole: func(context.Context, int64) (gendb.GetRoleRow, error) {
			return gendb.GetRoleRow{ID: 1, Code: "ops"}, nil
		},
		listPerms: func(context.Context) ([]gendb.ListPermissionsRow, error) { return nil, errBoom },
	}
	s, enf := newSvc(t, q)
	require.NoError(t, enf.SetPermissionsForRole("ops", [][2]string{{"/x", "GET"}}))
	_, err := s.GetRolePermissions(context.Background(), 1)
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "rbac: list permissions")
}

func TestSvcGetRolePermissions_Success_OnlyGranted(t *testing.T) {
	q := &fakeQ{
		getRole: func(context.Context, int64) (gendb.GetRoleRow, error) {
			return gendb.GetRoleRow{ID: 1, Code: "ops"}, nil
		},
		listPerms: func(context.Context) ([]gendb.ListPermissionsRow, error) {
			return []gendb.ListPermissionsRow{
				{ID: 1, Code: "p:read", Object: "/x", Action: "GET"},
				{ID: 2, Code: "p:write", Object: "/y", Action: "POST"},
			}, nil
		},
	}
	s, enf := newSvc(t, q)
	require.NoError(t, enf.SetPermissionsForRole("ops", [][2]string{{"/x", "GET"}}))
	out, err := s.GetRolePermissions(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "p:read", out[0].Code)
}

// --- SetRolePermissions ----------------------------------------------------

func TestSvcSetRolePermissions_NotFound(t *testing.T) {
	q := &fakeQ{getRole: func(context.Context, int64) (gendb.GetRoleRow, error) {
		return gendb.GetRoleRow{}, pgx.ErrNoRows
	}}
	s, _ := newSvc(t, q)
	_, err := s.SetRolePermissions(context.Background(), 1, []int64{1})
	require.ErrorIs(t, err, ErrRoleNotFound)
}

func TestSvcSetRolePermissions_LoadRoleError_Wrapped(t *testing.T) {
	q := &fakeQ{getRole: func(context.Context, int64) (gendb.GetRoleRow, error) {
		return gendb.GetRoleRow{}, errBoom
	}}
	s, _ := newSvc(t, q)
	_, err := s.SetRolePermissions(context.Background(), 1, []int64{1})
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "rbac: load role")
}

func TestSvcSetRolePermissions_LoadPermsError_Wrapped(t *testing.T) {
	q := &fakeQ{
		getRole: func(context.Context, int64) (gendb.GetRoleRow, error) {
			return gendb.GetRoleRow{ID: 1, Code: "ops"}, nil
		},
		permsByIDs: func(context.Context, []int64) ([]gendb.PermissionsByIDsRow, error) {
			return nil, errBoom
		},
	}
	s, _ := newSvc(t, q)
	_, err := s.SetRolePermissions(context.Background(), 1, []int64{1})
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "rbac: load permissions")
}

func TestSvcSetRolePermissions_Success(t *testing.T) {
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
	s, enf := newSvc(t, q)
	out, err := s.SetRolePermissions(context.Background(), 1, []int64{1})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "p:read", out[0].Code)
	// The casbin policy was actually written.
	perms, err := enf.PermissionsForRole("ops")
	require.NoError(t, err)
	assert.Len(t, perms, 1)
}

// --- ListPermissions -------------------------------------------------------

func TestSvcListPermissions_DBError_Wrapped(t *testing.T) {
	q := &fakeQ{listPerms: func(context.Context) ([]gendb.ListPermissionsRow, error) { return nil, errBoom }}
	s, _ := newSvc(t, q)
	_, err := s.ListPermissions(context.Background())
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "rbac: list permissions")
}

func TestSvcListPermissions_Success(t *testing.T) {
	q := &fakeQ{listPerms: func(context.Context) ([]gendb.ListPermissionsRow, error) {
		return []gendb.ListPermissionsRow{{ID: 1, Code: "p:read", Object: "/x", Action: "GET"}}, nil
	}}
	s, _ := newSvc(t, q)
	out, err := s.ListPermissions(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "p:read", out[0].Code)
}

// --- GetUserRoles ----------------------------------------------------------

func TestSvcGetUserRoles_UserNotFound(t *testing.T) {
	q := &fakeQ{getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
		return gendb.GetUserByIDRow{}, pgx.ErrNoRows
	}}
	s, _ := newSvc(t, q)
	_, err := s.GetUserRoles(context.Background(), 1)
	require.ErrorIs(t, err, ErrUserNotFound)
}

func TestSvcGetUserRoles_LoadUserError_Wrapped(t *testing.T) {
	q := &fakeQ{getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
		return gendb.GetUserByIDRow{}, errBoom
	}}
	s, _ := newSvc(t, q)
	_, err := s.GetUserRoles(context.Background(), 1)
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "rbac: load user")
}

func TestSvcGetUserRoles_LoadRoleError_Wrapped(t *testing.T) {
	q := &fakeQ{
		getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
			return gendb.GetUserByIDRow{ID: 1, Username: "alice"}, nil
		},
		getRoleByCode: func(context.Context, string) (gendb.GetRoleByCodeRow, error) {
			return gendb.GetRoleByCodeRow{}, errBoom
		},
	}
	s, enf := newSvc(t, q)
	require.NoError(t, enf.SetRolesForUser("alice", []string{"ops"}))
	_, err := s.GetUserRoles(context.Background(), 1)
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "rbac: load role")
}

// A casbin grant whose role row was deleted is skipped, not fatal.
func TestSvcGetUserRoles_MissingRoleSkipped(t *testing.T) {
	q := &fakeQ{
		getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
			return gendb.GetUserByIDRow{ID: 1, Username: "alice"}, nil
		},
		getRoleByCode: func(context.Context, string) (gendb.GetRoleByCodeRow, error) {
			return gendb.GetRoleByCodeRow{}, pgx.ErrNoRows
		},
	}
	s, enf := newSvc(t, q)
	require.NoError(t, enf.SetRolesForUser("alice", []string{"ghost"}))
	out, err := s.GetUserRoles(context.Background(), 1)
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestSvcGetUserRoles_Success(t *testing.T) {
	q := &fakeQ{
		getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
			return gendb.GetUserByIDRow{ID: 1, Username: "alice"}, nil
		},
		getRoleByCode: func(_ context.Context, code string) (gendb.GetRoleByCodeRow, error) {
			return gendb.GetRoleByCodeRow{ID: 7, Code: code, Name: "Ops"}, nil
		},
	}
	s, enf := newSvc(t, q)
	require.NoError(t, enf.SetRolesForUser("alice", []string{"ops"}))
	out, err := s.GetUserRoles(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "ops", out[0].Code)
}

// --- SetUserRoles ----------------------------------------------------------

func TestSvcSetUserRoles_UserNotFound(t *testing.T) {
	q := &fakeQ{getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
		return gendb.GetUserByIDRow{}, pgx.ErrNoRows
	}}
	s, _ := newSvc(t, q)
	_, err := s.SetUserRoles(context.Background(), 1, []int64{1})
	require.ErrorIs(t, err, ErrUserNotFound)
}

func TestSvcSetUserRoles_LoadUserError_Wrapped(t *testing.T) {
	q := &fakeQ{getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
		return gendb.GetUserByIDRow{}, errBoom
	}}
	s, _ := newSvc(t, q)
	_, err := s.SetUserRoles(context.Background(), 1, []int64{1})
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "rbac: load user")
}

func TestSvcSetUserRoles_LoadRolesError_Wrapped(t *testing.T) {
	q := &fakeQ{
		getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
			return gendb.GetUserByIDRow{ID: 1, Username: "alice"}, nil
		},
		rolesByIDs: func(context.Context, []int64) ([]gendb.RolesByIDsRow, error) {
			return nil, errBoom
		},
	}
	s, _ := newSvc(t, q)
	_, err := s.SetUserRoles(context.Background(), 1, []int64{1})
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "rbac: load roles")
}

func TestSvcSetUserRoles_Success(t *testing.T) {
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
	s, enf := newSvc(t, q)
	out, err := s.SetUserRoles(context.Background(), 1, []int64{7})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "ops", out[0].Code)
	// The casbin g-rule was written.
	roles, err := enf.RolesForUser("alice")
	require.NoError(t, err)
	assert.Equal(t, []string{"ops"}, roles)
}
