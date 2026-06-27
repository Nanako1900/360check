package media

import "github.com/nnkglobal/c5-backend/internal/gen/oapi"

// captureOrder is the canonical forward-only ordering of the capture state
// machine (docs/00 D4): CAPTURED_RAW → STITCHED → QUEUED → UPLOADING → UPLOADED →
// CONFIRMED. The lower the index, the earlier the state; a transition is legal
// only when it moves strictly forward (or is a no-op self-transition).
var captureOrder = map[oapi.CaptureState]int{
	oapi.CAPTUREDRAW: 0,
	oapi.STITCHED:    1,
	oapi.QUEUED:      2,
	oapi.UPLOADING:   3,
	oapi.UPLOADED:    4,
	oapi.CONFIRMED:   5,
}

// captureRank returns the ordinal of a capture state, or -1 if unknown.
func captureRank(s oapi.CaptureState) int {
	if r, ok := captureOrder[s]; ok {
		return r
	}
	return -1
}

// canTransition reports whether moving from -> to is a legal forward-only
// capture-state transition. Equal states are allowed (idempotent re-apply);
// any backward move or unknown state is rejected. The DB is the source of
// truth, but this guards service-level logic before it issues an UPDATE so a
// replayed/out-of-order client report can never rewind a row.
func canTransition(from, to oapi.CaptureState) bool {
	rf, rt := captureRank(from), captureRank(to)
	if rf < 0 || rt < 0 {
		return false
	}
	return rt >= rf
}
