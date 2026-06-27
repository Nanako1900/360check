//go:build integration

package stats

import (
	"context"
	"testing"
	"time"

	"github.com/paulmach/orb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	migrations "github.com/nnkglobal/c5-backend/db/migrations"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
	"github.com/nnkglobal/c5-backend/internal/platform/geo"
)

// statsEnv bundles the migrated pool + service plus the seeded ids the assertions
// reference (admin, two projects, two inspections, and the dict items used).
type statsEnv struct {
	pool *db.Pool
	svc  *Service

	adminID    int64
	otherUser  int64
	projectA   int64
	projectB   int64
	inspA1     int64
	typeRoadID int64
	statusOpen int64
}

func newStatsEnv(t *testing.T) *statsEnv {
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

	e := &statsEnv{pool: pool, svc: NewService(pool)}
	require.NoError(t, pool.QueryRow(ctx, "SELECT id FROM users WHERE username='admin'").Scan(&e.adminID))

	// A second user (display_name set, to assert the inspector label).
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO users (username, password_hash, display_name, is_active)
		 VALUES ('lisi', 'x', '李四', true) RETURNING id`).Scan(&e.otherUser))

	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO projects (code, name) VALUES ('PA','项目甲') RETURNING id`).Scan(&e.projectA))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO projects (code, name) VALUES ('PB','项目乙') RETURNING id`).Scan(&e.projectB))

	e.typeRoadID = dictID(t, ctx, pool, "problem_type", "ROAD")
	e.statusOpen = dictID(t, ctx, pool, "problem_status", "OPEN")
	return e
}

func dictID(t *testing.T, ctx context.Context, pool *db.Pool, typeCode, itemCode string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT di.id FROM dict_item di JOIN dict_type dt ON dt.id = di.dict_type_id
		WHERE dt.code = $1 AND di.code = $2`, typeCode, itemCode).Scan(&id))
	return id
}

// insertInspection inserts a finished inspection with explicit mileage/duration.
func (e *statsEnv) insertInspection(t *testing.T, ctx context.Context, project, inspector int64, started time.Time, durationS int64, mileageM float64) int64 {
	t.Helper()
	var id int64
	require.NoError(t, e.pool.QueryRow(ctx, `
		INSERT INTO inspections
			(project_id, inspector_id, status, started_at, ended_at, duration_seconds, mileage_meters, point_count)
		VALUES ($1,$2,'FINISHED',$3::timestamptz,
		        $3::timestamptz + make_interval(secs => $4::int),$4::bigint,$5::double precision,0)
		RETURNING id`, project, inspector, started, durationS, mileageM).Scan(&id))
	return id
}

// insertProblem inserts a problem at (113,23) with the given type/status/captured.
func (e *statsEnv) insertProblem(t *testing.T, ctx context.Context, project, inspector, typeID, statusID int64, captured time.Time) {
	t.Helper()
	ewkb, err := geo.EncodePointWGS84(orb.Point{113.0, 23.0})
	require.NoError(t, err)
	_, err = e.pool.Exec(ctx, `
		INSERT INTO problems
			(project_id, inspector_id, geom, type_item_id, status_item_id, captured_at)
		VALUES ($1,$2,ST_GeomFromEWKB($3),$4,$5,$6)`,
		project, inspector, ewkb, typeID, statusID, captured)
	require.NoError(t, err)
}

func TestOverview_D2Aggregates(t *testing.T) {
	e := newStatsEnv(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	// Project A: 2 inspections (admin + lisi), mileage 1000 + 2000, duration 600 + 1200.
	e.inspA1 = e.insertInspection(t, ctx, e.projectA, e.adminID, base, 600, 1000)
	e.insertInspection(t, ctx, e.projectA, e.otherUser, base.Add(time.Hour), 1200, 2000)
	// Project B: 1 inspection (admin), mileage 500, duration 300.
	e.insertInspection(t, ctx, e.projectB, e.adminID, base.Add(2*time.Hour), 300, 500)

	// Problems: 3 in A (2 admin OPEN/ROAD, 1 lisi OPEN/ROAD), 1 in B (admin OPEN/ROAD).
	e.insertProblem(t, ctx, e.projectA, e.adminID, e.typeRoadID, e.statusOpen, base)
	e.insertProblem(t, ctx, e.projectA, e.adminID, e.typeRoadID, e.statusOpen, base)
	e.insertProblem(t, ctx, e.projectA, e.otherUser, e.typeRoadID, e.statusOpen, base)
	e.insertProblem(t, ctx, e.projectB, e.adminID, e.typeRoadID, e.statusOpen, base)

	ov, err := e.svc.Overview(ctx, Filter{})
	require.NoError(t, err)

	// Totals: 3 inspections, 4 problems.
	assert.EqualValues(t, 3, ov.InspectionCount)
	assert.EqualValues(t, 4, ov.ProblemCount)
	// Mileage SUM = 1000+2000+500 = 3500.
	assert.InDelta(t, 3500.0, ov.TotalMileageMeters, 1e-9)
	// Duration SUM = 600+1200+300 = 2100; AVG = 700.
	assert.EqualValues(t, 2100, ov.TotalDurationSeconds)
	assert.InDelta(t, 700.0, ov.AvgDurationSeconds, 1e-9)

	// counts_by_type: all 4 problems are ROAD.
	require.Len(t, ov.CountsByType, 1)
	assert.EqualValues(t, 4, ov.CountsByType[0].Count)
	require.NotNil(t, ov.CountsByType[0].ItemId)
	assert.Equal(t, e.typeRoadID, *ov.CountsByType[0].ItemId)
	assert.Equal(t, "路面", ov.CountsByType[0].Label)
	require.NotNil(t, ov.CountsByType[0].Color)
	assert.Equal(t, "#EF4444", *ov.CountsByType[0].Color)

	// counts_by_status: all 4 OPEN.
	require.Len(t, ov.CountsByStatus, 1)
	assert.EqualValues(t, 4, ov.CountsByStatus[0].Count)
	assert.Equal(t, "待处理", ov.CountsByStatus[0].Label)

	// counts_by_inspector: admin=3, lisi=1 (ordered by count DESC).
	require.Len(t, ov.CountsByInspector, 2)
	assert.EqualValues(t, 3, ov.CountsByInspector[0].Count)
	require.NotNil(t, ov.CountsByInspector[0].InspectorId)
	assert.Equal(t, e.adminID, *ov.CountsByInspector[0].InspectorId)
	assert.EqualValues(t, 1, ov.CountsByInspector[1].Count)
	assert.Equal(t, "李四", ov.CountsByInspector[1].Label)

	// counts_by_project: A=3, B=1.
	require.Len(t, ov.CountsByProject, 2)
	assert.EqualValues(t, 3, ov.CountsByProject[0].Count)
	require.NotNil(t, ov.CountsByProject[0].ProjectId)
	assert.Equal(t, e.projectA, *ov.CountsByProject[0].ProjectId)
	assert.Equal(t, "项目甲", ov.CountsByProject[0].Label)
}

func TestOverview_FilterByProjectAndInspector(t *testing.T) {
	e := newStatsEnv(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	e.insertInspection(t, ctx, e.projectA, e.adminID, base, 600, 1000)
	e.insertInspection(t, ctx, e.projectB, e.adminID, base, 300, 500)
	e.insertProblem(t, ctx, e.projectA, e.adminID, e.typeRoadID, e.statusOpen, base)
	e.insertProblem(t, ctx, e.projectB, e.adminID, e.typeRoadID, e.statusOpen, base)

	// Filter to project A only.
	ov, err := e.svc.Overview(ctx, Filter{ProjectID: &e.projectA})
	require.NoError(t, err)
	assert.EqualValues(t, 1, ov.InspectionCount)
	assert.EqualValues(t, 1, ov.ProblemCount)
	assert.InDelta(t, 1000.0, ov.TotalMileageMeters, 1e-9)
	require.Len(t, ov.CountsByProject, 1)
	assert.Equal(t, e.projectA, *ov.CountsByProject[0].ProjectId)
}

func TestOverview_FilterByTimeWindow(t *testing.T) {
	e := newStatsEnv(t)
	ctx := context.Background()
	early := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)

	e.insertInspection(t, ctx, e.projectA, e.adminID, early, 600, 1000)
	e.insertInspection(t, ctx, e.projectA, e.adminID, late, 300, 500)
	e.insertProblem(t, ctx, e.projectA, e.adminID, e.typeRoadID, e.statusOpen, early)
	e.insertProblem(t, ctx, e.projectA, e.adminID, e.typeRoadID, e.statusOpen, late)

	from := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	ov, err := e.svc.Overview(ctx, Filter{From: &from, To: &to})
	require.NoError(t, err)

	// Only the late inspection/problem fall in [from, to).
	assert.EqualValues(t, 1, ov.InspectionCount)
	assert.EqualValues(t, 1, ov.ProblemCount)
	assert.InDelta(t, 500.0, ov.TotalMileageMeters, 1e-9)
}

func TestOverview_RetiredDictLabelStillAppears(t *testing.T) {
	e := newStatsEnv(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	e.insertProblem(t, ctx, e.projectA, e.adminID, e.typeRoadID, e.statusOpen, base)

	// Retire the ROAD dict item: is_active=false (kept for history, soft FK intact).
	_, err := e.pool.Exec(ctx, `UPDATE dict_item SET is_active = false WHERE id = $1`, e.typeRoadID)
	require.NoError(t, err)

	ov, err := e.svc.Overview(ctx, Filter{})
	require.NoError(t, err)

	// The retired type still produces a labeled bucket (historical readability).
	require.Len(t, ov.CountsByType, 1)
	assert.Equal(t, "路面", ov.CountsByType[0].Label, "retired dict label must still resolve")
	assert.EqualValues(t, 1, ov.CountsByType[0].Count)
}

func TestProjectsForExport_MatchesOverview(t *testing.T) {
	e := newStatsEnv(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	e.insertInspection(t, ctx, e.projectA, e.adminID, base, 600, 1000)
	e.insertInspection(t, ctx, e.projectB, e.adminID, base, 300, 500)
	e.insertProblem(t, ctx, e.projectA, e.adminID, e.typeRoadID, e.statusOpen, base)
	e.insertProblem(t, ctx, e.projectA, e.adminID, e.typeRoadID, e.statusOpen, base)
	e.insertProblem(t, ctx, e.projectB, e.adminID, e.typeRoadID, e.statusOpen, base)

	rows, err := e.svc.ProjectsForExport(ctx, Filter{})
	require.NoError(t, err)
	require.Len(t, rows, 2)

	// Each ProjectRow's Overview must equal a direct project-scoped Overview call.
	for _, r := range rows {
		want, err := e.svc.Overview(ctx, Filter{ProjectID: &r.ProjectID})
		require.NoError(t, err)
		assert.Equal(t, want.InspectionCount, r.Overview.InspectionCount, "project %d inspection_count", r.ProjectID)
		assert.Equal(t, want.ProblemCount, r.Overview.ProblemCount, "project %d problem_count", r.ProjectID)
		assert.InDelta(t, want.TotalMileageMeters, r.Overview.TotalMileageMeters, 1e-9)
		assert.Equal(t, want.TotalDurationSeconds, r.Overview.TotalDurationSeconds)
	}
}
