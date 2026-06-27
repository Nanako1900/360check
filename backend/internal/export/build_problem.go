package export

import (
	"context"
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"
)

// problemListCols is the PROBLEM_LIST header (Chinese labels — CJK).
var problemListCols = []string{
	"问题ID", "项目", "巡查人", "类型", "状态", "分类",
	"拍摄时间", "描述", "备注",
}

// buildProblemList writes the 问题列表 sheet: one row per problem with its
// type/status/category dict labels (resolved via plain LEFT JOINs so RETIRED dict
// items still label, preserving history), project, inspector, captured_at rendered
// in Asia/Shanghai, and the description/note. Paginated; progress advances per page.
//
// params: {project_id, type, status, category, from, to, inspector_id, inspection_id}
// where type/status/category are dict_item ids and the time window is captured_at.
func (w *Worker) buildProblemList(ctx context.Context, f *excelize.File, job *jobRow, p exportParams) (int, error) {
	sw, err := newSheet(f, "问题列表")
	if err != nil {
		return 0, err
	}
	if err := sw.writeHeader(problemListCols); err != nil {
		return 0, err
	}

	where, args := problemExportWhere(p)
	total, err := w.countRows(ctx, "problems p", where, args)
	if err != nil {
		return 0, fmt.Errorf("count problems: %w", err)
	}
	if err := w.svc.setTotalRows(ctx, job.ID, total); err != nil {
		return 0, err
	}

	const pageSize = 500
	processed := 0
	for offset := 0; ; offset += pageSize {
		rows, err := w.pool.Query(ctx, `
			SELECT p.id, pr.name,
			       COALESCE(NULLIF(u.display_name, ''), u.username),
			       COALESCE(ti.label, ''), COALESCE(si.label, ''), COALESCE(ci.label, ''),
			       p.captured_at, p.description, p.note
			FROM problems p
			JOIN projects pr ON pr.id = p.project_id
			JOIN users u     ON u.id = p.inspector_id
			LEFT JOIN dict_item ti ON ti.id = p.type_item_id
			LEFT JOIN dict_item si ON si.id = p.status_item_id
			LEFT JOIN dict_item ci ON ci.id = p.category_item_id
			`+where+`
			ORDER BY p.captured_at DESC, p.id DESC
			LIMIT $`+fmt.Sprint(len(args)+1)+`::int OFFSET $`+fmt.Sprint(len(args)+2)+`::int`,
			append(append([]any{}, args...), pageSize, offset)...)
		if err != nil {
			return processed, fmt.Errorf("query problems: %w", err)
		}

		pageCount := 0
		for rows.Next() {
			var (
				id          int64
				project     string
				inspector   string
				typeLabel   string
				statusLabel string
				catLabel    string
				capturedAt  time.Time
				description string
				note        string
			)
			if err := rows.Scan(&id, &project, &inspector, &typeLabel, &statusLabel,
				&catLabel, &capturedAt, &description, &note); err != nil {
				rows.Close()
				return processed, fmt.Errorf("scan problem: %w", err)
			}
			if err := sw.writeRow([]any{
				id, project, inspector, typeLabel, statusLabel, catLabel,
				shanghaiStringV(capturedAt), description, note,
			}); err != nil {
				rows.Close()
				return processed, err
			}
			pageCount++
			processed++
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return processed, fmt.Errorf("iterate problems: %w", err)
		}

		if err := w.svc.updateProgress(ctx, job.ID, processed, percent(processed, total)); err != nil {
			return processed, err
		}
		if pageCount < pageSize {
			break
		}
	}
	return processed, nil
}

// problemExportWhere builds the WHERE clause + args for PROBLEM_LIST. status here
// is a problem status_item_id (dict). The caller appends LIMIT/OFFSET after.
func problemExportWhere(p exportParams) (string, []any) {
	args := []any{}
	clause := "WHERE p.deleted_at IS NULL"

	if p.ProjectID != nil {
		args = append(args, *p.ProjectID)
		clause += fmt.Sprintf(" AND p.project_id = $%d", len(args))
	}
	if p.InspectorID != nil {
		args = append(args, *p.InspectorID)
		clause += fmt.Sprintf(" AND p.inspector_id = $%d", len(args))
	}
	if p.InspectionID != nil {
		args = append(args, *p.InspectionID)
		clause += fmt.Sprintf(" AND p.inspection_id = $%d", len(args))
	}
	if p.Type != nil {
		args = append(args, *p.Type)
		clause += fmt.Sprintf(" AND p.type_item_id = $%d", len(args))
	}
	if st := p.statusAsID(); st != nil {
		args = append(args, *st)
		clause += fmt.Sprintf(" AND p.status_item_id = $%d", len(args))
	}
	if p.Category != nil {
		args = append(args, *p.Category)
		clause += fmt.Sprintf(" AND p.category_item_id = $%d", len(args))
	}
	if from := timeBound(p.From); from != nil {
		args = append(args, *from)
		clause += fmt.Sprintf(" AND p.captured_at >= $%d", len(args))
	}
	if to := timeBound(p.To); to != nil {
		args = append(args, *to)
		clause += fmt.Sprintf(" AND p.captured_at < $%d", len(args))
	}
	return clause, args
}
