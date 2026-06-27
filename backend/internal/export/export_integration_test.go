//go:build integration

package export

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/paulmach/orb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/xuri/excelize/v2"

	migrations "github.com/nnkglobal/c5-backend/db/migrations"
	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/cos"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
	"github.com/nnkglobal/c5-backend/internal/platform/geo"
	"github.com/nnkglobal/c5-backend/internal/stats"
)

const testBucket = "c5-exports-test"

// fakeEnq records enqueued task payloads instead of touching Redis.
type fakeEnq struct {
	tasks []*asynq.Task
}

func (f *fakeEnq) Enqueue(task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	f.tasks = append(f.tasks, task)
	return &asynq.TaskInfo{ID: task.Type()}, nil
}

// expEnv bundles the migrated pool, the export service+worker, the mock COS, and
// the seeded ids the assertions reference.
type expEnv struct {
	pool      *db.Pool
	svc       *Service
	worker    *Worker
	statsSvc  *stats.Service
	mockCOS   *cos.Mock
	enq       *fakeEnq
	adminID   int64
	projectID int64
	inspID    int64
	typeRoad  int64
	statusOK  int64
}

func newExpEnv(t *testing.T) *expEnv {
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

	mockCOS := cos.NewMock()
	statsSvc := stats.NewService(pool)
	enq := &fakeEnq{}
	e := &expEnv{
		pool:     pool,
		svc:      NewService(pool, enq),
		statsSvc: statsSvc,
		mockCOS:  mockCOS,
		enq:      enq,
	}
	e.worker = &Worker{
		svc:          NewService(pool, nil),
		pool:         pool,
		cos:          mockCOS,
		stats:        statsSvc,
		resultBucket: testBucket,
	}

	require.NoError(t, pool.QueryRow(ctx, "SELECT id FROM users WHERE username='admin'").Scan(&e.adminID))
	_, err = pool.Exec(ctx, `UPDATE users SET display_name='张三' WHERE id=$1`, e.adminID)
	require.NoError(t, err)

	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO projects (code, name) VALUES ('PX','某路段') RETURNING id`).Scan(&e.projectID))

	started := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC) // 08:00 Asia/Shanghai
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO inspections
			(project_id, inspector_id, status, started_at, ended_at, duration_seconds, mileage_meters, point_count)
		VALUES ($1,$2,'FINISHED',$3::timestamptz,$3::timestamptz + interval '600 seconds',600,1025.2,42)
		RETURNING id`, e.projectID, e.adminID, started).Scan(&e.inspID))

	e.typeRoad = dictID(t, ctx, pool, "problem_type", "ROAD")
	e.statusOK = dictID(t, ctx, pool, "problem_status", "OPEN")

	ewkb, err := geo.EncodePointWGS84(orb.Point{113.0, 23.0})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO problems
			(project_id, inspection_id, inspector_id, geom, type_item_id, status_item_id, captured_at, description, note)
		VALUES ($1,$2,$3,ST_GeomFromEWKB($4),$5,$6,$7,'路面破损','需尽快处理')`,
		e.projectID, e.inspID, e.adminID, ewkb, e.typeRoad, e.statusOK, started)
	require.NoError(t, err)

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

// runJob creates the export_jobs row via the service (recording the enqueued
// task), runs the worker handler synchronously on that task, and returns the
// finished job row.
func (e *expEnv) runJob(t *testing.T, ctx context.Context, typ oapi.ExportType, params map[string]any) *jobRow {
	t.Helper()
	job, err := e.svc.Create(ctx, CreateInput{Type: typ, Params: params, RequestedBy: e.adminID})
	require.NoError(t, err)
	require.Equal(t, oapi.PENDING, job.Status)

	require.Len(t, e.enq.tasks, 1)
	require.NoError(t, e.worker.handleRun(ctx, e.enq.tasks[0]))
	e.enq.tasks = nil

	done, err := e.svc.GetByUUID(ctx, job.JobUUID)
	require.NoError(t, err)
	return done
}

// openResult downloads the produced .xlsx from the mock COS and opens it.
func (e *expEnv) openResult(t *testing.T, ctx context.Context, job *jobRow) *excelize.File {
	t.Helper()
	require.Equal(t, oapi.SUCCEEDED, job.Status)
	require.NotNil(t, job.ResultBucket)
	require.NotNil(t, job.ResultCosKey)

	rc, err := e.mockCOS.GetObject(ctx, *job.ResultBucket, *job.ResultCosKey)
	require.NoError(t, err)
	defer rc.Close()
	var buf bytes.Buffer
	_, err = buf.ReadFrom(rc)
	require.NoError(t, err)

	f, err := excelize.OpenReader(&buf)
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestWorker_InspectionRecords_XLSX(t *testing.T) {
	e := newExpEnv(t)
	ctx := context.Background()

	job := e.runJob(t, ctx, oapi.INSPECTIONRECORDS, map[string]any{"project_id": e.projectID})
	assert.Equal(t, 100, job.Progress)
	require.NotNil(t, job.TotalRows)
	assert.Equal(t, 1, *job.TotalRows)
	assert.Equal(t, 1, job.ProcessedRows)

	f := e.openResult(t, ctx, job)
	sheet := "巡查记录"

	// Header CJK cell renders the expected string.
	assert.Equal(t, "巡查ID", cell(t, f, sheet, "A1"))
	assert.Equal(t, "项目", cell(t, f, sheet, "B1"))

	// Data row: project name (CJK), inspector display_name, status.
	assert.Equal(t, "某路段", cell(t, f, sheet, "B2"))
	assert.Equal(t, "张三", cell(t, f, sheet, "C2"))
	assert.Equal(t, "FINISHED", cell(t, f, sheet, "D2"))

	// started_at rendered in Asia/Shanghai: 2026-06-26 00:00 UTC -> 08:00 CST.
	assert.Equal(t, "2026-06-26 08:00:00", cell(t, f, sheet, "E2"))

	// mileage (km) = 1025.2 m -> 1.025 km.
	assert.Equal(t, "1.025", cell(t, f, sheet, "H2"))

	// CJK font style was applied to the data row (the style is SET; glyph
	// rendering depends on the font being installed in the worker/viewer image).
	styleID, err := f.GetCellStyle(sheet, "B2")
	require.NoError(t, err)
	st, err := f.GetStyle(styleID)
	require.NoError(t, err)
	require.NotNil(t, st.Font)
	assert.Equal(t, cjkFontName, st.Font.Family, "CJK font name set on the cell style")
}

func TestWorker_ProblemList_XLSX(t *testing.T) {
	e := newExpEnv(t)
	ctx := context.Background()

	job := e.runJob(t, ctx, oapi.PROBLEMLIST, map[string]any{"project_id": e.projectID})
	f := e.openResult(t, ctx, job)
	sheet := "问题列表"

	// Dict labels resolved (type/status), CJK description rendered.
	assert.Equal(t, "路面", cell(t, f, sheet, "D2"))
	assert.Equal(t, "待处理", cell(t, f, sheet, "E2"))
	assert.Equal(t, "路面破损", cell(t, f, sheet, "H2"))

	// captured_at in Asia/Shanghai.
	assert.Equal(t, "2026-06-26 08:00:00", cell(t, f, sheet, "G2"))
}

func TestWorker_ProblemList_RetiredDictLabel(t *testing.T) {
	e := newExpEnv(t)
	ctx := context.Background()

	// Retire the ROAD type; the export must still resolve its label (history).
	_, err := e.pool.Exec(ctx, `UPDATE dict_item SET is_active=false WHERE id=$1`, e.typeRoad)
	require.NoError(t, err)

	job := e.runJob(t, ctx, oapi.PROBLEMLIST, map[string]any{"project_id": e.projectID})
	f := e.openResult(t, ctx, job)
	assert.Equal(t, "路面", cell(t, f, "问题列表", "D2"), "retired dict label still exported")
}

func TestWorker_ProjectStats_MatchesOverview(t *testing.T) {
	e := newExpEnv(t)
	ctx := context.Background()

	job := e.runJob(t, ctx, oapi.PROJECTSTATS, map[string]any{})
	f := e.openResult(t, ctx, job)
	sheet := "项目统计"

	// One project row, CJK name.
	assert.Equal(t, "某路段", cell(t, f, sheet, "B2"))

	// The aggregated numbers must equal stats.Overview for that project.
	ov, err := e.statsSvc.Overview(ctx, stats.Filter{ProjectID: &e.projectID})
	require.NoError(t, err)

	assert.Equal(t, "1", cell(t, f, sheet, "C2"))
	assert.EqualValues(t, 1, ov.InspectionCount)
	assert.Equal(t, "1", cell(t, f, sheet, "D2"))
	assert.EqualValues(t, 1, ov.ProblemCount)

	// total mileage km = 1025.2 m -> 1.025 km.
	assert.Equal(t, "1.025", cell(t, f, sheet, "E2"))

	// counts_by_type compact string carries the CJK label and equals formatBuckets.
	assert.Equal(t, "路面×1", cell(t, f, sheet, "H2"))
	assert.Equal(t, "路面×1", formatBuckets(ov.CountsByType))
}

func TestWorker_FailureMarksJobFailed(t *testing.T) {
	e := newExpEnv(t)
	ctx := context.Background()

	// A worker whose COS always fails PutObject exercises the FAILED terminal path.
	w := &Worker{
		svc:          NewService(e.pool, nil),
		pool:         e.pool,
		cos:          &putFailCOS{Mock: cos.NewMock()},
		stats:        e.statsSvc,
		resultBucket: testBucket,
	}

	job, err := e.svc.Create(ctx, CreateInput{Type: oapi.PROBLEMLIST, RequestedBy: e.adminID})
	require.NoError(t, err)
	require.Len(t, e.enq.tasks, 1)

	err = w.handleRun(ctx, e.enq.tasks[0])
	require.Error(t, err, "handler returns the build error to asynq")

	done, err := e.svc.GetByUUID(ctx, job.JobUUID)
	require.NoError(t, err)
	assert.Equal(t, oapi.FAILED, done.Status)
	require.NotNil(t, done.Error)
	assert.NotEmpty(t, *done.Error)
	require.NotNil(t, done.FinishedAt)
}

func TestService_PollReturnsTerminalStateAndURL(t *testing.T) {
	e := newExpEnv(t)
	ctx := context.Background()

	job := e.runJob(t, ctx, oapi.INSPECTIONRECORDS, map[string]any{"project_id": e.projectID})

	// toAPI signs the result URL for a SUCCEEDED job (poll/SSE share this path).
	api := toAPI(ctx, job, e.mockCOS)
	assert.Equal(t, oapi.SUCCEEDED, api.Status)
	require.NotNil(t, api.ResultUrl)
	assert.Contains(t, *api.ResultUrl, "mock-cdn.example.com")
}

// cell reads one cell value, failing the test on error.
func cell(t *testing.T, f *excelize.File, sheet, ref string) string {
	t.Helper()
	v, err := f.GetCellValue(sheet, ref)
	require.NoError(t, err)
	return v
}

// putFailCOS wraps the mock COS but fails every PutObject so the worker FAILS.
type putFailCOS struct {
	*cos.Mock
}

func (p *putFailCOS) PutObject(_ context.Context, _, _, _ string, _ io.Reader) error {
	return errors.New("simulated COS upload failure")
}
