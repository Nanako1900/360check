package problem

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// Sentinel errors mapped to envelope codes by the handler.
var (
	// ErrNotFound — problem id missing or soft-deleted.
	ErrNotFound = errors.New("problem not found")
	// ErrProjectMissing — referenced project absent (FK RESTRICT would fire).
	ErrProjectMissing = errors.New("project not found")
	// ErrInspectorMissing — referenced inspector absent (FK RESTRICT would fire).
	ErrInspectorMissing = errors.New("inspector not found")
	// ErrInvalidLogAction — POST /logs carried an action other than COMMENT/REASSIGN.
	ErrInvalidLogAction = errors.New("log action must be COMMENT or REASSIGN")
)

// isForeignKeyViolation reports whether err is a Postgres foreign-key violation
// (SQLSTATE 23503) — a missing project_id / inspector_id (FK RESTRICT) on create
// maps to a validation error rather than a generic 500. The dict soft FKs are
// also RESTRICT, but historical tolerance means we never reject a referenced
// dict_item here (retired items keep their id and pass the FK).
func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23503"
	}
	return false
}
