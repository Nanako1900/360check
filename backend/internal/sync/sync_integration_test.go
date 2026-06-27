//go:build integration

package sync

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	migrations "github.com/nnkglobal/c5-backend/db/migrations"
	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
)

// testEnv bundles the migrated pool, the sync service, the seeded admin id and a
// fresh project id that synced items can reference.
type testEnv struct {
	pool      *db.Pool
	svc       *Service
	adminID   int64
	projectID int64
}

// newTestEnv spins a fresh PostGIS container, runs all migrations (000002 seeds the
// admin user + the problem_type dict), connects a pool, and inserts a project to
// own the synced entities.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	ctx := context.Background()

	pg, err := tcpostgres.Run(ctx, "postgis/postgis:16-3.4",
		tcpostgres.WithDatabase("c5"),
		tcpostgres.WithUsername("c5"),
		tcpostgres.WithPassword("c5"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, db.RunMigrations(dsn, migrations.FS))

	pool, err := db.New(ctx, dsn, 5, 1, 30*time.Second)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, pool.Ping(ctx))

	var adminID int64
	require.NoError(t, pool.QueryRow(ctx, "SELECT id FROM users WHERE username='admin'").Scan(&adminID))

	var projectID int64
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO projects (code, name) VALUES ('P-SYNC','sync test') RETURNING id`).Scan(&projectID))

	return &testEnv{pool: pool, svc: NewService(pool), adminID: adminID, projectID: projectID}
}

// pt builds a WGS84 GeoJSON Point.
func pt(lon, lat float32) oapi.GeoJSONPoint {
	return oapi.GeoJSONPoint{Type: oapi.Point, Coordinates: []float32{lon, lat}}
}

// byUUID indexes per-item results by client_uuid for assertions.
func byUUID(results []oapi.SyncItemResult) map[string]oapi.SyncItemResult {
	m := make(map[string]oapi.SyncItemResult, len(results))
	for _, r := range results {
		m[r.ClientUuid.String()] = r
	}
	return m
}

// TestBatch_FirstAcceptedReplayDuplicate is the core idempotency matrix: a batch
// that cross-references an inspection -> trajectory -> problem -> media by
// client_uuid is all accepted on first ingest; the EXACT same batch replayed is
// all duplicate, returning the SAME server_ids (idempotent replay).
func TestBatch_FirstAcceptedReplayDuplicate(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	inspUUID := uuid.New()
	trajUUID := uuid.New()
	probUUID := uuid.New()
	mediaUUID := uuid.New()
	started := time.Date(2026, 6, 26, 8, 0, 0, 0, time.UTC)
	recorded := started.Add(time.Minute)

	req := oapi.SyncBatchRequest{
		Inspections: &[]oapi.SyncInspectionItem{{
			ClientUuid: inspUUID,
			ProjectId:  env.projectID,
			StartedAt:  started,
		}},
		TrajectoryPoints: &[]oapi.SyncTrajectoryPointItem{{
			ClientUuid:           trajUUID,
			InspectionClientUuid: inspUUID, // resolves to the inspection in THIS batch
			Seq:                  1,
			Geom:                 pt(113.0, 23.0),
			RecordedAt:           recorded,
		}},
		Problems: &[]oapi.SyncProblemItem{{
			ClientUuid:           probUUID,
			ProjectId:            env.projectID,
			InspectionClientUuid: &inspUUID,
			Geom:                 pt(113.01, 23.0),
			CapturedAt:           recorded,
		}},
		Media: &[]oapi.SyncMediaItem{{
			ClientUuid:      mediaUUID,
			OwnerType:       oapi.MediaOwnerTypeProblem,
			OwnerClientUuid: &probUUID, // resolves to the problem in THIS batch
			Tier:            oapi.Original,
			CosBucket:       "c5-media-1300000000",
			CosKey:          "uploads/" + mediaUUID.String() + "/original.jpg",
			CaptureState:    oapi.QUEUED,
		}},
	}

	first, err := env.svc.Process(ctx, req, env.adminID)
	require.NoError(t, err)
	require.Len(t, first.Results, 4)

	fm := byUUID(first.Results)
	for _, u := range []uuid.UUID{inspUUID, trajUUID, probUUID, mediaUUID} {
		r := fm[u.String()]
		assert.Equalf(t, oapi.Accepted, r.Status, "item %s should be accepted; error=%v", u, r.Error)
		require.NotNilf(t, r.ServerId, "accepted item %s must carry server_id", u)
		assert.Positive(t, *r.ServerId)
	}
	require.NotNil(t, first.Accepted)
	assert.Len(t, *first.Accepted, 4)
	assert.Nil(t, first.Rejected)
	assert.Nil(t, first.Duplicate)

	// Replay the identical batch — every item is now a duplicate with the SAME id.
	second, err := env.svc.Process(ctx, req, env.adminID)
	require.NoError(t, err)
	require.Len(t, second.Results, 4)

	sm := byUUID(second.Results)
	for _, u := range []uuid.UUID{inspUUID, trajUUID, probUUID, mediaUUID} {
		r := sm[u.String()]
		assert.Equalf(t, oapi.Duplicate, r.Status, "replayed item %s should be duplicate", u)
		require.NotNil(t, r.ServerId)
		assert.Equalf(t, *fm[u.String()].ServerId, *r.ServerId,
			"replay must return the SAME server_id for %s", u)
	}
	require.NotNil(t, second.Duplicate)
	assert.Len(t, *second.Duplicate, 4)
	assert.Nil(t, second.Accepted)

	// Exactly one row each persisted (no duplicate inserts).
	assertRowCount(t, env, ctx, "inspections", inspUUID, 1)
	assertRowCount(t, env, ctx, "trajectory_points", trajUUID, 1)
	assertRowCount(t, env, ctx, "problems", probUUID, 1)
	assertRowCount(t, env, ctx, "media_assets", mediaUUID, 1)
}

// TestBatch_SavepointIsolation proves per-item isolation: a single batch carrying a
// good inspection, two trajectory points where the second CLASHES on
// (inspection_id, seq) with the first, and a good problem rejects ONLY the clashing
// point (a real 23505 constraint failure inside its savepoint) while every other
// item in the same batch still commits.
func TestBatch_SavepointIsolation(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	goodInsp := uuid.New()
	goodPoint := uuid.New()
	clashPoint := uuid.New()
	goodProb := uuid.New()
	started := time.Now().UTC().Add(-time.Hour)

	req := oapi.SyncBatchRequest{
		Inspections: &[]oapi.SyncInspectionItem{{
			ClientUuid: goodInsp, ProjectId: env.projectID, StartedAt: started,
		}},
		TrajectoryPoints: &[]oapi.SyncTrajectoryPointItem{
			{
				ClientUuid: goodPoint, InspectionClientUuid: goodInsp, Seq: 1,
				Geom: pt(113.0, 23.0), RecordedAt: started,
			},
			{
				// Same (inspection, seq=1), different client_uuid -> a genuine 23505
				// on uq_traj_inspection_seq. Only THIS savepoint rolls back.
				ClientUuid: clashPoint, InspectionClientUuid: goodInsp, Seq: 1,
				Geom: pt(113.1, 23.1), RecordedAt: started,
			},
		},
		Problems: &[]oapi.SyncProblemItem{{
			ClientUuid: goodProb, ProjectId: env.projectID,
			Geom: pt(113.2, 23.2), CapturedAt: started,
		}},
	}

	res, err := env.svc.Process(ctx, req, env.adminID)
	require.NoError(t, err)
	require.Len(t, res.Results, 4)

	m := byUUID(res.Results)
	assert.Equal(t, oapi.Accepted, m[goodInsp.String()].Status)
	assert.Equal(t, oapi.Accepted, m[goodPoint.String()].Status)
	assert.Equal(t, oapi.Accepted, m[goodProb.String()].Status, "good problem persists despite a sibling failing")
	require.Equal(t, oapi.Rejected, m[clashPoint.String()].Status)
	require.NotNil(t, m[clashPoint.String()].Error)
	assert.Equal(t, reasonSeqConflict, *m[clashPoint.String()].Error)

	// Everything except the clashing point committed; the savepoint isolated it.
	assertRowCount(t, env, ctx, "inspections", goodInsp, 1)
	assertRowCount(t, env, ctx, "trajectory_points", goodPoint, 1)
	assertRowCount(t, env, ctx, "trajectory_points", clashPoint, 0)
	assertRowCount(t, env, ctx, "problems", goodProb, 1)
}

// TestBatch_SeqConflict proves the (inspection_id, seq) integrity rule: a second
// trajectory point with a DIFFERENT client_uuid but the same (inspection, seq) as
// an already-stored point is rejected with SEQ_CONFLICT, and only that item.
func TestBatch_SeqConflict(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	inspUUID := uuid.New()
	firstPoint := uuid.New()
	started := time.Now().UTC().Add(-time.Hour)

	// Seed the inspection + a point at seq=1 in a first batch.
	seed := oapi.SyncBatchRequest{
		Inspections: &[]oapi.SyncInspectionItem{{
			ClientUuid: inspUUID, ProjectId: env.projectID, StartedAt: started,
		}},
		TrajectoryPoints: &[]oapi.SyncTrajectoryPointItem{{
			ClientUuid: firstPoint, InspectionClientUuid: inspUUID, Seq: 1,
			Geom: pt(113.0, 23.0), RecordedAt: started,
		}},
	}
	seedRes, err := env.svc.Process(ctx, seed, env.adminID)
	require.NoError(t, err)
	assert.Equal(t, oapi.Accepted, byUUID(seedRes.Results)[firstPoint.String()].Status)

	// Now a NEW client_uuid reuses (inspection_id, seq=1) — a reordered/clashing
	// sample. It must be rejected SEQ_CONFLICT; a second valid seq=2 still accepts.
	clashPoint := uuid.New()
	okPoint := uuid.New()
	clash := oapi.SyncBatchRequest{
		TrajectoryPoints: &[]oapi.SyncTrajectoryPointItem{
			{
				ClientUuid: clashPoint, InspectionClientUuid: inspUUID, Seq: 1,
				Geom: pt(113.5, 23.5), RecordedAt: started,
			},
			{
				ClientUuid: okPoint, InspectionClientUuid: inspUUID, Seq: 2,
				Geom: pt(113.6, 23.6), RecordedAt: started,
			},
		},
	}
	res, err := env.svc.Process(ctx, clash, env.adminID)
	require.NoError(t, err)

	m := byUUID(res.Results)
	require.Equal(t, oapi.Rejected, m[clashPoint.String()].Status)
	require.NotNil(t, m[clashPoint.String()].Error)
	assert.Equal(t, reasonSeqConflict, *m[clashPoint.String()].Error)
	assert.Equal(t, oapi.Accepted, m[okPoint.String()].Status, "the non-clashing seq still accepts")

	assertRowCount(t, env, ctx, "trajectory_points", clashPoint, 0)
	assertRowCount(t, env, ctx, "trajectory_points", okPoint, 1)
}

// TestBatch_UnresolvedInspectionRejected: a trajectory point whose parent
// inspection is neither in the batch nor on the server is rejected
// INSPECTION_NOT_FOUND.
func TestBatch_UnresolvedInspectionRejected(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	orphanPoint := uuid.New()
	missingInsp := uuid.New() // never synced

	req := oapi.SyncBatchRequest{
		TrajectoryPoints: &[]oapi.SyncTrajectoryPointItem{{
			ClientUuid: orphanPoint, InspectionClientUuid: missingInsp, Seq: 1,
			Geom: pt(113.0, 23.0), RecordedAt: time.Now().UTC(),
		}},
	}
	res, err := env.svc.Process(ctx, req, env.adminID)
	require.NoError(t, err)

	r := byUUID(res.Results)[orphanPoint.String()]
	require.Equal(t, oapi.Rejected, r.Status)
	require.NotNil(t, r.Error)
	assert.Equal(t, reasonInspectionNotFound, *r.Error)
}

// TestBatch_RetiredDictItemAccepted proves historical dictionary tolerance: a
// problem referencing a RETIRED type dict_item (is_active=false) with an older
// dict_version_used is accepted, not rejected (the soft FK still passes).
func TestBatch_RetiredDictItemAccepted(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// Create then retire a problem_type dict item; reference it from an offline
	// problem tagged with an older dict_version_used.
	var typeID int64
	require.NoError(t, env.pool.QueryRow(ctx, `
		INSERT INTO dict_item (dict_type_id, code, label, is_active)
		SELECT id, 'retired_crack', '退役-裂缝', false FROM dict_type WHERE code='problem_type'
		RETURNING id`).Scan(&typeID))

	probUUID := uuid.New()
	oldVersion := 1
	req := oapi.SyncBatchRequest{
		Problems: &[]oapi.SyncProblemItem{{
			ClientUuid:      probUUID,
			ProjectId:       env.projectID,
			Geom:            pt(113.0, 23.0),
			CapturedAt:      time.Now().UTC(),
			TypeItemId:      &typeID,
			DictVersionUsed: &oldVersion,
		}},
	}
	res, err := env.svc.Process(ctx, req, env.adminID)
	require.NoError(t, err)

	r := byUUID(res.Results)[probUUID.String()]
	require.Equalf(t, oapi.Accepted, r.Status,
		"problem referencing a retired dict_item must be accepted; error=%v", r.Error)
	require.NotNil(t, r.ServerId)

	// The persisted row pins the supplied (older) dict_version_used and the retired type.
	var (
		gotType    int64
		gotVersion int
	)
	require.NoError(t, env.pool.QueryRow(ctx,
		`SELECT type_item_id, dict_version_used FROM problems WHERE client_uuid=$1`,
		probUUID).Scan(&gotType, &gotVersion))
	assert.Equal(t, typeID, gotType)
	assert.Equal(t, oldVersion, gotVersion)
}

// TestBatch_MediaOwnerResolvesToExisting: a media item can attach to an owner that
// already exists on the server (not just one created in the same batch).
func TestBatch_MediaOwnerResolvesToExisting(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// First batch: a problem.
	probUUID := uuid.New()
	first := oapi.SyncBatchRequest{
		Problems: &[]oapi.SyncProblemItem{{
			ClientUuid: probUUID, ProjectId: env.projectID,
			Geom: pt(113.0, 23.0), CapturedAt: time.Now().UTC(),
		}},
	}
	fr, err := env.svc.Process(ctx, first, env.adminID)
	require.NoError(t, err)
	probID := *byUUID(fr.Results)[probUUID.String()].ServerId

	// Second batch: a media reference to that existing problem.
	mediaUUID := uuid.New()
	second := oapi.SyncBatchRequest{
		Media: &[]oapi.SyncMediaItem{{
			ClientUuid:      mediaUUID,
			OwnerType:       oapi.MediaOwnerTypeProblem,
			OwnerClientUuid: &probUUID,
			Tier:            oapi.Original,
			CosBucket:       "c5-media-1300000000",
			CosKey:          "uploads/" + mediaUUID.String() + "/original.jpg",
			CaptureState:    oapi.STITCHED,
		}},
	}
	sr, err := env.svc.Process(ctx, second, env.adminID)
	require.NoError(t, err)

	r := byUUID(sr.Results)[mediaUUID.String()]
	require.Equalf(t, oapi.Accepted, r.Status, "media owner should resolve to the existing problem; error=%v", r.Error)

	var ownerID int64
	var ownerType string
	require.NoError(t, env.pool.QueryRow(ctx,
		`SELECT owner_id, owner_type::text FROM media_assets WHERE client_uuid=$1`,
		mediaUUID).Scan(&ownerID, &ownerType))
	assert.Equal(t, probID, ownerID)
	assert.Equal(t, "problem", ownerType)
}

// TestBatch_MediaOwnerNotFound: a media item whose owner client_uuid resolves to
// nothing is rejected OWNER_NOT_FOUND.
func TestBatch_MediaOwnerNotFound(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	mediaUUID := uuid.New()
	missingOwner := uuid.New()
	req := oapi.SyncBatchRequest{
		Media: &[]oapi.SyncMediaItem{{
			ClientUuid:      mediaUUID,
			OwnerType:       oapi.MediaOwnerTypeProblem,
			OwnerClientUuid: &missingOwner,
			Tier:            oapi.Original,
			CosBucket:       "c5-media-1300000000",
			CosKey:          "uploads/" + mediaUUID.String() + "/original.jpg",
			CaptureState:    oapi.QUEUED,
		}},
	}
	res, err := env.svc.Process(ctx, req, env.adminID)
	require.NoError(t, err)

	r := byUUID(res.Results)[mediaUUID.String()]
	require.Equal(t, oapi.Rejected, r.Status)
	require.NotNil(t, r.Error)
	assert.Equal(t, reasonOwnerNotFound, *r.Error)
}

// TestBatch_FKViolationRejected: an inspection referencing a missing project is
// rejected FK_VIOLATION, isolated to its savepoint, while a sibling valid
// inspection still commits.
func TestBatch_FKViolationRejected(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	good := uuid.New()
	bad := uuid.New()
	started := time.Now().UTC()

	req := oapi.SyncBatchRequest{
		Inspections: &[]oapi.SyncInspectionItem{
			{ClientUuid: good, ProjectId: env.projectID, StartedAt: started},
			{ClientUuid: bad, ProjectId: 999999, StartedAt: started}, // no such project
		},
	}
	res, err := env.svc.Process(ctx, req, env.adminID)
	require.NoError(t, err)

	m := byUUID(res.Results)
	assert.Equal(t, oapi.Accepted, m[good.String()].Status)
	require.Equal(t, oapi.Rejected, m[bad.String()].Status)
	require.NotNil(t, m[bad.String()].Error)
	assert.Equal(t, reasonFKViolation, *m[bad.String()].Error)

	assertRowCount(t, env, ctx, "inspections", good, 1)
	assertRowCount(t, env, ctx, "inspections", bad, 0)
}

// TestBatch_DefaultsInspectorToActor: an inspection/problem that omits inspector_id
// is persisted with the authenticated actor as inspector/creator.
func TestBatch_DefaultsInspectorToActor(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	inspUUID := uuid.New()
	req := oapi.SyncBatchRequest{
		Inspections: &[]oapi.SyncInspectionItem{{
			ClientUuid: inspUUID, ProjectId: env.projectID, StartedAt: time.Now().UTC(),
		}},
	}
	_, err := env.svc.Process(ctx, req, env.adminID)
	require.NoError(t, err)

	var inspectorID int64
	require.NoError(t, env.pool.QueryRow(ctx,
		`SELECT inspector_id FROM inspections WHERE client_uuid=$1`, inspUUID).Scan(&inspectorID))
	assert.Equal(t, env.adminID, inspectorID, "omitted inspector defaults to the acting user")
}

// assertRowCount asserts the number of rows for a client_uuid in a table.
func assertRowCount(t *testing.T, env *testEnv, ctx context.Context, table string, clientUUID uuid.UUID, want int) {
	t.Helper()
	var n int
	require.NoError(t, env.pool.QueryRow(ctx,
		`SELECT count(*) FROM `+table+` WHERE client_uuid = $1`, clientUUID).Scan(&n))
	assert.Equalf(t, want, n, "row count for %s in %s", clientUUID, table)
}
