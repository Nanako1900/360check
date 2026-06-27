package inspection

import (
	"github.com/paulmach/orb"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/geo"
)

// pointToGeoJSON maps an orb.Point (WGS84) to the generated GeoJSONPoint shape.
// Coordinates are [longitude, latitude]; the client converts to GCJ-02 at its
// Tencent-map render boundary (never persisted server-side).
func pointToGeoJSON(p orb.Point) oapi.GeoJSONPoint {
	return oapi.GeoJSONPoint{
		Type:        oapi.Point,
		Coordinates: []float32{float32(p.Lon()), float32(p.Lat())},
	}
}

// lineStringToGeoJSON maps an orb.LineString (WGS84) to the generated
// GeoJSONLineString shape, each vertex emitted as [longitude, latitude].
func lineStringToGeoJSON(l orb.LineString) oapi.GeoJSONLineString {
	coords := make([][]float32, 0, len(l))
	for _, p := range l {
		coords = append(coords, []float32{float32(p.Lon()), float32(p.Lat())})
	}
	return oapi.GeoJSONLineString{
		Type:        oapi.LineString,
		Coordinates: coords,
	}
}

// decodePointEWKB decodes a stored Point geometry (EWKB bytea via ST_AsEWKB) into
// the GeoJSONPoint API shape, asserting SRID 4326.
func decodePointEWKB(ewkb []byte) (oapi.GeoJSONPoint, error) {
	p, err := geo.DecodePoint(ewkb)
	if err != nil {
		return oapi.GeoJSONPoint{}, err
	}
	return pointToGeoJSON(p), nil
}

// decodeLineStringEWKB decodes a stored LineString geometry (EWKB bytea via
// ST_AsEWKB) into the GeoJSONLineString API shape, asserting SRID 4326. A NULL
// route_geom column yields nil bytes and a nil result (route not yet finalized).
func decodeLineStringEWKB(ewkb []byte) (*oapi.GeoJSONLineString, error) {
	if ewkb == nil {
		return nil, nil
	}
	l, err := geo.DecodeLineString(ewkb)
	if err != nil {
		return nil, err
	}
	gj := lineStringToGeoJSON(l)
	return &gj, nil
}
