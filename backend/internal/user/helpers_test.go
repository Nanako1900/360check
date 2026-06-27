package user

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

func TestIsUniqueViolation(t *testing.T) {
	assert.True(t, isUniqueViolation(&pgconn.PgError{Code: "23505"}))
	assert.False(t, isUniqueViolation(&pgconn.PgError{Code: "23503"}))
	assert.False(t, isUniqueViolation(errors.New("plain error")))
	assert.False(t, isUniqueViolation(nil))
}

func errCode(t *testing.T, w *httptest.ResponseRecorder) string {
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

func newCtx() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	return c, w
}

func TestFailGetErr(t *testing.T) {
	cases := []struct {
		err  error
		code int
		body string
	}{
		{ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
		{ErrUsernameTaken, http.StatusConflict, "CONFLICT"},
		{errors.New("boom"), http.StatusInternalServerError, "INTERNAL"},
	}
	for _, tc := range cases {
		c, w := newCtx()
		failGetErr(c, tc.err)
		assert.Equal(t, tc.code, w.Code)
		assert.Equal(t, tc.body, errCode(t, w))
	}
}

func TestPathID(t *testing.T) {
	// invalid id -> 422
	c, w := newCtx()
	c.Params = gin.Params{{Key: "id", Value: "abc"}}
	_, ok := pathID(c)
	assert.False(t, ok)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

	// valid id
	c2, _ := newCtx()
	c2.Params = gin.Params{{Key: "id", Value: "42"}}
	id, ok := pathID(c2)
	assert.True(t, ok)
	assert.Equal(t, int64(42), id)
}

func bodyCtx(method, body string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	return c, w
}

// Handler validation branches return before touching the service, so nil is safe.
func TestUserHandler_Validation(t *testing.T) {
	h := NewHandler(nil)
	for _, b := range []string{`x`, `{"username":"","password":"longpass1"}`, `{"username":"u","password":"short"}`} {
		c, w := bodyCtx(http.MethodPost, b, nil)
		h.Create(c)
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code, b)
	}
	// ResetPassword: too-short password (valid id)
	c, w := bodyCtx(http.MethodPut, `{"new_password":"short"}`, gin.Params{{Key: "id", Value: "5"}})
	h.ResetPassword(c)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	// ResetPassword: invalid id
	c, w = bodyCtx(http.MethodPut, `{"new_password":"longenough1"}`, gin.Params{{Key: "id", Value: "abc"}})
	h.ResetPassword(c)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	// Update: invalid id
	c, w = bodyCtx(http.MethodPut, `{}`, gin.Params{{Key: "id", Value: "abc"}})
	h.Update(c)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}
