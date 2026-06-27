package geo

import (
	"encoding/json"
	"testing"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/encoding/ewkb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPointRoundTrip(t *testing.T) {
	p := orb.Point{113.2644, 23.1291} // Guangzhou, WGS84 (lng, lat)
	b, err := EncodePointWGS84(p)
	require.NoError(t, err)
	require.NoError(t, AssertSRID4326(b))

	got, err := DecodePoint(b)
	require.NoError(t, err)
	assert.InDelta(t, p.Lon(), got.Lon(), 1e-9)
	assert.InDelta(t, p.Lat(), got.Lat(), 1e-9)
}

func TestLineStringRoundTrip(t *testing.T) {
	l := orb.LineString{{113.0, 23.0}, {113.01, 23.0}, {113.02, 23.01}}
	b, err := EncodeLineStringWGS84(l)
	require.NoError(t, err)

	got, err := DecodeLineString(b)
	require.NoError(t, err)
	require.Len(t, got, len(l))
	for i := range l {
		assert.InDelta(t, l[i].Lon(), got[i].Lon(), 1e-9)
		assert.InDelta(t, l[i].Lat(), got[i].Lat(), 1e-9)
	}
}

func TestMultiPolygonRoundTrip(t *testing.T) {
	ring := orb.Ring{{113.0, 23.0}, {113.1, 23.0}, {113.1, 23.1}, {113.0, 23.1}, {113.0, 23.0}}
	mp := orb.MultiPolygon{{ring}}
	b, err := EncodeMultiPolygonWGS84(mp)
	require.NoError(t, err)

	got, err := DecodeMultiPolygon(b)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Len(t, got[0], 1)
	assert.Len(t, got[0][0], len(ring))
}

func TestMultiLineStringRoundTrip(t *testing.T) {
	ml := orb.MultiLineString{
		{{113.0, 23.0}, {113.01, 23.0}},
		{{114.0, 24.0}, {114.01, 24.01}},
	}
	b, err := EncodeMultiLineStringWGS84(ml)
	require.NoError(t, err)

	got, err := DecodeMultiLineString(b)
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestDecodeRejectsNon4326(t *testing.T) {
	// Craft a valid EWKB tagged with the wrong SRID (Web Mercator).
	bad, err := ewkb.Marshal(orb.Point{12977000, 2644000}, 3857)
	require.NoError(t, err)

	_, err = Decode(bad)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SRID")

	require.Error(t, AssertSRID4326(bad))
}

func TestTypedDecoderRejectsWrongType(t *testing.T) {
	b, err := EncodePointWGS84(orb.Point{113.0, 23.0})
	require.NoError(t, err)

	_, err = DecodeLineString(b)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected LineString")
}

func TestEncodeNilGeometry(t *testing.T) {
	_, err := EncodeWGS84(nil)
	require.Error(t, err)
}

func TestGeoJSONRoundTrip(t *testing.T) {
	p := orb.Point{113.2644, 23.1291}
	b, err := ToGeoJSON(p)
	require.NoError(t, err)

	// Shape check: a GeoJSON Point with [lng, lat].
	var raw struct {
		Type        string    `json:"type"`
		Coordinates []float64 `json:"coordinates"`
	}
	require.NoError(t, json.Unmarshal(b, &raw))
	assert.Equal(t, "Point", raw.Type)
	require.Len(t, raw.Coordinates, 2)
	assert.InDelta(t, 113.2644, raw.Coordinates[0], 1e-9)
	assert.InDelta(t, 23.1291, raw.Coordinates[1], 1e-9)

	got, err := FromGeoJSON(b)
	require.NoError(t, err)
	gp, ok := got.(orb.Point)
	require.True(t, ok)
	assert.InDelta(t, p.Lon(), gp.Lon(), 1e-9)
	assert.InDelta(t, p.Lat(), gp.Lat(), 1e-9)
}
