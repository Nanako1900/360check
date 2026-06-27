//go:build integration

package server

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
)

// makeInspector creates an active user and grants it the inspector role; returns
// its access token. (Casbin subject = username; inspector policy from 000002.)
func makeInspector(t *testing.T, srv http.Handler, adminAccess, username, password string) string {
	t.Helper()
	w, env := doJSON(t, srv, http.MethodPost, "/api/v1/users", adminAccess,
		oapi.UserCreate{Username: username, Password: password})
	require.Equal(t, http.StatusCreated, w.Code)
	uid := int64(env["data"].(map[string]any)["id"].(float64))

	w, env = doJSON(t, srv, http.MethodGet, "/api/v1/roles", adminAccess, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var inspectorRoleID int64
	for _, r := range env["data"].([]any) {
		if r.(map[string]any)["code"] == "inspector" {
			inspectorRoleID = int64(r.(map[string]any)["id"].(float64))
		}
	}
	require.Positive(t, inspectorRoleID)
	w, _ = doJSON(t, srv, http.MethodPut, "/api/v1/users/"+itoa(uid)+"/roles", adminAccess,
		oapi.SetUserRolesRequest{RoleIds: []int64{inspectorRoleID}})
	require.Equal(t, http.StatusOK, w.Code)

	access, _, code := login(t, srv, username, password)
	require.Equal(t, http.StatusOK, code)
	return access
}

func TestP3HTTP_ProjectAndInspection_Integration(t *testing.T) {
	srv := newAuthTestServer(t)
	adminAccess, _, code := login(t, srv, "admin", bootstrapPassword)
	require.Equal(t, http.StatusOK, code)

	// Admin creates a project.
	w, env := doJSON(t, srv, http.MethodPost, "/api/v1/projects", adminAccess,
		oapi.ProjectCreate{Code: "P-HTTP", Name: "http test"})
	require.Equal(t, http.StatusCreated, w.Code)
	pid := int64(env["data"].(map[string]any)["id"].(float64))
	assert.Positive(t, pid)

	// Admin lists projects.
	w, _ = doJSON(t, srv, http.MethodGet, "/api/v1/projects", adminAccess, nil)
	require.Equal(t, http.StatusOK, w.Code)

	inspAccess := makeInspector(t, srv, adminAccess, "fieldworker", "Field@12345")

	// Inspector can read projects (casbin GET) but cannot create them (admin-only).
	w, _ = doJSON(t, srv, http.MethodGet, "/api/v1/projects", inspAccess, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	w, env = doJSON(t, srv, http.MethodPost, "/api/v1/projects", inspAccess,
		oapi.ProjectCreate{Code: "P-NO", Name: "nope"})
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "FORBIDDEN", env["error"].(map[string]any)["code"])

	// Inspector starts an inspection on the project.
	w, env = doJSON(t, srv, http.MethodPost, "/api/v1/inspections/start", inspAccess,
		oapi.InspectionStartRequest{ProjectId: pid, StartedAt: time.Now().UTC()})
	require.Equal(t, http.StatusCreated, w.Code)
	iid := int64(env["data"].(map[string]any)["id"].(float64))

	// Inspector finishes it (zero trajectory points -> route null, mileage 0).
	w, env = doJSON(t, srv, http.MethodPost, "/api/v1/inspections/"+itoa(iid)+"/finish", inspAccess,
		oapi.InspectionFinishRequest{})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "FINISHED", env["data"].(map[string]any)["status"])

	// Inspector reads the trajectory (route + points, WGS84).
	w, _ = doJSON(t, srv, http.MethodGet, "/api/v1/inspections/"+itoa(iid)+"/trajectory", inspAccess, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Inspector lists inspections.
	w, _ = doJSON(t, srv, http.MethodGet, "/api/v1/inspections?project_id="+itoa(pid), inspAccess, nil)
	assert.Equal(t, http.StatusOK, w.Code)
}
