package sync

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
)

// upsertReturningID runs the idempotent upsert shape every item shares:
//
//	INSERT ... ON CONFLICT (client_uuid) DO NOTHING RETURNING id
//
// then, when ON CONFLICT suppressed the insert (pgx.ErrNoRows, 0 rows), re-selects
// the existing id by client_uuid. It returns (id, inserted): inserted=true means a
// brand-new row (accepted); inserted=false means the row already existed
// (duplicate / idempotent replay). insertArgs binds insertSQL; selectSQL binds the
// single clientUUID. Any other error propagates so the savepoint rolls back and the
// item is reported rejected.
func upsertReturningID(ctx context.Context, q pgx.Tx, insertSQL, selectSQL, clientUUID string, insertArgs []any) (int64, bool, error) {
	var id int64
	err := q.QueryRow(ctx, insertSQL, insertArgs...).Scan(&id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Conflict on client_uuid: the row already exists — re-select its id.
		if e := q.QueryRow(ctx, selectSQL, clientUUID).Scan(&id); e != nil {
			return 0, false, e
		}
		return id, false, nil
	case err != nil:
		return 0, false, err
	}
	return id, true, nil
}

// reject builds a rejected per-item result carrying the reason string.
func reject(clientUUID openapi_types.UUID, reason string) oapi.SyncItemResult {
	r := reason
	return oapi.SyncItemResult{
		ClientUuid: clientUUID,
		Status:     oapi.Rejected,
		Error:      &r,
	}
}

// acceptedOrDuplicate builds the success result for an upsert: accepted when the
// row was freshly inserted, duplicate (idempotent replay) otherwise. Both carry
// the resolved server id.
func acceptedOrDuplicate(clientUUID openapi_types.UUID, serverID int64, inserted bool) oapi.SyncItemResult {
	id := serverID
	status := oapi.Duplicate
	if inserted {
		status = oapi.Accepted
	}
	return oapi.SyncItemResult{
		ClientUuid: clientUUID,
		Status:     status,
		ServerId:   &id,
	}
}

// rejectInspectionErr maps an inspection savepoint error to a reject reason.
func rejectInspectionErr(clientUUID openapi_types.UUID, err error) oapi.SyncItemResult {
	switch {
	case isForeignKeyViolation(err):
		return reject(clientUUID, reasonFKViolation)
	default:
		return reject(clientUUID, reasonInternal)
	}
}

// rejectTrajectoryErr maps a trajectory savepoint error to a reject reason,
// distinguishing the unresolved-parent and seq-conflict cases the contract names.
func rejectTrajectoryErr(clientUUID openapi_types.UUID, err error) oapi.SyncItemResult {
	switch {
	case errors.Is(err, errInspectionNotFound):
		return reject(clientUUID, reasonInspectionNotFound)
	case errors.Is(err, errSeqConflict):
		return reject(clientUUID, reasonSeqConflict)
	case errors.Is(err, errInvalidGeom):
		return reject(clientUUID, reasonInvalidGeom)
	case isForeignKeyViolation(err):
		return reject(clientUUID, reasonFKViolation)
	default:
		return reject(clientUUID, reasonInternal)
	}
}

// rejectProblemErr maps a problem savepoint error to a reject reason.
func rejectProblemErr(clientUUID openapi_types.UUID, err error) oapi.SyncItemResult {
	switch {
	case errors.Is(err, errInspectionNotFound):
		return reject(clientUUID, reasonInspectionNotFound)
	case errors.Is(err, errInvalidGeom):
		return reject(clientUUID, reasonInvalidGeom)
	case isForeignKeyViolation(err):
		return reject(clientUUID, reasonFKViolation)
	default:
		return reject(clientUUID, reasonInternal)
	}
}

// rejectMediaErr maps a media savepoint error to a reject reason.
func rejectMediaErr(clientUUID openapi_types.UUID, err error) oapi.SyncItemResult {
	switch {
	case errors.Is(err, errOwnerNotFound):
		return reject(clientUUID, reasonOwnerNotFound)
	case isForeignKeyViolation(err):
		return reject(clientUUID, reasonFKViolation)
	default:
		return reject(clientUUID, reasonInternal)
	}
}

// bucketize returns the SyncBatchResult: the full ordered Results plus the
// accepted/rejected/duplicate convenience buckets (each a non-nil slice only when
// it has members, so an absent bucket marshals to omitted rather than []).
func bucketize(results []oapi.SyncItemResult) *oapi.SyncBatchResult {
	var accepted, rejected, duplicate []oapi.SyncItemResult
	for _, r := range results {
		switch r.Status {
		case oapi.Accepted:
			accepted = append(accepted, r)
		case oapi.Rejected:
			rejected = append(rejected, r)
		case oapi.Duplicate:
			duplicate = append(duplicate, r)
		}
	}
	out := &oapi.SyncBatchResult{Results: results}
	if len(accepted) > 0 {
		out.Accepted = &accepted
	}
	if len(rejected) > 0 {
		out.Rejected = &rejected
	}
	if len(duplicate) > 0 {
		out.Duplicate = &duplicate
	}
	return out
}

// countItems totals the items across the four envelope sections (capacity hint).
func countItems(req oapi.SyncBatchRequest) int {
	n := 0
	if req.Inspections != nil {
		n += len(*req.Inspections)
	}
	if req.TrajectoryPoints != nil {
		n += len(*req.TrajectoryPoints)
	}
	if req.Problems != nil {
		n += len(*req.Problems)
	}
	if req.Media != nil {
		n += len(*req.Media)
	}
	return n
}

// endedAtUTC normalizes an optional ended_at to UTC, preserving nil.
func endedAtUTC(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

// derefStr returns the pointed-to string or "" when nil.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
