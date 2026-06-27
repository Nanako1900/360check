//go:build integration

package server

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
)

// ptr helpers for optional request fields.
func sptr(s string) *string { return &s }
func bptr(b bool) *bool     { return &b }

func TestUserCRUDAndCasbin_Integration(t *testing.T) {
	srv := newAuthTestServer(t)

	adminAccess, _, code := login(t, srv, "admin", bootstrapPassword)
	require.Equal(t, http.StatusOK, code)

	// Admin can list users (casbin allows admin wildcard).
	w, _ := doJSON(t, srv, http.MethodGet, "/api/v1/users", adminAccess, nil)
	require.Equal(t, http.StatusOK, w.Code)

	// Create an inspector user.
	w, env := doJSON(t, srv, http.MethodPost, "/api/v1/users", adminAccess, oapi.UserCreate{
		Username: "inspector1", Password: "Inspect@123", DisplayName: sptr("巡查员一号"),
	})
	require.Equal(t, http.StatusCreated, w.Code)
	newID := int64(env["data"].(map[string]any)["id"].(float64))
	assert.Positive(t, newID)

	// Duplicate username -> 409 CONFLICT.
	w, env = doJSON(t, srv, http.MethodPost, "/api/v1/users", adminAccess, oapi.UserCreate{
		Username: "inspector1", Password: "Inspect@123",
	})
	require.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, "CONFLICT", env["error"].(map[string]any)["code"])

	// Find the inspector role id and assign it to the new user.
	w, env = doJSON(t, srv, http.MethodGet, "/api/v1/roles", adminAccess, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var inspectorRoleID int64
	for _, r := range env["data"].([]any) {
		rm := r.(map[string]any)
		if rm["code"] == "inspector" {
			inspectorRoleID = int64(rm["id"].(float64))
		}
	}
	require.Positive(t, inspectorRoleID)

	w, env = doJSON(t, srv, http.MethodPut, "/api/v1/users/"+itoa(newID)+"/roles", adminAccess,
		oapi.SetUserRolesRequest{RoleIds: []int64{inspectorRoleID}})
	require.Equal(t, http.StatusOK, w.Code)
	roles := env["data"].([]any)
	require.Len(t, roles, 1)
	assert.Equal(t, "inspector", roles[0].(map[string]any)["code"])

	// Inspector logs in and is FORBIDDEN from /users (casbin denies).
	inspAccess, _, code := login(t, srv, "inspector1", "Inspect@123")
	require.Equal(t, http.StatusOK, code)
	w, env = doJSON(t, srv, http.MethodGet, "/api/v1/users", inspAccess, nil)
	require.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "FORBIDDEN", env["error"].(map[string]any)["code"])

	// Inspector /auth/me: role inspector, no admin-only permission codes.
	w, env = doJSON(t, srv, http.MethodGet, "/api/v1/auth/me", inspAccess, nil)
	require.Equal(t, http.StatusOK, w.Code)
	data := env["data"].(map[string]any)
	assert.NotContains(t, toStrings(data["permissions"].([]any)), "user:read")

	// Update the user (partial).
	w, env = doJSON(t, srv, http.MethodPut, "/api/v1/users/"+itoa(newID), adminAccess,
		oapi.UserUpdate{DisplayName: sptr("改名后"), IsActive: bptr(true)})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "改名后", env["data"].(map[string]any)["display_name"])

	// Admin resets the inspector's password; the new password logs in.
	w, _ = doJSON(t, srv, http.MethodPut, "/api/v1/users/"+itoa(newID)+"/password", adminAccess,
		oapi.ResetPasswordRequest{NewPassword: "Reset@9999"})
	require.Equal(t, http.StatusOK, w.Code)
	_, _, code = login(t, srv, "inspector1", "Reset@9999")
	assert.Equal(t, http.StatusOK, code)

	// GetUserRoles for the user.
	w, env = doJSON(t, srv, http.MethodGet, "/api/v1/users/"+itoa(newID)+"/roles", adminAccess, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, env["data"].([]any), 1)

	// 404 + 422 paths.
	w, _ = doJSON(t, srv, http.MethodGet, "/api/v1/users/99999", adminAccess, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w, _ = doJSON(t, srv, http.MethodGet, "/api/v1/users/not-an-int", adminAccess, nil)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	w, _ = doJSON(t, srv, http.MethodPost, "/api/v1/users", adminAccess,
		oapi.UserCreate{Username: "x", Password: "short"})
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

	// Soft-delete the user.
	w, _ = doJSON(t, srv, http.MethodDelete, "/api/v1/users/"+itoa(newID), adminAccess, nil)
	require.Equal(t, http.StatusOK, w.Code)
	w, _ = doJSON(t, srv, http.MethodGet, "/api/v1/users/"+itoa(newID), adminAccess, nil)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestRolePermissionsCRUD_Integration(t *testing.T) {
	srv := newAuthTestServer(t)
	adminAccess, _, code := login(t, srv, "admin", bootstrapPassword)
	require.Equal(t, http.StatusOK, code)

	// System role (admin) cannot be deleted -> 409.
	w, env := doJSON(t, srv, http.MethodGet, "/api/v1/roles", adminAccess, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var adminRoleID int64
	for _, r := range env["data"].([]any) {
		rm := r.(map[string]any)
		if rm["code"] == "admin" {
			adminRoleID = int64(rm["id"].(float64))
		}
	}
	w, env = doJSON(t, srv, http.MethodDelete, "/api/v1/roles/"+itoa(adminRoleID), adminAccess, nil)
	require.Equal(t, http.StatusConflict, w.Code)

	// Create a custom role and grant it permissions.
	w, env = doJSON(t, srv, http.MethodPost, "/api/v1/roles", adminAccess,
		oapi.RoleCreate{Code: "viewer", Name: "只读"})
	require.Equal(t, http.StatusCreated, w.Code)
	viewerID := int64(env["data"].(map[string]any)["id"].(float64))

	// Pick two permission ids from the catalog.
	w, env = doJSON(t, srv, http.MethodGet, "/api/v1/permissions", adminAccess, nil)
	require.Equal(t, http.StatusOK, w.Code)
	perms := env["data"].([]any)
	require.GreaterOrEqual(t, len(perms), 2)
	var pids []int64
	for _, p := range perms {
		if p.(map[string]any)["code"] == "project:read" || p.(map[string]any)["code"] == "problem:read" {
			pids = append(pids, int64(p.(map[string]any)["id"].(float64)))
		}
	}
	require.Len(t, pids, 2)

	w, env = doJSON(t, srv, http.MethodPut, "/api/v1/roles/"+itoa(viewerID)+"/permissions", adminAccess,
		oapi.SetRolePermissionsRequest{PermissionIds: pids})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, env["data"].([]any), 2)

	// Read back the role's permissions.
	w, env = doJSON(t, srv, http.MethodGet, "/api/v1/roles/"+itoa(viewerID)+"/permissions", adminAccess, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, env["data"].([]any), 2)

	// Update the custom role.
	w, env = doJSON(t, srv, http.MethodPut, "/api/v1/roles/"+itoa(viewerID), adminAccess,
		oapi.RoleUpdate{Name: sptr("只读角色")})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "只读角色", env["data"].(map[string]any)["name"])

	// Duplicate role code -> 409.
	w, _ = doJSON(t, srv, http.MethodPost, "/api/v1/roles", adminAccess, oapi.RoleCreate{Code: "viewer", Name: "dup"})
	assert.Equal(t, http.StatusConflict, w.Code)

	// 404 paths for a missing role.
	w, _ = doJSON(t, srv, http.MethodPut, "/api/v1/roles/99999", adminAccess, oapi.RoleUpdate{Name: sptr("x")})
	assert.Equal(t, http.StatusNotFound, w.Code)
	w, _ = doJSON(t, srv, http.MethodGet, "/api/v1/roles/99999/permissions", adminAccess, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Delete the custom (non-system) role succeeds.
	w, _ = doJSON(t, srv, http.MethodDelete, "/api/v1/roles/"+itoa(viewerID), adminAccess, nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
