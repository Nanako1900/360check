package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
)

func init() { gin.SetMode(gin.TestMode) }

// rawEnvelope mirrors the wire shape so we can assert key presence and null-ness
// independently of the typed Envelope struct.
type rawEnvelope struct {
	Success bool             `json:"success"`
	Data    json.RawMessage  `json:"data"`
	Error   *json.RawMessage `json:"error"`
	Meta    *json.RawMessage `json:"meta"`
}

func doRequest(t *testing.T, h gin.HandlerFunc) (*httptest.ResponseRecorder, rawEnvelope) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	h(c)

	var env rawEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	return w, env
}

func TestOK_ShapeAndStatus(t *testing.T) {
	w, env := doRequest(t, func(c *gin.Context) {
		OK(c, map[string]string{"hello": "world"})
	})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, env.Success)
	assert.JSONEq(t, `{"hello":"world"}`, string(env.Data))
	// error and meta must be present and null on a non-list success.
	assert.Nil(t, env.Error)
	assert.Nil(t, env.Meta)
	// keys must literally be present in the payload (null), not omitted.
	assert.Contains(t, w.Body.String(), `"error":null`)
	assert.Contains(t, w.Body.String(), `"meta":null`)
}

func TestCreated_Status201(t *testing.T) {
	w, env := doRequest(t, func(c *gin.Context) {
		Created(c, map[string]int{"id": 7})
	})
	assert.Equal(t, http.StatusCreated, w.Code)
	assert.True(t, env.Success)
	assert.JSONEq(t, `{"id":7}`, string(env.Data))
}

func TestList_MetaPopulated(t *testing.T) {
	w, env := doRequest(t, func(c *gin.Context) {
		List(c, []int{1, 2, 3}, MetaFor(42, Page{Page: 2, PageSize: 20}))
	})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, env.Success)
	assert.JSONEq(t, `[1,2,3]`, string(env.Data))
	require.NotNil(t, env.Meta)
	var meta oapi.Meta
	require.NoError(t, json.Unmarshal(*env.Meta, &meta))
	require.NotNil(t, meta.Total)
	assert.EqualValues(t, 42, *meta.Total)
	assert.Equal(t, 2, *meta.Page)
	assert.Equal(t, 20, *meta.PageSize)
}

func TestFail_ShapeStatusAndCode(t *testing.T) {
	w, env := doRequest(t, func(c *gin.Context) {
		Fail(c, NewError(oapi.CONFLICT, "username taken").
			WithDetails(oapi.ErrorDetail{Field: "username", Code: "duplicate", Message: "taken"}))
	})
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.False(t, env.Success)
	// data must be null on error.
	assert.Contains(t, w.Body.String(), `"data":null`)

	require.NotNil(t, env.Error)
	var eo oapi.ErrorObject
	require.NoError(t, json.Unmarshal(*env.Error, &eo))
	assert.Equal(t, oapi.CONFLICT, eo.Code)
	assert.Equal(t, "username taken", eo.Message)
	require.NotNil(t, eo.Details)
	require.Len(t, *eo.Details, 1)
	assert.Equal(t, "username", (*eo.Details)[0].Field)
}

func TestFailCode_NoDetails(t *testing.T) {
	w, env := doRequest(t, func(c *gin.Context) {
		FailCode(c, oapi.UNAUTHENTICATED, "missing bearer token")
	})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.False(t, env.Success)
	var eo oapi.ErrorObject
	require.NoError(t, json.Unmarshal(*env.Error, &eo))
	assert.Equal(t, oapi.UNAUTHENTICATED, eo.Code)
	assert.Nil(t, eo.Details)
}

func TestFailAbort_AbortsChain(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	FailAbort(c, ErrForbidden("nope"))
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.True(t, c.IsAborted())
}
