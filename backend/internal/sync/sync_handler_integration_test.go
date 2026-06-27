//go:build integration

package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/rbac"
)

// newRouter mounts POST /sync/batch on a fresh gin engine, injecting the seeded
// admin as the rbac actor so the handler's rbac.UserIDFromContext resolves to a
// real user (the default inspector/owner/creator). Mirrors the dict harness.
func newRouter(t *testing.T, env *testEnv) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(rbac.CtxUserID, env.adminID)
		c.Next()
	})
	NewHandler(env.svc).RegisterRoutes(r)
	return r
}

// postBatch issues POST /sync/batch with the given raw body and returns the recorder.
func postBatch(t *testing.T, r *gin.Engine, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/sync/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// decodeResult unmarshals the success envelope's data into a SyncBatchResult.
func decodeResult(t *testing.T, body []byte) oapi.SyncBatchResult {
	t.Helper()
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &env), "body=%s", body)
	require.True(t, env.Success, "expected success envelope, got: %s", body)
	var out oapi.SyncBatchResult
	require.NoError(t, json.Unmarshal(env.Data, &out))
	return out
}

// TestHandler_WellFormedEnvelopeIs200 proves the contract that a well-formed
// envelope is ALWAYS HTTP 200 — even when it carries a per-item rejection — with
// the per-item accept/reject status living in the body, never the status line.
func TestHandler_WellFormedEnvelopeIs200(t *testing.T) {
	env := newTestEnv(t)
	r := newRouter(t, env)

	goodInsp := uuid.New()
	badInsp := uuid.New()
	started := time.Now().UTC()
	req := oapi.SyncBatchRequest{
		Inspections: &[]oapi.SyncInspectionItem{
			{ClientUuid: goodInsp, ProjectId: env.projectID, StartedAt: started},
			{ClientUuid: badInsp, ProjectId: 999999, StartedAt: started}, // FK violation
		},
	}
	body, err := json.Marshal(req)
	require.NoError(t, err)

	w := postBatch(t, r, body)
	require.Equal(t, http.StatusOK, w.Code, "well-formed envelope is 200; body=%s", w.Body.String())

	res := decodeResult(t, w.Body.Bytes())
	m := byUUID(res.Results)
	assert.Equal(t, oapi.Accepted, m[goodInsp.String()].Status)
	assert.Equal(t, oapi.Rejected, m[badInsp.String()].Status, "per-item rejection rides in the body, not the status code")
	require.NotNil(t, res.Accepted)
	require.NotNil(t, res.Rejected)
}

// TestHandler_MalformedJSONIs422 proves the ONLY 422 path: a body that is not a
// decodable SyncBatchRequest fails validation before Process is ever called.
func TestHandler_MalformedJSONIs422(t *testing.T) {
	env := newTestEnv(t)
	r := newRouter(t, env)

	w := postBatch(t, r, []byte(`{"inspections": [ this is not json `))
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, "malformed JSON is the only 422; body=%s", w.Body.String())
}

// TestHandler_EmptyEnvelopeIs200 proves an empty (but well-formed) envelope is a
// no-op 200 with an empty results array — the boundary of "well-formed → 200".
func TestHandler_EmptyEnvelopeIs200(t *testing.T) {
	env := newTestEnv(t)
	r := newRouter(t, env)

	w := postBatch(t, r, []byte(`{}`))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	res := decodeResult(t, w.Body.Bytes())
	assert.Empty(t, res.Results)
	assert.Nil(t, res.Accepted)
	assert.Nil(t, res.Rejected)
	assert.Nil(t, res.Duplicate)
}

// TestHandler_ActorSuppliesDefaultInspector proves the acting user (rbac context)
// flows through the handler as the default inspector/creator for an item that
// omits inspector_id — exercising the full HTTP -> Process -> persist path.
func TestHandler_ActorSuppliesDefaultInspector(t *testing.T) {
	env := newTestEnv(t)
	r := newRouter(t, env)
	ctx := context.Background()

	probUUID := uuid.New()
	req := oapi.SyncBatchRequest{
		Problems: &[]oapi.SyncProblemItem{{
			ClientUuid: probUUID, ProjectId: env.projectID,
			Geom: pt(113.0, 23.0), CapturedAt: time.Now().UTC(),
		}},
	}
	body, err := json.Marshal(req)
	require.NoError(t, err)

	w := postBatch(t, r, body)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var inspectorID, createdBy int64
	require.NoError(t, env.pool.QueryRow(ctx,
		`SELECT inspector_id, created_by FROM problems WHERE client_uuid=$1`,
		probUUID).Scan(&inspectorID, &createdBy))
	assert.Equal(t, env.adminID, inspectorID, "omitted inspector defaults to the acting user")
	assert.Equal(t, env.adminID, createdBy)
}

// --- service-level branch coverage: problem rejection paths -----------------

// TestProblem_UnresolvedInspectionRejected: a problem whose inspection_client_uuid
// resolves to nothing (not in batch, not on server) is rejected INSPECTION_NOT_FOUND
// while a sibling problem with no inspection still commits.
func TestProblem_UnresolvedInspectionRejected(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	orphan := uuid.New()
	missingInsp := uuid.New()
	standalone := uuid.New()
	now := time.Now().UTC()

	req := oapi.SyncBatchRequest{
		Problems: &[]oapi.SyncProblemItem{
			{
				ClientUuid: orphan, ProjectId: env.projectID,
				InspectionClientUuid: &missingInsp,
				Geom:                 pt(113.0, 23.0), CapturedAt: now,
			},
			{
				ClientUuid: standalone, ProjectId: env.projectID,
				Geom: pt(113.1, 23.1), CapturedAt: now,
			},
		},
	}
	res, err := env.svc.Process(ctx, req, env.adminID)
	require.NoError(t, err)

	m := byUUID(res.Results)
	require.Equal(t, oapi.Rejected, m[orphan.String()].Status)
	require.NotNil(t, m[orphan.String()].Error)
	assert.Equal(t, reasonInspectionNotFound, *m[orphan.String()].Error)
	assert.Equal(t, oapi.Accepted, m[standalone.String()].Status, "an inspection-less problem still commits")

	assertRowCount(t, env, ctx, "problems", orphan, 0)
	assertRowCount(t, env, ctx, "problems", standalone, 1)
}

// TestProblem_ResolvesExistingInspection: a problem can attach to an inspection that
// already exists on the server (resolved by client_uuid, not just one in this batch),
// and an explicit inspector_id overrides the actor default.
func TestProblem_ResolvesExistingInspectionAndExplicitInspector(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// Seed an inspection in a first batch.
	inspUUID := uuid.New()
	seed := oapi.SyncBatchRequest{
		Inspections: &[]oapi.SyncInspectionItem{{
			ClientUuid: inspUUID, ProjectId: env.projectID, StartedAt: time.Now().UTC(),
		}},
	}
	sr, err := env.svc.Process(ctx, seed, env.adminID)
	require.NoError(t, err)
	inspID := *byUUID(sr.Results)[inspUUID.String()].ServerId

	// A later batch's problem references that existing inspection + an explicit inspector.
	probUUID := uuid.New()
	explicitInspector := env.adminID // a real user id, set explicitly
	req := oapi.SyncBatchRequest{
		Problems: &[]oapi.SyncProblemItem{{
			ClientUuid: probUUID, ProjectId: env.projectID,
			InspectionClientUuid: &inspUUID,
			InspectorId:          &explicitInspector,
			Geom:                 pt(113.0, 23.0), CapturedAt: time.Now().UTC(),
		}},
	}
	res, err := env.svc.Process(ctx, req, env.adminID)
	require.NoError(t, err)

	r := byUUID(res.Results)[probUUID.String()]
	require.Equalf(t, oapi.Accepted, r.Status, "problem should resolve the existing inspection; error=%v", r.Error)

	var gotInspectionID, gotInspector int64
	require.NoError(t, env.pool.QueryRow(ctx,
		`SELECT inspection_id, inspector_id FROM problems WHERE client_uuid=$1`,
		probUUID).Scan(&gotInspectionID, &gotInspector))
	assert.Equal(t, inspID, gotInspectionID)
	assert.Equal(t, explicitInspector, gotInspector)
}

// TestProblem_FKViolationRejected: a problem referencing a missing project is
// rejected FK_VIOLATION (the problem-specific reject path), isolated to its savepoint.
func TestProblem_FKViolationRejected(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	bad := uuid.New()
	req := oapi.SyncBatchRequest{
		Problems: &[]oapi.SyncProblemItem{{
			ClientUuid: bad, ProjectId: 999999, // no such project
			Geom: pt(113.0, 23.0), CapturedAt: time.Now().UTC(),
		}},
	}
	res, err := env.svc.Process(ctx, req, env.adminID)
	require.NoError(t, err)

	r := byUUID(res.Results)[bad.String()]
	require.Equal(t, oapi.Rejected, r.Status)
	require.NotNil(t, r.Error)
	assert.Equal(t, reasonFKViolation, *r.Error)
	assertRowCount(t, env, ctx, "problems", bad, 0)
}

// --- media owner resolution branches ---------------------------------------

// TestMedia_OwnerResolvesToInspectionInBatch: a media item whose owner_type is
// inspection resolves to an inspection created in the SAME batch (the inspection
// arm of resolveMediaOwner's batch-map lookup).
func TestMedia_OwnerResolvesToInspectionInBatch(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	inspUUID := uuid.New()
	mediaUUID := uuid.New()
	now := time.Now().UTC()
	req := oapi.SyncBatchRequest{
		Inspections: &[]oapi.SyncInspectionItem{{
			ClientUuid: inspUUID, ProjectId: env.projectID, StartedAt: now,
		}},
		Media: &[]oapi.SyncMediaItem{{
			ClientUuid:      mediaUUID,
			OwnerType:       oapi.MediaOwnerTypeInspection,
			OwnerClientUuid: &inspUUID, // resolves to the inspection in THIS batch
			Tier:            oapi.Original,
			CosBucket:       "c5-media-1300000000",
			CosKey:          "uploads/" + mediaUUID.String() + "/original.jpg",
			CaptureState:    oapi.CAPTUREDRAW,
		}},
	}
	res, err := env.svc.Process(ctx, req, env.adminID)
	require.NoError(t, err)

	r := byUUID(res.Results)[mediaUUID.String()]
	require.Equalf(t, oapi.Accepted, r.Status, "media owner should resolve to the in-batch inspection; error=%v", r.Error)

	var ownerType string
	require.NoError(t, env.pool.QueryRow(ctx,
		`SELECT owner_type::text FROM media_assets WHERE client_uuid=$1`, mediaUUID).Scan(&ownerType))
	assert.Equal(t, "inspection", ownerType)
}

// TestMedia_OwnerResolvesToExistingInspection: an inspection-owned media item
// resolves to an inspection already on the server (the existing-row branch for the
// inspection owner_type).
func TestMedia_OwnerResolvesToExistingInspection(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	inspUUID := uuid.New()
	seed := oapi.SyncBatchRequest{
		Inspections: &[]oapi.SyncInspectionItem{{
			ClientUuid: inspUUID, ProjectId: env.projectID, StartedAt: time.Now().UTC(),
		}},
	}
	sr, err := env.svc.Process(ctx, seed, env.adminID)
	require.NoError(t, err)
	inspID := *byUUID(sr.Results)[inspUUID.String()].ServerId

	mediaUUID := uuid.New()
	req := oapi.SyncBatchRequest{
		Media: &[]oapi.SyncMediaItem{{
			ClientUuid:      mediaUUID,
			OwnerType:       oapi.MediaOwnerTypeInspection,
			OwnerClientUuid: &inspUUID,
			Tier:            oapi.Web,
			CosBucket:       "c5-media-1300000000",
			CosKey:          "uploads/" + mediaUUID.String() + "/web.jpg",
			CaptureState:    oapi.UPLOADED,
		}},
	}
	res, err := env.svc.Process(ctx, req, env.adminID)
	require.NoError(t, err)

	r := byUUID(res.Results)[mediaUUID.String()]
	require.Equalf(t, oapi.Accepted, r.Status, "media owner should resolve to the existing inspection; error=%v", r.Error)

	var ownerID int64
	require.NoError(t, env.pool.QueryRow(ctx,
		`SELECT owner_id FROM media_assets WHERE client_uuid=$1`, mediaUUID).Scan(&ownerID))
	assert.Equal(t, inspID, ownerID)
}

// TestMedia_NilOwnerUUIDRejected: a media item with no owner_client_uuid cannot be
// attached and is rejected OWNER_NOT_FOUND (the nil-owner guard).
func TestMedia_NilOwnerUUIDRejected(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	mediaUUID := uuid.New()
	req := oapi.SyncBatchRequest{
		Media: &[]oapi.SyncMediaItem{{
			ClientUuid:   mediaUUID,
			OwnerType:    oapi.MediaOwnerTypeProblem,
			Tier:         oapi.Original,
			CosBucket:    "c5-media-1300000000",
			CosKey:       "uploads/" + mediaUUID.String() + "/original.jpg",
			CaptureState: oapi.QUEUED,
			// OwnerClientUuid intentionally nil
		}},
	}
	res, err := env.svc.Process(ctx, req, env.adminID)
	require.NoError(t, err)

	r := byUUID(res.Results)[mediaUUID.String()]
	require.Equal(t, oapi.Rejected, r.Status)
	require.NotNil(t, r.Error)
	assert.Equal(t, reasonOwnerNotFound, *r.Error)
	assertRowCount(t, env, ctx, "media_assets", mediaUUID, 0)
}

// TestMedia_UnsupportedOwnerTypeRejected: a media item declaring a non-syncable
// owner_type (user/project) is rejected OWNER_NOT_FOUND (the default arm of the
// owner-type switch — these owners are never offline-synced).
func TestMedia_UnsupportedOwnerTypeRejected(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	mediaUUID := uuid.New()
	ownerUUID := uuid.New()
	req := oapi.SyncBatchRequest{
		Media: &[]oapi.SyncMediaItem{{
			ClientUuid:      mediaUUID,
			OwnerType:       oapi.MediaOwnerTypeProject, // not offline-synced
			OwnerClientUuid: &ownerUUID,
			Tier:            oapi.Original,
			CosBucket:       "c5-media-1300000000",
			CosKey:          "uploads/" + mediaUUID.String() + "/original.jpg",
			CaptureState:    oapi.QUEUED,
		}},
	}
	res, err := env.svc.Process(ctx, req, env.adminID)
	require.NoError(t, err)

	r := byUUID(res.Results)[mediaUUID.String()]
	require.Equal(t, oapi.Rejected, r.Status)
	require.NotNil(t, r.Error)
	assert.Equal(t, reasonOwnerNotFound, *r.Error)
	assertRowCount(t, env, ctx, "media_assets", mediaUUID, 0)
}

// --- inspection optional-field branches ------------------------------------

// TestInspection_AllOptionalFieldsPersist exercises the optional-field arms of
// upsertInspection: ended_at (endedAtUTC non-nil), device_info, an explicit
// non-default status, and a note (derefStr non-nil) all persist.
func TestInspection_AllOptionalFieldsPersist(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	inspUUID := uuid.New()
	started := time.Date(2026, 6, 26, 8, 0, 0, 0, time.UTC)
	ended := started.Add(2 * time.Hour)
	status := oapi.InspectionStatusFINISHED
	note := "完成巡查"
	device := map[string]interface{}{"model": "Pixel 8", "os": "Android 15"}

	req := oapi.SyncBatchRequest{
		Inspections: &[]oapi.SyncInspectionItem{{
			ClientUuid: inspUUID,
			ProjectId:  env.projectID,
			StartedAt:  started,
			EndedAt:    &ended,
			Status:     &status,
			Note:       &note,
			DeviceInfo: &device,
		}},
	}
	res, err := env.svc.Process(ctx, req, env.adminID)
	require.NoError(t, err)
	require.Equal(t, oapi.Accepted, byUUID(res.Results)[inspUUID.String()].Status)

	var (
		gotStatus string
		gotNote   string
		gotEnded  *time.Time
		gotModel  string
	)
	require.NoError(t, env.pool.QueryRow(ctx, `
		SELECT status::text, note, ended_at, device_info->>'model'
		FROM inspections WHERE client_uuid=$1`, inspUUID).
		Scan(&gotStatus, &gotNote, &gotEnded, &gotModel))
	assert.Equal(t, string(oapi.InspectionStatusFINISHED), gotStatus)
	assert.Equal(t, note, gotNote)
	require.NotNil(t, gotEnded, "ended_at must persist (endedAtUTC non-nil branch)")
	assert.True(t, gotEnded.Equal(ended), "ended_at normalized to the supplied instant")
	assert.Equal(t, "Pixel 8", gotModel)
}

// --- media dimensions regression (H1) --------------------------------------

// TestMedia_WidthHeightNotTransposed is the regression guard for H1: upsertMedia
// once bound (Height, Width) into the (width, height) columns, silently transposing
// the dimensions of every non-square asset (e.g. a 4096x2048 equirectangular pano).
// It posts a media item with DISTINCT dims and asserts the PERSISTED row keeps
// width=4096 AND height=2048 — not swapped.
func TestMedia_WidthHeightNotTransposed(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// An owner the media can attach to.
	probUUID := uuid.New()
	mediaUUID := uuid.New()
	width, height := 4096, 2048 // distinct, landscape — transposition would flip them
	byteSize := int64(1234567)

	req := oapi.SyncBatchRequest{
		Problems: &[]oapi.SyncProblemItem{{
			ClientUuid: probUUID, ProjectId: env.projectID,
			Geom: pt(113.0, 23.0), CapturedAt: time.Now().UTC(),
		}},
		Media: &[]oapi.SyncMediaItem{{
			ClientUuid:      mediaUUID,
			OwnerType:       oapi.MediaOwnerTypeProblem,
			OwnerClientUuid: &probUUID,
			Tier:            oapi.Original,
			CosBucket:       "c5-media-1300000000",
			CosKey:          "uploads/" + mediaUUID.String() + "/original.jpg",
			CaptureState:    oapi.STITCHED,
			Width:           &width,
			Height:          &height,
			ByteSize:        &byteSize,
		}},
	}
	res, err := env.svc.Process(ctx, req, env.adminID)
	require.NoError(t, err)

	r := byUUID(res.Results)[mediaUUID.String()]
	require.Equalf(t, oapi.Accepted, r.Status, "media should be accepted; error=%v", r.Error)

	var (
		gotWidth, gotHeight int
		gotBytes            int64
	)
	require.NoError(t, env.pool.QueryRow(ctx,
		`SELECT width, height, byte_size FROM media_assets WHERE client_uuid=$1`,
		mediaUUID).Scan(&gotWidth, &gotHeight, &gotBytes))
	assert.Equal(t, width, gotWidth, "width column must hold the supplied width (not transposed)")
	assert.Equal(t, height, gotHeight, "height column must hold the supplied height (not transposed)")
	assert.Equal(t, byteSize, gotBytes, "byte_size must round-trip unchanged")
}
