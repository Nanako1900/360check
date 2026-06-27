// Package sync implements the offline-sync batch upsert domain (POST /sync/batch):
// a single idempotent envelope carrying inspections, trajectory points, problems
// and media references created offline by the APP, ingested per-item.
//
// Contract (00 "Offline-sync batch upsert contract" + 01 P5):
//   - Every item carries a client-generated client_uuid. Each item is upserted via
//     INSERT ... ON CONFLICT (client_uuid) DO NOTHING then re-selected, inside its
//     OWN savepoint (db.WithSavepoint) on ONE outer transaction (db.WithTx). A bad
//     item rolls back only its savepoint; the batch continues.
//   - Per item the result is accepted (newly inserted), duplicate (already ingested
//     — idempotent replay, returns the existing server_id) or rejected (constraint
//     or validation failure, with a reason string).
//   - Ordering matters for cross-references: inspections first (building a
//     client_uuid -> inspection server id map), then trajectory_points (whose
//     InspectionClientUuid resolves against that map OR an existing inspection),
//     then problems (InspectionClientUuid resolves the same way; nil is allowed),
//     then media (OwnerClientUuid resolves to the problem/inspection it annotates).
//   - Geometry is hand-written pgx: ST_GeomFromEWKB(geo.EncodePointWGS84(...)),
//     validated 4326 via db.ValidateGeomWGS84. The media BINARY never travels in
//     this JSON — only a reference row (cos_bucket/cos_key/capture_state).
//   - Historical dictionary tolerance: a problem referencing a retired type_item
//     with an older dict_version_used is accepted, not rejected (the dict soft FKs
//     are RESTRICT, so retired ids still pass).
package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
)

// Service ingests offline batches into the inspections / trajectory_points /
// problems / media_assets tables. It owns no table of its own; it composes the
// same client_uuid idempotency the individual domains use, but across one tx so
// the whole envelope commits or rolls back atomically while each item is isolated.
type Service struct {
	pool *db.Pool
}

// NewService wires the sync service onto the shared pool.
func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

// batchState threads the per-batch resolution maps through the ordered passes.
// inspectionIDs maps a synced inspection's client_uuid -> its server id so later
// trajectory/problem items in the SAME batch can resolve their parent without a
// round trip. ownerIDs maps every resolvable owner client_uuid (inspections and
// problems) -> server id so media items can attach to an owner created moments ago.
type batchState struct {
	inspectionIDs map[string]int64
	problemIDs    map[string]int64
}

// Process ingests one batch. actorID is the authenticated user (rbac), used as the
// default inspector/owner/creator when an item omits it. The returned result holds
// every per-item outcome in submission order plus accepted/rejected/duplicate
// buckets for convenience. The whole batch runs in one tx; a per-item failure is
// isolated to its savepoint and reported as rejected, never aborting the batch.
func (s *Service) Process(ctx context.Context, req oapi.SyncBatchRequest, actorID int64) (*oapi.SyncBatchResult, error) {
	results := make([]oapi.SyncItemResult, 0, countItems(req))
	state := &batchState{
		inspectionIDs: make(map[string]int64),
		problemIDs:    make(map[string]int64),
	}

	err := s.pool.WithTx(ctx, func(tx pgx.Tx) error {
		// 1. Inspections first — they are parents of trajectory points and problems.
		if req.Inspections != nil {
			for _, item := range *req.Inspections {
				results = append(results, s.upsertInspection(ctx, tx, item, actorID, state))
			}
		}
		// 2. Trajectory points — resolve InspectionClientUuid against the batch map
		//    or an existing inspection; (inspection_id, seq) integrity enforced.
		if req.TrajectoryPoints != nil {
			for _, item := range *req.TrajectoryPoints {
				results = append(results, s.upsertTrajectoryPoint(ctx, tx, item, state))
			}
		}
		// 3. Problems — InspectionClientUuid optional; dict references tolerated.
		if req.Problems != nil {
			for _, item := range *req.Problems {
				results = append(results, s.upsertProblem(ctx, tx, item, actorID, state))
			}
		}
		// 4. Media references — resolve OwnerClientUuid to a problem/inspection.
		if req.Media != nil {
			for _, item := range *req.Media {
				results = append(results, s.upsertMedia(ctx, tx, item, actorID, state))
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("sync: batch tx: %w", err)
	}

	return bucketize(results), nil
}

// upsertInspection ingests one inspection item inside its own savepoint. On accept
// or duplicate it records client_uuid -> server id so later items in the batch can
// reference it. A failure is isolated and returned as a rejected result.
func (s *Service) upsertInspection(ctx context.Context, tx pgx.Tx, item oapi.SyncInspectionItem, actorID int64, state *batchState) oapi.SyncItemResult {
	clientUUID := item.ClientUuid.String()

	inspectorID := actorID
	if item.InspectorId != nil && *item.InspectorId != 0 {
		inspectorID = *item.InspectorId
	}
	device := []byte("{}")
	if item.DeviceInfo != nil {
		if b, err := json.Marshal(*item.DeviceInfo); err == nil {
			device = b
		}
	}
	status := string(oapi.InspectionStatusINPROGRESS)
	if item.Status != nil && *item.Status != "" {
		status = string(*item.Status)
	}
	note := derefStr(item.Note)

	var (
		serverID int64
		inserted bool
	)
	spErr := db.WithSavepoint(ctx, tx, func(sp pgx.Tx) error {
		// ON CONFLICT DO NOTHING then re-select: an inserted row returns inserted=true
		// with the new id; a replay (0 rows) re-selects the existing id (duplicate).
		var err error
		serverID, inserted, err = upsertReturningID(ctx, sp, `
			INSERT INTO inspections
				(client_uuid, project_id, task_id, inspector_id, status, started_at,
				 ended_at, device_info, note, created_by, updated_by)
			VALUES ($1, $2, $3, $4, $5::inspection_status, $6, $7, $8, $9, $10, $10)
			ON CONFLICT (client_uuid) DO NOTHING
			RETURNING id`,
			`SELECT id FROM inspections WHERE client_uuid = $1`,
			clientUUID,
			[]any{clientUUID, item.ProjectId, item.TaskId, inspectorID, status,
				item.StartedAt.UTC(), endedAtUTC(item.EndedAt), device, note, actorID})
		return err
	})
	if spErr != nil {
		return rejectInspectionErr(item.ClientUuid, spErr)
	}

	// Record the mapping for child items regardless of accepted/duplicate.
	state.inspectionIDs[clientUUID] = serverID
	return acceptedOrDuplicate(item.ClientUuid, serverID, inserted)
}

// upsertTrajectoryPoint ingests one trajectory point. It resolves the parent
// inspection from this batch's map first, then from an existing inspection by
// client_uuid; an unresolved parent is rejected with INSPECTION_NOT_FOUND. A
// (inspection_id, seq) collision held by a different client_uuid is rejected with
// SEQ_CONFLICT (the route's ordering integrity is never silently corrupted).
func (s *Service) upsertTrajectoryPoint(ctx context.Context, tx pgx.Tx, item oapi.SyncTrajectoryPointItem, state *batchState) oapi.SyncItemResult {
	clientUUID := item.ClientUuid.String()
	parentUUID := item.InspectionClientUuid.String()

	geomEWKB, err := geoJSONToPointEWKB(item.Geom)
	if err != nil {
		return reject(item.ClientUuid, reasonInvalidGeom)
	}

	var (
		serverID int64
		inserted bool
	)
	spErr := db.WithSavepoint(ctx, tx, func(sp pgx.Tx) error {
		// Resolve the parent inspection: batch map first, else an existing row.
		inspectionID, ok := state.inspectionIDs[parentUUID]
		if !ok {
			if e := sp.QueryRow(ctx,
				`SELECT id FROM inspections WHERE client_uuid = $1 AND deleted_at IS NULL`,
				parentUUID).Scan(&inspectionID); errors.Is(e, pgx.ErrNoRows) {
				return errInspectionNotFound
			} else if e != nil {
				return e
			}
		}

		// Validate geometry inside the savepoint so a bad point only fails itself.
		valid, _, e := db.ValidateGeomWGS84(ctx, sp, geomEWKB)
		if e != nil {
			return e
		}
		if !valid {
			return errInvalidGeom
		}

		serverID, inserted, e = upsertReturningID(ctx, sp, `
			INSERT INTO trajectory_points
				(client_uuid, inspection_id, seq, geom, recorded_at, speed, bearing, altitude, accuracy)
			VALUES ($1, $2, $3, ST_GeomFromEWKB($4), $5, $6, $7, $8, $9)
			ON CONFLICT (client_uuid) DO NOTHING
			RETURNING id`,
			`SELECT id FROM trajectory_points WHERE client_uuid = $1`,
			clientUUID,
			[]any{clientUUID, inspectionID, item.Seq, geomEWKB, item.RecordedAt.UTC(),
				item.Speed, item.Bearing, item.Altitude, item.Accuracy})
		if isUniqueViolation(e) {
			// client_uuid is handled by ON CONFLICT; a 23505 here is the
			// (inspection_id, seq) unique index hit by a DIFFERENT client_uuid.
			return errSeqConflict
		}
		return e
	})
	if spErr != nil {
		return rejectTrajectoryErr(item.ClientUuid, spErr)
	}
	return acceptedOrDuplicate(item.ClientUuid, serverID, inserted)
}

// upsertProblem ingests one problem. InspectionClientUuid is optional: when set it
// resolves like a trajectory parent (batch map, else existing) and an unresolved
// reference is rejected; when nil the problem simply has no inspection. dict
// references (type/status/category) are tolerated even when retired, and
// dict_version_used is taken from the item or the current problem_type version.
func (s *Service) upsertProblem(ctx context.Context, tx pgx.Tx, item oapi.SyncProblemItem, actorID int64, state *batchState) oapi.SyncItemResult {
	clientUUID := item.ClientUuid.String()

	geomEWKB, err := geoJSONToPointEWKB(item.Geom)
	if err != nil {
		return reject(item.ClientUuid, reasonInvalidGeom)
	}

	inspectorID := actorID
	if item.InspectorId != nil && *item.InspectorId != 0 {
		inspectorID = *item.InspectorId
	}
	title, description, note := derefStr(item.Title), derefStr(item.Description), derefStr(item.Note)

	var (
		serverID int64
		inserted bool
	)
	spErr := db.WithSavepoint(ctx, tx, func(sp pgx.Tx) error {
		// Optional parent inspection by client_uuid.
		var inspectionID *int64
		if item.InspectionClientUuid != nil {
			parentUUID := item.InspectionClientUuid.String()
			if id, ok := state.inspectionIDs[parentUUID]; ok {
				inspectionID = &id
			} else {
				var id int64
				if e := sp.QueryRow(ctx,
					`SELECT id FROM inspections WHERE client_uuid = $1 AND deleted_at IS NULL`,
					parentUUID).Scan(&id); errors.Is(e, pgx.ErrNoRows) {
					return errInspectionNotFound
				} else if e != nil {
					return e
				}
				inspectionID = &id
			}
		}

		// dict_version_used: item value, else the current problem_type version.
		dictVersion, e := resolveDictVersion(ctx, sp, item.DictVersionUsed)
		if e != nil {
			return e
		}

		valid, _, e := db.ValidateGeomWGS84(ctx, sp, geomEWKB)
		if e != nil {
			return e
		}
		if !valid {
			return errInvalidGeom
		}

		serverID, inserted, e = upsertReturningID(ctx, sp, `
			INSERT INTO problems
				(client_uuid, project_id, inspection_id, inspector_id, geom,
				 type_item_id, status_item_id, category_item_id, dict_version_used,
				 title, description, note, captured_at, created_by, updated_by)
			VALUES ($1, $2, $3, $4, ST_GeomFromEWKB($5),
				 $6, $7, $8, $9, $10, $11, $12, $13, $14, $14)
			ON CONFLICT (client_uuid) DO NOTHING
			RETURNING id`,
			`SELECT id FROM problems WHERE client_uuid = $1 AND deleted_at IS NULL`,
			clientUUID,
			[]any{clientUUID, item.ProjectId, inspectionID, inspectorID, geomEWKB,
				item.TypeItemId, item.StatusItemId, item.CategoryItemId, dictVersion,
				title, description, note, item.CapturedAt.UTC(), actorID})
		return e
	})
	if spErr != nil {
		return rejectProblemErr(item.ClientUuid, spErr)
	}

	state.problemIDs[clientUUID] = serverID
	return acceptedOrDuplicate(item.ClientUuid, serverID, inserted)
}

// upsertMedia ingests one media REFERENCE row (never the binary). owner_id is
// resolved from OwnerClientUuid against this batch (a problem/inspection just
// synced) or an existing row of the declared owner_type. The capture state the
// client reports (CAPTURED_RAW/STITCHED/QUEUED) is persisted verbatim; advancing
// to CONFIRMED + verified_at is the separate HeadObject confirm flow, not sync.
func (s *Service) upsertMedia(ctx context.Context, tx pgx.Tx, item oapi.SyncMediaItem, actorID int64, state *batchState) oapi.SyncItemResult {
	clientUUID := item.ClientUuid.String()

	region := "ap-guangzhou"
	if item.CosRegion != nil && *item.CosRegion != "" {
		region = *item.CosRegion
	}
	contentType := "image/jpeg"
	if item.ContentType != nil && *item.ContentType != "" {
		contentType = *item.ContentType
	}
	var mediaGroup *string
	if item.MediaGroup != nil {
		g := item.MediaGroup.String()
		mediaGroup = &g
	}

	var (
		serverID int64
		inserted bool
	)
	spErr := db.WithSavepoint(ctx, tx, func(sp pgx.Tx) error {
		// Resolve the owner row by (owner_type, owner_client_uuid). Batch map first
		// (the owner may have been created in this very envelope), else an existing
		// problem/inspection. owner_client_uuid is required to attach a reference.
		ownerID, e := resolveMediaOwner(ctx, sp, item, state)
		if e != nil {
			return e
		}

		serverID, inserted, e = upsertReturningID(ctx, sp, `
			INSERT INTO media_assets
				(client_uuid, owner_type, owner_id, tier, cos_bucket, cos_key,
				 cos_region, content_type, byte_size, width, height, etag,
				 capture_state, media_group, created_by, updated_by)
			VALUES ($1, $2::media_owner_type, $3, $4::media_tier, $5, $6,
				 $7, $8, $9, $10, $11, $12, $13::capture_state, $14, $15, $15)
			ON CONFLICT (client_uuid) DO NOTHING
			RETURNING id`,
			`SELECT id FROM media_assets WHERE client_uuid = $1 AND deleted_at IS NULL`,
			clientUUID,
			[]any{clientUUID, string(item.OwnerType), ownerID, string(item.Tier),
				item.CosBucket, item.CosKey, region, contentType, item.ByteSize,
				item.Width, item.Height, item.Etag, string(item.CaptureState),
				mediaGroup, actorID})
		return e
	})
	if spErr != nil {
		return rejectMediaErr(item.ClientUuid, spErr)
	}
	return acceptedOrDuplicate(item.ClientUuid, serverID, inserted)
}

// resolveMediaOwner finds the server owner_id for a media item from its declared
// owner_type + OwnerClientUuid: the batch map for the matching type first, else an
// existing problem/inspection row. A nil OwnerClientUuid or an unresolved one ->
// errOwnerNotFound (the reference cannot be attached).
func resolveMediaOwner(ctx context.Context, q pgx.Tx, item oapi.SyncMediaItem, state *batchState) (int64, error) {
	if item.OwnerClientUuid == nil {
		return 0, errOwnerNotFound
	}
	ownerUUID := item.OwnerClientUuid.String()

	var table string
	switch item.OwnerType {
	case oapi.MediaOwnerTypeInspection:
		if id, ok := state.inspectionIDs[ownerUUID]; ok {
			return id, nil
		}
		table = "inspections"
	case oapi.MediaOwnerTypeProblem:
		if id, ok := state.problemIDs[ownerUUID]; ok {
			return id, nil
		}
		table = "problems"
	default:
		// project/user owners are not offline-synced; nothing to resolve here.
		return 0, errOwnerNotFound
	}

	var id int64
	err := q.QueryRow(ctx,
		`SELECT id FROM `+table+` WHERE client_uuid = $1 AND deleted_at IS NULL`,
		ownerUUID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, errOwnerNotFound
	}
	if err != nil {
		return 0, err
	}
	return id, nil
}

// resolveDictVersion returns the caller-supplied dict_version_used, or the current
// problem_type dict_type.version when nil (falling back to 1 if the dict is absent,
// which the seed always provides). Mirrors problem.Service.resolveDictVersion so
// offline-pinned versions and retired items are both tolerated.
func resolveDictVersion(ctx context.Context, q pgx.Tx, supplied *int) (int, error) {
	if supplied != nil {
		return *supplied, nil
	}
	var version int
	err := q.QueryRow(ctx, `SELECT version FROM dict_type WHERE code = 'problem_type'`).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 1, nil
	}
	if err != nil {
		return 0, fmt.Errorf("sync: read problem_type version: %w", err)
	}
	return version, nil
}

// Sentinels returned from inside a savepoint fn so the dispatcher can map them to
// the right per-item reject reason (instead of a generic FK/internal reason).
var (
	errInspectionNotFound = errors.New("parent inspection not found")
	errInvalidGeom        = errors.New("invalid geometry")
	errOwnerNotFound      = errors.New("media owner not found")
)
