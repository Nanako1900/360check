package export

import (
	"context"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/stats"
)

// projectStatsCols is the PROJECT_STATS header (Chinese labels — CJK).
var projectStatsCols = []string{
	"项目ID", "项目", "巡查次数", "问题数",
	"总里程(公里)", "总时长(秒)", "平均时长(秒)",
	"按类型", "按状态",
}

// buildProjectStats writes the 项目统计 sheet: one row per project, each carrying
// the per-project D2 overview computed by REUSING stats.Service.Overview — so the
// Excel numbers always equal /stats/overview. The counts_by_type / counts_by_status
// breakdowns are rendered as compact "label×count; ..." strings (CJK labels).
//
// params: {project_id?, from, to, inspector_id?}. When project_id is set only that
// project is rolled up; otherwise every project active in the window.
func (w *Worker) buildProjectStats(ctx context.Context, f *excelize.File, job *jobRow, p exportParams) (int, error) {
	sw, err := newSheet(f, "项目统计")
	if err != nil {
		return 0, err
	}
	if err := sw.writeHeader(projectStatsCols); err != nil {
		return 0, err
	}

	filter := stats.Filter{
		ProjectID:   p.ProjectID,
		InspectorID: p.InspectorID,
		From:        timeBound(p.From),
		To:          timeBound(p.To),
	}
	projectRows, err := w.stats.ProjectsForExport(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("project stats source: %w", err)
	}

	total := len(projectRows)
	if err := w.svc.setTotalRows(ctx, job.ID, total); err != nil {
		return 0, err
	}

	processed := 0
	for _, pr := range projectRows {
		ov := pr.Overview
		if err := sw.writeRow([]any{
			pr.ProjectID, pr.Name,
			ov.InspectionCount, ov.ProblemCount,
			metersToKm(ov.TotalMileageMeters), ov.TotalDurationSeconds, ov.AvgDurationSeconds,
			formatBuckets(ov.CountsByType), formatBuckets(ov.CountsByStatus),
		}); err != nil {
			return processed, err
		}
		processed++
		if err := w.svc.updateProgress(ctx, job.ID, processed, percent(processed, total)); err != nil {
			return processed, err
		}
	}
	return processed, nil
}

// formatBuckets renders a slice of count buckets as "label×count; label×count".
// An empty slice yields "" so the cell is blank rather than a stray separator.
func formatBuckets(buckets []oapi.CountBucket) string {
	if len(buckets) == 0 {
		return ""
	}
	parts := make([]string, 0, len(buckets))
	for _, b := range buckets {
		parts = append(parts, fmt.Sprintf("%s×%d", b.Label, b.Count))
	}
	return strings.Join(parts, "; ")
}
