package export

import (
	"context"
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"
)

// inspectionRecordsCols is the INSPECTION_RECORDS header (Chinese labels — CJK).
var inspectionRecordsCols = []string{
	"巡查ID", "项目", "巡查人", "状态",
	"开始时间", "结束时间", "时长(秒)", "里程(公里)", "轨迹点数",
}

// buildInspectionRecords writes the 巡查记录 sheet: one row per inspection joined
// to its project and inspector, with start/end rendered in Asia/Shanghai, mileage
// in km and the inspection status. Rows are paginated so a large dataset never
// loads at once; progress advances as pages are written. Returns the row count.
//
// params: {project_id, inspector_id, from, to, status} — status is an
// inspection_status enum string (IN_PROGRESS|FINISHED|ABANDONED).
func (w *Worker) buildInspectionRecords(ctx context.Context, f *excelize.File, job *jobRow, p exportParams) (int, error) {
	sw, err := newSheet(f, "巡查记录")
	if err != nil {
		return 0, err
	}
	if err := sw.writeHeader(inspectionRecordsCols); err != nil {
		return 0, err
	}

	where, args := inspectionExportWhere(p)
	total, err := w.countRows(ctx, "inspections i", where, args)
	if err != nil {
		return 0, fmt.Errorf("count inspections: %w", err)
	}
	if err := w.svc.setTotalRows(ctx, job.ID, total); err != nil {
		return 0, err
	}

	const pageSize = 500
	processed := 0
	for offset := 0; ; offset += pageSize {
		rows, err := w.pool.Query(ctx, `
			SELECT i.id, pr.name,
			       COALESCE(NULLIF(u.display_name, ''), u.username),
			       i.status::text, i.started_at, i.ended_at,
			       i.duration_seconds, i.mileage_meters, i.point_count
			FROM inspections i
			JOIN projects pr ON pr.id = i.project_id
			JOIN users u     ON u.id = i.inspector_id
			`+where+`
			ORDER BY i.started_at DESC, i.id DESC
			LIMIT $`+fmt.Sprint(len(args)+1)+`::int OFFSET $`+fmt.Sprint(len(args)+2)+`::int`,
			append(append([]any{}, args...), pageSize, offset)...)
		if err != nil {
			return processed, fmt.Errorf("query inspections: %w", err)
		}

		pageCount := 0
		for rows.Next() {
			var (
				id         int64
				project    string
				inspector  string
				status     string
				startedAt  time.Time
				endedAt    *time.Time
				durationS  int64
				mileageM   float64
				pointCount int
			)
			if err := rows.Scan(&id, &project, &inspector, &status, &startedAt,
				&endedAt, &durationS, &mileageM, &pointCount); err != nil {
				rows.Close()
				return processed, fmt.Errorf("scan inspection: %w", err)
			}
			if err := sw.writeRow([]any{
				id, project, inspector, status,
				shanghaiStringV(startedAt), shanghaiString(endedAt),
				durationS, metersToKm(mileageM), pointCount,
			}); err != nil {
				rows.Close()
				return processed, err
			}
			pageCount++
			processed++
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return processed, fmt.Errorf("iterate inspections: %w", err)
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

// inspectionExportWhere builds the WHERE clause + args for INSPECTION_RECORDS.
// Bind order is dynamic; the caller appends LIMIT/OFFSET after these args.
func inspectionExportWhere(p exportParams) (string, []any) {
	args := []any{}
	clause := "WHERE i.deleted_at IS NULL"

	if p.ProjectID != nil {
		args = append(args, *p.ProjectID)
		clause += fmt.Sprintf(" AND i.project_id = $%d", len(args))
	}
	if p.InspectorID != nil {
		args = append(args, *p.InspectorID)
		clause += fmt.Sprintf(" AND i.inspector_id = $%d", len(args))
	}
	if from := timeBound(p.From); from != nil {
		args = append(args, *from)
		clause += fmt.Sprintf(" AND i.started_at >= $%d", len(args))
	}
	if to := timeBound(p.To); to != nil {
		args = append(args, *to)
		clause += fmt.Sprintf(" AND i.started_at < $%d", len(args))
	}
	if st := p.statusAsString(); st != nil {
		args = append(args, *st)
		clause += fmt.Sprintf(" AND i.status = $%d::inspection_status", len(args))
	}
	return clause, args
}
