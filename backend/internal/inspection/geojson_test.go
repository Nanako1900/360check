package inspection

import (
	"testing"

	"github.com/paulmach/orb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/geo"
)

func TestPointToGeoJSON_LonLatOrder(t *testing.T) {
	gj := pointToGeoJSON(orb.Point{113.0, 23.0})
	assert.Equal(t, oapi.GeoJSONPointType(oapi.Point), gj.Type)
	require.Len(t, gj.Coordinates, 2)
	assert.InDelta(t, 113.0, gj.Coordinates[0], 1e-4, "coordinates are [lon, lat]")
	assert.InDelta(t, 23.0, gj.Coordinates[1], 1e-4)
}

func TestLineStringToGeoJSON_PreservesVertices(t *testing.T) {
	gj := lineStringToGeoJSON(orb.LineString{{113.0, 23.0}, {113.01, 23.0}})
	assert.Equal(t, oapi.GeoJSONLineStringType(oapi.LineString), gj.Type)
	require.Len(t, gj.Coordinates, 2)
	assert.InDelta(t, 113.0, gj.Coordinates[0][0], 1e-4)
	assert.InDelta(t, 113.01, gj.Coordinates[1][0], 1e-4)
}

func TestDecodeLineStringEWKB_NilIsNilRoute(t *testing.T) {
	// A NULL route_geom column scans to nil bytes -> nil route, no error.
	route, err := decodeLineStringEWKB(nil)
	require.NoError(t, err)
	assert.Nil(t, route)
}

func TestDecodeLineStringEWKB_RoundTrip(t *testing.T) {
	ewkb, err := geo.EncodeLineStringWGS84(orb.LineString{{113.0, 23.0}, {113.01, 23.0}})
	require.NoError(t, err)
	route, err := decodeLineStringEWKB(ewkb)
	require.NoError(t, err)
	require.NotNil(t, route)
	require.Len(t, route.Coordinates, 2)
	assert.InDelta(t, 113.01, route.Coordinates[1][0], 1e-4)
}

func TestDecodePointEWKB_RoundTrip(t *testing.T) {
	ewkb, err := geo.EncodePointWGS84(orb.Point{113.5, 22.5})
	require.NoError(t, err)
	gj, err := decodePointEWKB(ewkb)
	require.NoError(t, err)
	assert.InDelta(t, 113.5, gj.Coordinates[0], 1e-4)
	assert.InDelta(t, 22.5, gj.Coordinates[1], 1e-4)
}

func TestValidStatus(t *testing.T) {
	for _, s := range []string{"IN_PROGRESS", "FINISHED", "ABANDONED"} {
		assert.True(t, validStatus(s), s)
	}
	for _, s := range []string{"", "OPEN", "in_progress", "DONE"} {
		assert.False(t, validStatus(s), s)
	}
}
