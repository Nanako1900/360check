package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	gendb "github.com/nnkglobal/c5-backend/internal/gen/db"
	"github.com/nnkglobal/c5-backend/internal/rbac"
)

// errBoom is a generic DB failure used to drive the defensive error-wrap
// branches that the happy-path integration tests never hit.
var errBoom = errors.New("boom")

// fakeQ embeds gendb.Querier so it satisfies the interface with only the
// methods a given test overrides; every un-overridden method panics, which
// keeps each test honest about exactly which query it exercises.
type fakeQ struct {
	gendb.Querier

	getByUsername func(ctx context.Context, username string) (gendb.GetUserByUsernameRow, error)
	getByID       func(ctx context.Context, id int64) (gendb.GetUserByIDRow, error)
	touchLogin    func(ctx context.Context, id int64) error
	getPwHash     func(ctx context.Context, id int64) (string, error)
	updatePw      func(ctx context.Context, arg gendb.UpdateUserPasswordParams) (int64, error)
	getRoleByCode func(ctx context.Context, code string) (gendb.GetRoleByCodeRow, error)
	listPerms     func(ctx context.Context) ([]gendb.ListPermissionsRow, error)
}

func (f *fakeQ) GetUserByUsername(ctx context.Context, username string) (gendb.GetUserByUsernameRow, error) {
	return f.getByUsername(ctx, username)
}
func (f *fakeQ) GetUserByID(ctx context.Context, id int64) (gendb.GetUserByIDRow, error) {
	return f.getByID(ctx, id)
}
func (f *fakeQ) TouchUserLastLogin(ctx context.Context, id int64) error {
	return f.touchLogin(ctx, id)
}
func (f *fakeQ) GetUserPasswordHash(ctx context.Context, id int64) (string, error) {
	return f.getPwHash(ctx, id)
}
func (f *fakeQ) UpdateUserPassword(ctx context.Context, arg gendb.UpdateUserPasswordParams) (int64, error) {
	return f.updatePw(ctx, arg)
}
func (f *fakeQ) GetRoleByCode(ctx context.Context, code string) (gendb.GetRoleByCodeRow, error) {
	return f.getRoleByCode(ctx, code)
}
func (f *fakeQ) ListPermissions(ctx context.Context) ([]gendb.ListPermissionsRow, error) {
	return f.listPerms(ctx)
}

// fakeJWT is a stub jwtIssuer; errFn lets a test force the issuance branch.
type fakeJWT struct{ err error }

func (j fakeJWT) Issue(int64, string, []string, time.Time) (string, int, error) {
	return "access-token", 900, j.err
}

// newTestService wires a Service with the fake querier, an in-memory enforcer,
// and a miniredis-backed refresh store. The bcrypt hash is for "correctpass".
func newTestService(t *testing.T, q gendb.Querier, jwtErr error) *Service {
	t.Helper()
	enf, err := rbac.NewInMemoryEnforcer()
	require.NoError(t, err)
	store, _ := newTestStore(t, time.Hour)
	return NewService(q, fakeJWT{err: jwtErr}, store, enf)
}

func mustHash(t *testing.T, pw string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(pw), BcryptCost)
	require.NoError(t, err)
	return string(h)
}

// --- Login -----------------------------------------------------------------

func TestLogin_UnknownUser_NoEnumeration(t *testing.T) {
	q := &fakeQ{getByUsername: func(context.Context, string) (gendb.GetUserByUsernameRow, error) {
		return gendb.GetUserByUsernameRow{}, pgx.ErrNoRows
	}}
	_, err := newTestService(t, q, nil).Login(context.Background(), "nobody", "x")
	// Unknown user must look identical to a wrong password: ErrInvalidCredentials.
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestLogin_LoadUserDBError_Wrapped(t *testing.T) {
	q := &fakeQ{getByUsername: func(context.Context, string) (gendb.GetUserByUsernameRow, error) {
		return gendb.GetUserByUsernameRow{}, errBoom
	}}
	_, err := newTestService(t, q, nil).Login(context.Background(), "u", "x")
	require.ErrorIs(t, err, errBoom)
	assert.NotErrorIs(t, err, ErrInvalidCredentials, "a real DB error must not masquerade as bad credentials")
	assert.Contains(t, err.Error(), "auth: load user")
}

func TestLogin_InactiveUser_InvalidCredentials(t *testing.T) {
	q := &fakeQ{getByUsername: func(context.Context, string) (gendb.GetUserByUsernameRow, error) {
		return gendb.GetUserByUsernameRow{
			ID: 1, Username: "u", PasswordHash: mustHash(t, "correctpass"), IsActive: false,
		}, nil
	}}
	// Even with the correct password, a disabled account is rejected as ErrInvalidCredentials.
	_, err := newTestService(t, q, nil).Login(context.Background(), "u", "correctpass")
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestLogin_WrongPassword_InvalidCredentials(t *testing.T) {
	q := &fakeQ{getByUsername: func(context.Context, string) (gendb.GetUserByUsernameRow, error) {
		return gendb.GetUserByUsernameRow{
			ID: 1, Username: "u", PasswordHash: mustHash(t, "correctpass"), IsActive: true,
		}, nil
	}}
	_, err := newTestService(t, q, nil).Login(context.Background(), "u", "wrongpass")
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestLogin_JWTIssueError(t *testing.T) {
	q := &fakeQ{getByUsername: func(context.Context, string) (gendb.GetUserByUsernameRow, error) {
		return gendb.GetUserByUsernameRow{
			ID: 1, Username: "u", PasswordHash: mustHash(t, "correctpass"), IsActive: true,
		}, nil
	}}
	_, err := newTestService(t, q, errBoom).Login(context.Background(), "u", "correctpass")
	require.ErrorIs(t, err, errBoom)
}

func TestLogin_TouchLastLoginError_Wrapped(t *testing.T) {
	q := &fakeQ{
		getByUsername: func(context.Context, string) (gendb.GetUserByUsernameRow, error) {
			return gendb.GetUserByUsernameRow{
				ID: 1, Username: "u", PasswordHash: mustHash(t, "correctpass"), IsActive: true,
			}, nil
		},
		touchLogin: func(context.Context, int64) error { return errBoom },
	}
	_, err := newTestService(t, q, nil).Login(context.Background(), "u", "correctpass")
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "auth: touch last_login")
}

func TestLogin_Success(t *testing.T) {
	q := &fakeQ{
		getByUsername: func(context.Context, string) (gendb.GetUserByUsernameRow, error) {
			return gendb.GetUserByUsernameRow{
				ID: 7, Username: "u", PasswordHash: mustHash(t, "correctpass"), IsActive: true,
			}, nil
		},
		touchLogin: func(context.Context, int64) error { return nil },
	}
	tokens, err := newTestService(t, q, nil).Login(context.Background(), "u", "correctpass")
	require.NoError(t, err)
	assert.Equal(t, "access-token", tokens.AccessToken)
	assert.NotEmpty(t, tokens.RefreshToken)
	assert.Equal(t, int64(7), tokens.User.Id)
}

// --- Refresh ---------------------------------------------------------------

func TestRefresh_UnknownToken(t *testing.T) {
	q := &fakeQ{}
	// No refresh token was ever created, so Rotate fails before any query runs.
	_, err := newTestService(t, q, nil).Refresh(context.Background(), "no-such-token")
	require.ErrorIs(t, err, ErrRefreshNotFound)
}

func TestRefresh_UserGoneAfterRotate(t *testing.T) {
	q := &fakeQ{getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
		return gendb.GetUserByIDRow{}, pgx.ErrNoRows
	}}
	svc := newTestService(t, q, nil)
	tok, err := svc.refresh.Create(context.Background(), 1)
	require.NoError(t, err)
	_, err = svc.Refresh(context.Background(), tok)
	require.ErrorIs(t, err, ErrRefreshNotFound)
}

func TestRefresh_LoadUserDBError_Wrapped(t *testing.T) {
	q := &fakeQ{getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
		return gendb.GetUserByIDRow{}, errBoom
	}}
	svc := newTestService(t, q, nil)
	tok, err := svc.refresh.Create(context.Background(), 1)
	require.NoError(t, err)
	_, err = svc.Refresh(context.Background(), tok)
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "auth: load user")
}

func TestRefresh_InactiveUser_RevokesAndRejects(t *testing.T) {
	q := &fakeQ{getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
		return gendb.GetUserByIDRow{ID: 1, Username: "u", IsActive: false}, nil
	}}
	svc := newTestService(t, q, nil)
	tok, err := svc.refresh.Create(context.Background(), 1)
	require.NoError(t, err)
	newTok, err := svc.refresh.Create(context.Background(), 1)
	require.NoError(t, err)
	_ = newTok
	_, err = svc.Refresh(context.Background(), tok)
	require.ErrorIs(t, err, ErrRefreshNotFound)
}

func TestRefresh_JWTIssueError(t *testing.T) {
	q := &fakeQ{getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
		return gendb.GetUserByIDRow{ID: 1, Username: "u", IsActive: true}, nil
	}}
	svc := newTestService(t, q, errBoom)
	tok, err := svc.refresh.Create(context.Background(), 1)
	require.NoError(t, err)
	_, err = svc.Refresh(context.Background(), tok)
	require.ErrorIs(t, err, errBoom)
}

func TestRefresh_Success(t *testing.T) {
	q := &fakeQ{getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
		return gendb.GetUserByIDRow{ID: 3, Username: "u", IsActive: true}, nil
	}}
	svc := newTestService(t, q, nil)
	tok, err := svc.refresh.Create(context.Background(), 3)
	require.NoError(t, err)
	out, err := svc.Refresh(context.Background(), tok)
	require.NoError(t, err)
	assert.NotEqual(t, tok, out.RefreshToken, "refresh must rotate (single-use)")
	assert.Equal(t, int64(3), out.User.Id)
}

// --- Me --------------------------------------------------------------------

func TestMe_UserNotFound(t *testing.T) {
	q := &fakeQ{getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
		return gendb.GetUserByIDRow{}, pgx.ErrNoRows
	}}
	_, err := newTestService(t, q, nil).Me(context.Background(), 1)
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestMe_LoadUserDBError_Wrapped(t *testing.T) {
	q := &fakeQ{getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
		return gendb.GetUserByIDRow{}, errBoom
	}}
	_, err := newTestService(t, q, nil).Me(context.Background(), 1)
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "auth: load user")
}

func TestMe_ListPermissionsError_Wrapped(t *testing.T) {
	q := &fakeQ{
		getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
			return gendb.GetUserByIDRow{ID: 1, Username: "u", IsActive: true}, nil
		},
		listPerms: func(context.Context) ([]gendb.ListPermissionsRow, error) {
			return nil, errBoom
		},
	}
	_, err := newTestService(t, q, nil).Me(context.Background(), 1)
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "auth: load permissions")
}

func TestMe_Success_NoRoles(t *testing.T) {
	q := &fakeQ{
		getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
			return gendb.GetUserByIDRow{ID: 1, Username: "u", IsActive: true}, nil
		},
		listPerms: func(context.Context) ([]gendb.ListPermissionsRow, error) {
			return nil, nil // user has no granted permissions
		},
	}
	me, err := newTestService(t, q, nil).Me(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), me.User.Id)
	assert.Empty(t, me.Roles)
	assert.Empty(t, me.Permissions)
}

// --- ChangePassword --------------------------------------------------------

func TestChangePassword_UserNotFound(t *testing.T) {
	q := &fakeQ{getPwHash: func(context.Context, int64) (string, error) {
		return "", pgx.ErrNoRows
	}}
	err := newTestService(t, q, nil).ChangePassword(context.Background(), 1, "old", "newpassword123")
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestChangePassword_LoadHashDBError_Wrapped(t *testing.T) {
	q := &fakeQ{getPwHash: func(context.Context, int64) (string, error) {
		return "", errBoom
	}}
	err := newTestService(t, q, nil).ChangePassword(context.Background(), 1, "old", "newpassword123")
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "auth: load password hash")
}

func TestChangePassword_WrongOldPassword(t *testing.T) {
	q := &fakeQ{getPwHash: func(context.Context, int64) (string, error) {
		return mustHash(t, "correctold"), nil
	}}
	err := newTestService(t, q, nil).ChangePassword(context.Background(), 1, "wrongold", "newpassword123")
	require.ErrorIs(t, err, ErrOldPasswordWrong)
}

func TestChangePassword_UpdateDBError_Wrapped(t *testing.T) {
	q := &fakeQ{
		getPwHash: func(context.Context, int64) (string, error) {
			return mustHash(t, "correctold"), nil
		},
		updatePw: func(context.Context, gendb.UpdateUserPasswordParams) (int64, error) {
			return 0, errBoom
		},
	}
	err := newTestService(t, q, nil).ChangePassword(context.Background(), 1, "correctold", "newpassword123")
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "auth: update password")
}

func TestChangePassword_Success(t *testing.T) {
	var stored gendb.UpdateUserPasswordParams
	q := &fakeQ{
		getPwHash: func(context.Context, int64) (string, error) {
			return mustHash(t, "correctold"), nil
		},
		updatePw: func(_ context.Context, arg gendb.UpdateUserPasswordParams) (int64, error) {
			stored = arg
			return 1, nil
		},
	}
	err := newTestService(t, q, nil).ChangePassword(context.Background(), 1, "correctold", "newpassword123")
	require.NoError(t, err)
	// The new password is stored as a bcrypt hash at cost >= 12 (security baseline).
	cost, err := bcrypt.Cost([]byte(stored.PasswordHash))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, cost, 12)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("newpassword123")))
}

// TestBcryptCostMeetsBaseline locks the documented security baseline (cost >= 12).
func TestBcryptCostMeetsBaseline(t *testing.T) {
	assert.GreaterOrEqual(t, BcryptCost, 12)
}
