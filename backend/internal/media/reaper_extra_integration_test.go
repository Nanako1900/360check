//go:build integration

package media

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// backdateCreatedAt moves a row's created_at into the past by the given SQL
// interval. The BEFORE-UPDATE trigger clobbers updated_at, so the reaper measures
// staleness by created_at — this helper proves that.
func (e *testEnv) backdateCreatedAt(t *testing.T, ctx context.Context, cu uuid.UUID, interval string) {
	t.Helper()
	_, err := e.pool.Exec(ctx,
		`UPDATE media_assets SET created_at = now() - $2::interval WHERE client_uuid = $1`, cu, interval)
	require.NoError(t, err)
}

// TestReaper_HandleReap_FlagsStale: the asynq handler form (HandleReap) flags a
// stale UPLOADING row and leaves a fresh one alone, identical to a direct Reap.
func TestReaper_HandleReap_FlagsStale(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	staleCU, _ := env.issueOriginal(t, ctx, 70)
	env.backdateCreatedAt(t, ctx, staleCU, "48 hours")
	freshCU, _ := env.issueOriginal(t, ctx, 71)

	reaper := NewReaper(env.pool, env.cfg.UploadingTTL)
	require.NoError(t, reaper.HandleReap(ctx, NewReapTask()))

	assert.True(t, env.reapedFlag(t, ctx, staleCU), "HandleReap flags the stale row")
	assert.False(t, env.reapedFlag(t, ctx, freshCU), "HandleReap leaves the fresh row")

	// Idempotent: a second HandleReap pass changes nothing new.
	require.NoError(t, reaper.HandleReap(ctx, NewReapTask()))
	assert.True(t, env.reapedFlag(t, ctx, staleCU))
}

// TestReaper_MeasuresByCreatedAt_NotUpdatedAt: a row whose updated_at is brand new
// (because a trigger reset it) but whose created_at is old must still be reaped.
// This is the core invariant of reaper.go — staleness keys off created_at.
func TestReaper_MeasuresByCreatedAt_NotUpdatedAt(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	cu, _ := env.issueOriginal(t, ctx, 72)
	// Age created_at past the TTL.
	env.backdateCreatedAt(t, ctx, cu, "48 hours")
	// Force a write so the BEFORE-UPDATE trigger bumps updated_at to ~now; if the
	// reaper (wrongly) measured updated_at, the row would look fresh and be skipped.
	_, err := env.pool.Exec(ctx,
		`UPDATE media_assets SET byte_size = 999 WHERE client_uuid = $1`, cu)
	require.NoError(t, err)

	// Sanity: updated_at is fresh, created_at is old.
	var updatedFresh, createdOld bool
	require.NoError(t, env.pool.QueryRow(ctx, `
		SELECT updated_at > now() - interval '1 hour',
		       created_at < now() - interval '24 hours'
		FROM media_assets WHERE client_uuid = $1`, cu).Scan(&updatedFresh, &createdOld))
	require.True(t, updatedFresh, "precondition: updated_at was reset to ~now")
	require.True(t, createdOld, "precondition: created_at is aged past the TTL")

	reaper := NewReaper(env.pool, env.cfg.UploadingTTL)
	n, err := reaper.Reap(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n, "row is reaped by created_at despite a fresh updated_at")
	assert.True(t, env.reapedFlag(t, ctx, cu))
}

// TestReaper_ConfirmedRowNeverReaped: only UPLOADING rows are eligible; a stale
// CONFIRMED original is never flagged.
func TestReaper_ConfirmedRowNeverReaped(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	cu, creds := env.issueOriginal(t, ctx, 73)
	body := jpegBytes(t, 64, 64)
	env.cos.SetObject(creds.Bucket, creds.Key, "etag-c", body, "image/jpeg")
	_, _, err := env.svc.Confirm(ctx, ConfirmInput{
		ClientUUID: cu, Key: creds.Key, Etag: "etag-c", ByteSize: int64(len(body)),
	}, env.adminID)
	require.NoError(t, err)
	env.backdateCreatedAt(t, ctx, cu, "72 hours") // old, but CONFIRMED

	reaper := NewReaper(env.pool, env.cfg.UploadingTTL)
	n, err := reaper.Reap(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 0, n, "a CONFIRMED row is never reaped no matter how old")
	assert.False(t, env.reapedFlag(t, ctx, cu))
}

// TestNewReaper_DefaultTTL: a non-positive TTL defaults to 24h, so a 48h-old row
// is still reaped under the default.
func TestNewReaper_DefaultTTL(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	cu, _ := env.issueOriginal(t, ctx, 74)
	env.backdateCreatedAt(t, ctx, cu, "48 hours")

	reaper := NewReaper(env.pool, 0) // 0 -> default 24h
	n, err := reaper.Reap(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n, "default 24h TTL reaps a 48h-old row")
}

// TestNewReapTask_Type: the scheduled task carries the reaper type and no payload.
func TestNewReapTask_Type(t *testing.T) {
	task := NewReapTask()
	assert.Equal(t, TaskReapUploading, task.Type())
	assert.Empty(t, task.Payload())
}

// TestRegisterReaperWorker_DispatchesReap: RegisterReaperWorker mounts the reap
// handler on a mux and a dispatched reap task flags the stale row end-to-end.
func TestRegisterReaperWorker_DispatchesReap(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	cu, _ := env.issueOriginal(t, ctx, 75)
	env.backdateCreatedAt(t, ctx, cu, "48 hours")

	mux := asynq.NewServeMux()
	RegisterReaperWorker(mux, env.pool, env.cfg.UploadingTTL)
	require.NoError(t, mux.ProcessTask(ctx, NewReapTask()))
	assert.True(t, env.reapedFlag(t, ctx, cu), "the registered reaper worker flagged the stale row")
}
