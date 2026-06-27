package rbac

import (
	"fmt"

	"github.com/casbin/casbin/v2"
)

// NewInMemoryEnforcer builds an Enforcer backed by an in-memory casbin
// SyncedEnforcer (the C5 model, no DB adapter, no watcher). Policy rules are
// added via the returned enforcer's Set* methods. It exists so service-layer
// unit tests in this and dependent packages (auth) can construct a *Service
// without a live database; production wiring always uses NewEnforcer.
func NewInMemoryEnforcer() (*Enforcer, error) {
	m, err := newModel()
	if err != nil {
		return nil, err
	}
	e, err := casbin.NewSyncedEnforcer(m)
	if err != nil {
		return nil, fmt.Errorf("rbac: new in-memory enforcer: %w", err)
	}
	return &Enforcer{e: e}, nil
}
