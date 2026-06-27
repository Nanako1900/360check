//go:build integration

package export

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/paulmach/orb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/geo"
)

// rowCount returns the number of data rows on a sheet (total rows minus the header).
func rowCount(t *testing.T, e *expEnv, ctx context.Context, job *jobRow, sheet string) int {
	t.Helper()
	f := e.openResult(t, ctx, job)
	rows, err := f.GetRows(sheet)
	require.NoError(t, err)
	if len(rows) == 0 {
		return 0
	}
	return len(rows) - 1 // drop header
}

// --- Empty-data builds: every type produces a header-only workbook and a
// SUCCEEDED job with zero processed rows (percent(0,0) -> 100). ---

func TestBuild_InspectionRecords_EmptyData(t *testing.T) {
	e := newExpEnv(t)
	ctx := context.Background()

	// Filter to a non-existent project so the query matches nothing.
	job := e.runJob(t, ctx, oapi.INSPECTIONRECORDS, map[string]any{"project_id": 999999})
	assert.Equal(t, oapi.SUCCEEDED, job.Status)
	assert.Equal(t, 100, job.Progress, "no rows = complete")
	require.NotNil(t, job.TotalRows)
	assert.Equal(t, 0, *job.TotalRows)
	assert.Equal(t, 0, job.ProcessedRows)
	assert.Equal(t, 0, rowCount(t, e, ctx, job, "巡查记录"))
}

func TestBuild_ProblemList_EmptyData(t *testing.T) {
	e := newExpEnv(t)
	ctx := context.Background()

	job := e.runJob(t, ctx, oapi.PROBLEMLIST, map[string]any{"project_id": 999999})
	assert.Equal(t, oapi.SUCCEEDED, job.Status)
	assert.Equal(t, 0, rowCount(t, e, ctx, job, "问题列表"))
}

func TestBuild_ProjectStats_EmptyData(t *testing.T) {
	e := newExpEnv(t)
	ctx := context.Background()

	// A project filter that matches nothing yields zero project rows.
	job := e.runJob(t, ctx, oapi.PROJECTSTATS, map[string]any{"project_id": 999999})
	assert.Equal(t, oapi.SUCCEEDED, job.Status)
	assert.Equal(t, 0, rowCount(t, e, ctx, job, "项目统计"))
}

// --- Filter-branch coverage: every optional WHERE clause is exercised by
// passing the corresponding param. These all resolve to the single seeded row. ---

func TestBuild_InspectionRecords_AllFilters(t *testing.T) {
	e := newExpEnv(t)
	ctx := context.Background()

	// project_id + inspector_id + from + to + status (enum string) all set.
	job := e.runJob(t, ctx, oapi.INSPECTIONRECORDS, map[string]any{
		"project_id":   e.projectID,
		"inspector_id": e.adminID,
		"from":         "2026-06-01T00:00:00Z",
		"to":           "2026-07-01T00:00:00Z",
		"status":       "FINISHED",
	})
	require.NotNil(t, job.TotalRows)
	assert.Equal(t, 1, *job.TotalRows, "the seeded inspection matches every filter")
	assert.Equal(t, 1, rowCount(t, e, ctx, job, "巡查记录"))
}

func TestBuild_InspectionRecords_StatusFilterExcludes(t *testing.T) {
	e := newExpEnv(t)
	ctx := context.Background()

	// The seeded inspection is FINISHED; filtering for IN_PROGRESS excludes it.
	job := e.runJob(t, ctx, oapi.INSPECTIONRECORDS, map[string]any{
		"project_id": e.projectID,
		"status":     "IN_PROGRESS",
	})
	require.NotNil(t, job.TotalRows)
	assert.Equal(t, 0, *job.TotalRows)
}

func TestBuild_ProblemList_AllFilters(t *testing.T) {
	e := newExpEnv(t)
	ctx := context.Background()

	// project_id + inspector_id + inspection_id + type + status(id) + from + to.
	job := e.runJob(t, ctx, oapi.PROBLEMLIST, map[string]any{
		"project_id":    e.projectID,
		"inspector_id":  e.adminID,
		"inspection_id": e.inspID,
		"type":          e.typeRoad,
		"status":        e.statusOK, // numeric dict_item id -> statusAsID branch
		"from":          "2026-06-01T00:00:00Z",
		"to":            "2026-07-01T00:00:00Z",
	})
	require.NotNil(t, job.TotalRows)
	assert.Equal(t, 1, *job.TotalRows, "the seeded problem matches every filter")
	assert.Equal(t, 1, rowCount(t, e, ctx, job, "问题列表"))
}

func TestBuild_ProblemList_CategoryFilterExcludes(t *testing.T) {
	e := newExpEnv(t)
	ctx := context.Background()

	// Seed a category dict item the problem does NOT carry; filtering by it
	// excludes the seeded row (exercises the category WHERE branch).
	catID := dictID(t, ctx, e.pool, "problem_category", anyCategoryCode(t, ctx, e))
	job := e.runJob(t, ctx, oapi.PROBLEMLIST, map[string]any{
		"project_id": e.projectID,
		"category":   catID,
	})
	require.NotNil(t, job.TotalRows)
	assert.Equal(t, 0, *job.TotalRows, "seeded problem has no category -> excluded")
}

func TestBuild_ProjectStats_WithInspectorAndWindow(t *testing.T) {
	e := newExpEnv(t)
	ctx := context.Background()

	// inspector_id + from + to populate the stats.Filter branches.
	job := e.runJob(t, ctx, oapi.PROJECTSTATS, map[string]any{
		"inspector_id": e.adminID,
		"from":         "2026-06-01T00:00:00Z",
		"to":           "2026-07-01T00:00:00Z",
	})
	assert.Equal(t, oapi.SUCCEEDED, job.Status)
	assert.GreaterOrEqual(t, rowCount(t, e, ctx, job, "项目统计"), 1)
}

// --- Multi-page: seed > pageSize (500) problems so the paginated loop runs
// more than one page, exercising the page-boundary break and per-page progress
// updates. ---

func TestBuild_ProblemList_MultiPage(t *testing.T) {
	e := newExpEnv(t)
	ctx := context.Background()

	const extra = 600 // total becomes 601 with the seeded row -> 2 pages
	ewkb, err := geo.EncodePointWGS84(orb.Point{113.1, 23.1})
	require.NoError(t, err)
	captured := time.Date(2026, 6, 26, 1, 0, 0, 0, time.UTC)
	for i := 0; i < extra; i++ {
		_, err := e.pool.Exec(ctx, `
			INSERT INTO problems
				(project_id, inspection_id, inspector_id, geom, type_item_id, status_item_id, captured_at, description, note)
			VALUES ($1,$2,$3,ST_GeomFromEWKB($4),$5,$6,$7,$8,'')`,
			e.projectID, e.inspID, e.adminID, ewkb, e.typeRoad, e.statusOK, captured,
			fmt.Sprintf("批量问题 %d", i))
		require.NoError(t, err)
	}

	job := e.runJob(t, ctx, oapi.PROBLEMLIST, map[string]any{"project_id": e.projectID})
	require.NotNil(t, job.TotalRows)
	assert.Equal(t, extra+1, *job.TotalRows)
	assert.Equal(t, extra+1, job.ProcessedRows)
	assert.Equal(t, 100, job.Progress)
	assert.Equal(t, extra+1, rowCount(t, e, ctx, job, "问题列表"))
}

// anyCategoryCode returns the code of any active problem_category dict item, so
// the category-filter test can reference a real id without hardcoding a seed code.
func anyCategoryCode(t *testing.T, ctx context.Context, e *expEnv) string {
	t.Helper()
	var code string
	require.NoError(t, e.pool.QueryRow(ctx, `
		SELECT di.code FROM dict_item di JOIN dict_type dt ON dt.id = di.dict_type_id
		WHERE dt.code = 'problem_category' LIMIT 1`).Scan(&code))
	return code
}
