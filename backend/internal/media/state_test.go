package media

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
)

// TestCanTransition_ForwardOnly: every forward (or equal) move along the capture
// chain is legal; every backward move is rejected; unknown states are rejected.
func TestCanTransition_ForwardOnly(t *testing.T) {
	chain := []oapi.CaptureState{
		oapi.CAPTUREDRAW, oapi.STITCHED, oapi.QUEUED,
		oapi.UPLOADING, oapi.UPLOADED, oapi.CONFIRMED,
	}

	for i, from := range chain {
		for j, to := range chain {
			got := canTransition(from, to)
			want := j >= i // forward or self
			assert.Equalf(t, want, got, "transition %s -> %s", from, to)
		}
	}
}

// TestCanTransition_Unknown: an unrecognized state never transitions.
func TestCanTransition_Unknown(t *testing.T) {
	assert.False(t, canTransition(oapi.CaptureState("BOGUS"), oapi.CONFIRMED))
	assert.False(t, canTransition(oapi.UPLOADING, oapi.CaptureState("BOGUS")))
}

// TestCaptureRank_Order: ranks are strictly increasing along the chain.
func TestCaptureRank_Order(t *testing.T) {
	assert.Equal(t, 0, captureRank(oapi.CAPTUREDRAW))
	assert.Equal(t, 5, captureRank(oapi.CONFIRMED))
	assert.Equal(t, -1, captureRank(oapi.CaptureState("NOPE")))
	assert.Less(t, captureRank(oapi.UPLOADING), captureRank(oapi.CONFIRMED))
}
