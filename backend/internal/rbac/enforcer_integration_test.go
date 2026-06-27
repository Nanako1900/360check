//go:build integration

package rbac

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	migrations "github.com/nnkglobal/c5-backend/db/migrations"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
)

func newRbacPool(t *testing.T) *db.Pool {
	t.Helper()
	ctx := context.Background()
	pg, err := tcpostgres.Run(ctx, "postgis/postgis:16-3.4",
		tcpostgres.WithDatabase("c5"), tcpostgres.WithUsername("c5"), tcpostgres.WithPassword("c5"),
		tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, db.RunMigrations(dsn, migrations.FS))
	pool, err := db.New(ctx, dsn, 5, 1, 30*time.Second)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

// TestEnforcer_SeededPolicies verifies the pgx adapter loads the 000002 seed
// policies and enforces them (admin all, inspector restricted).
func TestEnforcer_SeededPolicies(t *testing.T) {
	pool := newRbacPool(t)
	en, err := NewEnforcer(pool, "", "", 0) // no watcher
	require.NoError(t, err)

	// Bootstrap admin subject ('admin') is bound to the admin role via g-rule.
	ok, err := en.Enforce("admin", "/api/v1/users", "GET")
	require.NoError(t, err)
	assert.True(t, ok, "admin should access /users")

	// inspector role is restricted (no /users), but allowed problems.
	ok, _ = en.Enforce("inspector", "/api/v1/users", "GET")
	assert.False(t, ok)
	ok, _ = en.Enforce("inspector", "/api/v1/problems", "GET")
	assert.True(t, ok)
}

// TestEnforcer_ReloadAfterChange proves the policy reload path the redis-watcher
// triggers: a change persisted by enforcer A is visible to enforcer B after a
// reload (same DB).
func TestEnforcer_ReloadAfterChange(t *testing.T) {
	pool := newRbacPool(t)
	enA, err := NewEnforcer(pool, "", "", 0)
	require.NoError(t, err)
	enB, err := NewEnforcer(pool, "", "", 0)
	require.NoError(t, err)

	// Initially neither grants inspector DELETE on /api/v1/problems/*.
	ok, _ := enB.Enforce("inspector", "/api/v1/problems/5", "DELETE")
	require.False(t, ok)

	// A grants it (persists to casbin_rule + A's in-memory model).
	require.NoError(t, enA.SetPermissionsForRole("inspector_test", [][2]string{{"/api/v1/problems/*", "DELETE"}}))
	require.NoError(t, enA.SetRolesForUser("tester", []string{"inspector_test"}))
	okA, _ := enA.Enforce("tester", "/api/v1/problems/9", "DELETE")
	assert.True(t, okA, "enforcer A reflects its own change immediately")

	// B does not see it until it reloads (the watcher would auto-trigger this).
	okB, _ := enB.Enforce("tester", "/api/v1/problems/9", "DELETE")
	assert.False(t, okB, "enforcer B stale before reload")

	require.NoError(t, enB.ReloadPolicy())
	okB, _ = enB.Enforce("tester", "/api/v1/problems/9", "DELETE")
	assert.True(t, okB, "enforcer B reflects the change after reload")
}

// TestPgxAdapter_Persistence exercises the adapter's Load/Add/Remove/RemoveFiltered
// and full SavePolicy round-trip directly against casbin_rule.
func TestPgxAdapter_Persistence(t *testing.T) {
	pool := newRbacPool(t)
	a := newAdapter(pool)

	m, err := newModel()
	require.NoError(t, err)
	require.NoError(t, a.LoadPolicy(m)) // loads seed policies

	require.NoError(t, a.AddPolicy("p", "p", []string{"tester", "/api/v1/x", "GET"}))
	require.NoError(t, a.RemovePolicy("p", "p", []string{"tester", "/api/v1/x", "GET"}))

	require.NoError(t, a.AddPolicy("p", "p", []string{"tester", "/api/v1/y", "GET"}))
	require.NoError(t, a.AddPolicy("p", "p", []string{"tester", "/api/v1/z", "GET"}))
	require.NoError(t, a.RemoveFilteredPolicy("p", "p", 0, "tester"))

	// SavePolicy fully rewrites casbin_rule from a fresh model.
	m2, err := newModel()
	require.NoError(t, err)
	m2.AddPolicy("p", "p", []string{"r1", "/api/v1/saved", "GET"})
	m2.AddPolicy("g", "g", []string{"u1", "r1"})
	require.NoError(t, a.SavePolicy(m2))

	m3, err := newModel()
	require.NoError(t, err)
	require.NoError(t, a.LoadPolicy(m3))
	hasP, err := m3.HasPolicy("p", "p", []string{"r1", "/api/v1/saved", "GET"})
	require.NoError(t, err)
	assert.True(t, hasP)
	hasG, err := m3.HasPolicy("g", "g", []string{"u1", "r1"})
	require.NoError(t, err)
	assert.True(t, hasG)
}
