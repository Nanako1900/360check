package sync

import (
	"github.com/paulmach/orb"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/geo"
)

// geoJSONToPointEWKB converts a client-supplied GeoJSONPoint into EWKB tagged with
// SRID 4326, ready for ST_GeomFromEWKB on the write path. The result is still
// validated server-side via db.ValidateGeomWGS84 (ST_IsValid + SRID) before
// insert, exactly as the problem/inspection write paths do. Coordinates are
// [longitude, latitude]; the binary media itself never travels here — only the
// capture point of a problem or a GPS sample.
func geoJSONToPointEWKB(p oapi.GeoJSONPoint) ([]byte, error) {
	lon, lat := 0.0, 0.0
	if len(p.Coordinates) >= 2 {
		lon = float64(p.Coordinates[0])
		lat = float64(p.Coordinates[1])
	}
	return geo.EncodePointWGS84(orb.Point{lon, lat})
}
