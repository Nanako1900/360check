//go:build integration

package inspection

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

// testEnv bundles the migrated pool plus the seeded admin id and a fresh project
// id that inspections can reference.
type testEnv struct {
	pool      *db.Pool
	svc       *Service
	adminID   int64
	projectID int64
}

// newTestEnv spins a fresh PostGIS container, runs all migrations (000002 seeds
// the admin user), connects a pool, and inserts a project to own inspections.
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
		`INSERT INTO projects (code, name) VALUES ('P-INSP','inspection test') RETURNING id`).Scan(&projectID))

	return &testEnv{pool: pool, svc: NewService(pool), adminID: adminID, projectID: projectID}
}

// insertPoint writes a trajectory point directly (mirrors the offline batch that
// P5 will own). geom round-trips through EWKB exactly like the db golden test.
func (e *testEnv) insertPoint(t *testing.T, ctx context.Context, inspID int64, seq int, lon, lat float64) {
	t.Helper()
	ewkb, err := geo.EncodePointWGS84(orb.Point{lon, lat})
	require.NoError(t, err)
	_, err = e.pool.Exec(ctx,
		`INSERT INTO trajectory_points (inspection_id, seq, geom, recorded_at)
		 VALUES ($1, $2, ST_GeomFromEWKB($3), now())`, inspID, seq, ewkb)
	require.NoError(t, err)
}

// startBasic starts an IN_PROGRESS session at a fixed time and returns it.
func (e *testEnv) startBasic(t *testing.T, ctx context.Context, startedAt time.Time) *oapi.Inspection {
	t.Helper()
	insp, err := e.svc.Start(ctx, StartInput{
		ProjectID:   e.projectID,
		StartedAt:   startedAt,
		InspectorID: e.adminID,
	}, e.adminID)
	require.NoError(t, err)
	require.NotNil(t, insp)
	require.Equal(t, oapi.InspectionStatusINPROGRESS, insp.Status)
	return insp
}

// TestFinish_MileageGeography is the golden case: two known points 0.01deg apart
// at 23N -> ~1025.2 m via ST_Length(::geography), within 1%.
func TestFinish_MileageGeography(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	started := time.Date(2026, 6, 26, 8, 0, 0, 0, time.UTC)
	insp := env.startBasic(t, ctx, started)

	env.insertPoint(t, ctx, insp.Id, 1, 113.0, 23.0)
	env.insertPoint(t, ctx, insp.Id, 2, 113.01, 23.0)

	ended := started.Add(10 * time.Minute)
	fin, err := env.svc.Finish(ctx, insp.Id, FinishInput{EndedAt: &ended}, env.adminID)
	require.NoError(t, err)

	assert.Equal(t, oapi.InspectionStatusFINISHED, fin.Status)
	assert.InEpsilon(t, 1025.2, fin.MileageMeters, 0.01, "mileage via ::geography must be meters")
	assert.Equal(t, int64(600), fin.DurationSeconds, "duration = ended-started in seconds")
	assert.Equal(t, 2, fin.PointCount)
	require.NotNil(t, fin.RouteGeom, "route_geom must be built from >= 2 points")
	require.NotNil(t, fin.EndedAt)
	assert.True(t, fin.EndedAt.Equal(ended))
	// Route is a 2-vertex WGS84 LineString [lon,lat].
	require.Len(t, fin.RouteGeom.Coordinates, 2)
	assert.InDelta(t, 113.0, fin.RouteGeom.Coordinates[0][0], 1e-4)
	assert.InDelta(t, 23.0, fin.RouteGeom.Coordinates[0][1], 1e-4)
}

// TestFinish_SinglePoint_NoRoute: a single point cannot form a line -> route NULL,
// mileage 0, point_count 1, no error.
func TestFinish_SinglePoint_NoRoute(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	insp := env.startBasic(t, ctx, time.Now().UTC().Add(-time.Hour))
	env.insertPoint(t, ctx, insp.Id, 1, 113.0, 23.0)

	fin, err := env.svc.Finish(ctx, insp.Id, FinishInput{}, env.adminID)
	require.NoError(t, err)
	assert.Equal(t, oapi.InspectionStatusFINISHED, fin.Status)
	assert.Zero(t, fin.MileageMeters)
	assert.Equal(t, 1, fin.PointCount)
	assert.Nil(t, fin.RouteGeom, "single point -> NULL route")
}

// TestFinish_ZeroPoints_NoRoute: no points -> route NULL, mileage 0, count 0.
func TestFinish_ZeroPoints_NoRoute(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	insp := env.startBasic(t, ctx, time.Now().UTC().Add(-time.Hour))

	fin, err := env.svc.Finish(ctx, insp.Id, FinishInput{}, env.adminID)
	require.NoError(t, err)
	assert.Equal(t, oapi.InspectionStatusFINISHED, fin.Status)
	assert.Zero(t, fin.MileageMeters)
	assert.Equal(t, 0, fin.PointCount)
	assert.Nil(t, fin.RouteGeom)
}

// TestFinish_RepeatedFinish_Conflict: finishing an already-FINISHED session is an
// illegal transition -> ErrNotInProgress (handler maps to 409 CONFLICT).
func TestFinish_RepeatedFinish_Conflict(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	insp := env.startBasic(t, ctx, time.Now().UTC().Add(-time.Hour))
	_, err := env.svc.Finish(ctx, insp.Id, FinishInput{}, env.adminID)
	require.NoError(t, err)

	_, err = env.svc.Finish(ctx, insp.Id, FinishInput{}, env.adminID)
	require.ErrorIs(t, err, ErrNotInProgress)
}

// TestFinish_EndedBeforeStarted_Validation: ended_at < started_at -> validation error.
func TestFinish_EndedBeforeStarted_Validation(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	started := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	insp := env.startBasic(t, ctx, started)

	before := started.Add(-time.Minute)
	_, err := env.svc.Finish(ctx, insp.Id, FinishInput{EndedAt: &before}, env.adminID)
	require.ErrorIs(t, err, ErrEndedBeforeStarted)

	// The session stays IN_PROGRESS (the failed finish was rolled back).
	got, err := env.svc.Get(ctx, insp.Id)
	require.NoError(t, err)
	assert.Equal(t, oapi.InspectionStatusINPROGRESS, got.Status)
}

// TestFinish_Discard_Abandoned: Status=ABANDONED discards the session and still
// computes available metrics.
func TestFinish_Discard_Abandoned(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	started := time.Now().UTC().Add(-30 * time.Minute)
	insp := env.startBasic(t, ctx, started)
	env.insertPoint(t, ctx, insp.Id, 1, 113.0, 23.0)
	env.insertPoint(t, ctx, insp.Id, 2, 113.01, 23.0)

	fin, err := env.svc.Finish(ctx, insp.Id, FinishInput{Discard: true}, env.adminID)
	require.NoError(t, err)
	assert.Equal(t, oapi.InspectionStatusABANDONED, fin.Status)
	assert.Equal(t, 2, fin.PointCount, "metrics still computed on discard")
	assert.InEpsilon(t, 1025.2, fin.MileageMeters, 0.01)
	require.NotNil(t, fin.RouteGeom)
}

// TestStart_Idempotent_ClientUUID: replaying start with the same client_uuid
// returns the original session (no duplicate row).
func TestStart_Idempotent_ClientUUID(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	cu := "22222222-2222-2222-2222-222222222222"
	started := time.Now().UTC()

	first, err := env.svc.Start(ctx, StartInput{
		ProjectID: env.projectID, StartedAt: started, InspectorID: env.adminID, ClientUUID: &cu,
	}, env.adminID)
	require.NoError(t, err)

	// Replay with a different started_at — must return the original row unchanged.
	second, err := env.svc.Start(ctx, StartInput{
		ProjectID: env.projectID, StartedAt: started.Add(time.Hour), InspectorID: env.adminID, ClientUUID: &cu,
	}, env.adminID)
	require.NoError(t, err)

	assert.Equal(t, first.Id, second.Id, "same client_uuid -> same session")
	assert.True(t, first.StartedAt.Equal(second.StartedAt), "replay must not mutate started_at")

	var count int
	require.NoError(t, env.pool.QueryRow(ctx,
		`SELECT count(*) FROM inspections WHERE client_uuid = $1`, cu).Scan(&count))
	assert.Equal(t, 1, count, "exactly one row for the client_uuid")
}

// TestStart_DefaultsInspectorToActor: the handler-resolved inspector is persisted.
func TestStart_DefaultsInspector(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	insp := env.startBasic(t, ctx, time.Now().UTC())
	assert.Equal(t, env.adminID, insp.InspectorId)
}

// TestTrajectory_OrderedPointsAndRoute: trajectory returns points ordered by seq
// plus the finalized route, all WGS84.
func TestTrajectory_OrderedPointsAndRoute(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	started := time.Now().UTC().Add(-time.Hour)
	insp := env.startBasic(t, ctx, started)
	// Insert out of natural order to prove ORDER BY seq.
	env.insertPoint(t, ctx, insp.Id, 2, 113.01, 23.0)
	env.insertPoint(t, ctx, insp.Id, 1, 113.0, 23.0)

	ended := started.Add(5 * time.Minute)
	_, err := env.svc.Finish(ctx, insp.Id, FinishInput{EndedAt: &ended}, env.adminID)
	require.NoError(t, err)

	traj, err := env.svc.Trajectory(ctx, insp.Id)
	require.NoError(t, err)
	assert.Equal(t, insp.Id, traj.InspectionId)
	require.Len(t, traj.Points, 2)
	assert.Equal(t, 1, traj.Points[0].Seq)
	assert.Equal(t, 2, traj.Points[1].Seq)
	// First point is (113.0, 23.0) in [lon,lat].
	assert.InDelta(t, 113.0, traj.Points[0].Geom.Coordinates[0], 1e-4)
	assert.InDelta(t, 23.0, traj.Points[0].Geom.Coordinates[1], 1e-4)
	require.NotNil(t, traj.Route)
	assert.Len(t, traj.Route.Coordinates, 2)
}

// TestTrajectory_NotFound: unknown inspection -> ErrNotFound.
func TestTrajectory_NotFound(t *testing.T) {
	env := newTestEnv(t)
	_, err := env.svc.Trajectory(context.Background(), 999999)
	require.ErrorIs(t, err, ErrNotFound)
}

// TestGet_NotFound: unknown / soft-deleted -> ErrNotFound.
func TestGet_NotFound(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	insp := env.startBasic(t, ctx, time.Now().UTC())
	_, err := env.pool.Exec(ctx, `UPDATE inspections SET deleted_at = now() WHERE id = $1`, insp.Id)
	require.NoError(t, err)

	_, err = env.svc.Get(ctx, insp.Id)
	require.ErrorIs(t, err, ErrNotFound)
}

// TestList_Filters: status/inspector/project/time filters narrow the page, and
// soft-deleted sessions are excluded; meta total matches.
func TestList_Filters(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// Three sessions: one IN_PROGRESS, one FINISHED, one soft-deleted.
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	a := env.startBasic(t, ctx, t0)                   // IN_PROGRESS
	b := env.startBasic(t, ctx, t0.Add(24*time.Hour)) // -> FINISHED
	_, err := env.svc.Finish(ctx, b.Id, FinishInput{}, env.adminID)
	require.NoError(t, err)
	c := env.startBasic(t, ctx, t0.Add(48*time.Hour)) // -> soft-deleted
	_, err = env.pool.Exec(ctx, `UPDATE inspections SET deleted_at = now() WHERE id = $1`, c.Id)
	require.NoError(t, err)

	// Unfiltered: two live sessions (deleted excluded).
	all, total, err := env.svc.List(ctx, ListFilter{ProjectID: &env.projectID}, 50, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	assert.Len(t, all, 2)

	// Status filter -> only the IN_PROGRESS one.
	inProgress := statusInProgress
	live, total, err := env.svc.List(ctx, ListFilter{ProjectID: &env.projectID, Status: &inProgress}, 50, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, live, 1)
	assert.Equal(t, a.Id, live[0].Id)

	// Inspector filter (admin) -> both live sessions.
	_, total, err = env.svc.List(ctx, ListFilter{InspectorID: &env.adminID}, 50, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)

	// Time window [t0, t0+12h) captures only session a.
	to := t0.Add(12 * time.Hour)
	windowed, total, err := env.svc.List(ctx, ListFilter{From: &t0, To: &to}, 50, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, windowed, 1)
	assert.Equal(t, a.Id, windowed[0].Id)
}

// TestStart_ProjectMissing: a non-existent project_id surfaces ErrProjectMissing
// (FK RESTRICT) rather than a 500.
func TestStart_ProjectMissing(t *testing.T) {
	env := newTestEnv(t)
	_, err := env.svc.Start(context.Background(), StartInput{
		ProjectID: 999999, StartedAt: time.Now().UTC(), InspectorID: env.adminID,
	}, env.adminID)
	require.ErrorIs(t, err, ErrProjectMissing)
}

// TestStart_TaskMissing: a bad task_id is classified as ErrTaskMissing, NOT
// mislabeled as a missing project (the FK granularity fix).
func TestStart_TaskMissing(t *testing.T) {
	env := newTestEnv(t)
	badTask := int64(999999)
	_, err := env.svc.Start(context.Background(), StartInput{
		ProjectID: env.projectID, StartedAt: time.Now().UTC(), InspectorID: env.adminID, TaskID: &badTask,
	}, env.adminID)
	require.ErrorIs(t, err, ErrTaskMissing)
	require.NotErrorIs(t, err, ErrProjectMissing)
}

// TestStart_InspectorMissing: a bad inspector_id is classified as
// ErrInspectorMissing, NOT mislabeled as a missing project.
func TestStart_InspectorMissing(t *testing.T) {
	env := newTestEnv(t)
	_, err := env.svc.Start(context.Background(), StartInput{
		ProjectID: env.projectID, StartedAt: time.Now().UTC(), InspectorID: 999999,
	}, env.adminID)
	require.ErrorIs(t, err, ErrInspectorMissing)
	require.NotErrorIs(t, err, ErrProjectMissing)
}
