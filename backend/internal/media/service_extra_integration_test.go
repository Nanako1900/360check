//go:build integration

package media

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
)

// TestConfirm_Idempotent_AlreadyConfirmed: re-confirming an already-CONFIRMED row
// returns the current asset and does NOT request a second derive enqueue.
func TestConfirm_Idempotent_AlreadyConfirmed(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	cu, creds := env.issueOriginal(t, ctx, 80)
	body := []byte("confirm-me-twice")
	env.cos.SetObject(creds.Bucket, creds.Key, "etag-id", body, "image/jpeg")
	in := ConfirmInput{ClientUUID: cu, Key: creds.Key, Etag: "etag-id", ByteSize: int64(len(body))}

	first, enqueue1, err := env.svc.Confirm(ctx, in, env.adminID)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.True(t, enqueue1, "first confirm of an original requests derive enqueue")

	// Second confirm: idempotent — same row, no second enqueue.
	second, enqueue2, err := env.svc.Confirm(ctx, in, env.adminID)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.False(t, enqueue2, "re-confirm must NOT request a second enqueue")
	assert.Equal(t, oapi.CONFIRMED, second.CaptureState)
	assert.Equal(t, first.Id, second.Id, "re-confirm returns the same row")
	require.NotNil(t, second.VerifiedAt)
}

// TestConfirm_KeyMismatch_VerifyFailed: a client-reported key that disagrees with
// the row's stored (STS-scoped) key fails verification before HeadObject; the row
// stays UPLOADING.
func TestConfirm_KeyMismatch_VerifyFailed(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	cu, creds := env.issueOriginal(t, ctx, 81)
	body := []byte("bytes")
	env.cos.SetObject(creds.Bucket, creds.Key, "etag-k", body, "image/jpeg")

	_, _, err := env.svc.Confirm(ctx, ConfirmInput{
		ClientUUID: cu,
		Key:        "media/problem/81/forged-different-prefix/original.jpg",
		Etag:       "etag-k",
		ByteSize:   int64(len(body)),
	}, env.adminID)
	require.ErrorIs(t, err, ErrVerifyFailed)
	assert.Equal(t, oapi.UPLOADING, env.captureStateOf(t, ctx, cu), "row stays UPLOADING on key mismatch")
}

// TestConfirm_NotFound: confirming an unknown client_uuid → ErrNotFound.
func TestConfirm_NotFound(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	_, _, err := env.svc.Confirm(ctx, ConfirmInput{
		ClientUUID: uuid.New(), Etag: "x", ByteSize: 1,
	}, env.adminID)
	require.ErrorIs(t, err, ErrNotFound)
}

// TestConfirm_SoftDeleted_NotFound: a soft-deleted row is never confirmable — the
// initial FOR UPDATE load and the idempotent re-confirm re-select both filter on
// deleted_at IS NULL (L3), so even an already-CONFIRMED-then-deleted row returns
// ErrNotFound rather than leaking a tombstoned asset.
func TestConfirm_SoftDeleted_NotFound(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// Confirm an original, then soft-delete it.
	cu, creds := env.issueOriginal(t, ctx, 83)
	body := []byte("soon-deleted")
	env.cos.SetObject(creds.Bucket, creds.Key, "etag-sd", body, "image/jpeg")
	in := ConfirmInput{ClientUUID: cu, Key: creds.Key, Etag: "etag-sd", ByteSize: int64(len(body))}
	_, _, err := env.svc.Confirm(ctx, in, env.adminID)
	require.NoError(t, err)

	_, err = env.pool.Exec(ctx,
		`UPDATE media_assets SET deleted_at = now() WHERE client_uuid = $1`, cu)
	require.NoError(t, err)

	// A replayed confirm against the now-deleted row must not resurrect it.
	_, _, err = env.svc.Confirm(ctx, in, env.adminID)
	require.ErrorIs(t, err, ErrNotFound, "a soft-deleted row is not confirmable")
}

// TestConfirm_HeadObjectError_Propagates: a HeadObject failure that is NOT
// ErrNotFound (e.g. a COS/network fault) propagates as a wrapped error, NOT as the
// client-facing ErrVerifyFailed, and the row stays UPLOADING.
func TestConfirm_HeadObjectError_Propagates(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	cu, creds := env.issueOriginal(t, ctx, 82)
	env.cos.SetObject(creds.Bucket, creds.Key, "etag-h", []byte("data"), "image/jpeg")
	// Force HeadObject to return an infra error (not ErrNotFound).
	sentinel := errors.New("cos: simulated network fault")
	env.cos.HeadErr = sentinel

	_, _, err := env.svc.Confirm(ctx, ConfirmInput{
		ClientUUID: cu, Key: creds.Key, Etag: "etag-h", ByteSize: 4,
	}, env.adminID)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel, "an infra HeadObject error propagates")
	assert.NotErrorIs(t, err, ErrVerifyFailed, "an infra fault is not a verification failure")
	assert.Equal(t, oapi.UPLOADING, env.captureStateOf(t, ctx, cu))
}

// TestIssueCredentials_InvalidOwnerType_Rejected: an owner_type outside the frozen
// enum is rejected with ErrInvalidOwnerType (handler maps to 422).
func TestIssueCredentials_InvalidOwnerType_Rejected(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	_, err := env.svc.IssueUploadCredentials(ctx, CredentialsInput{
		OwnerType:  oapi.MediaOwnerType("galaxy"),
		OwnerID:    1,
		Tier:       oapi.Original,
		ClientUUID: uuid.New(),
	}, env.adminID)
	require.ErrorIs(t, err, ErrInvalidOwnerType)
}

// TestIssueCredentials_AllOwnerTypes_Accepted: every frozen owner_type literal is
// accepted (covers all validOwnerType branches).
func TestIssueCredentials_AllOwnerTypes_Accepted(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	for _, ot := range []oapi.MediaOwnerType{
		oapi.MediaOwnerTypeProblem, oapi.MediaOwnerTypeInspection,
		oapi.MediaOwnerTypeProject, oapi.MediaOwnerTypeUser,
	} {
		creds, err := env.svc.IssueUploadCredentials(ctx, CredentialsInput{
			OwnerType:  ot,
			OwnerID:    2,
			Tier:       oapi.Original,
			ClientUUID: uuid.New(),
		}, env.adminID)
		require.NoErrorf(t, err, "owner_type %s must be accepted", ot)
		assert.Equal(t, bucketOriginal, creds.Bucket)
		assert.Contains(t, creds.Key, string(ot), "key is namespaced by owner_type")
	}
}

// TestIssueCredentials_STSIssuerError_Propagates: an STS failure surfaces as a
// wrapped error (the row was already inserted; the client retries).
func TestIssueCredentials_STSIssuerError_Propagates(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	env.sts.Err = errors.New("sts: simulated AssumeRole denial")

	cu := uuid.New()
	_, err := env.svc.IssueUploadCredentials(ctx, CredentialsInput{
		OwnerType:  oapi.MediaOwnerTypeProblem,
		OwnerID:    3,
		Tier:       oapi.Original,
		ClientUUID: cu,
	}, env.adminID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "issue sts", "the STS failure is wrapped with media context")

	// The UPLOADING row was still inserted (idempotent on the client_uuid) so a
	// retry reuses it.
	assert.Equal(t, oapi.UPLOADING, env.captureStateOf(t, ctx, cu))
}

// TestIssueCredentials_DefaultContentType: an empty content_type defaults to
// image/jpeg on the persisted row.
func TestIssueCredentials_DefaultContentType(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	cu := uuid.New()
	_, err := env.svc.IssueUploadCredentials(ctx, CredentialsInput{
		OwnerType:  oapi.MediaOwnerTypeProblem,
		OwnerID:    4,
		Tier:       oapi.Original,
		ClientUUID: cu,
		// ContentType intentionally empty.
	}, env.adminID)
	require.NoError(t, err)

	var ct string
	require.NoError(t, env.pool.QueryRow(ctx,
		`SELECT content_type FROM media_assets WHERE client_uuid = $1`, cu).Scan(&ct))
	assert.Equal(t, "image/jpeg", ct, "empty content_type defaults to image/jpeg")
}

// TestGet_UnknownID_NotFound: a never-inserted id → ErrNotFound (distinct from the
// soft-deleted case already covered).
func TestGet_UnknownID_NotFound(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	_, err := env.svc.Get(ctx, 7_777_777)
	require.ErrorIs(t, err, ErrNotFound)
}
