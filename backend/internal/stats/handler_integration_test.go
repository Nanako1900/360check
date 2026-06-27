//go:build integration

package stats

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/paulmach/orb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/geo"
)

// newRouter mounts the real stats handler (over a live service) on a bare gin
// engine, returning a ServeHTTP-able router for end-to-end envelope assertions.
func (e *statsEnv) newRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewHandler(e.svc).RegisterRoutes(r)
	return r
}

// statsEnvelope is the standard wrapper specialized to the StatsOverview payload.
type statsEnvelope struct {
	Success bool                `json:"success"`
	Data    *oapi.StatsOverview `json:"data"`
	Error   *oapi.ErrorObject   `json:"error"`
	Meta    *oapi.Meta          `json:"meta"`
}

// TestHandlerOverview_HTTPSuccessEnvelope drives the full HTTP path: real router,
// real service, seeded data. It asserts the success envelope (success=true, data
// populated, error/meta null) AND that the D2 aggregates surface end-to-end — this
// is the only test that exercises Handler.Overview's success branch + httpx.OK.
func TestHandlerOverview_HTTPSuccessEnvelope(t *testing.T) {
	e := newStatsEnv(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	e.insertInspection(t, ctx, e.projectA, e.adminID, base, 600, 1000)
	e.insertProblem(t, ctx, e.projectA, e.adminID, e.typeRoadID, e.statusOpen, base)
	e.insertProblem(t, ctx, e.projectA, e.adminID, e.typeRoadID, e.statusOpen, base)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stats/overview", nil)
	e.newRouter().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env statsEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))

	// Standard envelope: success true, data present, error/meta null.
	assert.True(t, env.Success)
	assert.Nil(t, env.Error)
	assert.Nil(t, env.Meta)
	require.NotNil(t, env.Data)

	// D2 shape surfaced through the handler.
	assert.EqualValues(t, 1, env.Data.InspectionCount)
	assert.EqualValues(t, 2, env.Data.ProblemCount)
	assert.InDelta(t, 1000.0, env.Data.TotalMileageMeters, 1e-9)
	require.Len(t, env.Data.CountsByType, 1)
	assert.EqualValues(t, 2, env.Data.CountsByType[0].Count)
}

// TestHandlerOverview_HTTPFiltered confirms the handler threads parsed filters
// (project_id + RFC3339 from/to) into the service: only project A's single
// in-window inspection/problem are counted.
func TestHandlerOverview_HTTPFiltered(t *testing.T) {
	e := newStatsEnv(t)
	ctx := context.Background()
	early := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)

	e.insertInspection(t, ctx, e.projectA, e.adminID, late, 300, 500)
	e.insertInspection(t, ctx, e.projectB, e.adminID, late, 999, 9999)                // excluded by project_id
	e.insertProblem(t, ctx, e.projectA, e.adminID, e.typeRoadID, e.statusOpen, early) // excluded by from
	e.insertProblem(t, ctx, e.projectA, e.adminID, e.typeRoadID, e.statusOpen, late)

	w := httptest.NewRecorder()
	url := "/stats/overview?project_id=" + itoa(e.projectA) +
		"&from=2026-06-10T00:00:00Z&to=2026-06-30T00:00:00Z"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	e.newRouter().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env statsEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.NotNil(t, env.Data)
	assert.EqualValues(t, 1, env.Data.InspectionCount, "only project A, in-window")
	assert.EqualValues(t, 1, env.Data.ProblemCount, "only the late problem")
	assert.InDelta(t, 500.0, env.Data.TotalMileageMeters, 1e-9)
}

// TestHandlerOverview_HTTPBadTimestamp asserts a malformed `to` produces a 422
// VALIDATION_FAILED envelope through the live router (end-to-end parse rejection).
func TestHandlerOverview_HTTPBadTimestamp(t *testing.T) {
	e := newStatsEnv(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stats/overview?to=not-a-timestamp", nil)
	e.newRouter().ServeHTTP(w, req)

	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	var env statsEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.False(t, env.Success)
	require.NotNil(t, env.Error)
	assert.Equal(t, oapi.VALIDATIONFAILED, env.Error.Code)
	assert.Nil(t, env.Data)
}

// itoa is a tiny local int64->string for URL building (avoids importing strconv
// for a single call site in the integration build).
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// TestOverview_EmptyDatasetIsZeroed asserts an empty/no-match dataset yields a
// well-formed zeroed Overview (no error): empty count slices (never nil, so JSON
// is []) and COALESCE'd numeric zeros for the mileage/duration roll-ups.
func TestOverview_EmptyDatasetIsZeroed(t *testing.T) {
	e := newStatsEnv(t)
	ctx := context.Background()

	ov, err := e.svc.Overview(ctx, Filter{})
	require.NoError(t, err)

	assert.EqualValues(t, 0, ov.InspectionCount)
	assert.EqualValues(t, 0, ov.ProblemCount)
	assert.EqualValues(t, 0, ov.TotalMileageMeters)
	assert.EqualValues(t, 0, ov.TotalDurationSeconds)
	assert.EqualValues(t, 0, ov.AvgDurationSeconds)

	// Empty (non-nil) slices so the JSON arrays serialize as [] not null.
	require.NotNil(t, ov.CountsByType)
	require.NotNil(t, ov.CountsByStatus)
	require.NotNil(t, ov.CountsByInspector)
	require.NotNil(t, ov.CountsByProject)
	assert.Empty(t, ov.CountsByType)
	assert.Empty(t, ov.CountsByStatus)
	assert.Empty(t, ov.CountsByInspector)
	assert.Empty(t, ov.CountsByProject)
}

// TestOverview_FilterMatchesNothingIsZeroed is the non-empty-DB analogue: data
// exists, but a filter window in the future matches none of it. The result must be
// zeroed/empty, not an error (the filters bind, the aggregates just return no rows).
func TestOverview_FilterMatchesNothingIsZeroed(t *testing.T) {
	e := newStatsEnv(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	e.insertInspection(t, ctx, e.projectA, e.adminID, base, 600, 1000)
	e.insertProblem(t, ctx, e.projectA, e.adminID, e.typeRoadID, e.statusOpen, base)

	// Window entirely after the seeded data.
	from := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC)
	ov, err := e.svc.Overview(ctx, Filter{From: &from, To: &to})
	require.NoError(t, err)

	assert.EqualValues(t, 0, ov.InspectionCount)
	assert.EqualValues(t, 0, ov.ProblemCount)
	assert.EqualValues(t, 0, ov.TotalMileageMeters)
	assert.Empty(t, ov.CountsByType)
	assert.Empty(t, ov.CountsByProject)
}

// TestOverview_FilterByInspectorOnly isolates the inspector_id filter branch
// (untested in isolation by the existing suite): two inspectors, filter to one.
func TestOverview_FilterByInspectorOnly(t *testing.T) {
	e := newStatsEnv(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	e.insertInspection(t, ctx, e.projectA, e.adminID, base, 600, 1000)
	e.insertInspection(t, ctx, e.projectA, e.otherUser, base, 1200, 2000)
	e.insertProblem(t, ctx, e.projectA, e.adminID, e.typeRoadID, e.statusOpen, base)
	e.insertProblem(t, ctx, e.projectA, e.adminID, e.typeRoadID, e.statusOpen, base)
	e.insertProblem(t, ctx, e.projectA, e.otherUser, e.typeRoadID, e.statusOpen, base)

	ov, err := e.svc.Overview(ctx, Filter{InspectorID: &e.adminID})
	require.NoError(t, err)

	assert.EqualValues(t, 1, ov.InspectionCount, "only admin's inspection")
	assert.InDelta(t, 1000.0, ov.TotalMileageMeters, 1e-9)
	assert.EqualValues(t, 2, ov.ProblemCount, "only admin's two problems")
	require.Len(t, ov.CountsByInspector, 1)
	require.NotNil(t, ov.CountsByInspector[0].InspectorId)
	assert.Equal(t, e.adminID, *ov.CountsByInspector[0].InspectorId)
}

// TestOverview_InspectorLabelFallsBackToUsername asserts countByInspector labels a
// user whose display_name is empty with the username (the COALESCE(NULLIF(...))
// branch), complementing the existing display_name ("李四") assertion. A dedicated
// user with display_name=” is created so the NULLIF→username path actually runs.
func TestOverview_InspectorLabelFallsBackToUsername(t *testing.T) {
	e := newStatsEnv(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	var noNameUser int64
	require.NoError(t, e.pool.QueryRow(ctx,
		`INSERT INTO users (username, password_hash, display_name, is_active)
		 VALUES ('wangwu', 'x', '', true) RETURNING id`).Scan(&noNameUser))

	e.insertProblem(t, ctx, e.projectA, noNameUser, e.typeRoadID, e.statusOpen, base)

	ov, err := e.svc.Overview(ctx, Filter{InspectorID: &noNameUser})
	require.NoError(t, err)
	require.Len(t, ov.CountsByInspector, 1)
	assert.Equal(t, "wangwu", ov.CountsByInspector[0].Label,
		"empty display_name falls back to username")
}

// TestOverview_MileageIsMetersNotDegrees is the mileage-semantics guard. It stores
// a mileage_meters value computed by PostGIS itself — ST_Length(line::geography)
// for a known ~1° span at the equator — then asserts the stats SUM returns that
// metric value (≈111 km), NOT the degree-space length (~1.0). This proves the
// roll-up carries meters end-to-end. The expected meters come from the same
// geography cast the inspection-finish path uses, so the two stay consistent.
func TestOverview_MileageIsMetersNotDegrees(t *testing.T) {
	e := newStatsEnv(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	// A two-vertex line from (113,23) to (114,23): ~1° of longitude near lat 23.
	// Compute its true metric length the same way Finish() does:
	// ST_Length(ST_MakeLine(...)::geography).
	var meters float64
	require.NoError(t, e.pool.QueryRow(ctx, `
		SELECT ST_Length(
			ST_SetSRID(ST_MakeLine(ST_MakePoint(113,23), ST_MakePoint(114,23)), 4326)::geography
		)`).Scan(&meters))

	// Sanity: a metric length is on the order of 10^5 m, NOT the ~1.0 a degree-space
	// ST_Length would yield. This is the core "meters, not degrees" assertion.
	require.Greater(t, meters, 100_000.0, "geography length must be ~111 km, not ~1 degree")
	require.Less(t, meters, 120_000.0)

	// Store it as the inspection's mileage and confirm the stats SUM echoes it.
	e.insertInspection(t, ctx, e.projectA, e.adminID, base, 600, meters)

	ov, err := e.svc.Overview(ctx, Filter{})
	require.NoError(t, err)
	assert.InDelta(t, meters, ov.TotalMileageMeters, 1e-6,
		"stats SUM(mileage_meters) must return the stored metric value")

	// And it is decisively metric-scale (would be ~1.0 if degrees leaked through).
	assert.Greater(t, ov.TotalMileageMeters, 100_000.0)
}

// TestOverview_MultipleProjectsOrderedByCount seeds three projects with distinct
// problem counts and asserts counts_by_project is ordered by count DESC and each
// bucket is labeled with the project name — covering the multi-project ordering
// path of countByProject beyond the existing two-project case.
func TestOverview_MultipleProjectsOrderedByCount(t *testing.T) {
	e := newStatsEnv(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	var projectC int64
	require.NoError(t, e.pool.QueryRow(ctx,
		`INSERT INTO projects (code, name) VALUES ('PC','项目丙') RETURNING id`).Scan(&projectC))

	// A: 3 problems, B: 1, C: 2 -> expected order A(3), C(2), B(1).
	for i := 0; i < 3; i++ {
		e.insertProblem(t, ctx, e.projectA, e.adminID, e.typeRoadID, e.statusOpen, base)
	}
	e.insertProblem(t, ctx, e.projectB, e.adminID, e.typeRoadID, e.statusOpen, base)
	for i := 0; i < 2; i++ {
		e.insertProblem(t, ctx, projectC, e.adminID, e.typeRoadID, e.statusOpen, base)
	}

	ov, err := e.svc.Overview(ctx, Filter{})
	require.NoError(t, err)
	require.Len(t, ov.CountsByProject, 3)

	assert.EqualValues(t, 3, ov.CountsByProject[0].Count)
	assert.Equal(t, e.projectA, *ov.CountsByProject[0].ProjectId)
	assert.Equal(t, "项目甲", ov.CountsByProject[0].Label)
	assert.EqualValues(t, 2, ov.CountsByProject[1].Count)
	assert.Equal(t, projectC, *ov.CountsByProject[1].ProjectId)
	assert.EqualValues(t, 1, ov.CountsByProject[2].Count)
	assert.Equal(t, e.projectB, *ov.CountsByProject[2].ProjectId)
}

// TestOverview_NullDictColumnSkipped covers countByDict's "NULL dimension is
// skipped" branch: a problem with a NULL status_item_id must not produce a
// counts_by_status bucket (the JOIN drops it), while a sibling with a real status
// still counts. Asserts the NULL row neither errors nor inflates the totals.
func TestOverview_NullDictColumnSkipped(t *testing.T) {
	e := newStatsEnv(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	// One problem with a real OPEN status.
	e.insertProblem(t, ctx, e.projectA, e.adminID, e.typeRoadID, e.statusOpen, base)
	// One problem with a NULL status_item_id (allowed by schema; skipped by the JOIN).
	require.NoError(t, e.insertProblemNullStatus(ctx, e.projectA, e.adminID, e.typeRoadID, base))

	ov, err := e.svc.Overview(ctx, Filter{})
	require.NoError(t, err)

	// Both problems count toward the total, but only the OPEN one labels a status.
	assert.EqualValues(t, 2, ov.ProblemCount)
	require.Len(t, ov.CountsByStatus, 1, "the NULL-status problem produces no bucket")
	assert.EqualValues(t, 1, ov.CountsByStatus[0].Count)
	assert.Equal(t, "待处理", ov.CountsByStatus[0].Label)
	// counts_by_type still sees both (both have a type).
	require.Len(t, ov.CountsByType, 1)
	assert.EqualValues(t, 2, ov.CountsByType[0].Count)
}

// insertProblemNullStatus inserts a problem with status_item_id = NULL (the column
// is nullable; this exercises countByDict's NULL-dimension skip). Returns any error
// so the caller can require.NoError it.
func (e *statsEnv) insertProblemNullStatus(ctx context.Context, project, inspector, typeID int64, captured time.Time) error {
	ewkb, err := geo.EncodePointWGS84(orb.Point{113.0, 23.0})
	if err != nil {
		return err
	}
	_, err = e.pool.Exec(ctx, `
		INSERT INTO problems
			(project_id, inspector_id, geom, type_item_id, status_item_id, captured_at)
		VALUES ($1,$2,ST_GeomFromEWKB($3),$4,NULL,$5)`,
		project, inspector, ewkb, typeID, captured)
	return err
}

// TestOverview_SoftDeletedExcluded asserts the deleted_at IS NULL guard: a
// soft-deleted problem and a soft-deleted inspection are excluded from every
// aggregate (covers the soft-delete filter the existing suite never deletes into).
func TestOverview_SoftDeletedExcluded(t *testing.T) {
	e := newStatsEnv(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	live := e.insertInspection(t, ctx, e.projectA, e.adminID, base, 600, 1000)
	dead := e.insertInspection(t, ctx, e.projectA, e.adminID, base, 999, 9999)
	_, err := e.pool.Exec(ctx, `UPDATE inspections SET deleted_at = now() WHERE id=$1`, dead)
	require.NoError(t, err)

	e.insertProblem(t, ctx, e.projectA, e.adminID, e.typeRoadID, e.statusOpen, base)
	e.insertProblem(t, ctx, e.projectA, e.adminID, e.typeRoadID, e.statusOpen, base)
	_, err = e.pool.Exec(ctx,
		`UPDATE problems SET deleted_at = now() WHERE project_id=$1 AND id = (
			SELECT max(id) FROM problems WHERE project_id=$1)`, e.projectA)
	require.NoError(t, err)

	ov, err := e.svc.Overview(ctx, Filter{})
	require.NoError(t, err)

	assert.EqualValues(t, 1, ov.InspectionCount, "deleted inspection excluded")
	assert.InDelta(t, 1000.0, ov.TotalMileageMeters, 1e-9, "deleted mileage not summed")
	assert.EqualValues(t, 1, ov.ProblemCount, "deleted problem excluded")
	_ = live
}

// TestOverview_ContextCancelledErrors covers Service.Overview's error-wrap branch:
// a cancelled context makes the first sub-query (counts_by_type) fail, so Overview
// returns a wrapped, non-nil error rather than a partial result. This is the only
// path that exercises the `fmt.Errorf("stats: ...: %w", err)` return in Overview.
func TestOverview_ContextCancelledErrors(t *testing.T) {
	e := newStatsEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the query runs

	ov, err := e.svc.Overview(ctx, Filter{})
	require.Error(t, err)
	assert.Nil(t, ov)
	assert.ErrorContains(t, err, "stats:", "error is wrapped with the stats prefix")
	assert.ErrorIs(t, err, context.Canceled, "underlying cause is preserved via %w")
}

// TestProjectsForExport_ContextCancelledErrors covers the projectsInWindow error
// path (the first query in ProjectsForExport): a cancelled context makes it return
// a wrapped error instead of rows.
func TestProjectsForExport_ContextCancelledErrors(t *testing.T) {
	e := newStatsEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rows, err := e.svc.ProjectsForExport(ctx, Filter{})
	require.Error(t, err)
	assert.Nil(t, rows)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestProjectsForExport_SingleProjectFilter asserts ProjectsForExport honors an
// explicit f.ProjectID (single-project export): only that project's row returns,
// even when other projects have data in the window.
func TestProjectsForExport_SingleProjectFilter(t *testing.T) {
	e := newStatsEnv(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	e.insertInspection(t, ctx, e.projectA, e.adminID, base, 600, 1000)
	e.insertInspection(t, ctx, e.projectB, e.adminID, base, 300, 500)
	e.insertProblem(t, ctx, e.projectA, e.adminID, e.typeRoadID, e.statusOpen, base)
	e.insertProblem(t, ctx, e.projectB, e.adminID, e.typeRoadID, e.statusOpen, base)

	rows, err := e.svc.ProjectsForExport(ctx, Filter{ProjectID: &e.projectB})
	require.NoError(t, err)
	require.Len(t, rows, 1, "only the requested project")
	assert.Equal(t, e.projectB, rows[0].ProjectID)
	assert.Equal(t, "项目乙", rows[0].Name)
	require.NotNil(t, rows[0].Overview)
	assert.EqualValues(t, 1, rows[0].Overview.InspectionCount)
	assert.InDelta(t, 500.0, rows[0].Overview.TotalMileageMeters, 1e-9)
}

// TestProjectsForExport_EmptyWindowReturnsNoRows asserts a window with no matching
// data yields zero ProjectRows (projects with neither inspection nor problem in the
// window are excluded by the EXISTS guards).
func TestProjectsForExport_EmptyWindowReturnsNoRows(t *testing.T) {
	e := newStatsEnv(t)
	ctx := context.Background()

	rows, err := e.svc.ProjectsForExport(ctx, Filter{})
	require.NoError(t, err)
	assert.Empty(t, rows)
}
