// Package export implements the async Excel export domain (P6): the POST /exports
// enqueue endpoint, the GET /exports/{job_uuid} poll endpoint, the hand-written
// GET /exports/{job_uuid}/events SSE stream, and the asynq worker that builds the
// three .xlsx report types (INSPECTION_RECORDS, PROBLEM_LIST, PROJECT_STATS),
// uploads them to COS, and advances the export_jobs row PENDING→RUNNING→SUCCEEDED.
//
// The worker reuses internal/stats.Service.Overview for PROJECT_STATS so the
// statistics Excel always matches /stats/overview exactly. Timestamps render in
// Asia/Shanghai; CJK cells carry an explicit font (the worker image ships
// "Noto Sans CJK SC") so Chinese text is not tofu-blocked.
package export

import (
	"encoding/json"
	"time"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
)

// TaskTypeRun is the asynq task type name for the export worker. The payload is
// the job's UUID (see runPayload); the worker loads the export_jobs row by it.
const TaskTypeRun = "export:run"

// resultBucketDefault names the COS bucket the produced .xlsx is written to when
// the configured bucket is empty. The worker is constructed with the real bucket
// from config; tests use the mock COS where the bucket is arbitrary.
const resultBucketDefault = "c5-exports"

// SignedURLTTL is how long a poll-returned result_url stays valid.
const SignedURLTTL = 15 * time.Minute

// shanghai is the render timezone for all human-facing timestamps in the reports.
// Loaded once at init; nil only if tzdata is missing (the worker image ships it).
var shanghai = mustLoadShanghai()

func mustLoadShanghai() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		// Fixed offset fallback (UTC+8) so a tzdata-less environment still renders
		// Beijing time rather than crashing; the worker image installs tzdata.
		return time.FixedZone("CST", 8*60*60)
	}
	return loc
}

// cjkFontName is the font applied to every cell so CJK glyphs render. The worker
// container installs this font family (Noto Sans CJK SC); excelize stores the
// name in the workbook, the spreadsheet app resolves the glyphs.
const cjkFontName = "Noto Sans CJK SC"

// runPayload is the asynq task payload: just the public job handle. Keeping the
// payload minimal (UUID only) means the worker always reads the authoritative job
// row (status/params) from the DB, never a stale enqueue-time snapshot.
type runPayload struct {
	JobUUID string `json:"job_uuid"`
}

// jobRow is the in-memory projection of an export_jobs row used across the
// service, poll handler and worker. Nullable DB columns map to pointers.
type jobRow struct {
	ID            int64
	JobUUID       string
	Type          oapi.ExportType
	Params        []byte // raw params jsonb
	Status        oapi.JobStatus
	Progress      int
	TotalRows     *int
	ProcessedRows int
	ResultCosKey  *string
	ResultBucket  *string
	Error         *string
	RequestedBy   *int64
	StartedAt     *time.Time
	FinishedAt    *time.Time
	CreatedAt     time.Time
}

// exportParams is the union of every report type's filter fields. Each builder
// reads the subset it needs; absent fields stay nil and disable that filter.
// from/to are RFC3339 strings in the stored jsonb (parsed to time.Time on use).
//
// Status is intentionally a RawMessage: for PROBLEM_LIST it is a problem
// status_item_id (number); for INSPECTION_RECORDS it is an inspection_status enum
// (string, e.g. "FINISHED"). Each builder decodes it the way it needs.
type exportParams struct {
	ProjectID    *int64          `json:"project_id,omitempty"`
	InspectorID  *int64          `json:"inspector_id,omitempty"`
	InspectionID *int64          `json:"inspection_id,omitempty"`
	Type         *int64          `json:"type,omitempty"`     // problem type_item_id
	Category     *int64          `json:"category,omitempty"` // problem category_item_id
	Status       json.RawMessage `json:"status,omitempty"`   // id (PROBLEM_LIST) or enum string (INSPECTION_RECORDS)
	From         *string         `json:"from,omitempty"`
	To           *string         `json:"to,omitempty"`
}

// statusAsID decodes the status filter as a problem status_item_id (PROBLEM_LIST).
// Absent/non-numeric -> nil (no filter).
func (p exportParams) statusAsID() *int64 {
	if len(p.Status) == 0 {
		return nil
	}
	var id int64
	if err := json.Unmarshal(p.Status, &id); err != nil {
		return nil
	}
	return &id
}

// statusAsString decodes the status filter as an inspection_status enum
// (INSPECTION_RECORDS). Absent/non-string -> nil (no filter).
func (p exportParams) statusAsString() *string {
	if len(p.Status) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(p.Status, &s); err != nil {
		return nil
	}
	if s == "" {
		return nil
	}
	return &s
}

// timeBound parses an RFC3339 *string param to a *time.Time (UTC). nil/blank/bad
// -> nil (filter disabled).
func timeBound(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return nil
	}
	u := t.UTC()
	return &u
}
