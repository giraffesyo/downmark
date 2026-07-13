// Package xlsx converts Excel workbooks to Markdown for the downmark
// engine: each sheet becomes a heading plus a table.
package xlsx

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/internal/mdutil"
)

// New returns the XLSX converter.
func New() downmark.Converter { return converter{} }

// Register adds the XLSX converter to e at the standard priority.
func Register(e *downmark.Engine) { e.Register(New(), downmark.PrioritySpecific) }

type converter struct{}

func (converter) Name() string { return "xlsx" }

func (converter) Accepts(info downmark.StreamInfo) bool {
	return info.Matches(
		[]string{".xlsx", ".xlsm"},
		[]string{"application/vnd.openxmlformats-officedocument.spreadsheetml"},
	)
}

func (converter) Convert(ctx context.Context, input io.ReadSeeker, _ downmark.StreamInfo) (*downmark.Result, error) {
	wb, err := excelize.OpenReader(input)
	if err != nil {
		return nil, fmt.Errorf("xlsx: %w", err)
	}
	defer func() { _ = wb.Close() }() // read-only workbook

	var b strings.Builder
	for i, sheet := range wb.GetSheetList() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "## %s\n\n", sheet)
		rows, err := wb.GetRows(sheet)
		if err != nil {
			return nil, fmt.Errorf("xlsx: sheet %q: %w", sheet, err)
		}
		rows = normalizeGrid(rows)
		if len(rows) == 0 {
			continue
		}
		b.WriteString(mdutil.Table(rows))
	}
	return &downmark.Result{Markdown: b.String()}, nil
}

// normalizeGrid trims trailing all-empty rows and columns, then pads ragged
// rows (excelize omits trailing empty cells) to a uniform width.
func normalizeGrid(rows [][]string) [][]string {
	lastRow, width := -1, 0
	for i, row := range rows {
		for j := len(row) - 1; j >= 0; j-- {
			if strings.TrimSpace(row[j]) != "" {
				lastRow = i
				width = max(width, j+1)
				break
			}
		}
	}
	if lastRow < 0 {
		return nil
	}
	rows = rows[:lastRow+1]
	for i, row := range rows {
		if len(row) > width {
			row = row[:width]
		}
		for len(row) < width {
			row = append(row, "")
		}
		rows[i] = row
	}
	return rows
}
