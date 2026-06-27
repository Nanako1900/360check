package export

import (
	"context"
	"errors"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/cos"
)

// These cases exercise the pure (DB-free) branches of the service/mapping/worker
// helpers, so they run without the integration build tag and still count toward
// coverage of the validation and signing paths.

func TestCreate_RejectsBadType(t *testing.T) {
	// A bad type fails validation before the pool is touched, so a nil pool/enq
	// service is safe here.
	svc := NewService(nil, &fakeEnqUnit{})
	_, err := svc.Create(context.Background(), CreateInput{Type: oapi.ExportType("NOPE")})
	assert.ErrorIs(t, err, ErrInvalidType)
}

func TestCreate_RequiresEnqueuer(t *testing.T) {
	// Valid type but no enqueuer wired -> Create refuses before any insert.
	svc := NewService(nil, nil)
	_, err := svc.Create(context.Background(), CreateInput{Type: oapi.PROBLEMLIST})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrInvalidType, "fails for the missing enqueuer, not the type")
}

func TestGetByUUID_BadUUIDIsNotFound(t *testing.T) {
	// An unparsable uuid short-circuits to ErrNotFound before any query, so a nil
	// pool never gets dereferenced.
	svc := NewService(nil, nil)
	_, err := svc.GetByUUID(context.Background(), "not-a-uuid")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestDecodeParams(t *testing.T) {
	assert.Nil(t, decodeParams(nil), "empty -> nil (omitted)")
	assert.Nil(t, decodeParams([]byte{}), "zero-length -> nil")
	assert.Nil(t, decodeParams([]byte(`{}`)), "empty object -> nil")
	assert.Nil(t, decodeParams([]byte(`not json`)), "invalid -> nil")

	got := decodeParams([]byte(`{"project_id":7}`))
	require.NotNil(t, got)
	assert.EqualValues(t, 7, got["project_id"])
}

func TestParseUUID(t *testing.T) {
	u := parseUUID("11111111-1111-1111-1111-111111111111")
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", u.String())

	// Defensive path: an unparsable value yields the zero UUID rather than panic.
	zero := parseUUID("garbage")
	assert.Equal(t, "00000000-0000-0000-0000-000000000000", zero.String())
}

func TestClientSafe(t *testing.T) {
	assert.Equal(t, "", clientSafe(nil), "nil error -> empty message")
	msg := clientSafe(errors.New("SELECT * FROM secret_table -- internal"))
	assert.Equal(t, "export failed while generating the report", msg)
	assert.NotContains(t, msg, "secret_table", "no internal detail leaks to the client")
}

func TestToAPI_NilSignerOmitsResultURL(t *testing.T) {
	bucket, key := "c5-exports", "exports/PROBLEM_LIST/x.xlsx"
	j := &jobRow{
		Status:       oapi.SUCCEEDED,
		ResultBucket: &bucket,
		ResultCosKey: &key,
		Params:       []byte("{}"),
	}
	// nil signer -> no result_url, no crash.
	api := toAPI(context.Background(), j, nil)
	assert.Equal(t, oapi.SUCCEEDED, api.Status)
	assert.Nil(t, api.ResultUrl)
}

func TestToAPI_NonTerminalOmitsResultURL(t *testing.T) {
	bucket, key := "c5-exports", "exports/PROBLEM_LIST/x.xlsx"
	j := &jobRow{
		Status:       oapi.RUNNING, // not SUCCEEDED
		ResultBucket: &bucket,
		ResultCosKey: &key,
		Params:       []byte("{}"),
	}
	api := toAPI(context.Background(), j, cos.NewMock())
	assert.Nil(t, api.ResultUrl, "a running job has no signed url even with a signer")
}

func TestSignedResultURL_Guards(t *testing.T) {
	ctx := context.Background()
	bucket, key := "c5-exports", "exports/PROBLEM_LIST/x.xlsx"
	signer := cos.NewMock()

	// nil signer -> nil.
	assert.Nil(t, signedResultURL(ctx, &jobRow{Status: oapi.SUCCEEDED, ResultBucket: &bucket, ResultCosKey: &key}, nil))
	// non-terminal -> nil.
	assert.Nil(t, signedResultURL(ctx, &jobRow{Status: oapi.RUNNING, ResultBucket: &bucket, ResultCosKey: &key}, signer))
	// missing object -> nil.
	assert.Nil(t, signedResultURL(ctx, &jobRow{Status: oapi.SUCCEEDED}, signer))

	// Fully populated SUCCEEDED -> signed url present.
	got := signedResultURL(ctx, &jobRow{Status: oapi.SUCCEEDED, ResultBucket: &bucket, ResultCosKey: &key}, signer)
	require.NotNil(t, got)
	assert.Contains(t, *got, "mock-cdn.example.com")
}

// fakeEnqUnit is a no-op enqueuer for the DB-free service cases (it is never
// actually called because those cases fail before enqueueing).
type fakeEnqUnit struct{}

func (fakeEnqUnit) Enqueue(_ *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	return nil, nil
}
