//go:build integration

package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/rbac"
)

func init() { gin.SetMode(gin.TestMode) }

// fakeEnqueuer records enqueued tasks so a handler test can assert the derive
// task was scheduled without a Redis-backed asynq client.
type fakeEnqueuer struct {
	mu    sync.Mutex
	tasks []*asynq.Task
	err   error
}

func (f *fakeEnqueuer) Enqueue(task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	f.tasks = append(f.tasks, task)
	return &asynq.TaskInfo{ID: "fake", Queue: "default"}, nil
}

func (f *fakeEnqueuer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.tasks)
}

// wireRouter builds a gin router with the media routes mounted under a middleware
// that injects the admin user id (the routes themselves are auth-agnostic; the
// real auth/authz wrappers are tested in internal/server).
func (e *testEnv) wireRouter(enq Enqueuer) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(rbac.CtxUserID, e.adminID) })
	NewHandler(e.svc, enq).RegisterRoutes(r)
	return r
}

// do issues a request against the router and decodes the envelope.
func doReq(t *testing.T, r *gin.Engine, method, path string, body any) (*httptest.ResponseRecorder, envBody) {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var env envBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), "body: %s", w.Body.String())
	return w, env
}

// envBody mirrors the wire envelope so tests can assert success/code without the
// typed model.
type envBody struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// TestHandler_UploadCredentials_OK_200: a valid original request returns HTTP 200
// (the frozen OpenAPI declares only 200 → MediaUploadCredentialsResponse for this
// endpoint — NOT 201) with bucket/key/prefix, and the row is UPLOADING.
func TestHandler_UploadCredentials_OK_200(t *testing.T) {
	env := newTestEnv(t)
	r := env.wireRouter(nil)
	cu := uuid.New()

	w, resp := doReq(t, r, http.MethodPost, "/media/upload-credentials",
		oapi.MediaUploadCredentialsRequest{
			OwnerType:   oapi.MediaOwnerTypeProblem,
			OwnerId:     100,
			Tier:        oapi.Original,
			ContentType: "image/jpeg",
			ByteSize:    1234,
			ClientUuid:  cu,
		})
	require.Equal(t, http.StatusOK, w.Code, "contract: upload-credentials is 200, not 201; body: %s", w.Body.String())
	require.True(t, resp.Success)

	var creds oapi.MediaUploadCredentials
	require.NoError(t, json.Unmarshal(resp.Data, &creds))
	assert.Equal(t, bucketOriginal, creds.Bucket)
	assert.NotEmpty(t, creds.Key)
	assert.NotEmpty(t, creds.Prefix)
	assert.Contains(t, creds.Credentials.SessionToken, creds.Prefix)

	assert.Equal(t, oapi.UPLOADING, env.captureStateOf(t, context.Background(), cu))
}

// TestHandler_UploadCredentials_WebThumb_422: web/thumb are backend-derived (D4)
// and rejected with VALIDATION_FAILED → 422.
func TestHandler_UploadCredentials_WebThumb_422(t *testing.T) {
	env := newTestEnv(t)
	r := env.wireRouter(nil)

	for _, tier := range []oapi.MediaTier{oapi.Web, oapi.Thumb} {
		w, body := doReq(t, r, http.MethodPost, "/media/upload-credentials",
			oapi.MediaUploadCredentialsRequest{
				OwnerType:  oapi.MediaOwnerTypeProblem,
				OwnerId:    1,
				Tier:       tier,
				ClientUuid: uuid.New(),
			})
		require.Equalf(t, http.StatusUnprocessableEntity, w.Code, "tier %s", tier)
		require.NotNil(t, body.Error)
		assert.Equal(t, string(oapi.VALIDATIONFAILED), body.Error.Code)
	}
}

// TestHandler_UploadCredentials_MissingOwner_422: owner_id=0 fails validation.
func TestHandler_UploadCredentials_MissingOwner_422(t *testing.T) {
	env := newTestEnv(t)
	r := env.wireRouter(nil)

	w, body := doReq(t, r, http.MethodPost, "/media/upload-credentials",
		oapi.MediaUploadCredentialsRequest{
			OwnerType:  oapi.MediaOwnerTypeProblem,
			OwnerId:    0, // missing
			Tier:       oapi.Original,
			ClientUuid: uuid.New(),
		})
	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	require.NotNil(t, body.Error)
	assert.Equal(t, string(oapi.VALIDATIONFAILED), body.Error.Code)
}

// TestHandler_UploadCredentials_MissingClientUUID_422: a nil client_uuid fails
// validation.
func TestHandler_UploadCredentials_MissingClientUUID_422(t *testing.T) {
	env := newTestEnv(t)
	r := env.wireRouter(nil)

	w, body := doReq(t, r, http.MethodPost, "/media/upload-credentials",
		oapi.MediaUploadCredentialsRequest{
			OwnerType:  oapi.MediaOwnerTypeProblem,
			OwnerId:    1,
			Tier:       oapi.Original,
			ClientUuid: uuid.Nil, // missing
		})
	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	require.NotNil(t, body.Error)
	assert.Equal(t, string(oapi.VALIDATIONFAILED), body.Error.Code)
}

// TestHandler_UploadCredentials_BadBody_422: an unparseable body is rejected.
func TestHandler_UploadCredentials_BadBody_422(t *testing.T) {
	env := newTestEnv(t)
	r := env.wireRouter(nil)

	req := httptest.NewRequest(http.MethodPost, "/media/upload-credentials",
		bytes.NewReader([]byte("{ not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

// TestHandler_Confirm_OK_Enqueues: a verified original confirm returns 200 and
// enqueues the derive task.
func TestHandler_Confirm_OK_Enqueues(t *testing.T) {
	env := newTestEnv(t)
	enq := &fakeEnqueuer{}
	r := env.wireRouter(enq)
	ctx := context.Background()

	cu, creds := env.issueOriginal(t, ctx, 101)
	body := jpegBytes(t, 128, 128)
	env.cos.SetObject(creds.Bucket, creds.Key, "etag-confirm", body, "image/jpeg")

	w, resp := doReq(t, r, http.MethodPost, "/media/confirm", oapi.MediaConfirmRequest{
		ClientUuid: cu,
		Key:        creds.Key,
		Etag:       "etag-confirm",
		ByteSize:   int64(len(body)),
	})
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	require.True(t, resp.Success)

	var asset oapi.MediaAsset
	require.NoError(t, json.Unmarshal(resp.Data, &asset))
	assert.Equal(t, oapi.CONFIRMED, asset.CaptureState)
	assert.Equal(t, 1, enq.count(), "confirming an original enqueues exactly one derive task")
}

// TestHandler_Confirm_VerifyFailed_409: a missing COS object → MEDIA_VERIFY_FAILED
// mapped to 409, and no derive task is enqueued.
func TestHandler_Confirm_VerifyFailed_409(t *testing.T) {
	env := newTestEnv(t)
	enq := &fakeEnqueuer{}
	r := env.wireRouter(enq)
	ctx := context.Background()

	cu, creds := env.issueOriginal(t, ctx, 102)
	// Do not seed COS — HeadObject misses.

	w, body := doReq(t, r, http.MethodPost, "/media/confirm", oapi.MediaConfirmRequest{
		ClientUuid: cu,
		Key:        creds.Key,
		Etag:       "whatever",
		ByteSize:   10,
	})
	require.Equal(t, http.StatusConflict, w.Code)
	require.NotNil(t, body.Error)
	assert.Equal(t, string(oapi.MEDIAVERIFYFAILED), body.Error.Code)
	assert.Equal(t, 0, enq.count(), "a failed confirm enqueues nothing")
}

// TestHandler_Confirm_NotFound_404: an unknown client_uuid → 404.
func TestHandler_Confirm_NotFound_404(t *testing.T) {
	env := newTestEnv(t)
	r := env.wireRouter(nil)

	w, body := doReq(t, r, http.MethodPost, "/media/confirm", oapi.MediaConfirmRequest{
		ClientUuid: uuid.New(),
		Etag:       "x",
		ByteSize:   1,
	})
	require.Equal(t, http.StatusNotFound, w.Code)
	require.NotNil(t, body.Error)
	assert.Equal(t, string(oapi.NOTFOUND), body.Error.Code)
}

// TestHandler_Confirm_BadBody_422: an unparseable confirm body is rejected.
func TestHandler_Confirm_BadBody_422(t *testing.T) {
	env := newTestEnv(t)
	r := env.wireRouter(nil)

	req := httptest.NewRequest(http.MethodPost, "/media/confirm", bytes.NewReader([]byte("nope")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

// TestHandler_Confirm_NilEnqueuer_NoPanic: a handler built with a nil enqueuer
// confirms successfully and simply skips enqueueing (enqueueDerive nil-guard).
func TestHandler_Confirm_NilEnqueuer_NoPanic(t *testing.T) {
	env := newTestEnv(t)
	r := env.wireRouter(nil) // nil enqueuer
	ctx := context.Background()

	cu, creds := env.issueOriginal(t, ctx, 103)
	body := jpegBytes(t, 96, 96)
	env.cos.SetObject(creds.Bucket, creds.Key, "etag-nil", body, "image/jpeg")

	w, resp := doReq(t, r, http.MethodPost, "/media/confirm", oapi.MediaConfirmRequest{
		ClientUuid: cu,
		Key:        creds.Key,
		Etag:       "etag-nil",
		ByteSize:   int64(len(body)),
	})
	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, resp.Success)
}

// TestHandler_Get_OK_SignedURL: GET /media/:id returns the asset with a signed URL.
func TestHandler_Get_OK_SignedURL(t *testing.T) {
	env := newTestEnv(t)
	r := env.wireRouter(nil)
	ctx := context.Background()

	id := env.confirmOriginal(t, ctx, 104, jpegBytes(t, 100, 100))

	w, resp := doReq(t, r, http.MethodGet, "/media/"+strconv.FormatInt(id, 10), nil)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	require.True(t, resp.Success)

	var asset oapi.MediaAsset
	require.NoError(t, json.Unmarshal(resp.Data, &asset))
	assert.Equal(t, id, asset.Id)
	require.NotNil(t, asset.SignedUrl)
	assert.NotEmpty(t, *asset.SignedUrl)
}

// TestHandler_Get_NotFound_404: an unknown id → 404.
func TestHandler_Get_NotFound_404(t *testing.T) {
	env := newTestEnv(t)
	r := env.wireRouter(nil)

	w, body := doReq(t, r, http.MethodGet, "/media/6666666", nil)
	require.Equal(t, http.StatusNotFound, w.Code)
	require.NotNil(t, body.Error)
	assert.Equal(t, string(oapi.NOTFOUND), body.Error.Code)
}

// TestHandler_Get_InvalidID_422: a non-numeric / non-positive id is rejected
// before the service is consulted.
func TestHandler_Get_InvalidID_422(t *testing.T) {
	env := newTestEnv(t)
	r := env.wireRouter(nil)

	for _, bad := range []string{"abc", "0", "-5"} {
		w, body := doReq(t, r, http.MethodGet, "/media/"+bad, nil)
		require.Equalf(t, http.StatusUnprocessableEntity, w.Code, "id=%q", bad)
		require.NotNilf(t, body.Error, "id=%q", bad)
		assert.Equal(t, string(oapi.VALIDATIONFAILED), body.Error.Code)
	}
}

// TestHandler_Confirm_EnqueuerError_StillOK: an enqueue failure must not fail the
// client's confirm — the confirm is the source of truth and derive can be
// re-driven.
func TestHandler_Confirm_EnqueuerError_StillOK(t *testing.T) {
	env := newTestEnv(t)
	enq := &fakeEnqueuer{err: fmt.Errorf("redis down")}
	r := env.wireRouter(enq)
	ctx := context.Background()

	cu, creds := env.issueOriginal(t, ctx, 105)
	body := jpegBytes(t, 80, 80)
	env.cos.SetObject(creds.Bucket, creds.Key, "etag-eq", body, "image/jpeg")

	w, resp := doReq(t, r, http.MethodPost, "/media/confirm", oapi.MediaConfirmRequest{
		ClientUuid: cu,
		Key:        creds.Key,
		Etag:       "etag-eq",
		ByteSize:   int64(len(body)),
	})
	require.Equal(t, http.StatusOK, w.Code, "a transient enqueue failure must not fail confirm")
	require.True(t, resp.Success)
}
