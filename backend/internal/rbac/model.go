// Package rbac is the casbin enforcement layer: the RBAC model, a pgx-backed
// adapter over the casbin_rule table, an enforcer with optional redis-watcher for
// multi-replica policy consistency, and the gin auth/casbin middleware.
//
// casbin_rule is the enforced source of truth; the roles/permissions tables are
// human-readable UI mirrors. The casbin subject is the username; g-rules bind
// username -> role code, p-rules bind role code -> (path, method).
package rbac

import (
	"fmt"

	"github.com/casbin/casbin/v2/model"
)

// modelText matches the assumptions baked into 000002_seed.up.sql:
//
//	r = sub, obj, act ; p = sub, obj, act ; g = _, _
//	m = g(r.sub, p.sub) && keyMatch2(r.obj, p.obj) && (r.act == p.act || p.act == "*")
//
// keyMatch2 lets '/api/v1/*' and '/api/v1/users/:id' patterns match request paths.
const modelText = `
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && keyMatch2(r.obj, p.obj) && (r.act == p.act || p.act == "*")
`

func newModel() (model.Model, error) {
	m, err := model.NewModelFromString(modelText)
	if err != nil {
		return nil, fmt.Errorf("rbac: parse model: %w", err)
	}
	return m, nil
}
