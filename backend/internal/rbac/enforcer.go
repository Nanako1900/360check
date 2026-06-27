package rbac

import (
	"fmt"

	"github.com/casbin/casbin/v2"
	rediswatcher "github.com/casbin/redis-watcher/v2"
	goredis "github.com/redis/go-redis/v9"

	"github.com/nnkglobal/c5-backend/internal/platform/db"
)

// Enforcer wraps a thread-safe casbin SyncedEnforcer with the C5 model + pgx
// adapter + optional redis-watcher (multi-replica policy reload).
type Enforcer struct {
	e *casbin.SyncedEnforcer
}

// NewEnforcer builds the enforcer and loads policy from casbin_rule. When
// redisAddr is non-empty it attaches a redis-watcher so policy edits on one
// replica reload the others.
func NewEnforcer(pool *db.Pool, redisAddr, redisPassword string, redisDB int) (*Enforcer, error) {
	m, err := newModel()
	if err != nil {
		return nil, err
	}
	e, err := casbin.NewSyncedEnforcer(m, newAdapter(pool))
	if err != nil {
		return nil, fmt.Errorf("rbac: new enforcer: %w", err)
	}
	if err := e.LoadPolicy(); err != nil {
		return nil, fmt.Errorf("rbac: load policy: %w", err)
	}
	en := &Enforcer{e: e}
	if redisAddr != "" {
		if err := en.attachWatcher(redisAddr, redisPassword, redisDB); err != nil {
			return nil, err
		}
	}
	return en, nil
}

func (en *Enforcer) attachWatcher(addr, password string, dbIdx int) error {
	w, err := rediswatcher.NewWatcher(addr, rediswatcher.WatcherOptions{
		Options:    goredis.Options{Addr: addr, Password: password, DB: dbIdx},
		Channel:    "/casbin",
		IgnoreSelf: true,
	})
	if err != nil {
		return fmt.Errorf("rbac: new watcher: %w", err)
	}
	if err := en.e.SetWatcher(w); err != nil {
		return fmt.Errorf("rbac: set watcher: %w", err)
	}
	if err := w.SetUpdateCallback(func(string) { _ = en.e.LoadPolicy() }); err != nil {
		return fmt.Errorf("rbac: set watcher callback: %w", err)
	}
	return nil
}

// Enforce checks (sub=username, obj=path, act=method).
func (en *Enforcer) Enforce(sub, obj, act string) (bool, error) {
	ok, err := en.e.Enforce(sub, obj, act)
	if err != nil {
		return false, fmt.Errorf("rbac: enforce: %w", err)
	}
	return ok, nil
}

// RolesForUser returns the role codes granted to a user (g-rules).
func (en *Enforcer) RolesForUser(username string) ([]string, error) {
	roles, err := en.e.GetRolesForUser(username)
	if err != nil {
		return nil, fmt.Errorf("rbac: roles for user: %w", err)
	}
	return roles, nil
}

// SetRolesForUser replaces a user's roles (g-rules); persists + notifies watcher.
func (en *Enforcer) SetRolesForUser(username string, roleCodes []string) error {
	if _, err := en.e.DeleteRolesForUser(username); err != nil {
		return fmt.Errorf("rbac: clear roles: %w", err)
	}
	for _, rc := range roleCodes {
		if _, err := en.e.AddRoleForUser(username, rc); err != nil {
			return fmt.Errorf("rbac: add role: %w", err)
		}
	}
	return nil
}

// PermissionsForRole returns each [roleCode, obj, act] p-rule for the role.
func (en *Enforcer) PermissionsForRole(roleCode string) ([][]string, error) {
	policy, err := en.e.GetFilteredPolicy(0, roleCode)
	if err != nil {
		return nil, fmt.Errorf("rbac: perms for role: %w", err)
	}
	return policy, nil
}

// SetPermissionsForRole replaces a role's p-rules with (obj, act) pairs.
func (en *Enforcer) SetPermissionsForRole(roleCode string, objActs [][2]string) error {
	if _, err := en.e.RemoveFilteredPolicy(0, roleCode); err != nil {
		return fmt.Errorf("rbac: clear role perms: %w", err)
	}
	// Add one rule at a time: the pgx adapter implements the single-rule
	// persist.Adapter, not the batch persist.BatchAdapter.
	for _, oa := range objActs {
		if _, err := en.e.AddPolicy(roleCode, oa[0], oa[1]); err != nil {
			return fmt.Errorf("rbac: add role perm: %w", err)
		}
	}
	return nil
}

// RemoveRole removes all p-rules and g-rules referencing a role code.
func (en *Enforcer) RemoveRole(roleCode string) error {
	if _, err := en.e.RemoveFilteredPolicy(0, roleCode); err != nil {
		return fmt.Errorf("rbac: remove role p-rules: %w", err)
	}
	if _, err := en.e.RemoveFilteredGroupingPolicy(1, roleCode); err != nil {
		return fmt.Errorf("rbac: remove role g-rules: %w", err)
	}
	return nil
}

// ReloadPolicy re-reads casbin_rule (used by tests and the watcher callback).
func (en *Enforcer) ReloadPolicy() error {
	if err := en.e.LoadPolicy(); err != nil {
		return fmt.Errorf("rbac: reload policy: %w", err)
	}
	return nil
}
