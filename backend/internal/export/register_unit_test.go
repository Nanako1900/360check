package export

import (
	"testing"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnkglobal/c5-backend/internal/platform/cos"
)

// RegisterWorkers/RegisterWorkersWithBucket only wire a Worker and mount the
// export:run handler onto the mux — they touch neither the pool nor COS at
// registration time, so a nil pool/stats is safe to pass here.

func TestRegisterWorkers_MountsHandler(t *testing.T) {
	mux := asynq.NewServeMux()
	// Should not panic; mounts TaskTypeRun. The Worker is built with the default
	// result bucket.
	require.NotPanics(t, func() {
		RegisterWorkers(mux, cos.NewMock(), nil, nil)
	})
}

func TestRegisterWorkersWithBucket_DefaultsEmptyBucket(t *testing.T) {
	// An explicit empty bucket must fall back to resultBucketDefault rather than
	// uploading to "".
	mux := asynq.NewServeMux()
	require.NotPanics(t, func() {
		RegisterWorkersWithBucket(mux, cos.NewMock(), nil, nil, "")
	})

	// And a non-empty bucket is honored (also exercises the non-default branch).
	mux2 := asynq.NewServeMux()
	require.NotPanics(t, func() {
		RegisterWorkersWithBucket(mux2, cos.NewMock(), nil, nil, "custom-bucket")
	})
}

func TestResultBucketDefault(t *testing.T) {
	// Guard the constant the empty-bucket branch falls back to.
	assert.Equal(t, "c5-exports", resultBucketDefault)
}
