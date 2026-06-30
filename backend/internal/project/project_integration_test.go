//go:build integration

package project

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	migrations "github.com/nnkglobal/c5-backend/db/migrations"
	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
)

// newService spins a fresh PostGIS container, runs all migrations, and returns a
// service plus the underlying pool (for direct seeding/assertions).
func newService(t *testing.T) (*Service, *db.Pool) {
	t.Helper()
	ctx := context.Background()

	pg, err := tcpostgres.Run(ctx, "postgis/postgis:16-3.4",
		tcpostgres.WithDatabase("c5"),
		tcpostgres.WithUsername("c5"),
		tcpostgres.WithPassword("c5"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, db.RunMigrations(dsn, migrations.FS))

	pool, err := db.New(ctx, dsn, 5, 1, 30*time.Second)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, pool.Ping(ctx))

	return NewService(pool), pool
}

// adminID returns the seeded admin user's id (acts as the actor for created_by).
func adminID(t *testing.T, pool *db.Pool) int64 {
	t.Helper()
	var id int64
	require.NoError(t, pool.QueryRow(context.Background(),
		"SELECT id FROM users WHERE username='admin'").Scan(&id))
	return id
}

// ptr is a small generic helper for taking the address of a literal.
func ptr[T any](v T) *T { return &v }

func TestCreateProject_WithoutGeom(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	actor := adminID(t, pool)

	p, err := svc.CreateProject(ctx, oapi.ProjectCreate{
		Code:        "P-001",
		Name:        "Highway survey",
		Description: ptr("north loop"),
	}, actor)
	require.NoError(t, err)
	assert.Positive(t, p.Id)
	assert.Equal(t, "P-001", p.Code)
	assert.Equal(t, oapi.ACTIVE, p.Status) // defaulted
	assert.Nil(t, p.AreaGeom)

	// created_by audit column persisted.
	var createdBy *int64
	require.NoError(t, pool.QueryRow(ctx,
		"SELECT created_by FROM projects WHERE id=$1", p.Id).Scan(&createdBy))
	require.NotNil(t, createdBy)
	assert.Equal(t, actor, *createdBy)

	// Round-trips through Get.
	got, err := svc.GetProject(ctx, p.Id)
	require.NoError(t, err)
	assert.Equal(t, p.Code, got.Code)
}

func TestCreateProject_WithAreaGeom(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	actor := adminID(t, pool)

	area := &oapi.GeoJSONMultiPolygon{
		Type: oapi.MultiPolygon,
		Coordinates: [][][][]float32{
			{{{113.0, 23.0}, {113.0, 23.01}, {113.01, 23.01}, {113.01, 23.0}, {113.0, 23.0}}},
		},
	}
	p, err := svc.CreateProject(ctx, oapi.ProjectCreate{
		Code: "P-GEO", Name: "with area", AreaGeom: area,
	}, actor)
	require.NoError(t, err)
	require.NotNil(t, p.AreaGeom, "area_geom should round-trip out as GeoJSON")
	assert.Equal(t, oapi.MultiPolygon, p.AreaGeom.Type)
	require.Len(t, p.AreaGeom.Coordinates, 1)

	// The stored geometry is SRID 4326.
	var srid int
	require.NoError(t, pool.QueryRow(ctx,
		"SELECT ST_SRID(area_geom) FROM projects WHERE id=$1", p.Id).Scan(&srid))
	assert.Equal(t, 4326, srid)
}

func TestCreateProject_InvalidAreaGeom(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	actor := adminID(t, pool)

	// Self-intersecting "bowtie" polygon: SRID 4326 but topologically invalid.
	bowtie := &oapi.GeoJSONMultiPolygon{
		Type: oapi.MultiPolygon,
		Coordinates: [][][][]float32{
			{{{0, 0}, {1, 1}, {1, 0}, {0, 1}, {0, 0}}},
		},
	}
	_, err := svc.CreateProject(ctx, oapi.ProjectCreate{
		Code: "P-BOWTIE", Name: "bad geom", AreaGeom: bowtie,
	}, actor)
	require.Error(t, err)
	var ve *ErrValidation
	require.True(t, errors.As(err, &ve), "expected *ErrValidation, got %T", err)
	require.NotEmpty(t, ve.Details)
	assert.Equal(t, "area_geom", ve.Details[0].Field)
}

func TestCreateProject_DuplicateCode(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	actor := adminID(t, pool)

	_, err := svc.CreateProject(ctx, oapi.ProjectCreate{Code: "DUP", Name: "first"}, actor)
	require.NoError(t, err)

	_, err = svc.CreateProject(ctx, oapi.ProjectCreate{Code: "DUP", Name: "second"}, actor)
	require.ErrorIs(t, err, ErrCodeTaken)
}

func TestCreateProject_InvalidStatus(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	actor := adminID(t, pool)

	_, err := svc.CreateProject(ctx, oapi.ProjectCreate{
		Code: "P-BADSTATUS", Name: "x", Status: ptr(oapi.ProjectStatus("OPEN")),
	}, actor)
	require.Error(t, err)
	var ve *ErrValidation
	require.True(t, errors.As(err, &ve))
}

func TestListProjects_FilterAndPaginate(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	actor := adminID(t, pool)

	// Seed: 3 ACTIVE + 1 PAUSED; codes share an "ALPHA" prefix for two.
	_, err := svc.CreateProject(ctx, oapi.ProjectCreate{Code: "ALPHA-1", Name: "Alpha road"}, actor)
	require.NoError(t, err)
	_, err = svc.CreateProject(ctx, oapi.ProjectCreate{Code: "ALPHA-2", Name: "Alpha bridge"}, actor)
	require.NoError(t, err)
	_, err = svc.CreateProject(ctx, oapi.ProjectCreate{Code: "BETA-1", Name: "Beta tunnel"}, actor)
	require.NoError(t, err)
	_, err = svc.CreateProject(ctx, oapi.ProjectCreate{
		Code: "GAMMA-1", Name: "Gamma paused", Status: ptr(oapi.PAUSED),
	}, actor)
	require.NoError(t, err)

	// No filter -> all 4.
	items, total, err := svc.ListProjects(ctx, nil, nil, 20, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 4, total)
	assert.Len(t, items, 4)

	// Filter by code/name substring "Alpha".
	q := "Alpha"
	items, total, err = svc.ListProjects(ctx, &q, nil, 20, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	assert.Len(t, items, 2)

	// Filter by status PAUSED.
	st := "PAUSED"
	items, total, err = svc.ListProjects(ctx, nil, &st, 20, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, "GAMMA-1", items[0].Code)

	// Pagination: total stays 4, page is bounded.
	items, total, err = svc.ListProjects(ctx, nil, nil, 2, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 4, total)
	assert.Len(t, items, 2)
	items, total, err = svc.ListProjects(ctx, nil, nil, 2, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 4, total)
	assert.Len(t, items, 2)
}

func TestUpdateProject_Partial(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	actor := adminID(t, pool)

	p, err := svc.CreateProject(ctx, oapi.ProjectCreate{Code: "U-1", Name: "before"}, actor)
	require.NoError(t, err)

	// Update only name + status; code stays.
	updated, err := svc.UpdateProject(ctx, p.Id, oapi.ProjectUpdate{
		Name: ptr("after"), Status: ptr(oapi.PAUSED),
	}, actor)
	require.NoError(t, err)
	assert.Equal(t, "after", updated.Name)
	assert.Equal(t, oapi.PAUSED, updated.Status)
	assert.Equal(t, "U-1", updated.Code)
}

func TestUpdateProject_NotFound(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.UpdateProject(context.Background(), 999999, oapi.ProjectUpdate{Name: ptr("x")}, 0)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestCustomFields_ValidationGated(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	actor := adminID(t, pool)

	// Before any project_field dict is configured, custom_fields validation is
	// skipped — unknown keys are accepted.
	_, err := svc.CreateProject(ctx, oapi.ProjectCreate{
		Code: "CF-OPEN", Name: "no dict",
		CustomFields: ptr(map[string]interface{}{"anything": "goes"}),
	}, actor)
	require.NoError(t, err, "no project_field dict -> validation skipped")

	// Configure a project_field dict_type with two active items: region, owner.
	seedProjectFieldDict(t, pool, []string{"region", "owner"})

	// Known keys pass.
	p, err := svc.CreateProject(ctx, oapi.ProjectCreate{
		Code: "CF-OK", Name: "known keys",
		CustomFields: ptr(map[string]interface{}{"region": "south", "owner": "alice"}),
	}, actor)
	require.NoError(t, err)
	require.NotNil(t, p.CustomFields)
	assert.Equal(t, "south", (*p.CustomFields)["region"])

	// Unknown key -> VALIDATION_FAILED with a field detail.
	_, err = svc.CreateProject(ctx, oapi.ProjectCreate{
		Code: "CF-BAD", Name: "unknown key",
		CustomFields: ptr(map[string]interface{}{"region": "south", "bogus": 1}),
	}, actor)
	require.Error(t, err)
	var ve *ErrValidation
	require.True(t, errors.As(err, &ve))
	require.Len(t, ve.Details, 1)
	assert.Equal(t, "custom_fields.bogus", ve.Details[0].Field)
}

// seedProjectFieldDict ensures the project_field dict_type exists and adds active
// items. Migration 000003 already seeds an EMPTY project_field type, so upsert
// (ON CONFLICT) to fetch its id whether or not the row pre-exists; the empty
// baseline keeps validation disabled until items are added here.
func seedProjectFieldDict(t *testing.T, pool *db.Pool, codes []string) {
	t.Helper()
	ctx := context.Background()
	var typeID int64
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO dict_type (code, name, scope, is_active)
		VALUES ('project_field', '项目字段', 'project_field', TRUE)
		ON CONFLICT (code) DO UPDATE SET is_active = TRUE
		RETURNING id`).Scan(&typeID))
	for _, code := range codes {
		_, err := pool.Exec(ctx, `
			INSERT INTO dict_item (dict_type_id, code, label, is_active)
			VALUES ($1, $2, $2, TRUE)`, typeID, code)
		require.NoError(t, err)
	}
}

func TestCreateTask_AndFilters(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	actor := adminID(t, pool)

	proj, err := svc.CreateProject(ctx, oapi.ProjectCreate{Code: "T-PROJ", Name: "task host"}, actor)
	require.NoError(t, err)

	plan := &oapi.GeoJSONMultiLineString{
		Type:        oapi.MultiLineString,
		Coordinates: [][][]float32{{{113.0, 23.0}, {113.01, 23.0}}},
	}
	tk, err := svc.CreateTask(ctx, oapi.InspectionTaskCreate{
		ProjectId: proj.Id, Title: "survey segment A",
		AssigneeId: &actor, PlanGeom: plan,
	}, actor)
	require.NoError(t, err)
	assert.Equal(t, oapi.TaskStatusPENDING, tk.Status) // defaulted
	require.NotNil(t, tk.PlanGeom)
	assert.Equal(t, oapi.MultiLineString, tk.PlanGeom.Type)
	require.NotNil(t, tk.AssigneeId)
	assert.Equal(t, actor, *tk.AssigneeId)

	// Second task, different status + no assignee.
	_, err = svc.CreateTask(ctx, oapi.InspectionTaskCreate{
		ProjectId: proj.Id, Title: "survey segment B",
		Status: ptr(oapi.TaskStatusINPROGRESS),
	}, actor)
	require.NoError(t, err)

	// Filter by project_id.
	items, total, err := svc.ListTasks(ctx, &proj.Id, nil, nil, 20, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	assert.Len(t, items, 2)

	// Filter by status.
	st := "IN_PROGRESS"
	items, total, err = svc.ListTasks(ctx, &proj.Id, &st, nil, 20, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, "survey segment B", items[0].Title)

	// Filter by assignee.
	items, total, err = svc.ListTasks(ctx, nil, nil, &actor, 20, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, "survey segment A", items[0].Title)
}

func TestCreateTask_RejectsOpenStatus(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	actor := adminID(t, pool)

	proj, err := svc.CreateProject(ctx, oapi.ProjectCreate{Code: "T-OPEN", Name: "x"}, actor)
	require.NoError(t, err)

	// OPEN is NOT a valid task_status -> rejected at the service layer.
	_, err = svc.CreateTask(ctx, oapi.InspectionTaskCreate{
		ProjectId: proj.Id, Title: "bad", Status: ptr(oapi.TaskStatus("OPEN")),
	}, actor)
	require.Error(t, err)
	var ve *ErrValidation
	require.True(t, errors.As(err, &ve))
}

func TestCreateTask_UnknownProject(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	actor := adminID(t, pool)

	_, err := svc.CreateTask(ctx, oapi.InspectionTaskCreate{
		ProjectId: 987654, Title: "orphan",
	}, actor)
	require.Error(t, err)
	var ve *ErrValidation
	require.True(t, errors.As(err, &ve))
	assert.Equal(t, "project_id", ve.Details[0].Field)
}

func TestUpdateTask_Partial(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	actor := adminID(t, pool)

	proj, err := svc.CreateProject(ctx, oapi.ProjectCreate{Code: "TU-PROJ", Name: "host"}, actor)
	require.NoError(t, err)
	tk, err := svc.CreateTask(ctx, oapi.InspectionTaskCreate{ProjectId: proj.Id, Title: "before"}, actor)
	require.NoError(t, err)

	updated, err := svc.UpdateTask(ctx, tk.Id, oapi.InspectionTaskUpdate{
		Title: ptr("after"), Status: ptr(oapi.TaskStatusCOMPLETED),
	}, actor)
	require.NoError(t, err)
	assert.Equal(t, "after", updated.Title)
	assert.Equal(t, oapi.TaskStatusCOMPLETED, updated.Status)
}

func TestDeleteProject_WithActiveInspection_Conflict(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	actor := adminID(t, pool)

	proj, err := svc.CreateProject(ctx, oapi.ProjectCreate{Code: "DEL-INSP", Name: "has inspection"}, actor)
	require.NoError(t, err)

	// Insert a live inspection referencing the project (raw — inspection domain
	// is a sibling package; this test only needs the FK reference to exist).
	_, err = pool.Exec(ctx, `
		INSERT INTO inspections (project_id, inspector_id, started_at)
		VALUES ($1, $2, now())`, proj.Id, actor)
	require.NoError(t, err)

	// Soft delete must be refused to avoid orphaning the inspection.
	err = svc.DeleteProject(ctx, proj.Id, actor)
	require.ErrorIs(t, err, ErrProjectInUse)

	// The project is still live.
	got, err := svc.GetProject(ctx, proj.Id)
	require.NoError(t, err)
	assert.Equal(t, proj.Id, got.Id)
}

func TestDeleteProject_WithActiveTask_Conflict(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	actor := adminID(t, pool)

	proj, err := svc.CreateProject(ctx, oapi.ProjectCreate{Code: "DEL-TASK", Name: "has task"}, actor)
	require.NoError(t, err)
	_, err = svc.CreateTask(ctx, oapi.InspectionTaskCreate{ProjectId: proj.Id, Title: "live task"}, actor)
	require.NoError(t, err)

	err = svc.DeleteProject(ctx, proj.Id, actor)
	require.ErrorIs(t, err, ErrProjectInUse)
}

func TestDeleteProject_SoftDeleteThen404(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	actor := adminID(t, pool)

	proj, err := svc.CreateProject(ctx, oapi.ProjectCreate{Code: "DEL-OK", Name: "deletable"}, actor)
	require.NoError(t, err)

	require.NoError(t, svc.DeleteProject(ctx, proj.Id, actor))

	// Subsequent reads 404.
	_, err = svc.GetProject(ctx, proj.Id)
	require.ErrorIs(t, err, ErrNotFound)

	// Re-deleting a deleted project also 404s.
	err = svc.DeleteProject(ctx, proj.Id, actor)
	require.ErrorIs(t, err, ErrNotFound)

	// Soft-deleted rows are excluded from list.
	_, total, err := svc.ListProjects(ctx, ptr("DEL-OK"), nil, 20, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 0, total)

	// The same code can be reused once the prior project is soft-deleted
	// (uq_projects_code_active is partial on deleted_at IS NULL).
	_, err = svc.CreateProject(ctx, oapi.ProjectCreate{Code: "DEL-OK", Name: "reused"}, actor)
	require.NoError(t, err)
}

func TestDeleteTask_SoftDeleteThen404(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	actor := adminID(t, pool)

	proj, err := svc.CreateProject(ctx, oapi.ProjectCreate{Code: "DT-PROJ", Name: "host"}, actor)
	require.NoError(t, err)
	tk, err := svc.CreateTask(ctx, oapi.InspectionTaskCreate{ProjectId: proj.Id, Title: "to delete"}, actor)
	require.NoError(t, err)

	require.NoError(t, svc.DeleteTask(ctx, tk.Id, actor))

	_, err = svc.GetTask(ctx, tk.Id)
	require.ErrorIs(t, err, ErrNotFound)

	err = svc.DeleteTask(ctx, tk.Id, actor)
	require.ErrorIs(t, err, ErrNotFound)
}
