package project

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
)

// repo is the hand-written pgx data-access layer for projects and inspection
// tasks. Geometry never flows through sqlc: writes bind SRID-4326 EWKB through
// ST_GeomFromEWKB($n), reads project geometry out as GeoJSON text via
// ST_AsGeoJSON. Every read filters deleted_at IS NULL.
type repo struct {
	pool *db.Pool
}

func newRepo(pool *db.Pool) *repo { return &repo{pool: pool} }

// --- projects ---------------------------------------------------------------

// projectColumns is the canonical SELECT projection for a project row, geometry
// rendered as GeoJSON text so the row scans into Go without PostGIS types.
const projectColumns = `
	id, code, name, description, status,
	custom_fields,
	ST_AsGeoJSON(area_geom)::text AS area_geojson,
	start_date, end_date,
	created_at, updated_at`

// projectRow is the raw scan target for a project SELECT.
type projectRow struct {
	id           int64
	code         string
	name         string
	description  string
	status       string
	customFields []byte
	areaGeoJSON  *string
	startDate    pgtype.Date
	endDate      pgtype.Date
	createdAt    pgtype.Timestamptz
	updatedAt    pgtype.Timestamptz
}

// scanProject reads one projectRow from a pgx.Row in projectColumns order.
func scanProject(row pgx.Row) (*projectRow, error) {
	var r projectRow
	err := row.Scan(
		&r.id, &r.code, &r.name, &r.description, &r.status,
		&r.customFields, &r.areaGeoJSON,
		&r.startDate, &r.endDate, &r.createdAt, &r.updatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// toProject converts a scanned row into the API model, decoding geometry/JSON.
func (r *projectRow) toProject() (*oapi.Project, error) {
	cf, err := decodeCustomFields(r.customFields)
	if err != nil {
		return nil, err
	}
	area, err := decodeMultiPolygonJSON(r.areaGeoJSON)
	if err != nil {
		return nil, err
	}
	desc := r.description
	return &oapi.Project{
		Id:           r.id,
		Code:         r.code,
		Name:         r.name,
		Description:  &desc,
		Status:       oapi.ProjectStatus(r.status),
		CustomFields: cf,
		AreaGeom:     area,
		StartDate:    pgToDate(r.startDate),
		EndDate:      pgToDate(r.endDate),
		CreatedAt:    r.createdAt.Time,
		UpdatedAt:    r.updatedAt.Time,
	}, nil
}

// createProjectArgs carries the validated, encoded inputs for an insert.
type createProjectArgs struct {
	code         string
	name         string
	description  string
	status       string
	customFields []byte
	areaEWKB     []byte // nil => NULL geometry
	startDate    pgtype.Date
	endDate      pgtype.Date
	createdBy    *int64
}

// createProject inserts a project and returns the freshly created row.
func (rp *repo) createProject(ctx context.Context, a createProjectArgs) (*oapi.Project, error) {
	const sql = `
		INSERT INTO projects
			(code, name, description, status, custom_fields, area_geom,
			 start_date, end_date, created_by, updated_by)
		VALUES
			($1, $2, $3, $4, $5,
			 CASE WHEN $6::bytea IS NULL THEN NULL ELSE ST_GeomFromEWKB($6) END,
			 $7, $8, $9, $9)
		RETURNING` + projectColumns
	row := rp.pool.QueryRow(ctx, sql,
		a.code, a.name, a.description, a.status, a.customFields, a.areaEWKB,
		a.startDate, a.endDate, a.createdBy,
	)
	pr, err := scanProject(row)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrCodeTaken
		}
		return nil, fmt.Errorf("project: insert: %w", err)
	}
	return pr.toProject()
}

// getProject returns a single non-deleted project by id.
func (rp *repo) getProject(ctx context.Context, id int64) (*oapi.Project, error) {
	const sql = `SELECT` + projectColumns + `
		FROM projects WHERE id = $1 AND deleted_at IS NULL`
	pr, err := scanProject(rp.pool.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("project: get: %w", err)
	}
	return pr.toProject()
}

// listProjectsFilter holds the normalized list filters.
type listProjectsFilter struct {
	q      *string
	status *string
	limit  int
	offset int
}

// listProjects returns a page of projects plus the total for the same filter.
func (rp *repo) listProjects(ctx context.Context, f listProjectsFilter) ([]oapi.Project, int64, error) {
	where, args := projectWhere(f.q, f.status)

	countSQL := `SELECT count(*) FROM projects` + where
	var total int64
	if err := rp.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("project: count: %w", err)
	}

	listSQL := `SELECT` + projectColumns + ` FROM projects` + where +
		fmt.Sprintf(` ORDER BY id DESC LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)
	args = append(args, f.limit, f.offset)

	rows, err := rp.pool.Query(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("project: list: %w", err)
	}
	defer rows.Close()

	out := make([]oapi.Project, 0)
	for rows.Next() {
		pr, err := scanProject(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("project: list scan: %w", err)
		}
		p, err := pr.toProject()
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("project: list rows: %w", err)
	}
	return out, total, nil
}

// projectWhere builds the shared WHERE clause and positional args for list/count.
func projectWhere(q, status *string) (string, []any) {
	conds := []string{"deleted_at IS NULL"}
	var args []any
	if q != nil && *q != "" {
		args = append(args, "%"+*q+"%")
		conds = append(conds, fmt.Sprintf("(code ILIKE $%d OR name ILIKE $%d)", len(args), len(args)))
	}
	if status != nil && *status != "" {
		args = append(args, *status)
		conds = append(conds, fmt.Sprintf("status = $%d", len(args)))
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// updateProjectArgs carries a partial update; nil fields are left unchanged via
// COALESCE, and geometry is replaced only when setArea is true.
type updateProjectArgs struct {
	id           int64
	name         *string
	description  *string
	status       *string
	customFields []byte // nil => leave unchanged
	setArea      bool
	areaEWKB     []byte // applied when setArea (nil clears the geometry)
	setStart     bool
	startDate    pgtype.Date
	setEnd       bool
	endDate      pgtype.Date
	updatedBy    *int64
}

// updateProject applies a partial update and returns the updated row. A missing
// or soft-deleted target yields ErrNotFound.
func (rp *repo) updateProject(ctx context.Context, a updateProjectArgs) (*oapi.Project, error) {
	const sql = `
		UPDATE projects SET
			name        = COALESCE($2, name),
			description = COALESCE($3, description),
			status      = COALESCE($4, status),
			custom_fields = COALESCE($5, custom_fields),
			area_geom = CASE
				WHEN $6 THEN (CASE WHEN $7::bytea IS NULL THEN NULL ELSE ST_GeomFromEWKB($7) END)
				ELSE area_geom END,
			start_date = CASE WHEN $8 THEN $9 ELSE start_date END,
			end_date   = CASE WHEN $10 THEN $11 ELSE end_date END,
			updated_by = $12
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING` + projectColumns
	row := rp.pool.QueryRow(ctx, sql,
		a.id, a.name, a.description, a.status, a.customFields,
		a.setArea, a.areaEWKB,
		a.setStart, a.startDate,
		a.setEnd, a.endDate,
		a.updatedBy,
	)
	pr, err := scanProject(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrCodeTaken
		}
		return nil, fmt.Errorf("project: update: %w", err)
	}
	return pr.toProject()
}

// projectHasDependents reports whether a project still has any non-deleted
// inspection or inspection_task (which would be orphaned by a soft delete).
func (rp *repo) projectHasDependents(ctx context.Context, id int64) (bool, error) {
	const sql = `
		SELECT
			EXISTS (SELECT 1 FROM inspections      WHERE project_id = $1 AND deleted_at IS NULL)
			OR
			EXISTS (SELECT 1 FROM inspection_tasks WHERE project_id = $1 AND deleted_at IS NULL)`
	var has bool
	if err := rp.pool.QueryRow(ctx, sql, id).Scan(&has); err != nil {
		return false, fmt.Errorf("project: dependents check: %w", err)
	}
	return has, nil
}

// softDeleteProject marks a project deleted. Returns ErrNotFound when no live row
// matched.
func (rp *repo) softDeleteProject(ctx context.Context, id int64, by *int64) error {
	const sql = `
		UPDATE projects SET deleted_at = now(), updated_by = $2
		WHERE id = $1 AND deleted_at IS NULL`
	tag, err := rp.pool.Exec(ctx, sql, id, by)
	if err != nil {
		return fmt.Errorf("project: soft delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// projectExists reports whether a non-deleted project with id exists (FK guard
// for task creation).
func (rp *repo) projectExists(ctx context.Context, id int64) (bool, error) {
	var exists bool
	err := rp.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM projects WHERE id = $1 AND deleted_at IS NULL)`, id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("project: exists check: %w", err)
	}
	return exists, nil
}

// --- inspection_tasks -------------------------------------------------------

// taskColumns is the canonical SELECT projection for a task row.
const taskColumns = `
	id, project_id, title, description, status, assignee_id,
	planned_start, planned_end,
	ST_AsGeoJSON(plan_geom)::text AS plan_geojson,
	created_at, updated_at`

// taskRow is the raw scan target for a task SELECT.
type taskRow struct {
	id           int64
	projectID    int64
	title        string
	description  string
	status       string
	assigneeID   *int64
	plannedStart pgtype.Timestamptz
	plannedEnd   pgtype.Timestamptz
	planGeoJSON  *string
	createdAt    pgtype.Timestamptz
	updatedAt    pgtype.Timestamptz
}

// scanTask reads one taskRow from a pgx.Row in taskColumns order.
func scanTask(row pgx.Row) (*taskRow, error) {
	var r taskRow
	err := row.Scan(
		&r.id, &r.projectID, &r.title, &r.description, &r.status, &r.assigneeID,
		&r.plannedStart, &r.plannedEnd, &r.planGeoJSON,
		&r.createdAt, &r.updatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// toTask converts a scanned row into the API model, decoding geometry.
func (r *taskRow) toTask() (*oapi.InspectionTask, error) {
	plan, err := decodeMultiLineStringJSON(r.planGeoJSON)
	if err != nil {
		return nil, err
	}
	desc := r.description
	return &oapi.InspectionTask{
		Id:           r.id,
		ProjectId:    r.projectID,
		Title:        r.title,
		Description:  &desc,
		Status:       oapi.TaskStatus(r.status),
		AssigneeId:   r.assigneeID,
		PlannedStart: pgToTimePtr(r.plannedStart),
		PlannedEnd:   pgToTimePtr(r.plannedEnd),
		PlanGeom:     plan,
		CreatedAt:    r.createdAt.Time,
		UpdatedAt:    r.updatedAt.Time,
	}, nil
}

// createTaskArgs carries the validated, encoded inputs for a task insert.
type createTaskArgs struct {
	projectID    int64
	title        string
	description  string
	status       string
	assigneeID   *int64
	plannedStart pgtype.Timestamptz
	plannedEnd   pgtype.Timestamptz
	planEWKB     []byte // nil => NULL geometry
	createdBy    *int64
}

// createTask inserts an inspection task and returns the created row.
func (rp *repo) createTask(ctx context.Context, a createTaskArgs) (*oapi.InspectionTask, error) {
	const sql = `
		INSERT INTO inspection_tasks
			(project_id, title, description, status, assignee_id,
			 planned_start, planned_end, plan_geom, created_by, updated_by)
		VALUES
			($1, $2, $3, $4, $5, $6, $7,
			 CASE WHEN $8::bytea IS NULL THEN NULL ELSE ST_GeomFromEWKB($8) END,
			 $9, $9)
		RETURNING` + taskColumns
	row := rp.pool.QueryRow(ctx, sql,
		a.projectID, a.title, a.description, a.status, a.assigneeID,
		a.plannedStart, a.plannedEnd, a.planEWKB, a.createdBy,
	)
	tr, err := scanTask(row)
	if err != nil {
		return nil, fmt.Errorf("project: insert task: %w", err)
	}
	return tr.toTask()
}

// getTask returns a single non-deleted task by id.
func (rp *repo) getTask(ctx context.Context, id int64) (*oapi.InspectionTask, error) {
	const sql = `SELECT` + taskColumns + `
		FROM inspection_tasks WHERE id = $1 AND deleted_at IS NULL`
	tr, err := scanTask(rp.pool.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("project: get task: %w", err)
	}
	return tr.toTask()
}

// listTasksFilter holds the normalized task list filters.
type listTasksFilter struct {
	projectID  *int64
	status     *string
	assigneeID *int64
	limit      int
	offset     int
}

// listTasks returns a page of tasks plus the total for the same filter.
func (rp *repo) listTasks(ctx context.Context, f listTasksFilter) ([]oapi.InspectionTask, int64, error) {
	where, args := taskWhere(f.projectID, f.status, f.assigneeID)

	countSQL := `SELECT count(*) FROM inspection_tasks` + where
	var total int64
	if err := rp.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("project: count tasks: %w", err)
	}

	listSQL := `SELECT` + taskColumns + ` FROM inspection_tasks` + where +
		fmt.Sprintf(` ORDER BY id DESC LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)
	args = append(args, f.limit, f.offset)

	rows, err := rp.pool.Query(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("project: list tasks: %w", err)
	}
	defer rows.Close()

	out := make([]oapi.InspectionTask, 0)
	for rows.Next() {
		tr, err := scanTask(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("project: list tasks scan: %w", err)
		}
		t, err := tr.toTask()
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("project: list tasks rows: %w", err)
	}
	return out, total, nil
}

// taskWhere builds the shared WHERE clause and positional args for list/count.
func taskWhere(projectID *int64, status *string, assigneeID *int64) (string, []any) {
	conds := []string{"deleted_at IS NULL"}
	var args []any
	if projectID != nil {
		args = append(args, *projectID)
		conds = append(conds, fmt.Sprintf("project_id = $%d", len(args)))
	}
	if status != nil && *status != "" {
		args = append(args, *status)
		conds = append(conds, fmt.Sprintf("status = $%d", len(args)))
	}
	if assigneeID != nil {
		args = append(args, *assigneeID)
		conds = append(conds, fmt.Sprintf("assignee_id = $%d", len(args)))
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// updateTaskArgs carries a partial task update; nil fields are left unchanged.
type updateTaskArgs struct {
	id           int64
	title        *string
	description  *string
	status       *string
	setAssignee  bool
	assigneeID   *int64
	setStart     bool
	plannedStart pgtype.Timestamptz
	setEnd       bool
	plannedEnd   pgtype.Timestamptz
	setPlan      bool
	planEWKB     []byte // applied when setPlan (nil clears the geometry)
	updatedBy    *int64
}

// updateTask applies a partial update and returns the updated row.
func (rp *repo) updateTask(ctx context.Context, a updateTaskArgs) (*oapi.InspectionTask, error) {
	const sql = `
		UPDATE inspection_tasks SET
			title       = COALESCE($2, title),
			description = COALESCE($3, description),
			status      = COALESCE($4, status),
			assignee_id = CASE WHEN $5 THEN $6 ELSE assignee_id END,
			planned_start = CASE WHEN $7 THEN $8 ELSE planned_start END,
			planned_end   = CASE WHEN $9 THEN $10 ELSE planned_end END,
			plan_geom = CASE
				WHEN $11 THEN (CASE WHEN $12::bytea IS NULL THEN NULL ELSE ST_GeomFromEWKB($12) END)
				ELSE plan_geom END,
			updated_by = $13
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING` + taskColumns
	row := rp.pool.QueryRow(ctx, sql,
		a.id, a.title, a.description, a.status,
		a.setAssignee, a.assigneeID,
		a.setStart, a.plannedStart,
		a.setEnd, a.plannedEnd,
		a.setPlan, a.planEWKB,
		a.updatedBy,
	)
	tr, err := scanTask(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("project: update task: %w", err)
	}
	return tr.toTask()
}

// softDeleteTask marks a task deleted. Returns ErrNotFound when no live row matched.
func (rp *repo) softDeleteTask(ctx context.Context, id int64, by *int64) error {
	const sql = `
		UPDATE inspection_tasks SET deleted_at = now(), updated_by = $2
		WHERE id = $1 AND deleted_at IS NULL`
	tag, err := rp.pool.Exec(ctx, sql, id, by)
	if err != nil {
		return fmt.Errorf("project: soft delete task: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// --- custom_fields dictionary ----------------------------------------------

// projectFieldCodes returns the set of active dict_item codes under the
// dict_type whose code is 'project_field'. When that dict_type is absent or has
// no active items, configured is false and the caller skips custom_fields
// validation (no constraint configured).
func (rp *repo) projectFieldCodes(ctx context.Context) (codes map[string]bool, configured bool, err error) {
	const sql = `
		SELECT di.code
		FROM dict_item di
		JOIN dict_type dt ON dt.id = di.dict_type_id
		WHERE dt.code = 'project_field'
		  AND dt.deleted_at IS NULL AND dt.is_active = TRUE
		  AND di.deleted_at IS NULL AND di.is_active = TRUE`
	rows, qerr := rp.pool.Query(ctx, sql)
	if qerr != nil {
		return nil, false, fmt.Errorf("project: load project_field dict: %w", qerr)
	}
	defer rows.Close()

	codes = make(map[string]bool)
	for rows.Next() {
		var code string
		if serr := rows.Scan(&code); serr != nil {
			return nil, false, fmt.Errorf("project: scan dict code: %w", serr)
		}
		codes[code] = true
	}
	if rerr := rows.Err(); rerr != nil {
		return nil, false, fmt.Errorf("project: dict rows: %w", rerr)
	}
	return codes, len(codes) > 0, nil
}

// validateGeom delegates to the platform validator (ST_IsValid + SRID 4326).
func (rp *repo) validateGeom(ctx context.Context, ewkb []byte) (bool, string, error) {
	return db.ValidateGeomWGS84(ctx, rp.pool, ewkb)
}
