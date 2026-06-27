//go:build integration

package media

import (
	"context"
	"errors"
	"image/color"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// insertConfirmedOriginal inserts a CONFIRMED original row directly (bypassing the
// COS confirm path) so a test can point it at a COS key that was never seeded.
// Returns the new row id.
func (e *testEnv) insertConfirmedOriginal(t *testing.T, ctx context.Context, ownerID int64, bucket, key string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, e.pool.QueryRow(ctx, `
		INSERT INTO media_assets
			(client_uuid, owner_type, owner_id, tier, cos_bucket, cos_key, cos_region,
			 content_type, byte_size, capture_state, verified_at, created_by, updated_by)
		VALUES ($1, 'problem', $2, 'original', $3, $4, $5, 'image/jpeg', 123,
			'CONFIRMED', now(), $6, $6)
		RETURNING id`,
		uuid.New(), ownerID, bucket, key, testRegion, e.adminID).Scan(&id))
	return id
}

// TestDeriveTiers_PNGOriginal_DerivesJPEGSiblings: a CONFIRMED PNG original derives
// web+thumb siblings that are stored as JPEG under the configured tier buckets,
// sharing the original's media_group.
func TestDeriveTiers_PNGOriginal_DerivesJPEGSiblings(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	origBytes := pngOf(t, 1280, 720, color.RGBA{R: 40, G: 80, B: 160, A: 255})
	origID := env.confirmOriginal(t, ctx, 60, origBytes)

	w := &Worker{pool: env.pool, cos: env.cos, cfg: env.cfg}
	task, err := NewDeriveTask(origID)
	require.NoError(t, err)
	require.NoError(t, w.HandleDeriveTiers(ctx, task))

	var group uuid.UUID
	require.NoError(t, env.pool.QueryRow(ctx,
		`SELECT media_group FROM media_assets WHERE id = $1`, origID).Scan(&group))

	// web sibling: lands in the web bucket and is a JPEG.
	var webBucket, webKey, webCT string
	require.NoError(t, env.pool.QueryRow(ctx, `
		SELECT cos_bucket, cos_key, content_type FROM media_assets
		WHERE media_group = $1 AND tier = 'web'`, group).Scan(&webBucket, &webKey, &webCT))
	assert.Equal(t, bucketWeb, webBucket, "web sibling stored under the web bucket")
	assert.Equal(t, "image/jpeg", webCT, "derived web sibling is JPEG")

	// thumb sibling: lands in the thumb bucket.
	var thumbBucket string
	require.NoError(t, env.pool.QueryRow(ctx, `
		SELECT cos_bucket FROM media_assets
		WHERE media_group = $1 AND tier = 'thumb'`, group).Scan(&thumbBucket))
	assert.Equal(t, bucketThumb, thumbBucket, "thumb sibling stored under the thumb bucket")

	// The bytes were actually uploaded to COS (HeadObject sees them).
	_, size, herr := env.cos.HeadObject(ctx, webBucket, webKey)
	require.NoError(t, herr)
	assert.Positive(t, size, "derived web object has bytes in COS")
}

// TestDeriveTiers_NonImageOriginal_CopiesSignableSibling: an undecodable original
// is handled non-fatally — the original bytes are copied verbatim so each sibling
// row still exists, is CONFIRMED, and yields a signable URL (task DoD).
func TestDeriveTiers_NonImageOriginal_CopiesSignableSibling(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// A non-image "original" (e.g. corrupt upload that still passed HeadObject).
	origBytes := []byte("this is definitely not a decodable image payload")
	origID := env.confirmOriginal(t, ctx, 61, origBytes)

	w := &Worker{pool: env.pool, cos: env.cos, cfg: env.cfg}
	task, err := NewDeriveTask(origID)
	require.NoError(t, err)
	require.NoError(t, w.HandleDeriveTiers(ctx, task), "non-image original must not fail derive")

	var group uuid.UUID
	require.NoError(t, env.pool.QueryRow(ctx,
		`SELECT media_group FROM media_assets WHERE id = $1`, origID).Scan(&group))

	// Both siblings exist and are CONFIRMED.
	var n int
	require.NoError(t, env.pool.QueryRow(ctx, `
		SELECT count(*) FROM media_assets
		WHERE media_group = $1 AND tier IN ('web','thumb') AND capture_state = 'CONFIRMED'`,
		group).Scan(&n))
	assert.Equal(t, 2, n, "web+thumb siblings exist even for a non-image original")

	// The web sibling is signable, and the copied bytes match the original verbatim.
	var webID int64
	var webBucket, webKey string
	require.NoError(t, env.pool.QueryRow(ctx, `
		SELECT id, cos_bucket, cos_key FROM media_assets
		WHERE media_group = $1 AND tier = 'web'`, group).Scan(&webID, &webBucket, &webKey))
	got, err := env.svc.Get(ctx, webID)
	require.NoError(t, err)
	require.NotNil(t, got.SignedUrl)
	assert.NotEmpty(t, *got.SignedUrl)

	rc, err := env.cos.GetObject(ctx, webBucket, webKey)
	require.NoError(t, err)
	copied, err := readAllClose(rc)
	require.NoError(t, err)
	assert.Equal(t, origBytes, copied, "non-image original is copied verbatim into the sibling")
}

// TestDeriveTiers_BadPayload_DroppedNoRetry: a malformed task payload is dropped
// (wrapped with asynq.SkipRetry) rather than retried forever.
func TestDeriveTiers_BadPayload_DroppedNoRetry(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	w := &Worker{pool: env.pool, cos: env.cos, cfg: env.cfg}
	bad := asynq.NewTask(TaskDeriveMediaTiers, []byte("{not valid json"))
	err := w.HandleDeriveTiers(ctx, bad)
	require.Error(t, err)
	assert.ErrorIs(t, err, asynq.SkipRetry, "a malformed payload must not be retried")
}

// TestDeriveTiers_MissingOriginal_DroppedNoRetry: an original id that does not
// exist (or is soft-deleted) is dropped with SkipRetry — there is nothing to
// retry against.
func TestDeriveTiers_MissingOriginal_DroppedNoRetry(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	w := &Worker{pool: env.pool, cos: env.cos, cfg: env.cfg}
	task, err := NewDeriveTask(9_999_999) // no such original
	require.NoError(t, err)
	err = w.HandleDeriveTiers(ctx, task)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
	assert.ErrorIs(t, err, asynq.SkipRetry, "a missing source is not retryable")
}

// TestDeriveTiers_GetObjectMissing_Retryable: the row is CONFIRMED but the COS
// original is absent — a transient infra fault, so the error is returned WITHOUT
// SkipRetry so asynq re-drives it.
func TestDeriveTiers_GetObjectMissing_Retryable(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// A CONFIRMED original row whose COS object was never seeded: loadOriginal
	// succeeds (DB says CONFIRMED+original) but GetObject inside derive fails.
	origID := env.insertConfirmedOriginal(t, ctx, 62,
		bucketOriginal, "media/problem/62/never-seeded/original.jpg")

	w := &Worker{pool: env.pool, cos: env.cos, cfg: env.cfg}
	task, err := NewDeriveTask(origID)
	require.NoError(t, err)
	err = w.HandleDeriveTiers(ctx, task)
	require.Error(t, err)
	assert.NotErrorIs(t, err, asynq.SkipRetry, "a missing COS object is a transient fault — must be retried")
}

// TestDeriveTiers_PreexistingGroup_Reused: when the original already carries a
// media_group, derive reuses it rather than minting a new one.
func TestDeriveTiers_PreexistingGroup_Reused(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	origID := env.confirmOriginal(t, ctx, 63, jpegBytes(t, 300, 300))
	existing := uuid.New()
	_, err := env.pool.Exec(ctx,
		`UPDATE media_assets SET media_group = $2 WHERE id = $1`, origID, existing)
	require.NoError(t, err)

	w := &Worker{pool: env.pool, cos: env.cos, cfg: env.cfg}
	task, err := NewDeriveTask(origID)
	require.NoError(t, err)
	require.NoError(t, w.HandleDeriveTiers(ctx, task))

	var group uuid.UUID
	require.NoError(t, env.pool.QueryRow(ctx,
		`SELECT media_group FROM media_assets WHERE id = $1`, origID).Scan(&group))
	assert.Equal(t, existing, group, "a preexisting media_group is reused, not replaced")

	var siblings int
	require.NoError(t, env.pool.QueryRow(ctx, `
		SELECT count(*) FROM media_assets
		WHERE media_group = $1 AND tier IN ('web','thumb')`, existing).Scan(&siblings))
	assert.Equal(t, 2, siblings, "siblings share the preexisting group")
}

// TestLoadOriginal_NonOriginalTier_Rejected: derive's loadOriginal rejects a row
// whose tier is not original.
func TestLoadOriginal_NonOriginalTier_Rejected(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// Build a CONFIRMED original, then derive its web sibling, then ask
	// loadOriginal to load the WEB sibling — it must reject with ErrNotOriginal.
	origID := env.confirmOriginal(t, ctx, 64, jpegBytes(t, 256, 256))
	w := &Worker{pool: env.pool, cos: env.cos, cfg: env.cfg}
	task, err := NewDeriveTask(origID)
	require.NoError(t, err)
	require.NoError(t, w.HandleDeriveTiers(ctx, task))

	var webID int64
	require.NoError(t, env.pool.QueryRow(ctx, `
		SELECT m.id FROM media_assets m
		JOIN media_assets o ON o.media_group = m.media_group
		WHERE o.id = $1 AND m.tier = 'web'`, origID).Scan(&webID))

	err = w.loadOriginalErr(ctx, webID)
	require.ErrorIs(t, err, ErrNotOriginal)
}

// TestLoadOriginal_NotFound: an unknown id → ErrNotFound.
func TestLoadOriginal_NotFound(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	w := &Worker{pool: env.pool, cos: env.cos, cfg: env.cfg}
	err := w.loadOriginalErr(ctx, 8_888_888)
	require.ErrorIs(t, err, ErrNotFound)
}

// TestRegisterWorkers_DispatchesDerive: RegisterWorkers mounts the derive handler
// on the asynq mux and a dispatched task produces the siblings end-to-end.
func TestRegisterWorkers_DispatchesDerive(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	origID := env.confirmOriginal(t, ctx, 65, jpegBytes(t, 800, 400))

	mux := asynq.NewServeMux()
	RegisterWorkers(mux, env.pool, env.cos, env.cfg)

	task, err := NewDeriveTask(origID)
	require.NoError(t, err)
	require.NoError(t, mux.ProcessTask(ctx, task), "registered handler processes the derive task")

	var siblings int
	require.NoError(t, env.pool.QueryRow(ctx, `
		SELECT count(*) FROM media_assets
		WHERE owner_type='problem' AND owner_id=65 AND tier IN ('web','thumb')`).Scan(&siblings))
	assert.Equal(t, 2, siblings, "the registered worker derived both siblings")
}

// TestRegisterWorkers_UnknownTask_NotFound: the media mux only knows its own task
// type; an unrelated task type is reported as not-found by asynq's mux.
//
// (M1 regression tests precede this.)

// TestDeriveTiers_TwiceSharesSingleGroup: deriving an original twice (an asynq
// retry) leaves exactly ONE distinct media_group across the original and every
// sibling — no orphan group from a concurrent/retried run (M1 regression).
func TestDeriveTiers_TwiceSharesSingleGroup(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	origID := env.confirmOriginal(t, ctx, 66, jpegBytes(t, 512, 384))
	w := &Worker{pool: env.pool, cos: env.cos, cfg: env.cfg}
	task, err := NewDeriveTask(origID)
	require.NoError(t, err)
	require.NoError(t, w.HandleDeriveTiers(ctx, task))
	require.NoError(t, w.HandleDeriveTiers(ctx, task)) // retry

	// Exactly one distinct media_group across the original + its web/thumb siblings.
	var groups int
	require.NoError(t, env.pool.QueryRow(ctx, `
		SELECT count(DISTINCT media_group) FROM media_assets
		WHERE owner_type='problem' AND owner_id=66 AND media_group IS NOT NULL`).Scan(&groups))
	assert.Equal(t, 1, groups, "original + siblings all share ONE media_group")

	// And no NULL-group rows leaked.
	var nullGroups int
	require.NoError(t, env.pool.QueryRow(ctx, `
		SELECT count(*) FROM media_assets
		WHERE owner_type='problem' AND owner_id=66 AND media_group IS NULL`).Scan(&nullGroups))
	assert.Zero(t, nullGroups, "every row carries the shared group")
}

// TestEnsureGroup_AdoptsExistingGroup: ensureGroup is the M1 fix — when a row
// already has a media_group (a concurrent derive won the race), a later run that
// proposes a different UUID must ADOPT the existing one, not overwrite it.
func TestEnsureGroup_AdoptsExistingGroup(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	origID := env.confirmOriginal(t, ctx, 67, jpegBytes(t, 100, 100))
	winner := uuid.New()
	_, err := env.pool.Exec(ctx,
		`UPDATE media_assets SET media_group = $2 WHERE id = $1`, origID, winner)
	require.NoError(t, err)

	w := &Worker{pool: env.pool, cos: env.cos, cfg: env.cfg}
	loser := uuid.New()
	effective, err := w.ensureGroup(ctx, origID, loser)
	require.NoError(t, err)
	assert.Equal(t, winner, effective, "ensureGroup adopts the winner's group, not the proposed loser UUID")
	assert.NotEqual(t, loser, effective)

	// The stored value is unchanged (COALESCE no-op under the existing group).
	var stored uuid.UUID
	require.NoError(t, env.pool.QueryRow(ctx,
		`SELECT media_group FROM media_assets WHERE id = $1`, origID).Scan(&stored))
	assert.Equal(t, winner, stored)
}

// TestEnsureGroup_SetsWhenNull: with no prior group, ensureGroup sets and returns
// the proposed UUID.
func TestEnsureGroup_SetsWhenNull(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	origID := env.confirmOriginal(t, ctx, 68, jpegBytes(t, 100, 100))
	proposed := uuid.New()

	w := &Worker{pool: env.pool, cos: env.cos, cfg: env.cfg}
	effective, err := w.ensureGroup(ctx, origID, proposed)
	require.NoError(t, err)
	assert.Equal(t, proposed, effective, "a NULL group adopts the proposed UUID")
}

func TestRegisterWorkers_UnknownTask_NotFound(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	mux := asynq.NewServeMux()
	RegisterWorkers(mux, env.pool, env.cos, env.cfg)

	err := mux.ProcessTask(ctx, asynq.NewTask("some:other-task", nil))
	require.Error(t, err, "an unregistered task type is rejected by the mux")
	// Sanity: it is not one of our domain sentinels.
	assert.False(t, errors.Is(err, ErrNotFound))
}
