package export

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/xuri/excelize/v2"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/cos"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
	"github.com/nnkglobal/c5-backend/internal/stats"
)

// Worker builds the .xlsx for an export job and uploads it to COS. It owns the
// row mutations (RUNNING → progress → SUCCEEDED/FAILED) via the Service.
type Worker struct {
	svc          *Service
	pool         *db.Pool
	cos          cos.Client
	stats        *stats.Service
	resultBucket string
}

// RegisterWorkers mounts the export:run handler onto the asynq mux. It wires the
// COS client (for the produced .xlsx upload), the shared pool (row reads), and the
// stats service (reused by PROJECT_STATS). The result bucket defaults when empty.
func RegisterWorkers(mux *asynq.ServeMux, c cos.Client, pool *db.Pool, statsSvc *stats.Service) {
	RegisterWorkersWithBucket(mux, c, pool, statsSvc, resultBucketDefault)
}

// RegisterWorkersWithBucket is RegisterWorkers with an explicit result bucket
// (the real worker passes the configured export bucket; tests pass any string).
func RegisterWorkersWithBucket(mux *asynq.ServeMux, c cos.Client, pool *db.Pool, statsSvc *stats.Service, bucket string) {
	if bucket == "" {
		bucket = resultBucketDefault
	}
	w := &Worker{
		svc:          NewService(pool, nil), // worker only reads/mutates; no enqueue
		pool:         pool,
		cos:          c,
		stats:        statsSvc,
		resultBucket: bucket,
	}
	mux.HandleFunc(TaskTypeRun, w.handleRun)
}

// handleRun is the asynq handler for export:run. It loads the job by uuid, marks
// it RUNNING, dispatches to the per-type builder, uploads the workbook to COS and
// finalizes SUCCEEDED. Any error marks the job FAILED (client-safe message) and is
// returned so asynq records the failure too.
func (w *Worker) handleRun(ctx context.Context, task *asynq.Task) error {
	var p runPayload
	if err := json.Unmarshal(task.Payload(), &p); err != nil {
		return fmt.Errorf("export worker: bad payload: %w", err)
	}

	job, err := w.svc.GetByUUID(ctx, p.JobUUID)
	if err != nil {
		return fmt.Errorf("export worker: load job %s: %w", p.JobUUID, err)
	}

	if err := w.svc.markRunning(ctx, job.ID); err != nil {
		return err
	}

	bucket, key, processed, err := w.build(ctx, job)
	if err != nil {
		_ = w.svc.markFailed(ctx, job.ID, clientSafe(err))
		return fmt.Errorf("export worker: build %s: %w", job.Type, err)
	}

	if err := w.svc.markSucceeded(ctx, job.ID, bucket, key, processed); err != nil {
		return err
	}
	return nil
}

// build dispatches to the per-type sheet builder, returning the COS location of
// the uploaded .xlsx and the processed row count.
func (w *Worker) build(ctx context.Context, job *jobRow) (bucket, key string, processed int, err error) {
	var params exportParams
	if len(job.Params) > 0 {
		if err := json.Unmarshal(job.Params, &params); err != nil {
			return "", "", 0, fmt.Errorf("decode params: %w", err)
		}
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	switch job.Type {
	case oapi.INSPECTIONRECORDS:
		processed, err = w.buildInspectionRecords(ctx, f, job, params)
	case oapi.PROBLEMLIST:
		processed, err = w.buildProblemList(ctx, f, job, params)
	case oapi.PROJECTSTATS:
		processed, err = w.buildProjectStats(ctx, f, job, params)
	default:
		return "", "", 0, ErrInvalidType
	}
	if err != nil {
		return "", "", 0, err
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return "", "", 0, fmt.Errorf("serialize workbook: %w", err)
	}

	key = resultKey(job)
	const xlsxContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	if err := w.cos.PutObject(ctx, w.resultBucket, key, xlsxContentType, &buf); err != nil {
		return "", "", 0, fmt.Errorf("upload workbook: %w", err)
	}
	return w.resultBucket, key, processed, nil
}

// resultKey is the COS object key for a job's produced .xlsx, namespaced by type
// and the public uuid so keys never collide across jobs.
func resultKey(job *jobRow) string {
	return fmt.Sprintf("exports/%s/%s.xlsx", job.Type, job.JobUUID)
}

// clientSafe reduces an internal error to a short, client-facing message stored
// in export_jobs.error (no SQL, stack traces, or internal identifiers).
func clientSafe(err error) string {
	if err == nil {
		return ""
	}
	// Keep it generic; the detailed error is returned to asynq + logs separately.
	return "export failed while generating the report"
}

// countRows runs a COUNT(*) over `FROM <from> <where>` with the given args. It is
// used to seed total_rows so progress has a denominator.
func (w *Worker) countRows(ctx context.Context, from, where string, args []any) (int, error) {
	var total int
	if err := w.pool.QueryRow(ctx,
		`SELECT count(*) FROM `+from+` `+where, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

// percent computes a clamped 0..100 progress from processed/total. total==0 means
// nothing to do, which is fully complete (100).
func percent(processed, total int) int {
	if total <= 0 {
		return 100
	}
	return clampProgress(processed * 100 / total)
}
