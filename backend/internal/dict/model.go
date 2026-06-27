// Package dict implements the dictionary + config domain: admin-configurable
// dict_type / dict_item catalogs and versioned app_config blocks, both served
// with an ETag (content_hash) so offline clients can pull-with-version and get a
// 304 when nothing changed.
//
// Versioning contract (P4):
//   - The ETag for a READ is the STORED content_hash; it is never recomputed on
//     read.
//   - On ANY dict_item insert/update/delete, in the SAME transaction the owning
//     dict_type's version is bumped and content_hash recomputed over the full
//     (id-ordered) item set, so the ETag changes iff the item set changes.
//   - app_config is written by version-swap: the current active row is set
//     is_active=false and a new version+1 is_active=true row is inserted, with a
//     content_hash recomputed over the value's canonical JSON form. The partial
//     unique index uq_app_config_active guarantees exactly one active row per key.
//
// Geometry never appears here, so this package is plain pgx + encoding/json with
// no PostGIS involvement.
package dict

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
)

// Sentinel errors mapped to envelope codes by the handler.
var (
	// ErrNotFound is returned for a missing or soft-deleted row.
	ErrNotFound = errors.New("not found")
	// ErrCodeTaken is returned when a dict_type code collides with a live type.
	ErrCodeTaken = errors.New("dict type code already taken")
	// ErrItemCodeTaken is returned when a dict_item code collides within its type.
	ErrItemCodeTaken = errors.New("dict item code already taken within type")
	// ErrConfigConflict is returned when the app_config active-row swap loses a
	// race against a concurrent writer (uq_app_config_active).
	ErrConfigConflict = errors.New("config version conflict")
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

// validScopes is the closed set of dict_scope enum literals.
var validScopes = map[oapi.DictScope]bool{
	oapi.ProblemType:     true,
	oapi.ProblemStatus:   true,
	oapi.ProblemCategory: true,
	oapi.ProjectField:    true,
	oapi.CapturePreset:   true,
	oapi.Misc:            true,
}

// isValidScope reports whether s is a recognized dict_scope literal.
func isValidScope(s oapi.DictScope) bool { return validScopes[s] }

// --- content_hash ----------------------------------------------------------

// hashItems computes the deterministic ETag for a dict_type's whole item set.
// The input rows MUST already be ordered by id (the SQL recompute orders by id),
// and each item is serialized as id|code|label|color|sort_order|is_active so the
// hash changes iff any item field — including is_active (retire) — changes, and
// is independent of how the rows were inserted. A nil color serializes as the
// empty string. Retired (is_active=false) items are included so retiring an item
// changes the hash without removing it from the payload.
func hashItems(rows []hashItemRow) string {
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(strconv.FormatInt(r.id, 10))
		b.WriteByte('|')
		b.WriteString(r.code)
		b.WriteByte('|')
		b.WriteString(r.label)
		b.WriteByte('|')
		b.WriteString(r.color) // "" when SQL NULL
		b.WriteByte('|')
		b.WriteString(strconv.Itoa(r.sortOrder))
		b.WriteByte('|')
		b.WriteString(strconv.FormatBool(r.isActive))
		b.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// hashItemRow is the minimal projection hashItems serializes over.
type hashItemRow struct {
	id        int64
	code      string
	label     string
	color     string // "" when NULL
	sortOrder int
	isActive  bool
}

// hashValue computes the deterministic ETag for an app_config value. The value
// is re-marshaled through encoding/json (which sorts map keys), giving a
// canonical byte form independent of the client's key ordering or whitespace.
func hashValue(value map[string]interface{}) (string, error) {
	canon, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("dict: marshal config value: %w", err)
	}
	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:]), nil
}

// marshalValue serializes an app_config value for storage. It uses the same
// encoding/json path as hashValue so the stored jsonb and the content_hash are
// always derived from identical canonical bytes.
func marshalValue(value map[string]interface{}) ([]byte, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("dict: marshal config value: %w", err)
	}
	return b, nil
}

// --- jsonb / time helpers ---------------------------------------------------

// extraJSON marshals an optional dict_item extra map into JSON bytes. A nil map
// yields the empty object so the column never stores SQL NULL.
func extraJSON(m *map[string]interface{}) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(*m)
	if err != nil {
		return nil, fmt.Errorf("dict: marshal extra: %w", err)
	}
	return b, nil
}

// decodeExtra unmarshals a JSONB extra column into an optional map (nil for the
// empty object so an absent value stays absent in the response).
func decodeExtra(b []byte) (*map[string]interface{}, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("dict: decode extra: %w", err)
	}
	if len(m) == 0 {
		return nil, nil
	}
	return &m, nil
}

// decodeValue unmarshals a JSONB value column into a map. A NULL/empty column
// yields the empty (non-nil) map so the response value is never null.
func decodeValue(b []byte) (map[string]interface{}, error) {
	if len(b) == 0 {
		return map[string]interface{}{}, nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("dict: decode config value: %w", err)
	}
	if m == nil {
		m = map[string]interface{}{}
	}
	return m, nil
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

// colorPtr returns a pointer to the color string, or nil for the empty string so
// the API omits an absent color rather than emitting "".
func colorPtr(s string) *string {
	if s == "" {
		return nil
	}
	c := s
	return &c
}

// strOrEmpty dereferences an optional string, defaulting to "".
func strOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// actorPtr returns a pointer to the actor id, or nil for the unauthenticated
// zero id so audit columns store SQL NULL rather than 0.
func actorPtr(id int64) *int64 {
	if id == 0 {
		return nil
	}
	return &id
}
