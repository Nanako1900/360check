//go:build integration

package server

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/jwt"
)

func TestAuthSecurity_Integration(t *testing.T) {
	srv := newAuthTestServer(t)
	adminAccess, _, code := login(t, srv, "admin", bootstrapPassword)
	require.Equal(t, http.StatusOK, code)

	// Create a user, log in while active (gets a refresh token), then deactivate.
	w, env := doJSON(t, srv, http.MethodPost, "/api/v1/users", adminAccess,
		oapi.UserCreate{Username: "sectest", Password: "Sec@12345"})
	require.Equal(t, http.StatusCreated, w.Code)
	uid := int64(env["data"].(map[string]any)["id"].(float64))

	_, refresh, code := login(t, srv, "sectest", "Sec@12345")
	require.Equal(t, http.StatusOK, code)

	w, _ = doJSON(t, srv, http.MethodPut, "/api/v1/users/"+itoa(uid), adminAccess,
		oapi.UserUpdate{IsActive: bptr(false)})
	require.Equal(t, http.StatusOK, w.Code)

	// Disabled user: login blocked.
	_, _, code = login(t, srv, "sectest", "Sec@12345")
	assert.Equal(t, http.StatusUnauthorized, code, "disabled user login must be blocked")

	// Disabled user: refresh blocked (and the rotated token revoked).
	w, _ = doJSON(t, srv, http.MethodPost, "/api/v1/auth/refresh", "",
		oapi.RefreshRequest{RefreshToken: refresh})
	assert.Equal(t, http.StatusUnauthorized, w.Code, "disabled user refresh must be blocked")

	// TOKEN_EXPIRED through the real Authn middleware: expired token signed with
	// the server's secret/issuer (must match newAuthTestServer).
	expMgr := jwt.NewManager("test-secret", "c5-api", time.Minute)
	expTok, _, err := expMgr.Issue(1, "admin", []string{"admin"}, time.Now().Add(-2*time.Hour))
	require.NoError(t, err)
	w, env = doJSON(t, srv, http.MethodGet, "/api/v1/auth/me", expTok, nil)
	require.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "TOKEN_EXPIRED", env["error"].(map[string]any)["code"])

	// Soft-deleted user with a still-valid access token -> /me 401.
	// The user must hold a role so casbin Authz lets the request reach the /me
	// handler (where the deleted-user lookup yields 401, not a 403 from Authz).
	w, env = doJSON(t, srv, http.MethodGet, "/api/v1/roles", adminAccess, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var inspectorRoleID int64
	for _, r := range env["data"].([]any) {
		if r.(map[string]any)["code"] == "inspector" {
			inspectorRoleID = int64(r.(map[string]any)["id"].(float64))
		}
	}
	require.Positive(t, inspectorRoleID)

	w, env = doJSON(t, srv, http.MethodPost, "/api/v1/users", adminAccess,
		oapi.UserCreate{Username: "delme", Password: "Del@12345"})
	require.Equal(t, http.StatusCreated, w.Code)
	delID := int64(env["data"].(map[string]any)["id"].(float64))
	w, _ = doJSON(t, srv, http.MethodPut, "/api/v1/users/"+itoa(delID)+"/roles", adminAccess,
		oapi.SetUserRolesRequest{RoleIds: []int64{inspectorRoleID}})
	require.Equal(t, http.StatusOK, w.Code)

	delAccess, _, code := login(t, srv, "delme", "Del@12345")
	require.Equal(t, http.StatusOK, code)
	w, _ = doJSON(t, srv, http.MethodDelete, "/api/v1/users/"+itoa(delID), adminAccess, nil)
	require.Equal(t, http.StatusOK, w.Code)
	w, _ = doJSON(t, srv, http.MethodGet, "/api/v1/auth/me", delAccess, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code, "deleted user's token must be rejected at /me")
}
