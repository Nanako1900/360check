package user

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	gendb "github.com/nnkglobal/c5-backend/internal/gen/db"
	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
)

var errBoom = errors.New("boom")

// fakeQ embeds gendb.Querier so it satisfies the interface with only the
// overridden methods; calling an un-overridden method panics (nil func), which
// keeps each test precise about which query it drives.
type fakeQ struct {
	gendb.Querier

	listUsers      func(ctx context.Context, arg gendb.ListUsersParams) ([]gendb.ListUsersRow, error)
	countUsers     func(ctx context.Context, arg gendb.CountUsersParams) (int64, error)
	getByID        func(ctx context.Context, id int64) (gendb.GetUserByIDRow, error)
	usernameExists func(ctx context.Context, lower string) (bool, error)
	createUser     func(ctx context.Context, arg gendb.CreateUserParams) (gendb.CreateUserRow, error)
	updateUser     func(ctx context.Context, arg gendb.UpdateUserParams) (gendb.UpdateUserRow, error)
	softDelete     func(ctx context.Context, arg gendb.SoftDeleteUserParams) (int64, error)
	updatePw       func(ctx context.Context, arg gendb.UpdateUserPasswordParams) (int64, error)
}

func (f *fakeQ) ListUsers(ctx context.Context, arg gendb.ListUsersParams) ([]gendb.ListUsersRow, error) {
	return f.listUsers(ctx, arg)
}
func (f *fakeQ) CountUsers(ctx context.Context, arg gendb.CountUsersParams) (int64, error) {
	return f.countUsers(ctx, arg)
}
func (f *fakeQ) GetUserByID(ctx context.Context, id int64) (gendb.GetUserByIDRow, error) {
	return f.getByID(ctx, id)
}
func (f *fakeQ) UsernameExists(ctx context.Context, lower string) (bool, error) {
	return f.usernameExists(ctx, lower)
}
func (f *fakeQ) CreateUser(ctx context.Context, arg gendb.CreateUserParams) (gendb.CreateUserRow, error) {
	return f.createUser(ctx, arg)
}
func (f *fakeQ) UpdateUser(ctx context.Context, arg gendb.UpdateUserParams) (gendb.UpdateUserRow, error) {
	return f.updateUser(ctx, arg)
}
func (f *fakeQ) SoftDeleteUser(ctx context.Context, arg gendb.SoftDeleteUserParams) (int64, error) {
	return f.softDelete(ctx, arg)
}
func (f *fakeQ) UpdateUserPassword(ctx context.Context, arg gendb.UpdateUserPasswordParams) (int64, error) {
	return f.updatePw(ctx, arg)
}

func svc(q gendb.Querier) *Service { return NewService(q) }

func strptr(s string) *string { return &s }

// --- List ------------------------------------------------------------------

func TestList_DBError_Wrapped(t *testing.T) {
	q := &fakeQ{listUsers: func(context.Context, gendb.ListUsersParams) ([]gendb.ListUsersRow, error) {
		return nil, errBoom
	}}
	_, _, err := svc(q).List(context.Background(), nil, nil, 20, 0)
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "user: list")
}

func TestList_CountError_Wrapped(t *testing.T) {
	q := &fakeQ{
		listUsers: func(context.Context, gendb.ListUsersParams) ([]gendb.ListUsersRow, error) {
			return []gendb.ListUsersRow{}, nil
		},
		countUsers: func(context.Context, gendb.CountUsersParams) (int64, error) {
			return 0, errBoom
		},
	}
	_, _, err := svc(q).List(context.Background(), nil, nil, 20, 0)
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "user: count")
}

func TestList_Success(t *testing.T) {
	q := &fakeQ{
		listUsers: func(context.Context, gendb.ListUsersParams) ([]gendb.ListUsersRow, error) {
			return []gendb.ListUsersRow{{ID: 1, Username: "a"}, {ID: 2, Username: "b"}}, nil
		},
		countUsers: func(context.Context, gendb.CountUsersParams) (int64, error) { return 2, nil },
	}
	out, total, err := svc(q).List(context.Background(), strptr("x"), nil, 20, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, out, 2)
	assert.Equal(t, "a", out[0].Username)
}

// --- Get -------------------------------------------------------------------

func TestGet_NotFound(t *testing.T) {
	q := &fakeQ{getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
		return gendb.GetUserByIDRow{}, pgx.ErrNoRows
	}}
	_, err := svc(q).Get(context.Background(), 1)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestGet_DBError_Wrapped(t *testing.T) {
	q := &fakeQ{getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
		return gendb.GetUserByIDRow{}, errBoom
	}}
	_, err := svc(q).Get(context.Background(), 1)
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "user: get")
}

func TestGet_Success(t *testing.T) {
	q := &fakeQ{getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
		return gendb.GetUserByIDRow{ID: 9, Username: "u", IsActive: true}, nil
	}}
	u, err := svc(q).Get(context.Background(), 9)
	require.NoError(t, err)
	assert.Equal(t, int64(9), u.Id)
}

// --- Create ----------------------------------------------------------------

func TestCreate_UsernameCheckError_Wrapped(t *testing.T) {
	q := &fakeQ{usernameExists: func(context.Context, string) (bool, error) {
		return false, errBoom
	}}
	_, err := svc(q).Create(context.Background(), oapi.UserCreate{Username: "u", Password: "longpass1"}, 1)
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "user: check username")
}

func TestCreate_UsernameTaken(t *testing.T) {
	q := &fakeQ{usernameExists: func(context.Context, string) (bool, error) { return true, nil }}
	_, err := svc(q).Create(context.Background(), oapi.UserCreate{Username: "u", Password: "longpass1"}, 1)
	require.ErrorIs(t, err, ErrUsernameTaken)
}

func TestCreate_InsertDBError_Wrapped(t *testing.T) {
	q := &fakeQ{
		usernameExists: func(context.Context, string) (bool, error) { return false, nil },
		createUser: func(context.Context, gendb.CreateUserParams) (gendb.CreateUserRow, error) {
			return gendb.CreateUserRow{}, errBoom
		},
	}
	_, err := svc(q).Create(context.Background(), oapi.UserCreate{Username: "u", Password: "longpass1"}, 1)
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "user: create")
}

// A unique-violation racing the pre-check still maps to ErrUsernameTaken.
func TestCreate_UniqueViolationRace(t *testing.T) {
	q := &fakeQ{
		usernameExists: func(context.Context, string) (bool, error) { return false, nil },
		createUser: func(context.Context, gendb.CreateUserParams) (gendb.CreateUserRow, error) {
			return gendb.CreateUserRow{}, &pgconn.PgError{Code: "23505"}
		},
	}
	_, err := svc(q).Create(context.Background(), oapi.UserCreate{Username: "u", Password: "longpass1"}, 1)
	require.ErrorIs(t, err, ErrUsernameTaken)
}

func TestCreate_Success_HashesPassword(t *testing.T) {
	var stored gendb.CreateUserParams
	q := &fakeQ{
		usernameExists: func(context.Context, string) (bool, error) { return false, nil },
		createUser: func(_ context.Context, arg gendb.CreateUserParams) (gendb.CreateUserRow, error) {
			stored = arg
			return gendb.CreateUserRow{ID: 42}, nil
		},
		getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
			return gendb.GetUserByIDRow{ID: 42, Username: "u", IsActive: true}, nil
		},
	}
	active := false
	out, err := svc(q).Create(context.Background(),
		oapi.UserCreate{Username: "u", Password: "longpass1", DisplayName: strptr("U"), IsActive: &active}, 7)
	require.NoError(t, err)
	assert.Equal(t, int64(42), out.Id)
	// Password is stored as a bcrypt hash (>=12), never plaintext (no leak).
	cost, err := bcrypt.Cost([]byte(stored.PasswordHash))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, cost, 12)
	assert.NotEqual(t, "longpass1", stored.PasswordHash)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("longpass1")))
	assert.False(t, stored.IsActive, "explicit IsActive=false is honored")
}

// --- Update ----------------------------------------------------------------

func TestUpdate_NotFound(t *testing.T) {
	q := &fakeQ{updateUser: func(context.Context, gendb.UpdateUserParams) (gendb.UpdateUserRow, error) {
		return gendb.UpdateUserRow{}, pgx.ErrNoRows
	}}
	_, err := svc(q).Update(context.Background(), 1, oapi.UserUpdate{}, 1)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestUpdate_DBError_Wrapped(t *testing.T) {
	q := &fakeQ{updateUser: func(context.Context, gendb.UpdateUserParams) (gendb.UpdateUserRow, error) {
		return gendb.UpdateUserRow{}, errBoom
	}}
	_, err := svc(q).Update(context.Background(), 1, oapi.UserUpdate{}, 1)
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "user: update")
}

func TestUpdate_Success(t *testing.T) {
	q := &fakeQ{
		updateUser: func(context.Context, gendb.UpdateUserParams) (gendb.UpdateUserRow, error) {
			return gendb.UpdateUserRow{ID: 3}, nil
		},
		getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
			return gendb.GetUserByIDRow{ID: 3, Username: "u", IsActive: true}, nil
		},
	}
	out, err := svc(q).Update(context.Background(), 3, oapi.UserUpdate{DisplayName: strptr("New")}, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(3), out.Id)
}

// --- Delete ----------------------------------------------------------------

func TestDelete_DBError_Wrapped(t *testing.T) {
	q := &fakeQ{softDelete: func(context.Context, gendb.SoftDeleteUserParams) (int64, error) {
		return 0, errBoom
	}}
	err := svc(q).Delete(context.Background(), 1, 1)
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "user: delete")
}

func TestDelete_NotFound(t *testing.T) {
	q := &fakeQ{softDelete: func(context.Context, gendb.SoftDeleteUserParams) (int64, error) {
		return 0, nil // zero rows affected -> already deleted / missing
	}}
	err := svc(q).Delete(context.Background(), 1, 1)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestDelete_Success(t *testing.T) {
	q := &fakeQ{softDelete: func(context.Context, gendb.SoftDeleteUserParams) (int64, error) {
		return 1, nil
	}}
	require.NoError(t, svc(q).Delete(context.Background(), 1, 1))
}

// --- ResetPassword ---------------------------------------------------------

func TestResetPassword_DBError_Wrapped(t *testing.T) {
	q := &fakeQ{updatePw: func(context.Context, gendb.UpdateUserPasswordParams) (int64, error) {
		return 0, errBoom
	}}
	err := svc(q).ResetPassword(context.Background(), 1, "newpassword123", 1)
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "user: reset password")
}

func TestResetPassword_NotFound(t *testing.T) {
	q := &fakeQ{updatePw: func(context.Context, gendb.UpdateUserPasswordParams) (int64, error) {
		return 0, nil
	}}
	err := svc(q).ResetPassword(context.Background(), 1, "newpassword123", 1)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestResetPassword_Success_HashesAtBaseline(t *testing.T) {
	var stored gendb.UpdateUserPasswordParams
	q := &fakeQ{updatePw: func(_ context.Context, arg gendb.UpdateUserPasswordParams) (int64, error) {
		stored = arg
		return 1, nil
	}}
	require.NoError(t, svc(q).ResetPassword(context.Background(), 1, "newpassword123", 7))
	cost, err := bcrypt.Cost([]byte(stored.PasswordHash))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, cost, 12)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("newpassword123")))
}
