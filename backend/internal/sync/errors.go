package sync

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// Per-item reject reason codes. These travel back to the client in
// SyncItemResult.Error so the offline outbox can decide whether to retry (a
// transient/server fault) or surface the item for manual fixing (a permanent
// data fault). They are stable string literals, NOT the envelope ErrorCode set:
// the batch envelope is always 200 success; rejection is per item.
const (
	// reasonInspectionNotFound — a trajectory_point/problem referenced a parent
	// inspection (by client_uuid) that is neither in this batch nor already on the
	// server. The item is rejected; the client should re-send once the parent syncs.
	reasonInspectionNotFound = "INSPECTION_NOT_FOUND"
	// reasonSeqConflict — a trajectory_point's (inspection_id, seq) is already held
	// by a DIFFERENT client_uuid (a reordered/clashing sample). Rejected so the
	// ordering integrity of the route is never silently corrupted.
	reasonSeqConflict = "SEQ_CONFLICT"
	// reasonOwnerNotFound — a media item referenced an owner (by client_uuid) that
	// could not be resolved to a problem/inspection in this batch or on the server.
	reasonOwnerNotFound = "OWNER_NOT_FOUND"
	// reasonInvalidGeom — geom failed the server-side SRID/ST_IsValid check.
	reasonInvalidGeom = "INVALID_GEOM"
	// reasonFKViolation — a NOT NULL RESTRICT FK (project_id/inspector_id/owner)
	// pointed at a row that does not exist.
	reasonFKViolation = "FK_VIOLATION"
	// reasonInternal — an unexpected DB/encoding fault while processing the item.
	reasonInternal = "INTERNAL"
)

// errSeqConflict is the sentinel a per-item savepoint fn returns when it detects
// a (inspection_id, seq) collision on a different client_uuid, so the dispatcher
// maps it to reasonSeqConflict instead of the generic FK/internal reason.
var errSeqConflict = errors.New("trajectory seq conflict")

// isUniqueViolation reports whether err is a Postgres unique-violation (23505).
// Inside a per-item savepoint a seq clash surfaces as 23505 on uq_traj_inspection_seq;
// we translate it to errSeqConflict before the savepoint is rolled back.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

// isForeignKeyViolation reports whether err is a Postgres foreign-key violation
// (23503) — a missing project_id/inspector_id (FK RESTRICT). The dict soft FKs
// are also RESTRICT but historically tolerant: a retired dict_item keeps its id
// and passes the FK, so this only fires on a genuinely absent reference.
func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23503"
	}
	return false
}
