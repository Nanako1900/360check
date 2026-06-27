package rbac

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

func newCtx() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	return c, w
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

func TestFailRBACErr(t *testing.T) {
	cases := []struct {
		err  error
		code int
		body string
	}{
		{ErrRoleNotFound, http.StatusNotFound, "NOT_FOUND"},
		{ErrUserNotFound, http.StatusNotFound, "NOT_FOUND"},
		{ErrRoleCodeTaken, http.StatusConflict, "CONFLICT"},
		{errors.New("boom"), http.StatusInternalServerError, "INTERNAL"},
	}
	for _, tc := range cases {
		c, w := newCtx()
		failRBACErr(c, tc.err)
		assert.Equal(t, tc.code, w.Code)
		assert.Equal(t, tc.body, errCode(t, w))
	}
}

func TestParsePathID(t *testing.T) {
	c, w := newCtx()
	c.Params = gin.Params{{Key: "id", Value: "notint"}}
	_, ok := parsePathID(c)
	assert.False(t, ok)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

	c2, _ := newCtx()
	c2.Params = gin.Params{{Key: "id", Value: "7"}}
	id, ok := parsePathID(c2)
	assert.True(t, ok)
	assert.Equal(t, int64(7), id)
}

func TestCurrentUserID(t *testing.T) {
	c, _ := newCtx()
	assert.Equal(t, int64(0), currentUserID(c))
	c.Set(CtxUserID, int64(9))
	assert.Equal(t, int64(9), currentUserID(c))
}

func bodyCtx(method, body string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	return c, w
}

// Validation branches return before touching the service, so nil is safe.
func TestRBACHandler_Validation(t *testing.T) {
	h := NewHandler(nil)
	id3 := gin.Params{{Key: "id", Value: "3"}}

	// CreateRole: bad body / empty code+name
	for _, b := range []string{`x`, `{"code":"","name":""}`} {
		c, w := bodyCtx(http.MethodPost, b, nil)
		h.CreateRole(c)
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code, b)
	}
	// UpdateRole / SetRolePermissions / SetUserRoles: bad body (valid id)
	for _, fn := range []func(*gin.Context){h.UpdateRole, h.SetRolePermissions, h.SetUserRoles} {
		c, w := bodyCtx(http.MethodPut, `notjson`, id3)
		fn(c)
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	}
	// invalid id path
	c, w := bodyCtx(http.MethodPut, `{}`, gin.Params{{Key: "id", Value: "abc"}})
	h.UpdateRole(c)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}
