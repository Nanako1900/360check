package project

import (
	"context"
	"fmt"
	"sort"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
)

// Service holds the project/inspection-task business logic: input validation,
// geometry validation (ST_IsValid + SRID), custom_fields dictionary validation,
// and the delete guard that prevents orphaning live inspections/tasks.
type Service struct {
	repo *repo
}

// NewService wires the project service onto a pgx pool.
func NewService(pool *db.Pool) *Service { return &Service{repo: newRepo(pool)} }

// --- projects ---------------------------------------------------------------

// ListProjects returns a page of projects plus the total for the same filter.
func (s *Service) ListProjects(ctx context.Context, q, status *string, limit, offset int) ([]oapi.Project, int64, error) {
	return s.repo.listProjects(ctx, listProjectsFilter{q: q, status: status, limit: limit, offset: offset})
}

// GetProject returns a single non-deleted project.
func (s *Service) GetProject(ctx context.Context, id int64) (*oapi.Project, error) {
	return s.repo.getProject(ctx, id)
}

// CreateProject validates and inserts a project. It enforces a non-empty code/
// name, a recognized status, validated area geometry, and known custom_fields
// keys (when a project_field dictionary is configured).
func (s *Service) CreateProject(ctx context.Context, in oapi.ProjectCreate, actorID int64) (*oapi.Project, error) {
	if in.Code == "" {
		return nil, newValidation("code is required", "code", "REQUIRED", "code must not be empty")
	}
	if in.Name == "" {
		return nil, newValidation("name is required", "name", "REQUIRED", "name must not be empty")
	}
	status := oapi.ACTIVE
	if in.Status != nil {
		if !isValidProjectStatus(*in.Status) {
			return nil, newValidation("invalid status", "status", "INVALID", "status must be ACTIVE, PAUSED or ARCHIVED")
		}
		status = *in.Status
	}
	if err := s.validateCustomFields(ctx, in.CustomFields); err != nil {
		return nil, err
	}
	areaEWKB, err := s.encodeAndValidateArea(ctx, in.AreaGeom)
	if err != nil {
		return nil, err
	}
	cf, err := customFieldsJSON(in.CustomFields)
	if err != nil {
		return nil, err
	}
	return s.repo.createProject(ctx, createProjectArgs{
		code:         in.Code,
		name:         in.Name,
		description:  strOrEmpty(in.Description),
		status:       string(status),
		customFields: cf,
		areaEWKB:     areaEWKB,
		startDate:    dateToPg(in.StartDate),
		endDate:      dateToPg(in.EndDate),
		createdBy:    actorPtr(actorID),
	})
}

// UpdateProject validates and applies a partial project update.
func (s *Service) UpdateProject(ctx context.Context, id int64, in oapi.ProjectUpdate, actorID int64) (*oapi.Project, error) {
	args := updateProjectArgs{id: id, updatedBy: actorPtr(actorID)}

	if in.Name != nil {
		if *in.Name == "" {
			return nil, newValidation("name must not be empty", "name", "INVALID", "name must not be empty")
		}
		args.name = in.Name
	}
	args.description = in.Description
	if in.Status != nil {
		if !isValidProjectStatus(*in.Status) {
			return nil, newValidation("invalid status", "status", "INVALID", "status must be ACTIVE, PAUSED or ARCHIVED")
		}
		st := string(*in.Status)
		args.status = &st
	}
	if in.CustomFields != nil {
		if err := s.validateCustomFields(ctx, in.CustomFields); err != nil {
			return nil, err
		}
		cf, err := customFieldsJSON(in.CustomFields)
		if err != nil {
			return nil, err
		}
		args.customFields = cf
	}
	if in.AreaGeom != nil {
		ewkb, err := s.encodeAndValidateArea(ctx, in.AreaGeom)
		if err != nil {
			return nil, err
		}
		args.setArea = true
		args.areaEWKB = ewkb
	}
	if in.StartDate != nil {
		args.setStart = true
		args.startDate = dateToPg(in.StartDate)
	}
	if in.EndDate != nil {
		args.setEnd = true
		args.endDate = dateToPg(in.EndDate)
	}
	return s.repo.updateProject(ctx, args)
}

// DeleteProject soft-deletes a project after confirming it has no live
// inspections or inspection_tasks (which the FK would otherwise orphan).
func (s *Service) DeleteProject(ctx context.Context, id, actorID int64) error {
	// Confirm the project exists (and is live) before the dependents check so a
	// missing id reports NotFound rather than a misleading conflict.
	if _, err := s.repo.getProject(ctx, id); err != nil {
		return err
	}
	has, err := s.repo.projectHasDependents(ctx, id)
	if err != nil {
		return err
	}
	if has {
		return ErrProjectInUse
	}
	return s.repo.softDeleteProject(ctx, id, actorPtr(actorID))
}

// --- inspection_tasks -------------------------------------------------------

// ListTasks returns a page of tasks plus the total for the same filter.
func (s *Service) ListTasks(ctx context.Context, projectID *int64, status *string, assigneeID *int64, limit, offset int) ([]oapi.InspectionTask, int64, error) {
	return s.repo.listTasks(ctx, listTasksFilter{
		projectID: projectID, status: status, assigneeID: assigneeID, limit: limit, offset: offset,
	})
}

// GetTask returns a single non-deleted task.
func (s *Service) GetTask(ctx context.Context, id int64) (*oapi.InspectionTask, error) {
	return s.repo.getTask(ctx, id)
}

// CreateTask validates and inserts an inspection task. It enforces a non-empty
// title, an existing live parent project, a recognized task status (never
// OPEN/DONE), and validated plan geometry.
func (s *Service) CreateTask(ctx context.Context, in oapi.InspectionTaskCreate, actorID int64) (*oapi.InspectionTask, error) {
	if in.Title == "" {
		return nil, newValidation("title is required", "title", "REQUIRED", "title must not be empty")
	}
	if in.ProjectId <= 0 {
		return nil, newValidation("project_id is required", "project_id", "REQUIRED", "project_id must be a positive id")
	}
	exists, err := s.repo.projectExists(ctx, in.ProjectId)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, newValidation("project not found", "project_id", "INVALID", "no live project with that id")
	}
	status := oapi.TaskStatusPENDING
	if in.Status != nil {
		if !isValidTaskStatus(*in.Status) {
			return nil, newValidation("invalid status", "status", "INVALID", "status must be PENDING, IN_PROGRESS, COMPLETED or ARCHIVED")
		}
		status = *in.Status
	}
	planEWKB, err := s.encodeAndValidatePlan(ctx, in.PlanGeom)
	if err != nil {
		return nil, err
	}
	return s.repo.createTask(ctx, createTaskArgs{
		projectID:    in.ProjectId,
		title:        in.Title,
		description:  strOrEmpty(in.Description),
		status:       string(status),
		assigneeID:   in.AssigneeId,
		plannedStart: timeToPg(in.PlannedStart),
		plannedEnd:   timeToPg(in.PlannedEnd),
		planEWKB:     planEWKB,
		createdBy:    actorPtr(actorID),
	})
}

// UpdateTask validates and applies a partial task update.
func (s *Service) UpdateTask(ctx context.Context, id int64, in oapi.InspectionTaskUpdate, actorID int64) (*oapi.InspectionTask, error) {
	args := updateTaskArgs{id: id, updatedBy: actorPtr(actorID)}

	if in.Title != nil {
		if *in.Title == "" {
			return nil, newValidation("title must not be empty", "title", "INVALID", "title must not be empty")
		}
		args.title = in.Title
	}
	args.description = in.Description
	if in.Status != nil {
		if !isValidTaskStatus(*in.Status) {
			return nil, newValidation("invalid status", "status", "INVALID", "status must be PENDING, IN_PROGRESS, COMPLETED or ARCHIVED")
		}
		st := string(*in.Status)
		args.status = &st
	}
	// assignee_id is nullable: a present key (even null) reassigns; the generated
	// struct can't distinguish absent from null, so any update may clear it.
	args.setAssignee = true
	args.assigneeID = in.AssigneeId
	if in.PlannedStart != nil {
		args.setStart = true
		args.plannedStart = timeToPg(in.PlannedStart)
	}
	if in.PlannedEnd != nil {
		args.setEnd = true
		args.plannedEnd = timeToPg(in.PlannedEnd)
	}
	if in.PlanGeom != nil {
		ewkb, err := s.encodeAndValidatePlan(ctx, in.PlanGeom)
		if err != nil {
			return nil, err
		}
		args.setPlan = true
		args.planEWKB = ewkb
	}
	return s.repo.updateTask(ctx, args)
}

// DeleteTask soft-deletes a task.
func (s *Service) DeleteTask(ctx context.Context, id, actorID int64) error {
	return s.repo.softDeleteTask(ctx, id, actorPtr(actorID))
}

// --- validation helpers -----------------------------------------------------

// validateCustomFields rejects custom_fields keys that are not active dict_item
// codes under dict_type 'project_field'. When that dictionary is not configured
// (absent or no active items), validation is skipped.
func (s *Service) validateCustomFields(ctx context.Context, fields *map[string]interface{}) error {
	if fields == nil || len(*fields) == 0 {
		return nil
	}
	codes, configured, err := s.repo.projectFieldCodes(ctx)
	if err != nil {
		return err
	}
	if !configured {
		return nil // no project_field dictionary configured -> no constraint
	}
	var unknown []string
	for key := range *fields {
		if !codes[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown) // stable, deterministic detail order
	details := make([]oapi.ErrorDetail, 0, len(unknown))
	for _, k := range unknown {
		details = append(details, oapi.ErrorDetail{
			Field:   "custom_fields." + k,
			Code:    "UNKNOWN_FIELD",
			Message: fmt.Sprintf("%q is not a configured project_field", k),
		})
	}
	return &ErrValidation{Message: "unknown custom_fields key(s)", Details: details}
}

// encodeAndValidateArea encodes an optional area MultiPolygon to EWKB and runs
// the PostGIS validity/SRID check. A nil geometry yields nil bytes (NULL).
func (s *Service) encodeAndValidateArea(ctx context.Context, g *oapi.GeoJSONMultiPolygon) ([]byte, error) {
	ewkb, err := encodeMultiPolygon(g)
	if err != nil {
		return nil, newValidation("invalid area_geom", "area_geom", "INVALID", err.Error())
	}
	return s.validateGeomOrErr(ctx, ewkb, "area_geom")
}

// encodeAndValidatePlan encodes an optional plan MultiLineString to EWKB and runs
// the PostGIS validity/SRID check. A nil geometry yields nil bytes (NULL).
func (s *Service) encodeAndValidatePlan(ctx context.Context, g *oapi.GeoJSONMultiLineString) ([]byte, error) {
	ewkb, err := encodeMultiLineString(g)
	if err != nil {
		return nil, newValidation("invalid plan_geom", "plan_geom", "INVALID", err.Error())
	}
	return s.validateGeomOrErr(ctx, ewkb, "plan_geom")
}

// validateGeomOrErr returns ewkb unchanged when valid (or nil), and an
// *ErrValidation tagged to field when PostGIS reports the geometry invalid.
func (s *Service) validateGeomOrErr(ctx context.Context, ewkb []byte, field string) ([]byte, error) {
	if ewkb == nil {
		return nil, nil
	}
	valid, reason, err := s.repo.validateGeom(ctx, ewkb)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, newValidation("invalid "+field, field, "INVALID", reason)
	}
	return ewkb, nil
}

// actorPtr returns a pointer to the actor id, or nil for the unauthenticated
// zero id so audit columns store SQL NULL rather than 0.
func actorPtr(id int64) *int64 {
	if id == 0 {
		return nil
	}
	return &id
}
