package rbac

import (
	"context"
	"fmt"
	"strings"

	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/nnkglobal/c5-backend/internal/platform/db"
)

// pgxAdapter is a minimal casbin persist.Adapter over the casbin_rule table
// (columns ptype, v0..v5), using the project's pgx pool — no extra DB driver.
type pgxAdapter struct {
	pool *db.Pool
}

func newAdapter(pool *db.Pool) *pgxAdapter { return &pgxAdapter{pool: pool} }

// pad6 returns rule padded to exactly 6 values with empty strings.
func pad6(rule []string) [6]string {
	var v [6]string
	for i := 0; i < len(rule) && i < 6; i++ {
		v[i] = rule[i]
	}
	return v
}

// LoadPolicy loads all rules from casbin_rule into the model.
func (a *pgxAdapter) LoadPolicy(m model.Model) error {
	ctx := context.Background()
	rows, err := a.pool.Query(ctx, `SELECT ptype, v0, v1, v2, v3, v4, v5 FROM casbin_rule ORDER BY id`)
	if err != nil {
		return fmt.Errorf("rbac: load policy: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ptype string
		var v [6]string
		if err := rows.Scan(&ptype, &v[0], &v[1], &v[2], &v[3], &v[4], &v[5]); err != nil {
			return fmt.Errorf("rbac: scan policy: %w", err)
		}
		rule := []string{ptype}
		for _, x := range v {
			if x == "" {
				break
			}
			rule = append(rule, x)
		}
		if err := persist.LoadPolicyArray(rule, m); err != nil {
			return fmt.Errorf("rbac: load policy line: %w", err)
		}
	}
	return rows.Err()
}

// SavePolicy rewrites the whole policy set inside a transaction.
func (a *pgxAdapter) SavePolicy(m model.Model) error {
	ctx := context.Background()
	return a.pool.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM casbin_rule`); err != nil {
			return fmt.Errorf("rbac: clear policy: %w", err)
		}
		for _, sec := range []string{"p", "g"} {
			for ptype, ast := range m[sec] {
				for _, rule := range ast.Policy {
					if err := insertRule(ctx, tx, ptype, rule); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
}

// AddPolicy inserts a single rule (idempotent against the unique index).
func (a *pgxAdapter) AddPolicy(_ string, ptype string, rule []string) error {
	return insertRule(context.Background(), a.pool, ptype, rule)
}

// RemovePolicy deletes a single exact rule.
func (a *pgxAdapter) RemovePolicy(_ string, ptype string, rule []string) error {
	v := pad6(rule)
	_, err := a.pool.Exec(context.Background(),
		`DELETE FROM casbin_rule WHERE ptype=$1 AND v0=$2 AND v1=$3 AND v2=$4 AND v3=$5 AND v4=$6 AND v5=$7`,
		ptype, v[0], v[1], v[2], v[3], v[4], v[5])
	if err != nil {
		return fmt.Errorf("rbac: remove policy: %w", err)
	}
	return nil
}

// RemoveFilteredPolicy deletes rules matching the non-empty field values starting
// at fieldIndex (e.g. RemoveFilteredPolicy("p","p",0,"admin") removes all of role admin).
func (a *pgxAdapter) RemoveFilteredPolicy(_ string, ptype string, fieldIndex int, fieldValues ...string) error {
	conds := []string{"ptype = $1"}
	args := []any{ptype}
	n := 2
	for i, fv := range fieldValues {
		if fv == "" {
			continue
		}
		conds = append(conds, fmt.Sprintf("v%d = $%d", fieldIndex+i, n))
		args = append(args, fv)
		n++
	}
	q := `DELETE FROM casbin_rule WHERE ` + strings.Join(conds, " AND ")
	if _, err := a.pool.Exec(context.Background(), q, args...); err != nil {
		return fmt.Errorf("rbac: remove filtered policy: %w", err)
	}
	return nil
}

// execer is the subset of pgx used to insert a rule (Pool or Tx).
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func insertRule(ctx context.Context, e execer, ptype string, rule []string) error {
	v := pad6(rule)
	_, err := e.Exec(ctx,
		`INSERT INTO casbin_rule (ptype,v0,v1,v2,v3,v4,v5) VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (ptype,v0,v1,v2,v3,v4,v5) DO NOTHING`,
		ptype, v[0], v[1], v[2], v[3], v[4], v[5])
	if err != nil {
		return fmt.Errorf("rbac: insert rule: %w", err)
	}
	return nil
}
