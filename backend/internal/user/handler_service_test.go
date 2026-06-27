package user

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

// hCtx builds a gin context for a request with an optional body and path params.
func hCtx(method, target, body string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var rdr *strings.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	} else {
		rdr = strings.NewReader("")
	}
	c.Request = httptest.NewRequest(method, target, rdr)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	return c, w
}

func hnd(q gendb.Querier) *Handler { return NewHandler(NewService(q)) }

// --- List handler ----------------------------------------------------------

func TestHandlerList_Success(t *testing.T) {
	q := &fakeQ{
		listUsers: func(_ context.Context, arg gendb.ListUsersParams) ([]gendb.ListUsersRow, error) {
			// q + is_active filters are threaded through.
			require.NotNil(t, arg.Q)
			require.NotNil(t, arg.IsActive)
			return []gendb.ListUsersRow{{ID: 1, Username: "a"}}, nil
		},
		countUsers: func(context.Context, gendb.CountUsersParams) (int64, error) { return 1, nil },
	}
	c, w := hCtx(http.MethodGet, "/users?q=al&is_active=true", "", nil)
	hnd(q).List(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandlerList_InternalError(t *testing.T) {
	q := &fakeQ{listUsers: func(context.Context, gendb.ListUsersParams) ([]gendb.ListUsersRow, error) {
		return nil, errBoom
	}}
	c, w := hCtx(http.MethodGet, "/users", "", nil)
	hnd(q).List(c)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "INTERNAL", errCode(t, w))
}

// --- Create handler --------------------------------------------------------

func TestHandlerCreate_Conflict(t *testing.T) {
	q := &fakeQ{usernameExists: func(context.Context, string) (bool, error) { return true, nil }}
	c, w := hCtx(http.MethodPost, "/users", `{"username":"u","password":"longpass1"}`, nil)
	hnd(q).Create(c)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, "CONFLICT", errCode(t, w))
}

func TestHandlerCreate_InternalError(t *testing.T) {
	q := &fakeQ{usernameExists: func(context.Context, string) (bool, error) { return false, errBoom }}
	c, w := hCtx(http.MethodPost, "/users", `{"username":"u","password":"longpass1"}`, nil)
	hnd(q).Create(c)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "INTERNAL", errCode(t, w))
}

func TestHandlerCreate_Success(t *testing.T) {
	q := &fakeQ{
		usernameExists: func(context.Context, string) (bool, error) { return false, nil },
		createUser: func(context.Context, gendb.CreateUserParams) (gendb.CreateUserRow, error) {
			return gendb.CreateUserRow{ID: 5}, nil
		},
		getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
			return gendb.GetUserByIDRow{ID: 5, Username: "u", IsActive: true}, nil
		},
	}
	c, w := hCtx(http.MethodPost, "/users", `{"username":"u","password":"longpass1"}`, nil)
	hnd(q).Create(c)
	assert.Equal(t, http.StatusCreated, w.Code)
}

// --- Get handler -----------------------------------------------------------

func TestHandlerGet_NotFound(t *testing.T) {
	q := &fakeQ{getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
		return gendb.GetUserByIDRow{}, pgx.ErrNoRows
	}}
	c, w := hCtx(http.MethodGet, "/users/1", "", gin.Params{{Key: "id", Value: "1"}})
	hnd(q).Get(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "NOT_FOUND", errCode(t, w))
}

func TestHandlerGet_InvalidID(t *testing.T) {
	c, w := hCtx(http.MethodGet, "/users/abc", "", gin.Params{{Key: "id", Value: "abc"}})
	hnd(&fakeQ{}).Get(c)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestHandlerGet_Success(t *testing.T) {
	q := &fakeQ{getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
		return gendb.GetUserByIDRow{ID: 1, Username: "u", IsActive: true}, nil
	}}
	c, w := hCtx(http.MethodGet, "/users/1", "", gin.Params{{Key: "id", Value: "1"}})
	hnd(q).Get(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

// --- Update handler --------------------------------------------------------

func TestHandlerUpdate_NotFound(t *testing.T) {
	q := &fakeQ{updateUser: func(context.Context, gendb.UpdateUserParams) (gendb.UpdateUserRow, error) {
		return gendb.UpdateUserRow{}, pgx.ErrNoRows
	}}
	c, w := hCtx(http.MethodPut, "/users/1", `{"display_name":"X"}`, gin.Params{{Key: "id", Value: "1"}})
	hnd(q).Update(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandlerUpdate_InternalError(t *testing.T) {
	q := &fakeQ{updateUser: func(context.Context, gendb.UpdateUserParams) (gendb.UpdateUserRow, error) {
		return gendb.UpdateUserRow{}, errBoom
	}}
	c, w := hCtx(http.MethodPut, "/users/1", `{"display_name":"X"}`, gin.Params{{Key: "id", Value: "1"}})
	hnd(q).Update(c)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandlerUpdate_Success(t *testing.T) {
	q := &fakeQ{
		updateUser: func(context.Context, gendb.UpdateUserParams) (gendb.UpdateUserRow, error) {
			return gendb.UpdateUserRow{ID: 1}, nil
		},
		getByID: func(context.Context, int64) (gendb.GetUserByIDRow, error) {
			return gendb.GetUserByIDRow{ID: 1, Username: "u", IsActive: true}, nil
		},
	}
	c, w := hCtx(http.MethodPut, "/users/1", `{"display_name":"X"}`, gin.Params{{Key: "id", Value: "1"}})
	hnd(q).Update(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

// --- Delete handler --------------------------------------------------------

func TestHandlerDelete_NotFound(t *testing.T) {
	q := &fakeQ{softDelete: func(context.Context, gendb.SoftDeleteUserParams) (int64, error) {
		return 0, nil
	}}
	c, w := hCtx(http.MethodDelete, "/users/1", "", gin.Params{{Key: "id", Value: "1"}})
	hnd(q).Delete(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandlerDelete_InvalidID(t *testing.T) {
	c, w := hCtx(http.MethodDelete, "/users/x", "", gin.Params{{Key: "id", Value: "x"}})
	hnd(&fakeQ{}).Delete(c)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestHandlerDelete_Success(t *testing.T) {
	q := &fakeQ{softDelete: func(context.Context, gendb.SoftDeleteUserParams) (int64, error) {
		return 1, nil
	}}
	c, w := hCtx(http.MethodDelete, "/users/1", "", gin.Params{{Key: "id", Value: "1"}})
	hnd(q).Delete(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

// --- ResetPassword handler -------------------------------------------------

func TestHandlerResetPassword_NotFound(t *testing.T) {
	q := &fakeQ{updatePw: func(context.Context, gendb.UpdateUserPasswordParams) (int64, error) {
		return 0, nil
	}}
	c, w := hCtx(http.MethodPut, "/users/1/password", `{"new_password":"longpass1"}`, gin.Params{{Key: "id", Value: "1"}})
	hnd(q).ResetPassword(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandlerResetPassword_BadBody(t *testing.T) {
	c, w := hCtx(http.MethodPut, "/users/1/password", `notjson`, gin.Params{{Key: "id", Value: "1"}})
	hnd(&fakeQ{}).ResetPassword(c)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestHandlerResetPassword_Success(t *testing.T) {
	q := &fakeQ{updatePw: func(context.Context, gendb.UpdateUserPasswordParams) (int64, error) {
		return 1, nil
	}}
	c, w := hCtx(http.MethodPut, "/users/1/password", `{"new_password":"longpass1"}`, gin.Params{{Key: "id", Value: "1"}})
	hnd(q).ResetPassword(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

// --- RegisterRoutes --------------------------------------------------------

func TestHandlerRegisterRoutes(t *testing.T) {
	r := gin.New()
	hnd(&fakeQ{}).RegisterRoutes(r)
	got := make(map[string]bool)
	for _, rt := range r.Routes() {
		got[rt.Method+" "+rt.Path] = true
	}
	for _, want := range []string{
		"GET /users", "POST /users", "GET /users/:id",
		"PUT /users/:id", "DELETE /users/:id", "PUT /users/:id/password",
	} {
		assert.True(t, got[want], want)
	}
}
