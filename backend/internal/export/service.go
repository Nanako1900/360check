package export

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
)

// exportRunTimeout bounds a single export:run execution. A pathological
// PROJECT_STATS/PROBLEM_LIST must not hold an asynq worker slot (concurrency is
// bounded) indefinitely; asynq cancels the handler's context past this deadline.
const exportRunTimeout = 10 * time.Minute

// Enqueuer is the minimal asynq surface the service needs to schedule the worker
// task. *asynq.Client satisfies it; tests pass a fake that records the payload.
type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// jobReader is the read surface shared by the service and the SSE/poll handler.
// It is satisfied by *Service.
type jobReader interface {
	GetByUUID(ctx context.Context, jobUUID string) (*jobRow, error)
}

// Service owns the export_jobs table plus task enqueueing.
type Service struct {
	pool *db.Pool
	enq  Enqueuer
}

// NewService wires the export service onto the shared pool and an asynq enqueuer.
// The enqueuer may be nil in read-only contexts (poll/SSE), but Create requires it.
func NewService(pool *db.Pool, enq Enqueuer) *Service {
	return &Service{pool: pool, enq: enq}
}

// jobCols is the canonical export_jobs projection; scanJob depends on this order.
const jobCols = `id, job_uuid, type, params, status, progress, total_rows,
	processed_rows, result_cos_key, result_bucket, error, requested_by,
	started_at, finished_at, created_at`

// scanJob materializes one export_jobs row from the canonical projection.
func scanJob(row pgx.Row) (*jobRow, error) {
	var (
		j         jobRow
		typeStr   string
		statusStr string
	)
	if err := row.Scan(
		&j.ID, &j.JobUUID, &typeStr, &j.Params, &statusStr, &j.Progress,
		&j.TotalRows, &j.ProcessedRows, &j.ResultCosKey, &j.ResultBucket,
		&j.Error, &j.RequestedBy, &j.StartedAt, &j.FinishedAt, &j.CreatedAt,
	); err != nil {
		return nil, err
	}
	j.Type = oapi.ExportType(typeStr)
	j.Status = oapi.JobStatus(statusStr)
	if len(j.Params) == 0 {
		j.Params = []byte("{}")
	}
	return &j, nil
}

// CreateInput is the validated payload for Create.
type CreateInput struct {
	Type   oapi.ExportType
	Params map[string]any
	// RequestedBy is the authenticated user id (0 -> NULL requested_by).
	RequestedBy int64
}

// Create inserts a PENDING export_jobs row and enqueues the worker task. It
// returns the persisted job (status PENDING, fresh job_uuid). Enqueue failure
// rolls back the row so a client never gets a job that will never run.
func (s *Service) Create(ctx context.Context, in CreateInput) (*jobRow, error) {
	if !validType(in.Type) {
		return nil, ErrInvalidType
	}
	if s.enq == nil {
		return nil, fmt.Errorf("export: service has no enqueuer")
	}

	paramsJSON, err := json.Marshal(in.Params)
	if err != nil {
		return nil, fmt.Errorf("export: marshal params: %w", err)
	}
	if in.Params == nil {
		paramsJSON = []byte("{}")
	}

	// Reject an invalid INSPECTION_RECORDS status enum here (422) rather than
	// letting the ::inspection_status cast fail the whole job mid-build (FAILED).
	if err := validateInspectionStatus(in.Type, paramsJSON); err != nil {
		return nil, err
	}

	var requestedBy *int64
	if in.RequestedBy != 0 {
		requestedBy = &in.RequestedBy
	}

	row := s.pool.QueryRow(ctx, `
		INSERT INTO export_jobs (type, params, status, requested_by)
		VALUES ($1, $2, 'PENDING', $3)
		RETURNING `+jobCols,
		string(in.Type), paramsJSON, requestedBy)
	job, err := scanJob(row)
	if err != nil {
		return nil, fmt.Errorf("export: insert job: %w", err)
	}

	// Enqueue the worker task keyed by the public uuid. A unique TaskID keyed on
	// the job uuid makes re-enqueue idempotent if the caller retries.
	payload, err := json.Marshal(runPayload{JobUUID: job.JobUUID})
	if err != nil {
		return nil, fmt.Errorf("export: marshal payload: %w", err)
	}
	task := asynq.NewTask(TaskTypeRun, payload,
		asynq.TaskID("export:"+job.JobUUID), asynq.MaxRetry(3), asynq.Timeout(exportRunTimeout))
	if _, err := s.enq.Enqueue(task); err != nil {
		// Roll back the orphaned PENDING row so it is not stuck forever.
		_, _ = s.pool.Exec(ctx, `DELETE FROM export_jobs WHERE id = $1`, job.ID)
		return nil, fmt.Errorf("export: enqueue task: %w", err)
	}
	return job, nil
}

// GetByUUID returns the export job for a public job_uuid, or ErrNotFound.
func (s *Service) GetByUUID(ctx context.Context, jobUUID string) (*jobRow, error) {
	if _, err := uuid.Parse(jobUUID); err != nil {
		return nil, ErrNotFound
	}
	row := s.pool.QueryRow(ctx, `SELECT `+jobCols+` FROM export_jobs WHERE job_uuid = $1`, jobUUID)
	job, err := scanJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("export: get job: %w", err)
	}
	return job, nil
}

// markRunning flips a PENDING/loaded job to RUNNING and stamps started_at. It is
// idempotent: re-running a job already RUNNING leaves started_at as-is.
func (s *Service) markRunning(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE export_jobs
		SET status = 'RUNNING',
		    started_at = COALESCE(started_at, now())
		WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("export: mark running: %w", err)
	}
	return nil
}

// setTotalRows records the planned row count (drives progress denominator).
func (s *Service) setTotalRows(ctx context.Context, id int64, total int) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE export_jobs SET total_rows = $2 WHERE id = $1`, id, total)
	if err != nil {
		return fmt.Errorf("export: set total_rows: %w", err)
	}
	return nil
}

// updateProgress writes processed_rows and a clamped 0..100 progress. The clamp
// satisfies chk_export_progress regardless of rounding.
func (s *Service) updateProgress(ctx context.Context, id int64, processed, progress int) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE export_jobs SET processed_rows = $2, progress = $3 WHERE id = $1`,
		id, processed, clampProgress(progress))
	if err != nil {
		return fmt.Errorf("export: update progress: %w", err)
	}
	return nil
}

// markSucceeded finalizes a job: result key/bucket, progress 100, finished_at.
func (s *Service) markSucceeded(ctx context.Context, id int64, bucket, key string, processed int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE export_jobs
		SET status = 'SUCCEEDED', progress = 100, processed_rows = $2,
		    result_bucket = $3, result_cos_key = $4, finished_at = now(), error = NULL
		WHERE id = $1`, id, processed, bucket, key)
	if err != nil {
		return fmt.Errorf("export: mark succeeded: %w", err)
	}
	return nil
}

// markFailed records a FAILED terminal state with a client-safe error message.
func (s *Service) markFailed(ctx context.Context, id int64, msg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE export_jobs
		SET status = 'FAILED', error = $2, finished_at = now()
		WHERE id = $1`, id, msg)
	if err != nil {
		return fmt.Errorf("export: mark failed: %w", err)
	}
	return nil
}

// clampProgress bounds a percentage into [0,100] for chk_export_progress.
func clampProgress(p int) int {
	switch {
	case p < 0:
		return 0
	case p > 100:
		return 100
	default:
		return p
	}
}

// validType reports whether t is one of the three supported export types.
func validType(t oapi.ExportType) bool {
	switch t {
	case oapi.INSPECTIONRECORDS, oapi.PROBLEMLIST, oapi.PROJECTSTATS:
		return true
	default:
		return false
	}
}

// validateInspectionStatus rejects an INSPECTION_RECORDS export whose status
// filter is not a frozen inspection_status literal. For every other type status
// is decoded differently (PROBLEM_LIST: numeric dict id) so it is left alone.
// Decoding mirrors the builder (exportParams.statusAsString) so an accepted
// value always survives the ::inspection_status cast at query time.
func validateInspectionStatus(typ oapi.ExportType, paramsJSON []byte) error {
	if typ != oapi.INSPECTIONRECORDS {
		return nil
	}
	var p exportParams
	if err := json.Unmarshal(paramsJSON, &p); err != nil {
		return ErrInvalidStatus
	}
	st := p.statusAsString()
	if st == nil {
		return nil // absent status -> no filter, nothing to validate
	}
	if !validInspectionStatus(*st) {
		return ErrInvalidStatus
	}
	return nil
}

// validInspectionStatus reports whether s is a frozen inspection_status literal.
func validInspectionStatus(s string) bool {
	switch oapi.InspectionStatus(s) {
	case oapi.InspectionStatusINPROGRESS, oapi.InspectionStatusFINISHED, oapi.InspectionStatusABANDONED:
		return true
	default:
		return false
	}
}
