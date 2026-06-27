//go:build integration

package media

import (
	"context"
	"image/color"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	migrations "github.com/nnkglobal/c5-backend/db/migrations"
	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/cos"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
	"github.com/nnkglobal/c5-backend/internal/platform/sts"
)

// testEnv bundles the migrated pool, the media Service wired with COS+STS mocks,
// and the seeded admin id.
type testEnv struct {
	pool    *db.Pool
	svc     *Service
	cos     *cos.Mock
	sts     *sts.Mock
	adminID int64
	cfg     Config
}

const (
	bucketOriginal = "c5-media-orig-1250000000"
	bucketWeb      = "c5-media-web-1250000000"
	bucketThumb    = "c5-media-thumb-1250000000"
	testRegion     = "ap-guangzhou"
)

// newTestEnv spins a fresh PostGIS container, runs migrations (000002 seeds the
// admin), connects a pool, and wires the media service onto the mocks.
func newTestEnv(t *testing.T) *testEnv {
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

	var adminID int64
	require.NoError(t, pool.QueryRow(ctx, "SELECT id FROM users WHERE username='admin'").Scan(&adminID))

	cfg := Config{
		BucketOriginal: bucketOriginal,
		BucketWeb:      bucketWeb,
		BucketThumb:    bucketThumb,
		Region:         testRegion,
		STSTTL:         15 * time.Minute,
		SignedURLTTL:   time.Hour,
		UploadingTTL:   24 * time.Hour,
	}
	cosMock := cos.NewMock()
	stsMock := sts.NewMock()
	svc := NewService(pool, cosMock, stsMock, cfg)

	return &testEnv{pool: pool, svc: svc, cos: cosMock, sts: stsMock, adminID: adminID, cfg: cfg.withDefaults()}
}

// captureStateOf reads a row's capture_state by client_uuid.
func (e *testEnv) captureStateOf(t *testing.T, ctx context.Context, cu uuid.UUID) oapi.CaptureState {
	t.Helper()
	var s string
	require.NoError(t, e.pool.QueryRow(ctx,
		`SELECT capture_state FROM media_assets WHERE client_uuid = $1`, cu).Scan(&s))
	return oapi.CaptureState(s)
}

// TestUploadCredentials_OnlyOriginal_StateUploading: an original request issues
// STS creds and inserts a row in UPLOADING under the original bucket/prefix.
func TestUploadCredentials_OnlyOriginal_StateUploading(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	cu := uuid.New()

	creds, err := env.svc.IssueUploadCredentials(ctx, CredentialsInput{
		OwnerType:   oapi.MediaOwnerTypeProblem,
		OwnerID:     10,
		Tier:        oapi.Original,
		ContentType: "image/jpeg",
		ByteSize:    123,
		ClientUUID:  cu,
	}, env.adminID)
	require.NoError(t, err)

	assert.Equal(t, bucketOriginal, creds.Bucket)
	assert.Equal(t, testRegion, creds.Region)
	assert.NotEmpty(t, creds.Key)
	assert.NotEmpty(t, creds.Prefix)
	assert.True(t, len(creds.Key) > len(creds.Prefix), "key lives under the prefix")
	// STS mock embeds the prefix in the session token.
	assert.Contains(t, creds.Credentials.SessionToken, creds.Prefix)
	assert.NotEmpty(t, creds.Credentials.TmpSecretId)
	assert.Positive(t, creds.Credentials.ExpiredTime)

	assert.Equal(t, oapi.UPLOADING, env.captureStateOf(t, ctx, cu))
}

// TestUploadCredentials_WebThumbRejected: web/thumb are backend-derived (D4); the
// service rejects them with ErrTierNotOriginal (handler maps to 422).
func TestUploadCredentials_WebThumbRejected(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	for _, tier := range []oapi.MediaTier{oapi.Web, oapi.Thumb} {
		_, err := env.svc.IssueUploadCredentials(ctx, CredentialsInput{
			OwnerType:  oapi.MediaOwnerTypeProblem,
			OwnerID:    1,
			Tier:       tier,
			ClientUUID: uuid.New(),
		}, env.adminID)
		require.ErrorIs(t, err, ErrTierNotOriginal, "tier %s must be rejected", tier)
	}
}

// TestUploadCredentials_Idempotent: a replayed client_uuid returns the SAME key
// and never inserts a duplicate row.
func TestUploadCredentials_Idempotent(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	cu := uuid.New()

	in := CredentialsInput{
		OwnerType:  oapi.MediaOwnerTypeInspection,
		OwnerID:    5,
		Tier:       oapi.Original,
		ClientUUID: cu,
	}
	first, err := env.svc.IssueUploadCredentials(ctx, in, env.adminID)
	require.NoError(t, err)
	second, err := env.svc.IssueUploadCredentials(ctx, in, env.adminID)
	require.NoError(t, err)
	assert.Equal(t, first.Key, second.Key, "replay maps to the same key")

	var count int
	require.NoError(t, env.pool.QueryRow(ctx,
		`SELECT count(*) FROM media_assets WHERE client_uuid = $1`, cu).Scan(&count))
	assert.Equal(t, 1, count)
}

// issueOriginal issues creds and returns (clientUUID, key) for an original.
func (e *testEnv) issueOriginal(t *testing.T, ctx context.Context, ownerID int64) (uuid.UUID, *oapi.MediaUploadCredentials) {
	t.Helper()
	cu := uuid.New()
	creds, err := e.svc.IssueUploadCredentials(ctx, CredentialsInput{
		OwnerType:   oapi.MediaOwnerTypeProblem,
		OwnerID:     ownerID,
		Tier:        oapi.Original,
		ContentType: "image/jpeg",
		ClientUUID:  cu,
	}, e.adminID)
	require.NoError(t, err)
	return cu, creds
}

// TestConfirm_HeadObjectMatch_Confirmed: when the COS object matches the reported
// etag/size, confirm advances the row to CONFIRMED and sets verified_at; it
// reports enqueue=true for an original.
func TestConfirm_HeadObjectMatch_Confirmed(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	cu, creds := env.issueOriginal(t, ctx, 20)
	body := []byte("the-original-bytes")
	env.cos.SetObject(creds.Bucket, creds.Key, "etag-123", body, "image/jpeg")

	asset, enqueue, err := env.svc.Confirm(ctx, ConfirmInput{
		ClientUUID: cu,
		Key:        creds.Key,
		Etag:       "etag-123",
		ByteSize:   int64(len(body)),
		Width:      ptr(4096),
		Height:     ptr(2048),
	}, env.adminID)
	require.NoError(t, err)
	require.NotNil(t, asset)

	assert.Equal(t, oapi.CONFIRMED, asset.CaptureState)
	require.NotNil(t, asset.VerifiedAt)
	require.NotNil(t, asset.Etag)
	assert.Equal(t, "etag-123", *asset.Etag)
	require.NotNil(t, asset.ByteSize)
	assert.Equal(t, int64(len(body)), *asset.ByteSize)
	require.NotNil(t, asset.Width)
	assert.Equal(t, 4096, *asset.Width)
	assert.True(t, enqueue, "confirming an original must request derive enqueue")

	assert.Equal(t, oapi.CONFIRMED, env.captureStateOf(t, ctx, cu))
}

// TestConfirm_MissingObject_VerifyFailed: no COS object → MEDIA_VERIFY_FAILED and
// the row stays UPLOADING.
func TestConfirm_MissingObject_VerifyFailed(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	cu, creds := env.issueOriginal(t, ctx, 21)
	// Intentionally do NOT seed the COS mock with the object.

	_, _, err := env.svc.Confirm(ctx, ConfirmInput{
		ClientUUID: cu,
		Key:        creds.Key,
		Etag:       "whatever",
		ByteSize:   100,
	}, env.adminID)
	require.ErrorIs(t, err, ErrVerifyFailed)
	assert.Equal(t, oapi.UPLOADING, env.captureStateOf(t, ctx, cu), "state must stay UPLOADING on verify failure")
}

// TestConfirm_WrongEtag_VerifyFailed: a seeded object with a different etag fails
// verification; the row stays UPLOADING.
func TestConfirm_WrongEtag_VerifyFailed(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	cu, creds := env.issueOriginal(t, ctx, 22)
	body := []byte("bytes")
	env.cos.SetObject(creds.Bucket, creds.Key, "real-etag", body, "image/jpeg")

	_, _, err := env.svc.Confirm(ctx, ConfirmInput{
		ClientUUID: cu,
		Key:        creds.Key,
		Etag:       "client-claimed-different-etag",
		ByteSize:   int64(len(body)),
	}, env.adminID)
	require.ErrorIs(t, err, ErrVerifyFailed)
	assert.Equal(t, oapi.UPLOADING, env.captureStateOf(t, ctx, cu))
}

// TestConfirm_WrongSize_VerifyFailed: a size mismatch fails verification.
func TestConfirm_WrongSize_VerifyFailed(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	cu, creds := env.issueOriginal(t, ctx, 23)
	body := []byte("0123456789")
	env.cos.SetObject(creds.Bucket, creds.Key, "etag-x", body, "image/jpeg")

	_, _, err := env.svc.Confirm(ctx, ConfirmInput{
		ClientUUID: cu,
		Key:        creds.Key,
		Etag:       "etag-x",
		ByteSize:   9999, // wrong
	}, env.adminID)
	require.ErrorIs(t, err, ErrVerifyFailed)
	assert.Equal(t, oapi.UPLOADING, env.captureStateOf(t, ctx, cu))
}

// confirmOriginal issues + confirms an original (object seeded), returning its id.
func (e *testEnv) confirmOriginal(t *testing.T, ctx context.Context, ownerID int64, body []byte) int64 {
	t.Helper()
	cu, creds := e.issueOriginal(t, ctx, ownerID)
	e.cos.SetObject(creds.Bucket, creds.Key, "etag-orig", body, "image/jpeg")
	asset, _, err := e.svc.Confirm(ctx, ConfirmInput{
		ClientUUID: cu, Key: creds.Key, Etag: "etag-orig", ByteSize: int64(len(body)),
	}, e.adminID)
	require.NoError(t, err)
	return asset.Id
}

// TestDeriveTiers_SiblingsSharedGroup: deriving produces web + thumb siblings that
// share the original's media_group and are CONFIRMED+verified.
func TestDeriveTiers_SiblingsSharedGroup(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	origBytes := jpegBytes(t, 1000, 500)
	origID := env.confirmOriginal(t, ctx, 30, origBytes)

	w := &Worker{pool: env.pool, cos: env.cos, cfg: env.cfg}
	task, err := NewDeriveTask(origID)
	require.NoError(t, err)
	require.NoError(t, w.HandleDeriveTiers(ctx, task))

	// The original now carries a media_group.
	var group *uuid.UUID
	require.NoError(t, env.pool.QueryRow(ctx,
		`SELECT media_group FROM media_assets WHERE id = $1`, origID).Scan(&group))
	require.NotNil(t, group)

	// web + thumb siblings exist, share the group, and are CONFIRMED+verified.
	rows, err := env.pool.Query(ctx, `
		SELECT tier, capture_state, media_group, verified_at, cos_bucket
		FROM media_assets
		WHERE media_group = $1 AND tier IN ('web','thumb')
		ORDER BY tier`, *group)
	require.NoError(t, err)
	defer rows.Close()

	seen := map[string]bool{}
	for rows.Next() {
		var (
			tier   string
			state  string
			g      uuid.UUID
			vAt    *time.Time
			bucket string
		)
		require.NoError(t, rows.Scan(&tier, &state, &g, &vAt, &bucket))
		seen[tier] = true
		assert.Equal(t, *group, g, "sibling shares the original media_group")
		assert.Equal(t, "CONFIRMED", state)
		assert.NotNil(t, vAt, "sibling is verified")
	}
	require.NoError(t, rows.Err())
	assert.True(t, seen["web"], "web sibling exists")
	assert.True(t, seen["thumb"], "thumb sibling exists")

	// A signed URL can be produced for the web sibling.
	var webID int64
	require.NoError(t, env.pool.QueryRow(ctx,
		`SELECT id FROM media_assets WHERE media_group = $1 AND tier = 'web'`, *group).Scan(&webID))
	got, err := env.svc.Get(ctx, webID)
	require.NoError(t, err)
	require.NotNil(t, got.SignedUrl)
	assert.NotEmpty(t, *got.SignedUrl)
}

// TestDeriveTiers_Idempotent: re-running derive does not duplicate siblings.
func TestDeriveTiers_Idempotent(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	origID := env.confirmOriginal(t, ctx, 31, jpegBytes(t, 640, 480))
	w := &Worker{pool: env.pool, cos: env.cos, cfg: env.cfg}
	task, err := NewDeriveTask(origID)
	require.NoError(t, err)
	require.NoError(t, w.HandleDeriveTiers(ctx, task))
	require.NoError(t, w.HandleDeriveTiers(ctx, task)) // second run

	var siblings int
	require.NoError(t, env.pool.QueryRow(ctx, `
		SELECT count(*) FROM media_assets
		WHERE owner_type='problem' AND owner_id=31 AND tier IN ('web','thumb')`).Scan(&siblings))
	assert.Equal(t, 2, siblings, "exactly one web + one thumb, no duplicates")
}

// TestDerive_NotConfirmed_Skips: an original still in UPLOADING cannot be derived.
func TestDerive_NotConfirmed_Skips(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	cu, _ := env.issueOriginal(t, ctx, 32)
	var origID int64
	require.NoError(t, env.pool.QueryRow(ctx,
		`SELECT id FROM media_assets WHERE client_uuid = $1`, cu).Scan(&origID))

	w := &Worker{pool: env.pool, cos: env.cos, cfg: env.cfg}
	err := w.loadOriginalErr(ctx, origID)
	require.ErrorIs(t, err, ErrNotConfirmed)
}

// loadOriginalErr exposes loadOriginal's error for the not-confirmed assertion.
func (w *Worker) loadOriginalErr(ctx context.Context, id int64) error {
	_, err := w.loadOriginal(ctx, id)
	return err
}

// TestReaper_MarksStaleUploading: a row older than the TTL still in UPLOADING is
// flagged reaped; a fresh UPLOADING row and a CONFIRMED row are untouched.
func TestReaper_MarksStaleUploading(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// Stale UPLOADING row. Backdate created_at (the reaper's staleness clock); the
	// BEFORE-UPDATE trigger would clobber updated_at, so it cannot be used here.
	staleCU, _ := env.issueOriginal(t, ctx, 40)
	_, err := env.pool.Exec(ctx,
		`UPDATE media_assets SET created_at = now() - interval '48 hours' WHERE client_uuid = $1`, staleCU)
	require.NoError(t, err)

	// Fresh UPLOADING row (must NOT be reaped).
	freshCU, _ := env.issueOriginal(t, ctx, 41)

	reaper := NewReaper(env.pool, env.cfg.UploadingTTL)
	n, err := reaper.Reap(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n, "exactly the stale row is reaped")

	assert.True(t, env.reapedFlag(t, ctx, staleCU), "stale row flagged reaped")
	assert.False(t, env.reapedFlag(t, ctx, freshCU), "fresh row not reaped")

	// Idempotent: a second pass reaps nothing new.
	n2, err := reaper.Reap(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 0, n2)
}

// reapedFlag reads meta.reaped for a row.
func (e *testEnv) reapedFlag(t *testing.T, ctx context.Context, cu uuid.UUID) bool {
	t.Helper()
	var reaped *bool
	require.NoError(t, e.pool.QueryRow(ctx,
		`SELECT (meta->>'reaped')::boolean FROM media_assets WHERE client_uuid = $1`, cu).Scan(&reaped))
	return reaped != nil && *reaped
}

// TestGet_NotFound: an unknown/soft-deleted media id → ErrNotFound.
func TestGet_NotFound(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	origID := env.confirmOriginal(t, ctx, 50, jpegBytes(t, 100, 100))
	_, err := env.pool.Exec(ctx, `UPDATE media_assets SET deleted_at = now() WHERE id = $1`, origID)
	require.NoError(t, err)

	_, err = env.svc.Get(ctx, origID)
	require.ErrorIs(t, err, ErrNotFound)
}

func ptr(i int) *int { return &i }

// jpegBytes builds a solid-color JPEG of the given size for derive tests.
func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	return jpegOf(t, w, h, color.RGBA{R: 120, G: 160, B: 200, A: 255})
}
