package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func init() { gin.SetMode(gin.TestMode) }

// postCtx builds a POST gin context with a JSON body. The handlers under test
// validate the request BEFORE touching the service, so a nil service is safe.
func postCtx(body string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func TestLogin_Validation(t *testing.T) {
	h := NewHandler(nil)
	for _, body := range []string{`not json`, `{"username":"","password":"x"}`, `{"username":"u","password":""}`} {
		c, w := postCtx(body)
		h.Login(c)
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code, body)
	}
}

func TestRefresh_Validation(t *testing.T) {
	h := NewHandler(nil)
	for _, body := range []string{`not json`, `{"refresh_token":""}`} {
		c, w := postCtx(body)
		h.Refresh(c)
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code, body)
	}
}

func TestChangePassword_Validation(t *testing.T) {
	h := NewHandler(nil)
	// bad body
	c, w := postCtx(`not json`)
	h.ChangePassword(c)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	// too-short new password (returns before service)
	c, w = postCtx(`{"old_password":"whatever","new_password":"short"}`)
	h.ChangePassword(c)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestValidatePassword(t *testing.T) {
	assert.Nil(t, validatePassword("longenough123"))
	assert.NotNil(t, validatePassword("short"))
	assert.NotNil(t, validatePassword(strings.Repeat("a", 73))) // >72 bytes
}
