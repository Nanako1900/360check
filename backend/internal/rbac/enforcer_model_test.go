package rbac

import (
	"testing"

	"github.com/casbin/casbin/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newInMemoryEnforcer builds an enforcer with the C5 model and seed-equivalent
// policies in memory (no DB), to validate the matcher (keyMatch2 + act wildcard).
func newInMemoryEnforcer(t *testing.T) *casbin.Enforcer {
	t.Helper()
	m, err := newModel()
	require.NoError(t, err)
	e, err := casbin.NewEnforcer(m)
	require.NoError(t, err)

	for _, p := range [][3]string{
		{"admin", "/api/v1/*", "*"},
		{"inspector", "/api/v1/problems", "GET"},
		{"inspector", "/api/v1/problems", "POST"},
		{"inspector", "/api/v1/problems/*", "PUT"},
		{"inspector", "/api/v1/inspections/start", "POST"},
		{"inspector", "/api/v1/inspections/*/finish", "POST"},
		{"inspector", "/api/v1/sync/batch", "POST"},
	} {
		_, err := e.AddPolicy(p[0], p[1], p[2])
		require.NoError(t, err)
	}
	_, err = e.AddGroupingPolicy("alice", "admin")
	require.NoError(t, err)
	_, err = e.AddGroupingPolicy("bob", "inspector")
	require.NoError(t, err)
	return e
}

func TestEnforceMatrix(t *testing.T) {
	e := newInMemoryEnforcer(t)
	cases := []struct {
		name          string
		sub, obj, act string
		want          bool
	}{
		{"admin wildcard read", "alice", "/api/v1/users", "GET", true},
		{"admin wildcard delete", "alice", "/api/v1/projects/9", "DELETE", true},
		{"admin act wildcard", "alice", "/api/v1/anything/here", "PATCH", true},
		{"inspector problems read", "bob", "/api/v1/problems", "GET", true},
		{"inspector problems create", "bob", "/api/v1/problems", "POST", true},
		{"inspector problem update keyMatch2", "bob", "/api/v1/problems/5", "PUT", true},
		{"inspector start inspection", "bob", "/api/v1/inspections/start", "POST", true},
		{"inspector finish inspection keyMatch2", "bob", "/api/v1/inspections/12/finish", "POST", true},
		{"inspector sync", "bob", "/api/v1/sync/batch", "POST", true},
		{"inspector denied users", "bob", "/api/v1/users", "GET", false},
		{"inspector denied delete problem", "bob", "/api/v1/problems/5", "DELETE", false},
		{"inspector denied create project", "bob", "/api/v1/projects", "POST", false},
		{"unknown subject denied", "carol", "/api/v1/problems", "GET", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := e.Enforce(tc.sub, tc.obj, tc.act)
			require.NoError(t, err)
			assert.Equal(t, tc.want, ok)
		})
	}
}
