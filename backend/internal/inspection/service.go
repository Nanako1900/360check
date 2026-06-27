// Package inspection implements the inspection-session domain: starting and
// finishing "开始巡查 -> 结束巡查" sessions, mileage/duration derivation, and
// trajectory queries. Every geometry-touching statement is hand-written pgx
// against the pool (never sqlc, which cannot resolve PostGIS functions):
// route_geom is built on finish via ST_MakeLine(geom ORDER BY seq); mileage is
// ST_Length(route::geography) so it is METERS, not degrees; geometry is read out
// as EWKB (ST_AsEWKB) and re-emitted as GeoJSON (WGS84) at the API boundary.
package inspection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
)

// Sentinel errors mapped to envelope codes by the handler.
var (
	// ErrNotFound — inspection id missing or soft-deleted.
	ErrNotFound = errors.New("inspection not found")
	// ErrNotInProgress — finish attempted on a session not in IN_PROGRESS
	// (illegal FSM transition; repeated finish on FINISHED/ABANDONED).
	ErrNotInProgress = errors.New("inspection is not in progress")
	// ErrEndedBeforeStarted — requested ended_at precedes started_at.
	ErrEndedBeforeStarted = errors.New("ended_at must be >= started_at")
	// ErrProjectMissing — start references a project_id with no live project (FK).
	ErrProjectMissing = errors.New("project not found")
	// ErrTaskMissing — start references a task_id with no live task (FK).
	ErrTaskMissing = errors.New("task not found")
	// ErrInspectorMissing — start references an inspector_id with no user (FK).
	ErrInspectorMissing = errors.New("inspector not found")
	// ErrForeignKeyViolated — start hit a 23503 on an unrecognized FK column.
	ErrForeignKeyViolated = errors.New("referenced entity not found")
)

// Status literals are frozen by the contract; the FSM is IN_PROGRESS -> FINISHED
// (normal end) or IN_PROGRESS -> ABANDONED (discard).
const (
	statusInProgress = string(oapi.InspectionStatusINPROGRESS)
	statusFinished   = string(oapi.InspectionStatusFINISHED)
	statusAbandoned  = string(oapi.InspectionStatusABANDONED)
)

// Service owns the inspections + trajectory_points tables.
type Service struct {
	pool *db.Pool
}

// NewService wires the inspection service onto the shared pool.
func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

// inspectionProjection returns the canonical column list used by every read,
// with each column qualified by the given table alias (e.g. "i." or "").
// route_geom is emitted as EWKB bytea (NULL until session end) so it round-trips
// through the geo package; device_info as JSON bytes. The column ORDER here is
// the contract scanInspection depends on.
func inspectionProjection(alias string) string {
	return alias + "id, " + alias + "client_uuid, " + alias + "project_id, " +
		alias + "task_id, " + alias + "inspector_id, " + alias + "status, " +
		alias + "started_at, " + alias + "ended_at, " + alias + "duration_seconds, " +
		alias + "mileage_meters, " + alias + "point_count, " +
		"ST_AsEWKB(" + alias + "route_geom), " + alias + "device_info, " +
		alias + "note, " + alias + "created_at, " + alias + "updated_at"
}

// inspectionCols is the unqualified canonical projection (plain SELECTs).
var inspectionCols = inspectionProjection("")

// scanInspection materializes one inspection row from the canonical projection.
func scanInspection(row pgx.Row) (oapi.Inspection, error) {
	var (
		insp       oapi.Inspection
		taskID     *int64
		endedAt    *time.Time
		routeEWKB  []byte
		deviceJSON []byte
		status     string
		note       string
	)
	if err := row.Scan(
		&insp.Id, &insp.ClientUuid, &insp.ProjectId, &taskID, &insp.InspectorId, &status,
		&insp.StartedAt, &endedAt, &insp.DurationSeconds, &insp.MileageMeters, &insp.PointCount,
		&routeEWKB, &deviceJSON, &note, &insp.CreatedAt, &insp.UpdatedAt,
	); err != nil {
		return oapi.Inspection{}, err
	}
	insp.TaskId = taskID
	insp.EndedAt = endedAt
	insp.Status = oapi.InspectionStatus(status)
	if note != "" {
		insp.Note = &note
	}
	route, err := decodeLineStringEWKB(routeEWKB)
	if err != nil {
		return oapi.Inspection{}, fmt.Errorf("decode route_geom: %w", err)
	}
	insp.RouteGeom = route
	if len(deviceJSON) > 0 {
		var m map[string]interface{}
		if err := json.Unmarshal(deviceJSON, &m); err == nil && len(m) > 0 {
			insp.DeviceInfo = &m
		}
	}
	return insp, nil
}

// ListFilter holds the optional filters for List (all nil = unfiltered page).
type ListFilter struct {
	ProjectID   *int64
	InspectorID *int64
	Status      *string
	From        *time.Time
	To          *time.Time
}

// List returns a page of non-deleted inspections plus the total for the filter.
// Newest-started first. Geometry columns round-trip through EWKB.
func (s *Service) List(ctx context.Context, f ListFilter, limit, offset int) ([]oapi.Inspection, int64, error) {
	where := `WHERE deleted_at IS NULL
		AND ($1::bigint IS NULL OR project_id = $1)
		AND ($2::bigint IS NULL OR inspector_id = $2)
		AND ($3::text   IS NULL OR status = $3::inspection_status)
		AND ($4::timestamptz IS NULL OR started_at >= $4)
		AND ($5::timestamptz IS NULL OR started_at <  $5)`
	args := []any{f.ProjectID, f.InspectorID, f.Status, f.From, f.To}

	rows, err := s.pool.Query(ctx,
		`SELECT `+inspectionCols+` FROM inspections `+where+`
		 ORDER BY started_at DESC, id DESC LIMIT $6 OFFSET $7`,
		append(append([]any{}, args...), limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("inspection: list: %w", err)
	}
	defer rows.Close()

	out := make([]oapi.Inspection, 0, limit)
	for rows.Next() {
		insp, err := scanInspection(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("inspection: scan: %w", err)
		}
		out = append(out, insp)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("inspection: list rows: %w", err)
	}

	var total int64
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM inspections `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("inspection: count: %w", err)
	}
	return out, total, nil
}

// Get returns one non-deleted inspection. Missing/deleted -> ErrNotFound.
func (s *Service) Get(ctx context.Context, id int64) (*oapi.Inspection, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+inspectionCols+` FROM inspections WHERE id = $1 AND deleted_at IS NULL`, id)
	insp, err := scanInspection(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("inspection: get: %w", err)
	}
	return &insp, nil
}

// StartInput is the validated payload for Start.
type StartInput struct {
	ProjectID   int64
	StartedAt   time.Time
	InspectorID int64   // resolved (request value or authenticated user)
	ClientUUID  *string // nil -> DB default gen_random_uuid()
	TaskID      *int64
	DeviceInfo  *map[string]interface{} // nil -> '{}'
	Note        *string
}

// Start creates an IN_PROGRESS session. It is idempotent on client_uuid:
// INSERT ... ON CONFLICT (client_uuid) DO NOTHING, then the row is re-selected so
// a replayed offline start returns the original session unchanged.
func (s *Service) Start(ctx context.Context, in StartInput, actorID int64) (*oapi.Inspection, error) {
	device := []byte("{}")
	if in.DeviceInfo != nil {
		b, err := json.Marshal(*in.DeviceInfo)
		if err != nil {
			return nil, fmt.Errorf("inspection: marshal device_info: %w", err)
		}
		device = b
	}
	note := ""
	if in.Note != nil {
		note = *in.Note
	}

	// client_uuid: use provided value or let the column default fill it. Capture
	// the resolved client_uuid so we can re-select the row after ON CONFLICT.
	var clientUUID string
	insertSQL := `
		INSERT INTO inspections
			(client_uuid, project_id, task_id, inspector_id, status, started_at,
			 device_info, note, created_by, updated_by)
		VALUES (COALESCE($1, gen_random_uuid()), $2, $3, $4, 'IN_PROGRESS', $5, $6, $7, $8, $8)
		ON CONFLICT (client_uuid) DO NOTHING
		RETURNING client_uuid`

	err := s.pool.QueryRow(ctx, insertSQL,
		in.ClientUUID, in.ProjectID, in.TaskID, in.InspectorID, in.StartedAt,
		device, note, actorID,
	).Scan(&clientUUID)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Conflict: a row with this client_uuid already exists — return it.
		if in.ClientUUID == nil {
			// Should not happen (no client_uuid -> no conflict), surface as error.
			return nil, fmt.Errorf("inspection: start: unexpected conflict without client_uuid")
		}
		return s.getByClientUUID(ctx, *in.ClientUUID)
	case err != nil:
		if fkErr, ok := fkViolation(err); ok {
			return nil, fkErr
		}
		return nil, fmt.Errorf("inspection: start: %w", err)
	}
	return s.getByClientUUID(ctx, clientUUID)
}

// getByClientUUID re-selects a freshly inserted/existing session by client_uuid.
func (s *Service) getByClientUUID(ctx context.Context, clientUUID string) (*oapi.Inspection, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+inspectionCols+` FROM inspections WHERE client_uuid = $1 AND deleted_at IS NULL`, clientUUID)
	insp, err := scanInspection(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("inspection: get by client_uuid: %w", err)
	}
	return &insp, nil
}

// FinishInput is the validated payload for Finish.
type FinishInput struct {
	EndedAt *time.Time // nil -> server now()
	Note    *string    // nil -> leave note unchanged
	Discard bool       // true (request Status=ABANDONED) -> ABANDONED instead of FINISHED
}

// Finish closes an IN_PROGRESS session in a single transaction: it builds
// route_geom = ST_MakeLine(geom ORDER BY seq) from this session's trajectory
// points, derives mileage_meters = ST_Length(route::geography) (meters; 0 with a
// NULL route when < 2 points), point_count, and duration_seconds, then sets
// status to FINISHED (or ABANDONED on discard) and ended_at. Finishing a session
// that is not IN_PROGRESS -> ErrNotInProgress (409). ended_at < started_at ->
// ErrEndedBeforeStarted (422).
func (s *Service) Finish(ctx context.Context, id int64, in FinishInput, actorID int64) (*oapi.Inspection, error) {
	var out oapi.Inspection
	err := s.pool.WithTx(ctx, func(tx pgx.Tx) error {
		// Lock the row and read started_at + current status.
		var (
			startedAt time.Time
			status    string
		)
		err := tx.QueryRow(ctx,
			`SELECT started_at, status FROM inspections
			 WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, id).Scan(&startedAt, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("inspection: finish load: %w", err)
		}
		if status != statusInProgress {
			return ErrNotInProgress
		}

		endedAt := time.Now().UTC()
		if in.EndedAt != nil {
			endedAt = *in.EndedAt
		}
		if endedAt.Before(startedAt) {
			return ErrEndedBeforeStarted
		}

		target := statusFinished
		if in.Discard {
			target = statusAbandoned
		}

		// Build route + derive metrics in one statement. ST_MakeLine yields a
		// 1-vertex LineString for a single point (NOT NULL), so we explicitly null
		// the route and zero the mileage when there are fewer than 2 points — the
		// contract's "<2 points -> route NULL + mileage 0". The geography cast
		// yields meters (plain geometry length would be degrees).
		updateSQL := `
			WITH route AS (
				SELECT
					CASE WHEN count(*) >= 2 THEN ST_MakeLine(geom ORDER BY seq) END AS line,
					count(*) AS n
				FROM trajectory_points WHERE inspection_id = $1
			)
			UPDATE inspections i SET
				status           = $2::inspection_status,
				ended_at         = $3,
				route_geom       = route.line,
				point_count      = route.n,
				mileage_meters   = COALESCE(ST_Length(route.line::geography), 0),
				duration_seconds = GREATEST(0, EXTRACT(EPOCH FROM ($3 - i.started_at))::bigint),
				note             = CASE WHEN $4::text IS NULL THEN i.note ELSE $4 END,
				updated_by       = $5
			FROM route
			WHERE i.id = $1
			RETURNING ` + inspectionProjection("i.")

		insp, err := scanInspection(tx.QueryRow(ctx, updateSQL,
			id, target, endedAt, in.Note, actorID))
		if err != nil {
			return fmt.Errorf("inspection: finish update: %w", err)
		}
		out = insp
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Trajectory returns the session's route (GeoJSON LineString or nil) plus its
// trajectory points ordered by seq, all WGS84. Missing inspection -> ErrNotFound.
func (s *Service) Trajectory(ctx context.Context, id int64) (*oapi.Trajectory, error) {
	// Confirm the inspection exists (and is not soft-deleted) and read its route.
	var routeEWKB []byte
	err := s.pool.QueryRow(ctx,
		`SELECT ST_AsEWKB(route_geom) FROM inspections WHERE id = $1 AND deleted_at IS NULL`, id).
		Scan(&routeEWKB)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("inspection: trajectory route: %w", err)
	}
	route, err := decodeLineStringEWKB(routeEWKB)
	if err != nil {
		return nil, fmt.Errorf("inspection: decode route: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, client_uuid, inspection_id, seq, ST_AsEWKB(geom), recorded_at,
		       speed, bearing, altitude, accuracy
		FROM trajectory_points WHERE inspection_id = $1 ORDER BY seq`, id)
	if err != nil {
		return nil, fmt.Errorf("inspection: trajectory points: %w", err)
	}
	defer rows.Close()

	points := make([]oapi.TrajectoryPoint, 0)
	for rows.Next() {
		var (
			pt       oapi.TrajectoryPoint
			geomEWKB []byte
		)
		if err := rows.Scan(
			&pt.Id, &pt.ClientUuid, &pt.InspectionId, &pt.Seq, &geomEWKB, &pt.RecordedAt,
			&pt.Speed, &pt.Bearing, &pt.Altitude, &pt.Accuracy,
		); err != nil {
			return nil, fmt.Errorf("inspection: scan point: %w", err)
		}
		geom, err := decodePointEWKB(geomEWKB)
		if err != nil {
			return nil, fmt.Errorf("inspection: decode point geom: %w", err)
		}
		pt.Geom = geom
		points = append(points, pt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("inspection: trajectory rows: %w", err)
	}

	return &oapi.Trajectory{
		InspectionId: id,
		Route:        route,
		Points:       points,
	}, nil
}
