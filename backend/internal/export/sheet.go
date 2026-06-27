package export

import (
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"
)

// sheetWriter wraps an excelize file + sheet with a CJK cell style and helpers to
// write a header row and stream data rows, applying the CJK font to every cell so
// Chinese text renders (not tofu blocks) once the worker image's font is present.
type sheetWriter struct {
	f         *excelize.File
	sheet     string
	cjkStyle  int
	headStyle int
	rowIdx    int // 1-based; 0 means nothing written yet
}

// newSheet renames the default first sheet (or creates a new one) and prepares
// the CJK styles. The first sheet of a fresh excelize file is "Sheet1".
func newSheet(f *excelize.File, name string) (*sheetWriter, error) {
	// Reuse the default sheet for the first builder; create otherwise.
	def := f.GetSheetName(0)
	if def == "Sheet1" {
		if err := f.SetSheetName("Sheet1", name); err != nil {
			return nil, fmt.Errorf("rename sheet: %w", err)
		}
	} else {
		if _, err := f.NewSheet(name); err != nil {
			return nil, fmt.Errorf("new sheet: %w", err)
		}
	}

	cjk, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Family: cjkFontName, Size: 11},
	})
	if err != nil {
		return nil, fmt.Errorf("cjk style: %w", err)
	}
	head, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Family: cjkFontName, Size: 11, Bold: true},
	})
	if err != nil {
		return nil, fmt.Errorf("head style: %w", err)
	}
	return &sheetWriter{f: f, sheet: name, cjkStyle: cjk, headStyle: head}, nil
}

// writeHeader writes the bold header row and applies the CJK head style across it.
func (s *sheetWriter) writeHeader(cols []string) error {
	s.rowIdx = 1
	for i, label := range cols {
		cell, err := excelize.CoordinatesToCellName(i+1, s.rowIdx)
		if err != nil {
			return err
		}
		if err := s.f.SetCellValue(s.sheet, cell, label); err != nil {
			return err
		}
	}
	return s.styleRow(s.rowIdx, len(cols), s.headStyle)
}

// writeRow appends one data row of arbitrary cell values and applies the CJK body
// style across it. Values are written as-is (strings, numbers, time.Time).
func (s *sheetWriter) writeRow(values []any) error {
	s.rowIdx++
	for i, v := range values {
		cell, err := excelize.CoordinatesToCellName(i+1, s.rowIdx)
		if err != nil {
			return err
		}
		if err := s.f.SetCellValue(s.sheet, cell, v); err != nil {
			return err
		}
	}
	return s.styleRow(s.rowIdx, len(values), s.cjkStyle)
}

// styleRow applies a style across [1..n] columns of a given 1-based row.
func (s *sheetWriter) styleRow(row, n, styleID int) error {
	if n == 0 {
		return nil
	}
	first, err := excelize.CoordinatesToCellName(1, row)
	if err != nil {
		return err
	}
	last, err := excelize.CoordinatesToCellName(n, row)
	if err != nil {
		return err
	}
	return s.f.SetCellStyle(s.sheet, first, last, styleID)
}

// shanghaiString renders a UTC timestamp in Asia/Shanghai as "2006-01-02 15:04:05".
// A nil time yields an empty string.
func shanghaiString(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.In(shanghai).Format("2006-01-02 15:04:05")
}

// shanghaiStringV is shanghaiString for a non-pointer timestamp.
func shanghaiStringV(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(shanghai).Format("2006-01-02 15:04:05")
}

// metersToKm converts mileage meters to kilometers rounded to 3 decimals.
func metersToKm(m float64) float64 {
	return float64(int64(m/1000*1000+0.5)) / 1000
}
