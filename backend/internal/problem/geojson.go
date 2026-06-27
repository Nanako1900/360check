package problem

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

// decodePointEWKB decodes a stored Point geometry (EWKB bytea via ST_AsEWKB) into
// the GeoJSONPoint API shape, asserting SRID 4326.
func decodePointEWKB(ewkb []byte) (oapi.GeoJSONPoint, error) {
	p, err := geo.DecodePoint(ewkb)
	if err != nil {
		return oapi.GeoJSONPoint{}, err
	}
	return pointToGeoJSON(p), nil
}

// geoJSONToPointEWKB converts a client-supplied GeoJSONPoint into EWKB tagged
// with SRID 4326, ready for ST_GeomFromEWKB on the write path. The result is
// still validated server-side via db.ValidateGeomWGS84 (ST_IsValid + SRID)
// before insert.
func geoJSONToPointEWKB(p oapi.GeoJSONPoint) ([]byte, error) {
	lon, lat := 0.0, 0.0
	if len(p.Coordinates) >= 2 {
		lon = float64(p.Coordinates[0])
		lat = float64(p.Coordinates[1])
	}
	return geo.EncodePointWGS84(orb.Point{lon, lat})
}
