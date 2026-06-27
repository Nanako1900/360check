package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/geo"
)

func TestStatusValidation(t *testing.T) {
	assert.True(t, isValidProjectStatus(oapi.ACTIVE))
	assert.True(t, isValidProjectStatus(oapi.PAUSED))
	assert.True(t, isValidProjectStatus(oapi.ARCHIVED))
	assert.False(t, isValidProjectStatus(oapi.ProjectStatus("OPEN")))
	assert.False(t, isValidProjectStatus(oapi.ProjectStatus("")))

	assert.True(t, isValidTaskStatus(oapi.TaskStatusPENDING))
	assert.True(t, isValidTaskStatus(oapi.TaskStatusINPROGRESS))
	assert.True(t, isValidTaskStatus(oapi.TaskStatusCOMPLETED))
	assert.True(t, isValidTaskStatus(oapi.TaskStatusARCHIVED))
	// OPEN/DONE are NOT part of task_status and must be rejected.
	assert.False(t, isValidTaskStatus(oapi.TaskStatus("OPEN")))
	assert.False(t, isValidTaskStatus(oapi.TaskStatus("DONE")))
}

func TestEncodeMultiPolygon_NilAndRoundTrip(t *testing.T) {
	// nil geometry -> nil bytes (NULL).
	b, err := encodeMultiPolygon(nil)
	require.NoError(t, err)
	assert.Nil(t, b)

	// A unit square encodes to SRID-4326 EWKB and decodes back to a MultiPolygon.
	g := &oapi.GeoJSONMultiPolygon{
		Type: oapi.MultiPolygon,
		Coordinates: [][][][]float32{
			{{{0, 0}, {0, 1}, {1, 1}, {1, 0}, {0, 0}}},
		},
	}
	ewkb, err := encodeMultiPolygon(g)
	require.NoError(t, err)
	require.NotNil(t, ewkb)

	decoded, err := geo.DecodeMultiPolygon(ewkb)
	require.NoError(t, err)
	require.Len(t, decoded, 1)
	require.Len(t, decoded[0], 1)
	assert.Len(t, decoded[0][0], 5) // closed ring
}

func TestEncodeMultiLineString_NilAndRoundTrip(t *testing.T) {
	b, err := encodeMultiLineString(nil)
	require.NoError(t, err)
	assert.Nil(t, b)

	g := &oapi.GeoJSONMultiLineString{
		Type: oapi.MultiLineString,
		Coordinates: [][][]float32{
			{{113.0, 23.0}, {113.01, 23.0}},
		},
	}
	ewkb, err := encodeMultiLineString(g)
	require.NoError(t, err)

	decoded, err := geo.DecodeMultiLineString(ewkb)
	require.NoError(t, err)
	require.Len(t, decoded, 1)
	assert.Len(t, decoded[0], 2)
}

func TestEncodeMultiPolygon_BadPosition(t *testing.T) {
	g := &oapi.GeoJSONMultiPolygon{
		Type:        oapi.MultiPolygon,
		Coordinates: [][][][]float32{{{{0}}}}, // 1-element position is invalid
	}
	_, err := encodeMultiPolygon(g)
	require.Error(t, err)
}

func TestDecodeGeoJSON_NilAndEmpty(t *testing.T) {
	mp, err := decodeMultiPolygonJSON(nil)
	require.NoError(t, err)
	assert.Nil(t, mp)

	empty := ""
	mp, err = decodeMultiPolygonJSON(&empty)
	require.NoError(t, err)
	assert.Nil(t, mp)

	mls, err := decodeMultiLineStringJSON(nil)
	require.NoError(t, err)
	assert.Nil(t, mls)
}

func TestDecodeMultiPolygonJSON_RoundTrip(t *testing.T) {
	// ST_AsGeoJSON-style payload decodes into the oapi wrapper.
	s := `{"type":"MultiPolygon","coordinates":[[[[0,0],[0,1],[1,1],[1,0],[0,0]]]]}`
	mp, err := decodeMultiPolygonJSON(&s)
	require.NoError(t, err)
	require.NotNil(t, mp)
	assert.Equal(t, oapi.MultiPolygon, mp.Type)
	require.Len(t, mp.Coordinates, 1)
}

func TestCustomFieldsJSON(t *testing.T) {
	// nil -> empty object (never SQL NULL).
	b, err := customFieldsJSON(nil)
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(b))

	m := map[string]interface{}{"region": "south", "priority": float64(1)}
	b, err = customFieldsJSON(&m)
	require.NoError(t, err)
	assert.JSONEq(t, `{"region":"south","priority":1}`, string(b))
}

func TestDecodeCustomFields(t *testing.T) {
	// empty object decodes to nil (absent in response).
	out, err := decodeCustomFields([]byte(`{}`))
	require.NoError(t, err)
	assert.Nil(t, out)

	out, err = decodeCustomFields([]byte(`{"region":"south"}`))
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, "south", (*out)["region"])

	// no bytes -> nil.
	out, err = decodeCustomFields(nil)
	require.NoError(t, err)
	assert.Nil(t, out)
}

func TestActorPtr(t *testing.T) {
	assert.Nil(t, actorPtr(0)) // zero id -> NULL audit column
	got := actorPtr(42)
	require.NotNil(t, got)
	assert.EqualValues(t, 42, *got)
}

func TestProjectWhere(t *testing.T) {
	// Base clause always excludes soft-deleted rows.
	where, args := projectWhere(nil, nil)
	assert.Contains(t, where, "deleted_at IS NULL")
	assert.Empty(t, args)

	q := "road"
	status := "ACTIVE"
	where, args = projectWhere(&q, &status)
	assert.Contains(t, where, "ILIKE $1")
	assert.Contains(t, where, "status = $2")
	require.Len(t, args, 2)
	assert.Equal(t, "%road%", args[0])
	assert.Equal(t, "ACTIVE", args[1])
}

func TestTaskWhere(t *testing.T) {
	pid := int64(7)
	status := "PENDING"
	aid := int64(3)
	where, args := taskWhere(&pid, &status, &aid)
	assert.Contains(t, where, "deleted_at IS NULL")
	assert.Contains(t, where, "project_id = $1")
	assert.Contains(t, where, "status = $2")
	assert.Contains(t, where, "assignee_id = $3")
	require.Len(t, args, 3)
}
