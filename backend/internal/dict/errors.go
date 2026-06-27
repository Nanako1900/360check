package dict

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// isUniqueViolation reports whether err is a Postgres unique-constraint violation
// (SQLSTATE 23505) — used to map a duplicate dict_type code / dict_item code /
// app_config active-row race onto the right sentinel error.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

// isForeignKeyViolation reports whether err is a Postgres FK violation (SQLSTATE
// 23503) — e.g. a problems.* row still RESTRICT-references a dict_item being
// hard-deleted, in which case the caller must retire (is_active=false) instead.
func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23503"
	}
	return false
}
