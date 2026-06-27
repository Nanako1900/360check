// Package geo bridges orb geometries and PostGIS at the SQL boundary. Everything
// is WGS84 / SRID 4326 — GCJ-02 is never persisted (it is a client-only render
// concern). Geometry crosses the SQL boundary as EWKB bytea: write via
// ST_GeomFromEWKB($1) with Encode* output; read via ST_AsEWKB(geom)::bytea then Decode*.
package geo

import (
	"fmt"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/encoding/ewkb"
	"github.com/paulmach/orb/geojson"
)

// SRID4326 is the only SRID C5 persists (WGS84).
const SRID4326 = 4326

// EncodeWGS84 encodes any orb geometry as EWKB tagged with SRID 4326.
func EncodeWGS84(g orb.Geometry) ([]byte, error) {
	if g == nil {
		return nil, fmt.Errorf("geo: nil geometry")
	}
	b, err := ewkb.Marshal(g, SRID4326)
	if err != nil {
		return nil, fmt.Errorf("geo: marshal ewkb: %w", err)
	}
	return b, nil
}

// Typed encoders express intent at call sites (Point/LineString/MultiPolygon/
// MultiLineString are the geometry types used by the schema).
func EncodePointWGS84(p orb.Point) ([]byte, error)           { return EncodeWGS84(p) }
func EncodeLineStringWGS84(l orb.LineString) ([]byte, error) { return EncodeWGS84(l) }
func EncodeMultiPolygonWGS84(m orb.MultiPolygon) ([]byte, error) {
	return EncodeWGS84(m)
}
func EncodeMultiLineStringWGS84(m orb.MultiLineString) ([]byte, error) {
	return EncodeWGS84(m)
}

// Decode parses EWKB into an orb geometry, asserting the embedded SRID is 4326.
func Decode(b []byte) (orb.Geometry, error) {
	g, srid, err := ewkb.Unmarshal(b)
	if err != nil {
		return nil, fmt.Errorf("geo: unmarshal ewkb: %w", err)
	}
	if srid != SRID4326 {
		return nil, fmt.Errorf("geo: unexpected SRID %d (want %d)", srid, SRID4326)
	}
	return g, nil
}

// AssertSRID4326 returns an error unless the EWKB carries SRID 4326.
func AssertSRID4326(b []byte) error {
	_, srid, err := ewkb.Unmarshal(b)
	if err != nil {
		return fmt.Errorf("geo: unmarshal ewkb: %w", err)
	}
	if srid != SRID4326 {
		return fmt.Errorf("geo: SRID %d != 4326", srid)
	}
	return nil
}

// DecodePoint decodes EWKB and asserts it is a Point.
func DecodePoint(b []byte) (orb.Point, error) {
	g, err := Decode(b)
	if err != nil {
		return orb.Point{}, err
	}
	p, ok := g.(orb.Point)
	if !ok {
		return orb.Point{}, fmt.Errorf("geo: expected Point, got %T", g)
	}
	return p, nil
}

// DecodeLineString decodes EWKB and asserts it is a LineString.
func DecodeLineString(b []byte) (orb.LineString, error) {
	g, err := Decode(b)
	if err != nil {
		return nil, err
	}
	l, ok := g.(orb.LineString)
	if !ok {
		return nil, fmt.Errorf("geo: expected LineString, got %T", g)
	}
	return l, nil
}

// DecodeMultiPolygon decodes EWKB and asserts it is a MultiPolygon.
func DecodeMultiPolygon(b []byte) (orb.MultiPolygon, error) {
	g, err := Decode(b)
	if err != nil {
		return nil, err
	}
	m, ok := g.(orb.MultiPolygon)
	if !ok {
		return nil, fmt.Errorf("geo: expected MultiPolygon, got %T", g)
	}
	return m, nil
}

// DecodeMultiLineString decodes EWKB and asserts it is a MultiLineString.
func DecodeMultiLineString(b []byte) (orb.MultiLineString, error) {
	g, err := Decode(b)
	if err != nil {
		return nil, err
	}
	m, ok := g.(orb.MultiLineString)
	if !ok {
		return nil, fmt.Errorf("geo: expected MultiLineString, got %T", g)
	}
	return m, nil
}

// ToGeoJSON marshals an orb geometry to a GeoJSON geometry object (WGS84). Used
// at the API boundary (/problems/map, /inspections/{id}/trajectory).
func ToGeoJSON(g orb.Geometry) ([]byte, error) {
	if g == nil {
		return nil, fmt.Errorf("geo: nil geometry")
	}
	b, err := geojson.NewGeometry(g).MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("geo: marshal geojson: %w", err)
	}
	return b, nil
}

// FromGeoJSON parses a GeoJSON geometry object into an orb geometry.
//
// GeoJSON carries no SRID (RFC 7946 mandates WGS84 by spec), so this returns a
// bare orb geometry. Callers that persist client-supplied GeoJSON MUST validate
// it server-side before storage — re-encode via EncodeWGS84 (which tags SRID
// 4326) and run db.ValidateGeomWGS84 (ST_IsValid + SRID) on the write path.
func FromGeoJSON(b []byte) (orb.Geometry, error) {
	gg, err := geojson.UnmarshalGeometry(b)
	if err != nil {
		return nil, fmt.Errorf("geo: unmarshal geojson: %w", err)
	}
	return gg.Geometry(), nil
}
