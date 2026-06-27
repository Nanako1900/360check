// Package stats implements the D2 /stats/overview aggregation: counts grouped by
// problem type/status (with dict labels+colors, INCLUDING retired items, so
// historical data stays readable), by inspector and by project, plus the
// inspection/problem totals and the mileage/duration roll-ups from inspections.
//
// Every aggregate is hand-written pgx against the shared pool (sqlc cannot model
// these grouped joins cleanly, and the same Overview() is reused by the
// PROJECT_STATS export worker). All five dimensions honor the same filter set
// (project_id, from, to, inspector_id); time bounds apply to inspections.started_at
// and problems.captured_at respectively so the two roll-ups stay comparable.
package stats

import (
	"context"
	"fmt"
	"time"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
)

// Service owns the read-only statistics aggregation over inspections + problems.
type Service struct {
	pool *db.Pool
}

// NewService wires the stats service onto the shared pool.
func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

// Filter holds the optional /stats/overview filters. All nil = whole dataset.
// From is an inclusive lower bound, To an exclusive upper bound (UTC); they apply
// to inspections.started_at and problems.captured_at.
type Filter struct {
	ProjectID   *int64
	InspectorID *int64
	From        *time.Time
	To          *time.Time
}

// Overview computes the full D2 shape for the filter. It is the single source of
// truth for both the web dashboard and the PROJECT_STATS export (which calls it
// per project). A nil/zero filter aggregates everything.
func (s *Service) Overview(ctx context.Context, f Filter) (*oapi.StatsOverview, error) {
	out := &oapi.StatsOverview{
		CountsByType:      []oapi.CountBucket{},
		CountsByStatus:    []oapi.CountBucket{},
		CountsByInspector: []oapi.CountBucket{},
		CountsByProject:   []oapi.CountBucket{},
	}

	var err error
	// problems-derived dimensions share the captured_at time window.
	if out.CountsByType, err = s.countByDict(ctx, f, "type_item_id"); err != nil {
		return nil, fmt.Errorf("stats: counts_by_type: %w", err)
	}
	if out.CountsByStatus, err = s.countByDict(ctx, f, "status_item_id"); err != nil {
		return nil, fmt.Errorf("stats: counts_by_status: %w", err)
	}
	if out.CountsByInspector, err = s.countByInspector(ctx, f); err != nil {
		return nil, fmt.Errorf("stats: counts_by_inspector: %w", err)
	}
	if out.CountsByProject, err = s.countByProject(ctx, f); err != nil {
		return nil, fmt.Errorf("stats: counts_by_project: %w", err)
	}
	if err = s.totals(ctx, f, out); err != nil {
		return nil, fmt.Errorf("stats: totals: %w", err)
	}
	return out, nil
}

// problemWhere builds the shared WHERE clause for problem-derived aggregates.
// Bind order is fixed: $1=project_id, $2=inspector_id, $3=from, $4=to. The caller
// may append more positional args after these four.
func problemWhere(alias string) string {
	return `WHERE ` + alias + `deleted_at IS NULL
		AND ($1::bigint      IS NULL OR ` + alias + `project_id = $1)
		AND ($2::bigint      IS NULL OR ` + alias + `inspector_id = $2)
		AND ($3::timestamptz IS NULL OR ` + alias + `captured_at >= $3)
		AND ($4::timestamptz IS NULL OR ` + alias + `captured_at <  $4)`
}

// inspectionWhere is the analogous clause for inspection-derived aggregates. The
// time window applies to started_at (a session's begin time).
func inspectionWhere(alias string) string {
	return `WHERE ` + alias + `deleted_at IS NULL
		AND ($1::bigint      IS NULL OR ` + alias + `project_id = $1)
		AND ($2::bigint      IS NULL OR ` + alias + `inspector_id = $2)
		AND ($3::timestamptz IS NULL OR ` + alias + `started_at >= $3)
		AND ($4::timestamptz IS NULL OR ` + alias + `started_at <  $4)`
}

// filterArgs returns the four positional args matching problemWhere/inspectionWhere.
func filterArgs(f Filter) []any {
	return []any{f.ProjectID, f.InspectorID, f.From, f.To}
}

// countByDict groups non-deleted problems by a dict_item soft-FK column
// (type_item_id or status_item_id) and joins dict_item for label+color. The join
// is a plain (non-filtered) LEFT JOIN, so RETIRED items (is_active=false) still
// resolve a label/color — required for historical readability. Rows with a NULL
// dimension are skipped (no item to label).
func (s *Service) countByDict(ctx context.Context, f Filter, col string) ([]oapi.CountBucket, error) {
	// col is a fixed internal literal (never user input), so interpolation is safe.
	sql := `
		SELECT p.` + col + ` AS item_id, di.label, di.color, count(*) AS cnt
		FROM problems p
		JOIN dict_item di ON di.id = p.` + col + `
		` + problemWhere("p.") + `
		GROUP BY p.` + col + `, di.label, di.color
		ORDER BY cnt DESC, item_id ASC`

	rows, err := s.pool.Query(ctx, sql, filterArgs(f)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]oapi.CountBucket, 0)
	for rows.Next() {
		var (
			itemID int64
			label  string
			color  *string
			count  int64
		)
		if err := rows.Scan(&itemID, &label, &color, &count); err != nil {
			return nil, err
		}
		id := itemID
		out = append(out, oapi.CountBucket{ItemId: &id, Label: label, Color: color, Count: count})
	}
	return out, rows.Err()
}

// countByInspector groups problems by inspector_id, labeling each with the user's
// display_name (falling back to username when display_name is empty).
func (s *Service) countByInspector(ctx context.Context, f Filter) ([]oapi.CountBucket, error) {
	sql := `
		SELECT p.inspector_id,
		       COALESCE(NULLIF(u.display_name, ''), u.username) AS label,
		       count(*) AS cnt
		FROM problems p
		JOIN users u ON u.id = p.inspector_id
		` + problemWhere("p.") + `
		GROUP BY p.inspector_id, label
		ORDER BY cnt DESC, p.inspector_id ASC`

	rows, err := s.pool.Query(ctx, sql, filterArgs(f)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]oapi.CountBucket, 0)
	for rows.Next() {
		var (
			inspectorID int64
			label       string
			count       int64
		)
		if err := rows.Scan(&inspectorID, &label, &count); err != nil {
			return nil, err
		}
		id := inspectorID
		out = append(out, oapi.CountBucket{InspectorId: &id, Label: label, Count: count})
	}
	return out, rows.Err()
}

// countByProject groups problems by project_id, labeling each with projects.name.
func (s *Service) countByProject(ctx context.Context, f Filter) ([]oapi.CountBucket, error) {
	sql := `
		SELECT p.project_id, pr.name, count(*) AS cnt
		FROM problems p
		JOIN projects pr ON pr.id = p.project_id
		` + problemWhere("p.") + `
		GROUP BY p.project_id, pr.name
		ORDER BY cnt DESC, p.project_id ASC`

	rows, err := s.pool.Query(ctx, sql, filterArgs(f)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]oapi.CountBucket, 0)
	for rows.Next() {
		var (
			projectID int64
			label     string
			count     int64
		)
		if err := rows.Scan(&projectID, &label, &count); err != nil {
			return nil, err
		}
		id := projectID
		out = append(out, oapi.CountBucket{ProjectId: &id, Label: label, Count: count})
	}
	return out, rows.Err()
}

// totals fills inspection_count, problem_count and the mileage/duration roll-ups.
// The inspection aggregate (count + SUM mileage + SUM/AVG duration) is one scan
// over inspections; problem_count is a separate count over problems. Empty sets
// yield zeros (COALESCE), never NULL, so the JSON stays numeric.
func (s *Service) totals(ctx context.Context, f Filter, out *oapi.StatsOverview) error {
	args := filterArgs(f)

	var (
		inspectionCount int64
		totalMileage    float64
		totalDuration   int64
		avgDuration     float64
	)
	err := s.pool.QueryRow(ctx, `
		SELECT count(*),
		       COALESCE(SUM(mileage_meters), 0),
		       COALESCE(SUM(duration_seconds), 0),
		       COALESCE(AVG(duration_seconds), 0)
		FROM inspections i `+inspectionWhere("i.")+``, args...).
		Scan(&inspectionCount, &totalMileage, &totalDuration, &avgDuration)
	if err != nil {
		return fmt.Errorf("inspection totals: %w", err)
	}

	var problemCount int64
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM problems p `+problemWhere("p."), args...).
		Scan(&problemCount); err != nil {
		return fmt.Errorf("problem count: %w", err)
	}

	out.InspectionCount = inspectionCount
	out.ProblemCount = problemCount
	out.TotalMileageMeters = totalMileage
	out.TotalDurationSeconds = totalDuration
	out.AvgDurationSeconds = avgDuration
	return nil
}

// ProjectRow is one project's aggregated stats, used by the PROJECT_STATS export
// to enumerate which projects to roll up. It carries the project identity plus a
// per-project Overview computed by reusing Service.Overview.
type ProjectRow struct {
	ProjectID int64
	Name      string
	Overview  *oapi.StatsOverview
}

// ProjectsForExport returns one ProjectRow per project that has any inspection OR
// problem inside the (project-less) filter window, each with a per-project
// Overview. When f.ProjectID is set, only that project is returned. This is the
// PROJECT_STATS data source — it reuses Overview() so the Excel always matches
// /stats/overview exactly.
func (s *Service) ProjectsForExport(ctx context.Context, f Filter) ([]ProjectRow, error) {
	ids, names, err := s.projectsInWindow(ctx, f)
	if err != nil {
		return nil, err
	}
	rows := make([]ProjectRow, 0, len(ids))
	for i, id := range ids {
		pf := f
		pid := id
		pf.ProjectID = &pid
		ov, err := s.Overview(ctx, pf)
		if err != nil {
			return nil, fmt.Errorf("stats: project %d overview: %w", id, err)
		}
		rows = append(rows, ProjectRow{ProjectID: id, Name: names[i], Overview: ov})
	}
	return rows, nil
}

// projectsInWindow returns the distinct project ids (and names) that have at least
// one inspection or problem matching the filter window, ordered by name. Honors an
// explicit f.ProjectID (single-project export).
func (s *Service) projectsInWindow(ctx context.Context, f Filter) ([]int64, []string, error) {
	args := filterArgs(f)
	// Projects appearing in either inspections or problems within the window.
	sql := `
		SELECT pr.id, pr.name
		FROM projects pr
		WHERE pr.deleted_at IS NULL
		  AND ($1::bigint IS NULL OR pr.id = $1)
		  AND (
		    EXISTS (SELECT 1 FROM inspections i ` + inspectionWhere("i.") + ` AND i.project_id = pr.id)
		    OR
		    EXISTS (SELECT 1 FROM problems p ` + problemWhere("p.") + ` AND p.project_id = pr.id)
		  )
		ORDER BY pr.name ASC, pr.id ASC`

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("stats: projects in window: %w", err)
	}
	defer rows.Close()

	var (
		ids   []int64
		names []string
	)
	for rows.Next() {
		var (
			id   int64
			name string
		)
		if err := rows.Scan(&id, &name); err != nil {
			return nil, nil, err
		}
		ids = append(ids, id)
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return ids, names, nil
}
