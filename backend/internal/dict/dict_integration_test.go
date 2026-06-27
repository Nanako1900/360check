//go:build integration

package dict

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	migrations "github.com/nnkglobal/c5-backend/db/migrations"
	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
	"github.com/nnkglobal/c5-backend/internal/rbac"
)

// newHarness spins a fresh PostGIS container, runs all migrations (incl. the
// 000002 seed that provides problem_status/type/category dict_types and the
// export.image/capture.default app_config), and returns a gin engine with the
// dict routes mounted plus the underlying pool and service.
func newHarness(t *testing.T) (*gin.Engine, *Service, *db.Pool) {
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

	svc := NewService(pool)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Inject the seeded admin as the actor so created_by/updated_by persist.
	r.Use(func(c *gin.Context) {
		c.Set(rbac.CtxUserID, adminID(t, pool))
		c.Next()
	})
	NewHandler(svc).RegisterRoutes(r)
	return r, svc, pool
}

// adminID returns the seeded admin user's id.
func adminID(t *testing.T, pool *db.Pool) int64 {
	t.Helper()
	var id int64
	require.NoError(t, pool.QueryRow(context.Background(),
		"SELECT id FROM users WHERE username='admin'").Scan(&id))
	return id
}

// do issues a request against the engine and returns the recorder.
func do(t *testing.T, r *gin.Engine, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
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
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// decodeData unmarshals the envelope's data field into out.
func decodeData(t *testing.T, body []byte, out any) {
	t.Helper()
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   json.RawMessage `json:"error"`
	}
	require.NoError(t, json.Unmarshal(body, &env), "body=%s", body)
	require.True(t, env.Success, "expected success envelope, got: %s", body)
	require.NoError(t, json.Unmarshal(env.Data, out))
}

// --- dict_item pull-with-version (ETag) -------------------------------------

func TestListItems_SeededTypeReturnsItemsAndETag(t *testing.T) {
	r, _, _ := newHarness(t)

	w := do(t, r, http.MethodGet, "/dict/types/problem_status/items", nil, nil)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	etag := w.Header().Get("ETag")
	require.NotEmpty(t, etag, "ETag header must be set")
	assert.True(t, len(etag) >= 2 && etag[0] == '"', "ETag must be quoted: %q", etag)

	var payload oapi.DictItemsPayload
	decodeData(t, w.Body.Bytes(), &payload)
	assert.Equal(t, "problem_status", payload.Type.Code)
	assert.Equal(t, payload.Type.ContentHash, payload.ContentHash)
	assert.Equal(t, payload.Type.Version, payload.Version)
	// Seed gives OPEN/PROCESSING/RESOLVED/CLOSED.
	assert.Len(t, payload.Items, 4)

	// The ETag matches the stored content_hash (quoted).
	assert.Equal(t, `"`+payload.ContentHash+`"`, etag)
}

func TestListItems_IfNoneMatchReturns304(t *testing.T) {
	r, _, _ := newHarness(t)

	first := do(t, r, http.MethodGet, "/dict/types/problem_status/items", nil, nil)
	require.Equal(t, http.StatusOK, first.Code)
	etag := first.Header().Get("ETag")
	require.NotEmpty(t, etag)

	// Re-request with If-None-Match equal to the ETag -> 304, no body.
	second := do(t, r, http.MethodGet, "/dict/types/problem_status/items", nil,
		map[string]string{"If-None-Match": etag})
	assert.Equal(t, http.StatusNotModified, second.Code)
	assert.Empty(t, second.Body.String(), "304 must have no body")
	assert.Equal(t, etag, second.Header().Get("ETag"), "304 still advertises the ETag")

	// A stale/mismatched If-None-Match -> full 200 body.
	third := do(t, r, http.MethodGet, "/dict/types/problem_status/items", nil,
		map[string]string{"If-None-Match": `"stale-hash"`})
	assert.Equal(t, http.StatusOK, third.Code)
}

func TestAddItem_BumpsVersionAndChangesETag(t *testing.T) {
	r, _, pool := newHarness(t)
	ctx := context.Background()

	// Capture the seeded version + ETag.
	before := do(t, r, http.MethodGet, "/dict/types/problem_status/items", nil, nil)
	require.Equal(t, http.StatusOK, before.Code)
	etagBefore := before.Header().Get("ETag")
	var payloadBefore oapi.DictItemsPayload
	decodeData(t, before.Body.Bytes(), &payloadBefore)

	var typeID int64
	require.NoError(t, pool.QueryRow(ctx,
		"SELECT id FROM dict_type WHERE code='problem_status'").Scan(&typeID))

	// Add a new item via POST /dict/items.
	created := do(t, r, http.MethodPost, "/dict/items", oapi.DictItemCreate{
		DictTypeId: typeID,
		Code:       "REOPENED",
		Label:      "重新打开",
		Color:      strptr("#A855F7"),
		SortOrder:  intptr(25),
	}, nil)
	require.Equal(t, http.StatusCreated, created.Code, "body=%s", created.Body.String())
	var item oapi.DictItem
	decodeData(t, created.Body.Bytes(), &item)
	assert.Equal(t, "REOPENED", item.Code)
	assert.True(t, item.IsActive)

	// Re-fetch: version bumped, content_hash changed, ETag differs, item present.
	after := do(t, r, http.MethodGet, "/dict/types/problem_status/items", nil, nil)
	require.Equal(t, http.StatusOK, after.Code)
	etagAfter := after.Header().Get("ETag")
	var payloadAfter oapi.DictItemsPayload
	decodeData(t, after.Body.Bytes(), &payloadAfter)

	assert.Greater(t, payloadAfter.Version, payloadBefore.Version, "version must bump")
	assert.NotEqual(t, payloadBefore.ContentHash, payloadAfter.ContentHash, "content_hash must change")
	assert.NotEqual(t, etagBefore, etagAfter, "ETag must differ after an item change")
	assert.Len(t, payloadAfter.Items, 5)

	// The previous ETag is now stale -> a conditional GET returns 200, not 304.
	stale := do(t, r, http.MethodGet, "/dict/types/problem_status/items", nil,
		map[string]string{"If-None-Match": etagBefore})
	assert.Equal(t, http.StatusOK, stale.Code, "stale ETag must not 304")
}

func TestRetireItem_StillReturnedInPayload(t *testing.T) {
	r, _, pool := newHarness(t)
	ctx := context.Background()

	var itemID, typeID int64
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT di.id, di.dict_type_id
		FROM dict_item di JOIN dict_type dt ON dt.id = di.dict_type_id
		WHERE dt.code='problem_status' AND di.code='CLOSED'`).Scan(&itemID, &typeID))

	before := do(t, r, http.MethodGet, "/dict/types/problem_status/items", nil, nil)
	var payloadBefore oapi.DictItemsPayload
	decodeData(t, before.Body.Bytes(), &payloadBefore)
	etagBefore := before.Header().Get("ETag")

	// Retire it via PUT /dict/items/{id} with is_active=false.
	active := false
	upd := do(t, r, http.MethodPut, "/dict/items/"+itoa(itemID), oapi.DictItemUpdate{
		IsActive: &active,
	}, nil)
	require.Equal(t, http.StatusOK, upd.Code, "body=%s", upd.Body.String())
	var updated oapi.DictItem
	decodeData(t, upd.Body.Bytes(), &updated)
	assert.False(t, updated.IsActive, "item retired")

	// Default include_inactive=true -> retired item still in the payload.
	after := do(t, r, http.MethodGet, "/dict/types/problem_status/items", nil, nil)
	var payloadAfter oapi.DictItemsPayload
	decodeData(t, after.Body.Bytes(), &payloadAfter)
	assert.Len(t, payloadAfter.Items, len(payloadBefore.Items), "retired item is kept, not removed")
	assert.True(t, containsRetired(payloadAfter.Items, "CLOSED"), "retired CLOSED must still render")

	// Retiring is an item-set change -> ETag differs.
	assert.NotEqual(t, etagBefore, after.Header().Get("ETag"))

	// include_inactive=false drops the retired item.
	activeOnly := do(t, r, http.MethodGet, "/dict/types/problem_status/items?include_inactive=false", nil, nil)
	var activePayload oapi.DictItemsPayload
	decodeData(t, activeOnly.Body.Bytes(), &activePayload)
	assert.False(t, containsRetired(activePayload.Items, "CLOSED"),
		"include_inactive=false hides retired items")
}

func TestListItems_UnknownType404(t *testing.T) {
	r, _, _ := newHarness(t)
	w := do(t, r, http.MethodGet, "/dict/types/does_not_exist/items", nil, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCreateDictType_DuplicateCode409(t *testing.T) {
	r, _, _ := newHarness(t)
	// problem_status is seeded; a duplicate code must 409 CONFLICT.
	w := do(t, r, http.MethodPost, "/dict/types", oapi.DictTypeCreate{
		Code:  "problem_status",
		Name:  "dup",
		Scope: oapi.ProblemStatus,
	}, nil)
	assert.Equal(t, http.StatusConflict, w.Code, "body=%s", w.Body.String())
}

func TestUpdateItem_InvalidID422(t *testing.T) {
	r, _, _ := newHarness(t)
	w := do(t, r, http.MethodPut, "/dict/items/not-a-number", oapi.DictItemUpdate{}, nil)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

// --- app_config -------------------------------------------------------------

func TestGetConfig_SeededWithETag(t *testing.T) {
	r, _, _ := newHarness(t)
	w := do(t, r, http.MethodGet, "/config/export.image", nil, nil)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	etag := w.Header().Get("ETag")
	require.NotEmpty(t, etag)

	var cfg oapi.AppConfig
	decodeData(t, w.Body.Bytes(), &cfg)
	assert.Equal(t, "export.image", cfg.ConfigKey)
	assert.True(t, cfg.IsActive)
	assert.Equal(t, 1, cfg.Version)
	assert.Equal(t, `"`+cfg.ContentHash+`"`, etag)

	// 304 round-trip.
	second := do(t, r, http.MethodGet, "/config/export.image", nil,
		map[string]string{"If-None-Match": etag})
	assert.Equal(t, http.StatusNotModified, second.Code)
	assert.Empty(t, second.Body.String())
}

func TestPutConfig_VersionSwapAndHistory(t *testing.T) {
	r, _, pool := newHarness(t)
	ctx := context.Background()

	// Current active version + ETag.
	before := do(t, r, http.MethodGet, "/config/capture.default", nil, nil)
	require.Equal(t, http.StatusOK, before.Code)
	var cfgBefore oapi.AppConfig
	decodeData(t, before.Body.Bytes(), &cfgBefore)
	etagBefore := before.Header().Get("ETag")

	// PUT a new value -> version+1, new active row, old retired.
	put := do(t, r, http.MethodPut, "/config/capture.default", oapi.AppConfigUpdate{
		Value:       map[string]interface{}{"mode": "HDR", "hdr": true},
		Description: strptr("switch default to HDR"),
	}, nil)
	require.Equal(t, http.StatusOK, put.Code, "body=%s", put.Body.String())
	var cfgAfter oapi.AppConfig
	decodeData(t, put.Body.Bytes(), &cfgAfter)
	assert.Equal(t, cfgBefore.Version+1, cfgAfter.Version, "version increments")
	assert.True(t, cfgAfter.IsActive)
	assert.NotEqual(t, cfgBefore.ContentHash, cfgAfter.ContentHash, "content_hash changes")
	assert.NotEqual(t, etagBefore, put.Header().Get("ETag"))

	// Exactly one active row for the key (uq_app_config_active).
	var activeCount int
	require.NoError(t, pool.QueryRow(ctx,
		"SELECT count(*) FROM app_config WHERE config_key='capture.default' AND is_active").Scan(&activeCount))
	assert.Equal(t, 1, activeCount, "exactly one active version")

	// The old row is now inactive with effective_to set.
	var inactiveWithEnd int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT count(*) FROM app_config
		WHERE config_key='capture.default' AND NOT is_active AND effective_to IS NOT NULL`).Scan(&inactiveWithEnd))
	assert.Equal(t, 1, inactiveWithEnd, "previous version retired with effective_to")

	// History grows and is newest-first.
	hist := do(t, r, http.MethodGet, "/config/capture.default/history", nil, nil)
	require.Equal(t, http.StatusOK, hist.Code)
	var versions []oapi.AppConfig
	decodeData(t, hist.Body.Bytes(), &versions)
	require.Len(t, versions, 2, "two versions in history")
	assert.Greater(t, versions[0].Version, versions[1].Version, "history is newest-first")
	assert.True(t, versions[0].IsActive)
	assert.False(t, versions[1].IsActive)

	// The active GET now reflects the new value + ETag, and the OLD ETag is stale.
	stale := do(t, r, http.MethodGet, "/config/capture.default", nil,
		map[string]string{"If-None-Match": etagBefore})
	assert.Equal(t, http.StatusOK, stale.Code, "old ETag must not 304 after a swap")
}

func TestGetConfig_UnknownKey404(t *testing.T) {
	r, _, _ := newHarness(t)
	w := do(t, r, http.MethodGet, "/config/no.such.key", nil, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	hist := do(t, r, http.MethodGet, "/config/no.such.key/history", nil, nil)
	assert.Equal(t, http.StatusNotFound, hist.Code)
}

func TestPutConfig_NewKeyStartsAtVersion1(t *testing.T) {
	r, _, _ := newHarness(t)
	put := do(t, r, http.MethodPut, "/config/feature.flags", oapi.AppConfigUpdate{
		Value: map[string]interface{}{"beta_export": true},
	}, nil)
	require.Equal(t, http.StatusOK, put.Code, "body=%s", put.Body.String())
	var cfg oapi.AppConfig
	decodeData(t, put.Body.Bytes(), &cfg)
	assert.Equal(t, 1, cfg.Version, "first version of a brand-new key is 1")
	assert.True(t, cfg.IsActive)
}

// --- small helpers ----------------------------------------------------------

func strptr(s string) *string { return &s }
func intptr(i int) *int       { return &i }

func itoa(n int64) string {
	return jsonNumber(n)
}

// jsonNumber renders an int64 as a base-10 string (avoids importing strconv just
// for the test path-id formatting).
func jsonNumber(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func containsRetired(items []oapi.DictItem, code string) bool {
	for _, it := range items {
		if it.Code == code {
			return true
		}
	}
	return false
}
