//go:build integration

package problem

import (
	"context"
	"testing"
	"time"

	"github.com/paulmach/orb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	migrations "github.com/nnkglobal/c5-backend/db/migrations"
	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
	"github.com/nnkglobal/c5-backend/internal/platform/geo"
)

// testEnv bundles the migrated pool, service, seeded admin id, a fresh project,
// an inspection, and the seeded problem dict_item ids the tests reference.
type testEnv struct {
	pool      *db.Pool
	svc       *Service
	adminID   int64
	projectID int64
	inspID    int64

	// Seeded dict_item ids (000002): problem_type ROAD, problem_status OPEN/PROCESSING.
	typeRoadID      int64
	statusOpenID    int64
	statusProcessID int64
}

// newTestEnv spins a fresh PostGIS container, runs all migrations (000002 seeds
// the admin user + problem dicts), connects a pool, and inserts prerequisite rows
// (a project, an inspection) directly via SQL — mirroring what other domains own.
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

	e := &testEnv{pool: pool, svc: NewService(pool)}

	require.NoError(t, pool.QueryRow(ctx, "SELECT id FROM users WHERE username='admin'").Scan(&e.adminID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO projects (code, name) VALUES ('P-PROB','problem test') RETURNING id`).Scan(&e.projectID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO inspections (project_id, inspector_id, started_at) VALUES ($1,$2, now()) RETURNING id`,
		e.projectID, e.adminID).Scan(&e.inspID))

	// Resolve seeded dict_item ids by code so tests do not hardcode serial ids.
	e.typeRoadID = e.dictItemID(t, ctx, "problem_type", "ROAD")
	e.statusOpenID = e.dictItemID(t, ctx, "problem_status", "OPEN")
	e.statusProcessID = e.dictItemID(t, ctx, "problem_status", "PROCESSING")
	return e
}

// dictItemID looks up a seeded dict_item id by its dict_type.code + item code.
func (e *testEnv) dictItemID(t *testing.T, ctx context.Context, typeCode, itemCode string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, e.pool.QueryRow(ctx, `
		SELECT di.id FROM dict_item di
		JOIN dict_type dt ON dt.id = di.dict_type_id
		WHERE dt.code = $1 AND di.code = $2`, typeCode, itemCode).Scan(&id))
	return id
}

// retireDictItem flips a dict_item to is_active=false (the "retired" state). The
// item keeps its id, so problems referencing it via soft FK stay valid (history).
func (e *testEnv) retireDictItem(t *testing.T, ctx context.Context, id int64) {
	t.Helper()
	_, err := e.pool.Exec(ctx, `UPDATE dict_item SET is_active = false WHERE id = $1`, id)
	require.NoError(t, err)
}

func ptrI64(v int64) *int64 { return &v }

// baseCreate returns a minimal valid CreateInput at point (113, 23).
func (e *testEnv) baseCreate(t *testing.T) CreateInput {
	t.Helper()
	ewkb, err := geo.EncodePointWGS84(orb.Point{113.0, 23.0})
	require.NoError(t, err)
	return CreateInput{
		ProjectID:    e.projectID,
		GeomEWKB:     ewkb,
		CapturedAt:   time.Now().UTC(),
		InspectorID:  e.adminID,
		InspectionID: ptrI64(e.inspID),
		TypeItemID:   ptrI64(e.typeRoadID),
		StatusItemID: ptrI64(e.statusOpenID),
	}
}

func TestCreate_GeomAndDictVersionPin(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	p, err := e.svc.Create(ctx, e.baseCreate(t), e.adminID)
	require.NoError(t, err)
	require.NotNil(t, p)

	// geom round-trips to WGS84 [lon, lat].
	require.Len(t, p.Geom.Coordinates, 2)
	assert.InDelta(t, 113.0, float64(p.Geom.Coordinates[0]), 1e-4)
	assert.InDelta(t, 23.0, float64(p.Geom.Coordinates[1]), 1e-4)

	// dict_version_used pinned to current problem_type version (seed = 1).
	assert.Equal(t, 1, p.DictVersionUsed)
	require.NotNil(t, p.StatusItemId)
	assert.Equal(t, e.statusOpenID, *p.StatusItemId)
	require.NotNil(t, p.InspectionId)
	assert.Equal(t, e.inspID, *p.InspectionId)
}

func TestCreate_IdempotentOnClientUUID(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	const cu = "22222222-2222-2222-2222-222222222222"
	in := e.baseCreate(t)
	in.ClientUUID = ptrStr(cu)

	first, err := e.svc.Create(ctx, in, e.adminID)
	require.NoError(t, err)

	// Replay with the same client_uuid returns the original row (DO NOTHING).
	second, err := e.svc.Create(ctx, in, e.adminID)
	require.NoError(t, err)
	assert.Equal(t, first.Id, second.Id)

	var count int
	require.NoError(t, e.pool.QueryRow(ctx,
		`SELECT count(*) FROM problems WHERE client_uuid = $1`, cu).Scan(&count))
	assert.Equal(t, 1, count, "client_uuid replay must not create a second row")
}

func ptrStr(s string) *string { return &s }

func TestCreate_AcceptsRetiredDictItem(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	// Retire the type item, then create a problem still referencing it.
	e.retireDictItem(t, ctx, e.typeRoadID)

	in := e.baseCreate(t)
	in.TypeItemID = ptrI64(e.typeRoadID) // retired but not hard-deleted

	p, err := e.svc.Create(ctx, in, e.adminID)
	require.NoError(t, err, "historical tolerance: a retired dict_item must be accepted")
	require.NotNil(t, p.TypeItemId)
	assert.Equal(t, e.typeRoadID, *p.TypeItemId)
}

func TestList_FiltersIncludingInspectionID(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	// A second inspection so inspection_id actually discriminates (D1).
	var insp2 int64
	require.NoError(t, e.pool.QueryRow(ctx,
		`INSERT INTO inspections (project_id, inspector_id, started_at) VALUES ($1,$2, now()) RETURNING id`,
		e.projectID, e.adminID).Scan(&insp2))

	in1 := e.baseCreate(t)
	in1.InspectionID = ptrI64(e.inspID)
	_, err := e.svc.Create(ctx, in1, e.adminID)
	require.NoError(t, err)

	in2 := e.baseCreate(t)
	in2.InspectionID = ptrI64(insp2)
	in2.StatusItemID = ptrI64(e.statusProcessID)
	_, err = e.svc.Create(ctx, in2, e.adminID)
	require.NoError(t, err)

	// Unfiltered: both.
	all, total, err := e.svc.List(ctx, ListFilter{}, 50, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	assert.Len(t, all, 2)

	// D1 — filter by inspection_id: only the first.
	byInsp, total, err := e.svc.List(ctx, ListFilter{InspectionID: ptrI64(e.inspID)}, 50, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, byInsp, 1)
	require.NotNil(t, byInsp[0].InspectionId)
	assert.Equal(t, e.inspID, *byInsp[0].InspectionId)

	// Filter by status_item_id (PROCESSING): only the second.
	byStatus, total, err := e.svc.List(ctx, ListFilter{Status: ptrI64(e.statusProcessID)}, 50, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, byStatus, 1)
	assert.Equal(t, e.statusProcessID, *byStatus[0].StatusItemId)

	// Filter by type (ROAD): both share it.
	_, total, err = e.svc.List(ctx, ListFilter{Type: ptrI64(e.typeRoadID)}, 50, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
}

func TestUpdate_StatusChangeAppendsExactlyOneLog(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	p, err := e.svc.Create(ctx, e.baseCreate(t), e.adminID) // status OPEN
	require.NoError(t, err)

	// D3: change status OPEN -> PROCESSING; expect exactly one STATUS_CHANGE log
	// with the correct from/to and operator, written in the same transaction.
	updated, err := e.svc.Update(ctx, p.Id, UpdateInput{StatusItemID: ptrI64(e.statusProcessID)}, e.adminID)
	require.NoError(t, err)
	require.NotNil(t, updated.StatusItemId)
	assert.Equal(t, e.statusProcessID, *updated.StatusItemId)

	logs, err := e.svc.ListLogs(ctx, p.Id)
	require.NoError(t, err)
	require.Len(t, logs, 1, "exactly one STATUS_CHANGE log must be appended")
	log := logs[0]
	assert.Equal(t, oapi.ProcessingActionSTATUSCHANGE, log.Action)
	require.NotNil(t, log.FromStatusItemId)
	assert.Equal(t, e.statusOpenID, *log.FromStatusItemId)
	require.NotNil(t, log.ToStatusItemId)
	assert.Equal(t, e.statusProcessID, *log.ToStatusItemId)
	require.NotNil(t, log.OperatorId)
	assert.Equal(t, e.adminID, *log.OperatorId)

	// A no-op update (same status) must NOT append another log.
	_, err = e.svc.Update(ctx, p.Id, UpdateInput{StatusItemID: ptrI64(e.statusProcessID)}, e.adminID)
	require.NoError(t, err)
	logs, err = e.svc.ListLogs(ctx, p.Id)
	require.NoError(t, err)
	assert.Len(t, logs, 1, "unchanged status must not append a second log")

	// An update that does not touch status must NOT append a log either.
	_, err = e.svc.Update(ctx, p.Id, UpdateInput{Note: ptrStr("just a note")}, e.adminID)
	require.NoError(t, err)
	logs, err = e.svc.ListLogs(ctx, p.Id)
	require.NoError(t, err)
	assert.Len(t, logs, 1)
}

func TestCreateLog_CommentOKAndStatusChangeRejected(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	p, err := e.svc.Create(ctx, e.baseCreate(t), e.adminID)
	require.NoError(t, err)

	// COMMENT is accepted.
	log, err := e.svc.CreateLog(ctx, p.Id, CreateLogInput{Action: "COMMENT", Note: ptrStr("looks bad")}, e.adminID)
	require.NoError(t, err)
	assert.Equal(t, oapi.ProcessingActionCOMMENT, log.Action)
	require.NotNil(t, log.OperatorId)
	assert.Equal(t, e.adminID, *log.OperatorId)

	// REASSIGN with an explicit operator is accepted.
	logR, err := e.svc.CreateLog(ctx, p.Id, CreateLogInput{Action: "REASSIGN", OperatorID: ptrI64(e.adminID)}, e.adminID)
	require.NoError(t, err)
	assert.Equal(t, oapi.ProcessingActionREASSIGN, logR.Action)

	// STATUS_CHANGE via the client log endpoint is rejected (D3).
	_, err = e.svc.CreateLog(ctx, p.Id, CreateLogInput{Action: "STATUS_CHANGE"}, e.adminID)
	assert.ErrorIs(t, err, ErrInvalidLogAction)

	// Logs against a missing problem -> ErrNotFound.
	_, err = e.svc.CreateLog(ctx, 999999, CreateLogInput{Action: "COMMENT"}, e.adminID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMap_ReturnsValidWGS84FeatureCollection(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	_, err := e.svc.Create(ctx, e.baseCreate(t), e.adminID)
	require.NoError(t, err)

	fc, err := e.svc.Map(ctx, ListFilter{ProjectID: ptrI64(e.projectID)}, 0)
	require.NoError(t, err)
	assert.Equal(t, oapi.FeatureCollection, fc.Type)
	require.Len(t, fc.Features, 1)

	feat := fc.Features[0]
	assert.Equal(t, oapi.Feature, feat.Type)
	assert.Equal(t, oapi.Point, feat.Geometry.Type)
	require.Len(t, feat.Geometry.Coordinates, 2)
	assert.InDelta(t, 113.0, float64(feat.Geometry.Coordinates[0]), 1e-4)
	assert.InDelta(t, 23.0, float64(feat.Geometry.Coordinates[1]), 1e-4)

	// Properties carry the joined dict label (ROAD type) for the map layer.
	require.NotNil(t, feat.Properties.TypeLabel)
	assert.Equal(t, "路面", *feat.Properties.TypeLabel)
	require.NotNil(t, feat.Properties.StatusItemId)
	assert.Equal(t, e.statusOpenID, *feat.Properties.StatusItemId)
}

func TestSoftDeleteThenNotFound(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	p, err := e.svc.Create(ctx, e.baseCreate(t), e.adminID)
	require.NoError(t, err)

	require.NoError(t, e.svc.Delete(ctx, p.Id, e.adminID))

	// Get after delete -> ErrNotFound.
	_, err = e.svc.Get(ctx, p.Id)
	assert.ErrorIs(t, err, ErrNotFound)

	// Deleting again -> ErrNotFound (idempotent soft delete).
	assert.ErrorIs(t, e.svc.Delete(ctx, p.Id, e.adminID), ErrNotFound)

	// Soft-deleted rows are excluded from the list.
	_, total, err := e.svc.List(ctx, ListFilter{}, 50, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 0, total)
}

func TestCreate_MissingProjectRejected(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	in := e.baseCreate(t)
	in.ProjectID = 999999 // no such project (FK RESTRICT)
	_, err := e.svc.Create(ctx, in, e.adminID)
	assert.ErrorIs(t, err, ErrProjectMissing)
}
