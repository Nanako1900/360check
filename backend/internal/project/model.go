// Package project implements the project and inspection-task domain (CRUD +
// filtering + pagination). Geometry is hand-written pgx — never sqlc: it is
// written via ST_GeomFromEWKB($n) with geo.Encode* output (validated first with
// db.ValidateGeomWGS84) and read back out as GeoJSON via ST_AsGeoJSON(geom)::text.
// All geometry is WGS84 / SRID 4326. Soft-deleted rows are excluded from reads.
package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/paulmach/orb"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/geo"
)

// Sentinel errors mapped to envelope codes by the handler.
var (
	// ErrNotFound is returned for a missing or soft-deleted row.
	ErrNotFound = errors.New("not found")
	// ErrCodeTaken is returned when a project code collides with a live project.
	ErrCodeTaken = errors.New("project code already taken")
	// ErrProjectInUse is returned when a project still has live inspections/tasks.
	ErrProjectInUse = errors.New("project has active inspections or tasks")
)

// ErrValidation carries a client-safe message plus field-level details so the
// handler can surface a 422 VALIDATION_FAILED with the offending fields.
type ErrValidation struct {
	Message string
	Details []oapi.ErrorDetail
}

func (e *ErrValidation) Error() string { return e.Message }

// newValidation builds an *ErrValidation with a single field detail.
func newValidation(message, field, code, detail string) *ErrValidation {
	return &ErrValidation{
		Message: message,
		Details: []oapi.ErrorDetail{{Field: field, Code: code, Message: detail}},
	}
}

// validProjectStatuses is the closed set of project_status enum literals.
var validProjectStatuses = map[oapi.ProjectStatus]bool{
	oapi.ACTIVE:   true,
	oapi.PAUSED:   true,
	oapi.ARCHIVED: true,
}

// validTaskStatuses is the closed set of task_status enum literals. OPEN/DONE are
// deliberately absent — they are not part of the schema's task_status enum.
var validTaskStatuses = map[oapi.TaskStatus]bool{
	oapi.TaskStatusPENDING:    true,
	oapi.TaskStatusINPROGRESS: true,
	oapi.TaskStatusCOMPLETED:  true,
	oapi.TaskStatusARCHIVED:   true,
}

// isValidProjectStatus reports whether s is a recognized project_status literal.
func isValidProjectStatus(s oapi.ProjectStatus) bool { return validProjectStatuses[s] }

// isValidTaskStatus reports whether s is a recognized task_status literal
// (PENDING/IN_PROGRESS/COMPLETED/ARCHIVED — never OPEN/DONE).
func isValidTaskStatus(s oapi.TaskStatus) bool { return validTaskStatuses[s] }

// encodeMultiPolygon converts a GeoJSON MultiPolygon into SRID-4326 EWKB ready
// for ST_GeomFromEWKB. A nil input yields nil bytes (NULL geometry).
func encodeMultiPolygon(g *oapi.GeoJSONMultiPolygon) ([]byte, error) {
	if g == nil {
		return nil, nil
	}
	mp := make(orb.MultiPolygon, 0, len(g.Coordinates))
	for _, poly := range g.Coordinates {
		p := make(orb.Polygon, 0, len(poly))
		for _, ring := range poly {
			r := make(orb.Ring, 0, len(ring))
			for _, pos := range ring {
				if len(pos) < 2 {
					return nil, fmt.Errorf("project: bad MultiPolygon position")
				}
				r = append(r, orb.Point{float64(pos[0]), float64(pos[1])})
			}
			p = append(p, r)
		}
		mp = append(mp, p)
	}
	return geo.EncodeMultiPolygonWGS84(mp)
}

// encodeMultiLineString converts a GeoJSON MultiLineString into SRID-4326 EWKB
// ready for ST_GeomFromEWKB. A nil input yields nil bytes (NULL geometry).
func encodeMultiLineString(g *oapi.GeoJSONMultiLineString) ([]byte, error) {
	if g == nil {
		return nil, nil
	}
	mls := make(orb.MultiLineString, 0, len(g.Coordinates))
	for _, line := range g.Coordinates {
		l := make(orb.LineString, 0, len(line))
		for _, pos := range line {
			if len(pos) < 2 {
				return nil, fmt.Errorf("project: bad MultiLineString position")
			}
			l = append(l, orb.Point{float64(pos[0]), float64(pos[1])})
		}
		mls = append(mls, l)
	}
	return geo.EncodeMultiLineStringWGS84(mls)
}

// decodeMultiPolygonJSON unmarshals ST_AsGeoJSON(geom)::text output into the
// oapi GeoJSON wrapper. An empty/NULL string yields nil (omitted from the response).
func decodeMultiPolygonJSON(s *string) (*oapi.GeoJSONMultiPolygon, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	var out oapi.GeoJSONMultiPolygon
	if err := json.Unmarshal([]byte(*s), &out); err != nil {
		return nil, fmt.Errorf("project: decode area_geom geojson: %w", err)
	}
	return &out, nil
}

// decodeMultiLineStringJSON unmarshals ST_AsGeoJSON(geom)::text output into the
// oapi GeoJSON wrapper. An empty/NULL string yields nil (omitted from the response).
func decodeMultiLineStringJSON(s *string) (*oapi.GeoJSONMultiLineString, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	var out oapi.GeoJSONMultiLineString
	if err := json.Unmarshal([]byte(*s), &out); err != nil {
		return nil, fmt.Errorf("project: decode plan_geom geojson: %w", err)
	}
	return &out, nil
}

// dateToPg converts an optional API date into a pgtype.Date for binding.
func dateToPg(d *openapi_types.Date) pgtype.Date {
	if d == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: d.Time, Valid: true}
}

// pgToDate converts a pgtype.Date into an optional API date (nil when NULL).
func pgToDate(d pgtype.Date) *openapi_types.Date {
	if !d.Valid {
		return nil
	}
	return &openapi_types.Date{Time: d.Time}
}

// pgToTimePtr converts a pgtype.Timestamptz into an optional time (nil when NULL).
func pgToTimePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	tt := t.Time
	return &tt
}

// timeToPg converts an optional time into a pgtype.Timestamptz for binding.
func timeToPg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// customFieldsJSON marshals an optional custom_fields map into a JSON []byte.
// A nil map yields the empty object so the column never stores SQL NULL.
func customFieldsJSON(m *map[string]interface{}) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(*m)
	if err != nil {
		return nil, fmt.Errorf("project: marshal custom_fields: %w", err)
	}
	return b, nil
}

// decodeCustomFields unmarshals a JSONB column into an optional map (nil for the
// empty object so an absent value stays absent in the response).
func decodeCustomFields(b []byte) (*map[string]interface{}, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("project: decode custom_fields: %w", err)
	}
	if len(m) == 0 {
		return nil, nil
	}
	return &m, nil
}

// strOrEmpty dereferences an optional string, defaulting to "".
func strOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
