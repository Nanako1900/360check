// Package problem implements the problem domain: problem CRUD, the append-only
// processing log, the D3 atomic status-change rule, and the map GeoJSON layer.
//
// Every geometry-touching statement is hand-written pgx against the pool (never
// sqlc, which cannot resolve PostGIS functions): the capture point is written via
// ST_GeomFromEWKB($n) from geo.EncodePointWGS84 (validated by db.ValidateGeomWGS84
// before insert) and read back as EWKB (ST_AsEWKB) or as GeoJSON (ST_AsGeoJSON) at
// the API boundary — always WGS84.
//
// Historical dictionary tolerance: type/status/category_item_id are soft FKs to
// dict_item with ON DELETE RESTRICT, so a referenced item can be retired
// (is_active=false) but never hard-deleted. Create/Update therefore accept
// references to retired items without complaint; dict_version_used pins the
// problem_type dict version observed at capture.
package problem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
)

// MapLimitDefault caps how many features /problems/map returns so the layer query
// never degenerates into a full-table dump. The handler may lower it per request
// but never raise it past MapLimitMax.
const (
	MapLimitDefault = 5000
	MapLimitMax     = 5000
)

// Service owns the problems + problem_processing_log tables.
type Service struct {
	pool *db.Pool
}

// NewService wires the problem service onto the shared pool.
func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

// problemCols is the canonical projection used by every problem read. geom is
// emitted as EWKB bytea so it round-trips through the geo package. The column
// ORDER here is the contract scanProblem depends on.
const problemCols = `id, client_uuid, project_id, inspection_id, inspector_id,
	ST_AsEWKB(geom), type_item_id, status_item_id, category_item_id,
	dict_version_used, title, description, note, captured_at, cover_media_id,
	created_at, updated_at`

// scanProblem materializes one problem row from the canonical projection.
func scanProblem(row pgx.Row) (oapi.Problem, error) {
	var (
		p           oapi.Problem
		geomEWKB    []byte
		title       string
		description string
		note        string
	)
	if err := row.Scan(
		&p.Id, &p.ClientUuid, &p.ProjectId, &p.InspectionId, &p.InspectorId,
		&geomEWKB, &p.TypeItemId, &p.StatusItemId, &p.CategoryItemId,
		&p.DictVersionUsed, &title, &description, &note, &p.CapturedAt, &p.CoverMediaId,
		&p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return oapi.Problem{}, err
	}
	geom, err := decodePointEWKB(geomEWKB)
	if err != nil {
		return oapi.Problem{}, fmt.Errorf("decode geom: %w", err)
	}
	p.Geom = geom
	if title != "" {
		p.Title = &title
	}
	if description != "" {
		p.Description = &description
	}
	if note != "" {
		p.Note = &note
	}
	return p, nil
}

// ListFilter holds the optional /problems filters (D1 includes InspectionID).
// All nil = unfiltered page. Type/Status/Category are dict_item ids.
type ListFilter struct {
	ProjectID    *int64
	Type         *int64
	Status       *int64
	Category     *int64
	InspectorID  *int64
	InspectionID *int64
	From         *time.Time
	To           *time.Time
}

// listWhere is the shared WHERE clause + positional args for List/Count. The
// bind order is fixed so every caller can append its own LIMIT/OFFSET. The
// columns are unqualified (no join, so no ambiguity).
func listWhere(f ListFilter) (string, []any) {
	return whereWithAlias(f, "")
}

// whereWithAlias builds the same filter WHERE clause with every problems column
// qualified by alias (e.g. "p."). Map joins dict_item (which also has a
// deleted_at column), so its filter must be qualified to avoid an ambiguous
// column reference.
func whereWithAlias(f ListFilter, alias string) (string, []any) {
	where := `WHERE ` + alias + `deleted_at IS NULL
		AND ($1::bigint IS NULL OR ` + alias + `project_id = $1)
		AND ($2::bigint IS NULL OR ` + alias + `type_item_id = $2)
		AND ($3::bigint IS NULL OR ` + alias + `status_item_id = $3)
		AND ($4::bigint IS NULL OR ` + alias + `category_item_id = $4)
		AND ($5::bigint IS NULL OR ` + alias + `inspector_id = $5)
		AND ($6::bigint IS NULL OR ` + alias + `inspection_id = $6)
		AND ($7::timestamptz IS NULL OR ` + alias + `captured_at >= $7)
		AND ($8::timestamptz IS NULL OR ` + alias + `captured_at <  $8)`
	args := []any{f.ProjectID, f.Type, f.Status, f.Category, f.InspectorID, f.InspectionID, f.From, f.To}
	return where, args
}

// List returns a page of non-deleted problems plus the total for the filter,
// newest-captured first.
func (s *Service) List(ctx context.Context, f ListFilter, limit, offset int) ([]oapi.Problem, int64, error) {
	where, args := listWhere(f)

	rows, err := s.pool.Query(ctx,
		`SELECT `+problemCols+` FROM problems `+where+`
		 ORDER BY captured_at DESC, id DESC LIMIT $9 OFFSET $10`,
		append(append([]any{}, args...), limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("problem: list: %w", err)
	}
	defer rows.Close()

	out := make([]oapi.Problem, 0, limit)
	for rows.Next() {
		p, err := scanProblem(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("problem: scan: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("problem: list rows: %w", err)
	}

	var total int64
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM problems `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("problem: count: %w", err)
	}
	return out, total, nil
}

// Get returns one non-deleted problem. Missing/deleted -> ErrNotFound.
func (s *Service) Get(ctx context.Context, id int64) (*oapi.Problem, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+problemCols+` FROM problems WHERE id = $1 AND deleted_at IS NULL`, id)
	p, err := scanProblem(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("problem: get: %w", err)
	}
	return &p, nil
}

// getByClientUUID re-selects a problem by client_uuid (used after ON CONFLICT).
func (s *Service) getByClientUUID(ctx context.Context, clientUUID string) (*oapi.Problem, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+problemCols+` FROM problems WHERE client_uuid = $1 AND deleted_at IS NULL`, clientUUID)
	p, err := scanProblem(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("problem: get by client_uuid: %w", err)
	}
	return &p, nil
}

// CreateInput is the validated payload for Create (geom already EWKB 4326).
type CreateInput struct {
	ProjectID      int64
	GeomEWKB       []byte
	CapturedAt     time.Time
	InspectorID    int64 // resolved (request value or authenticated user)
	ClientUUID     *string
	InspectionID   *int64
	TypeItemID     *int64
	StatusItemID   *int64
	CategoryItemID *int64
	// DictVersionUsed pins the problem_type dict version. nil -> the service reads
	// the current problem_type dict_type.version (offline items report their own).
	DictVersionUsed *int
	CoverMediaID    *int64
	Title           *string
	Description     *string
	Note            *string
}

// Create inserts a problem. It is idempotent on client_uuid (ON CONFLICT DO
// NOTHING then re-select), pins dict_version_used to the current problem_type
// version when the caller omits it, and tolerates references to retired
// dict_items (the soft FKs accept any existing id). A missing project/inspector
// surfaces as ErrProjectMissing/ErrInspectorMissing rather than a 500.
func (s *Service) Create(ctx context.Context, in CreateInput, actorID int64) (*oapi.Problem, error) {
	dictVersion, err := s.resolveDictVersion(ctx, in.DictVersionUsed)
	if err != nil {
		return nil, err
	}

	title, description, note := derefStr(in.Title), derefStr(in.Description), derefStr(in.Note)

	var clientUUID string
	insertSQL := `
		INSERT INTO problems
			(client_uuid, project_id, inspection_id, inspector_id, geom,
			 type_item_id, status_item_id, category_item_id, dict_version_used,
			 title, description, note, captured_at, cover_media_id,
			 created_by, updated_by)
		VALUES (COALESCE($1, gen_random_uuid()), $2, $3, $4, ST_GeomFromEWKB($5),
			 $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $15)
		ON CONFLICT (client_uuid) DO NOTHING
		RETURNING client_uuid`

	err = s.pool.QueryRow(ctx, insertSQL,
		in.ClientUUID, in.ProjectID, in.InspectionID, in.InspectorID, in.GeomEWKB,
		in.TypeItemID, in.StatusItemID, in.CategoryItemID, dictVersion,
		title, description, note, in.CapturedAt, in.CoverMediaID, actorID,
	).Scan(&clientUUID)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Conflict: a problem with this client_uuid already exists — return it.
		if in.ClientUUID == nil {
			return nil, fmt.Errorf("problem: create: unexpected conflict without client_uuid")
		}
		return s.getByClientUUID(ctx, *in.ClientUUID)
	case isForeignKeyViolation(err):
		return nil, classifyFK(err)
	case err != nil:
		return nil, fmt.Errorf("problem: create: %w", err)
	}
	return s.getByClientUUID(ctx, clientUUID)
}

// resolveDictVersion returns the caller-supplied dict_version_used, or the current
// problem_type dict_type.version when nil (falling back to 1 if the dict is
// absent, which the seed always provides).
func (s *Service) resolveDictVersion(ctx context.Context, supplied *int) (int, error) {
	if supplied != nil {
		return *supplied, nil
	}
	var version int
	err := s.pool.QueryRow(ctx,
		`SELECT version FROM dict_type WHERE code = 'problem_type'`).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 1, nil
	}
	if err != nil {
		return 0, fmt.Errorf("problem: read problem_type version: %w", err)
	}
	return version, nil
}

// UpdateInput is the validated partial update for Update (nil = leave unchanged).
type UpdateInput struct {
	StatusItemID   *int64
	CategoryItemID *int64
	TypeItemID     *int64
	InspectionID   *int64
	CoverMediaID   *int64
	Title          *string
	Description    *string
	Note           *string
}

// Update applies a partial update. D3: when StatusItemID is provided AND differs
// from the current status, the UPDATE and a STATUS_CHANGE processing-log insert
// run in the SAME transaction, recording from/to status and the acting operator.
// Clients never author STATUS_CHANGE directly. Missing/deleted -> ErrNotFound.
func (s *Service) Update(ctx context.Context, id int64, in UpdateInput, actorID int64) (*oapi.Problem, error) {
	var out oapi.Problem
	err := s.pool.WithTx(ctx, func(tx pgx.Tx) error {
		// Lock the row and read the current status to detect a D3 transition.
		var currentStatus *int64
		err := tx.QueryRow(ctx,
			`SELECT status_item_id FROM problems
			 WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, id).Scan(&currentStatus)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("problem: update load: %w", err)
		}

		// COALESCE keeps unset fields unchanged. Each $n::type cast lets a NULL bind
		// mean "leave as-is" without disturbing a column that is legitimately NULL.
		updateSQL := `
			UPDATE problems SET
				status_item_id   = COALESCE($2, status_item_id),
				category_item_id = COALESCE($3, category_item_id),
				type_item_id     = COALESCE($4, type_item_id),
				inspection_id    = COALESCE($5, inspection_id),
				cover_media_id   = COALESCE($6, cover_media_id),
				title            = COALESCE($7, title),
				description      = COALESCE($8, description),
				note             = COALESCE($9, note),
				updated_by       = $10
			WHERE id = $1 AND deleted_at IS NULL
			RETURNING ` + problemCols

		p, err := scanProblem(tx.QueryRow(ctx, updateSQL,
			id, in.StatusItemID, in.CategoryItemID, in.TypeItemID, in.InspectionID,
			in.CoverMediaID, in.Title, in.Description, in.Note, actorID))
		if isForeignKeyViolation(err) {
			return classifyFK(err)
		}
		if err != nil {
			return fmt.Errorf("problem: update: %w", err)
		}

		// D3: a real status change appends exactly one STATUS_CHANGE log in this tx.
		if statusChanged(in.StatusItemID, currentStatus) {
			if _, err := tx.Exec(ctx, `
				INSERT INTO problem_processing_log
					(problem_id, action, from_status_item_id, to_status_item_id, operator_id)
				VALUES ($1, 'STATUS_CHANGE', $2, $3, $4)`,
				id, currentStatus, in.StatusItemID, actorID); err != nil {
				return fmt.Errorf("problem: append status_change log: %w", err)
			}
		}
		out = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// statusChanged reports whether a non-nil requested status differs from the
// current one (nil current = previously unset, so any new value is a change).
func statusChanged(requested, current *int64) bool {
	if requested == nil {
		return false
	}
	if current == nil {
		return true
	}
	return *requested != *current
}

// Delete soft-deletes a problem. Missing/already-deleted -> ErrNotFound.
func (s *Service) Delete(ctx context.Context, id, actorID int64) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE problems SET deleted_at = now(), updated_by = $2
		 WHERE id = $1 AND deleted_at IS NULL`, id, actorID)
	if err != nil {
		return fmt.Errorf("problem: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListLogs returns a problem's processing log, oldest first (chronological audit
// order). A missing/deleted problem -> ErrNotFound.
func (s *Service) ListLogs(ctx context.Context, problemID int64) ([]oapi.ProblemProcessingLog, error) {
	if err := s.assertProblemExists(ctx, problemID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, problem_id, action, from_status_item_id, to_status_item_id, note,
		       operator_id, created_at
		FROM problem_processing_log WHERE problem_id = $1
		ORDER BY created_at ASC, id ASC`, problemID)
	if err != nil {
		return nil, fmt.Errorf("problem: list logs: %w", err)
	}
	defer rows.Close()

	out := make([]oapi.ProblemProcessingLog, 0)
	for rows.Next() {
		log, err := scanLog(rows)
		if err != nil {
			return nil, fmt.Errorf("problem: scan log: %w", err)
		}
		out = append(out, log)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("problem: list logs rows: %w", err)
	}
	return out, nil
}

// CreateLogInput is the validated payload for CreateLog. Action is already known
// to be COMMENT or REASSIGN (the handler rejects STATUS_CHANGE before this).
type CreateLogInput struct {
	Action     string
	Note       *string
	OperatorID *int64 // REASSIGN may carry the new operator/assignee
}

// CreateLog appends a COMMENT or REASSIGN entry to a problem's processing log.
// STATUS_CHANGE is rejected (ErrInvalidLogAction) — it is authored only by
// Update inside the status-change transaction (D3). The operator defaults to the
// acting user when none is supplied. Missing/deleted problem -> ErrNotFound.
func (s *Service) CreateLog(ctx context.Context, problemID int64, in CreateLogInput, actorID int64) (*oapi.ProblemProcessingLog, error) {
	if !validClientLogAction(in.Action) {
		return nil, ErrInvalidLogAction
	}
	if err := s.assertProblemExists(ctx, problemID); err != nil {
		return nil, err
	}
	operator := actorID
	if in.OperatorID != nil && *in.OperatorID != 0 {
		operator = *in.OperatorID
	}
	note := derefStr(in.Note)

	row := s.pool.QueryRow(ctx, `
		INSERT INTO problem_processing_log (problem_id, action, note, operator_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, problem_id, action, from_status_item_id, to_status_item_id,
		          note, operator_id, created_at`,
		problemID, in.Action, note, operator)
	log, err := scanLog(row)
	if isForeignKeyViolation(err) {
		return nil, ErrInspectorMissing
	}
	if err != nil {
		return nil, fmt.Errorf("problem: create log: %w", err)
	}
	return &log, nil
}

// assertProblemExists returns ErrNotFound unless a non-deleted problem with the
// id exists. Used by the log endpoints so a log against a missing problem 404s.
func (s *Service) assertProblemExists(ctx context.Context, id int64) error {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM problems WHERE id = $1 AND deleted_at IS NULL)`, id).
		Scan(&exists)
	if err != nil {
		return fmt.Errorf("problem: exists check: %w", err)
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

// scanLog materializes one processing-log row (fixed column order).
func scanLog(row pgx.Row) (oapi.ProblemProcessingLog, error) {
	var (
		log    oapi.ProblemProcessingLog
		action string
		note   string
	)
	if err := row.Scan(
		&log.Id, &log.ProblemId, &action, &log.FromStatusItemId, &log.ToStatusItemId,
		&note, &log.OperatorId, &log.CreatedAt,
	); err != nil {
		return oapi.ProblemProcessingLog{}, err
	}
	log.Action = oapi.ProcessingAction(action)
	if note != "" {
		log.Note = &note
	}
	return log, nil
}

// Map returns a GeoJSON FeatureCollection (WGS84) for the map layer, honoring the
// same filters as List and capped at limit features so the query never dumps the
// whole table. Geometry is rendered server-side via ST_AsGeoJSON; status/type
// labels (including retired dict items, for historical readability) feed the
// feature properties.
func (s *Service) Map(ctx context.Context, f ListFilter, limit int) (*oapi.ProblemFeatureCollection, error) {
	if limit <= 0 || limit > MapLimitMax {
		limit = MapLimitDefault
	}
	where, args := whereWithAlias(f, "p.")

	rows, err := s.pool.Query(ctx, `
		SELECT p.id, ST_AsGeoJSON(p.geom), p.title, p.captured_at,
		       p.type_item_id, p.status_item_id, p.category_item_id, p.cover_media_id,
		       ti.label, si.label, si.color
		FROM problems p
		LEFT JOIN dict_item ti ON ti.id = p.type_item_id
		LEFT JOIN dict_item si ON si.id = p.status_item_id
		`+where+`
		ORDER BY p.captured_at DESC, p.id DESC
		LIMIT $9`,
		append(append([]any{}, args...), limit)...)
	if err != nil {
		return nil, fmt.Errorf("problem: map: %w", err)
	}
	defer rows.Close()

	features := make([]oapi.ProblemFeature, 0)
	for rows.Next() {
		feat, err := scanFeature(rows)
		if err != nil {
			return nil, fmt.Errorf("problem: scan feature: %w", err)
		}
		features = append(features, feat)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("problem: map rows: %w", err)
	}

	return &oapi.ProblemFeatureCollection{
		Type:     oapi.FeatureCollection,
		Features: features,
	}, nil
}

// scanFeature materializes one map feature, parsing the ST_AsGeoJSON geometry
// string into the GeoJSONPoint API shape.
func scanFeature(row pgx.Row) (oapi.ProblemFeature, error) {
	var (
		id           int64
		geomJSON     string
		title        string
		capturedAt   time.Time
		typeID       *int64
		statusID     *int64
		categoryID   *int64
		coverMediaID *int64
		typeLabel    *string
		statusLabel  *string
		statusColor  *string
	)
	if err := row.Scan(&id, &geomJSON, &title, &capturedAt, &typeID, &statusID,
		&categoryID, &coverMediaID, &typeLabel, &statusLabel, &statusColor); err != nil {
		return oapi.ProblemFeature{}, err
	}

	var geom oapi.GeoJSONPoint
	if err := json.Unmarshal([]byte(geomJSON), &geom); err != nil {
		return oapi.ProblemFeature{}, fmt.Errorf("unmarshal geojson geometry: %w", err)
	}

	props := oapi.ProblemFeatureProperties{
		Id:             id,
		CapturedAt:     &capturedAt,
		TypeItemId:     typeID,
		StatusItemId:   statusID,
		CategoryItemId: categoryID,
		CoverMediaId:   coverMediaID,
		TypeLabel:      typeLabel,
		StatusLabel:    statusLabel,
		StatusColor:    statusColor,
	}
	if title != "" {
		props.Title = &title
	}

	return oapi.ProblemFeature{
		Type:       oapi.Feature,
		Geometry:   geom,
		Properties: props,
	}, nil
}

// classifyFK maps a foreign-key violation on create/update to the right sentinel.
// project_id and inspector_id are the NOT NULL RESTRICT FKs that a client can get
// wrong; everything else (inspection_id, the dict soft FKs) is nullable or
// historically tolerant, so a generic project-missing message is the safe default.
func classifyFK(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "inspector_id"):
		return ErrInspectorMissing
	case strings.Contains(msg, "inspection_id"):
		return ErrNotFound // referenced inspection absent
	default:
		return ErrProjectMissing
	}
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
