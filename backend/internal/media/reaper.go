package media

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"

	"github.com/nnkglobal/c5-backend/internal/platform/db"
)

// TaskReapUploading is the asynq task type for the periodic UPLOADING reaper. It
// is registered both on the ServeMux (handler) and scheduled (the boulder).
const TaskReapUploading = "media:reap-uploading"

// NewReapTask builds the (payload-free) reaper task for the scheduler to enqueue.
func NewReapTask() *asynq.Task { return asynq.NewTask(TaskReapUploading, nil) }

// Reaper flags media rows stuck in UPLOADING longer than the TTL. The capture
// state machine is forward-only with no terminal failure state, so the reaper
// does NOT rewind capture_state; instead it stamps meta.reaped_at (and
// meta.reaped=true) so an operator/cleanup job can find abandoned uploads and the
// originals never re-enter the derive path. It is idempotent: a row already
// carrying meta.reaped is skipped.
type Reaper struct {
	pool *db.Pool
	ttl  time.Duration
}

// NewReaper builds the UPLOADING reaper. ttl<=0 defaults to 24h.
func NewReaper(pool *db.Pool, ttl time.Duration) *Reaper {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Reaper{pool: pool, ttl: ttl}
}

// HandleReap is the asynq handler form; it runs one Reap pass.
func (r *Reaper) HandleReap(ctx context.Context, _ *asynq.Task) error {
	_, err := r.Reap(ctx)
	return err
}

// Reap marks every UPLOADING row older than the TTL as reaped and returns how
// many rows it newly flagged. Staleness is measured by created_at (immutable),
// NOT updated_at: the media_assets BEFORE-UPDATE trigger resets updated_at=now()
// on every write, so updated_at can never age past the TTL. created_at is set
// once at insert and is the true "how long has this upload been outstanding"
// clock. Safe to call repeatedly.
func (r *Reaper) Reap(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-r.ttl)
	tag, err := r.pool.Exec(ctx, `
		UPDATE media_assets
		SET meta = meta || $1::jsonb
		WHERE capture_state = 'UPLOADING'
		  AND deleted_at IS NULL
		  AND created_at < $2
		  AND COALESCE((meta->>'reaped')::boolean, false) = false`,
		reapedMeta(), cutoff)
	if err != nil {
		return 0, fmt.Errorf("media: reap uploading: %w", err)
	}
	return tag.RowsAffected(), nil
}

// reapedMeta is the JSON patch merged into meta for a reaped row.
func reapedMeta() []byte {
	b, _ := json.Marshal(map[string]any{
		"reaped":    true,
		"reaped_at": time.Now().UTC().Format(time.RFC3339),
	})
	return b
}

// RegisterReaperWorker mounts the reaper task handler onto the shared ServeMux so
// cmd/worker can process scheduled reap ticks.
func RegisterReaperWorker(mux *asynq.ServeMux, pool *db.Pool, ttl time.Duration) {
	r := NewReaper(pool, ttl)
	mux.HandleFunc(TaskReapUploading, r.HandleReap)
}
