package inspection

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// fkViolation classifies a Postgres foreign-key violation (SQLSTATE 23503) on the
// inspections INSERT into the specific sentinel for the offending column, so the
// caller can tell a bad project_id from a bad task_id or inspector_id instead of
// mislabeling all three as "project not found".
//
// It branches on pgErr.ConstraintName, which for the schema's inline FKs is the
// Postgres-generated `inspections_<column>_fkey`. Matching the column substring
// (not the full constraint literal) keeps it robust if the constraint is ever
// renamed while still referencing the same column. Returns (nil, false) when err
// is not a 23503 violation.
func fkViolation(err error) (error, bool) {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		return nil, false
	}
	name := pgErr.ConstraintName
	switch {
	case strings.Contains(name, "project_id"):
		return ErrProjectMissing, true
	case strings.Contains(name, "task_id"):
		return ErrTaskMissing, true
	case strings.Contains(name, "inspector_id"):
		return ErrInspectorMissing, true
	default:
		// Unknown FK on inspections — surface as a generic FK violation so the
		// caller returns a validation error rather than masking it as a 500.
		return ErrForeignKeyViolated, true
	}
}
