package export

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/cos"
)

// toAPI converts a jobRow to the oapi.ExportJob wire shape. When the job is
// SUCCEEDED and has a result object, result_url is filled with a signed COS URL
// (TTL-bounded). A nil signer (or non-terminal job) leaves result_url nil.
func toAPI(ctx context.Context, j *jobRow, signer cos.Client) oapi.ExportJob {
	out := oapi.ExportJob{
		Id:            j.ID,
		JobUuid:       parseUUID(j.JobUUID),
		Type:          j.Type,
		Status:        j.Status,
		Progress:      j.Progress,
		ProcessedRows: j.ProcessedRows,
		TotalRows:     j.TotalRows,
		ResultBucket:  j.ResultBucket,
		ResultCosKey:  j.ResultCosKey,
		Error:         j.Error,
		RequestedBy:   j.RequestedBy,
		StartedAt:     j.StartedAt,
		FinishedAt:    j.FinishedAt,
		CreatedAt:     j.CreatedAt,
	}
	if params := decodeParams(j.Params); params != nil {
		out.Params = &params
	}
	if j.Status == oapi.SUCCEEDED && signer != nil && j.ResultBucket != nil && j.ResultCosKey != nil {
		if url, err := signer.SignedURL(ctx, *j.ResultBucket, *j.ResultCosKey, SignedURLTTL); err == nil {
			out.ResultUrl = &url
		}
	}
	return out
}

// parseUUID converts the stored string to the openapi UUID type; an unparsable
// value yields the zero UUID (the DB guarantees a valid uuid, so this is defensive).
func parseUUID(s string) uuid.UUID {
	u, err := uuid.Parse(s)
	if err != nil {
		return uuid.UUID{}
	}
	return u
}

// decodeParams unmarshals the stored params jsonb into a generic map for the wire
// shape. Invalid/empty -> nil (Params omitted).
func decodeParams(raw []byte) map[string]interface{} {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil || len(m) == 0 {
		return nil
	}
	return m
}

// progressEvent is the SSE `event: progress` data payload (D2 SSE contract).
type progressEvent struct {
	Progress      int            `json:"progress"`
	ProcessedRows int            `json:"processed_rows"`
	TotalRows     *int           `json:"total_rows"`
	Status        oapi.JobStatus `json:"status"`
}

// doneEvent is the terminal SSE `event: done` data payload.
type doneEvent struct {
	Status    oapi.JobStatus `json:"status"`
	ResultURL *string        `json:"result_url,omitempty"`
	Error     *string        `json:"error,omitempty"`
}

// isTerminal reports whether a job status is a final state (the SSE loop stops).
func isTerminal(s oapi.JobStatus) bool {
	switch s {
	case oapi.SUCCEEDED, oapi.FAILED, oapi.CANCELLED:
		return true
	default:
		return false
	}
}

// signedResultURL returns a signed URL for a terminal SUCCEEDED job, or nil.
func signedResultURL(ctx context.Context, j *jobRow, signer cos.Client) *string {
	if j.Status != oapi.SUCCEEDED || signer == nil || j.ResultBucket == nil || j.ResultCosKey == nil {
		return nil
	}
	url, err := signer.SignedURL(ctx, *j.ResultBucket, *j.ResultCosKey, SignedURLTTL)
	if err != nil {
		return nil
	}
	return &url
}

// pollInterval is how often the SSE loop re-reads the job row. ~1s per the brief.
const pollInterval = 1 * time.Second
