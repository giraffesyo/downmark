// Package xlsx converts Excel workbooks to Markdown for the downmark
// engine: each sheet becomes a heading plus a table.
package xlsx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/internal/limitbuf"
	"github.com/giraffesyo/downmark/internal/mdutil"
	"github.com/giraffesyo/downmark/internal/ooxml"
	"github.com/giraffesyo/downmark/internal/readerat"
)

const (
	maxWorksheetRows  = 100_000
	maxWorksheetCells = 1_000_000
)

// New returns the XLSX converter.
func New() downmark.Converter { return converter{} }

// Register adds the XLSX converter to e at the standard priority.
func Register(e *downmark.Engine) { e.Register(New(), downmark.PrioritySpecific) }

type converter struct{}

func (converter) Name() string { return "xlsx" }

func (converter) InputLimit() int64 { return ooxml.MaxArchiveBytes }

func (converter) Accepts(info downmark.StreamInfo) bool {
	return info.Matches(
		[]string{".xlsx", ".xlsm"},
		[]string{"application/vnd.openxmlformats-officedocument.spreadsheetml"},
	)
}

func (converter) Convert(ctx context.Context, input io.ReadSeeker, _ downmark.StreamInfo) (*downmark.Result, error) {
	ra, size, err := readerat.FromLimit(input, ooxml.MaxArchiveBytes)
	if errors.Is(err, readerat.ErrTooLarge) {
		return nil, fmt.Errorf("%w: xlsx archive is %d bytes; limit is %d", downmark.ErrInputTooLarge, size, ooxml.MaxArchiveBytes)
	}
	if err != nil {
		return nil, fmt.Errorf("xlsx: %w", err)
	}
	if _, err := ooxml.Open(ctx, ra, size); err != nil {
		return nil, fmt.Errorf("xlsx: %w", err)
	}
	wb, err := excelize.OpenReader(io.NewSectionReader(ra, 0, size), excelize.Options{
		UnzipSizeLimit:    ooxml.MaxTotalBytes,
		UnzipXMLSizeLimit: ooxml.MaxPartBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("xlsx: %w", err)
	}
	defer func() { _ = wb.Close() }() // read-only workbook

	resultLimit, _ := downmark.ResultLimit(ctx)
	b := limitbuf.New(resultLimit)
	for i, sheet := range wb.GetSheetList() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if i > 0 {
			if _, err := b.WriteString("\n"); err != nil {
				return nil, xlsxResultLimitError(resultLimit)
			}
		}
		if _, err := fmt.Fprintf(b, "## %s\n\n", sheet); err != nil {
			return nil, xlsxResultLimitError(resultLimit)
		}
		rows, err := readRows(ctx, wb, sheet, resultLimit)
		if err != nil {
			return nil, fmt.Errorf("xlsx: sheet %q: %w", sheet, err)
		}
		rows = normalizeGrid(rows)
		if len(rows) == 0 {
			continue
		}
		if err := mdutil.WriteTable(b, rows); err != nil {
			if errors.Is(err, limitbuf.ErrTooLarge) {
				return nil, xlsxResultLimitError(resultLimit)
			}
			return nil, err
		}
	}
	return &downmark.Result{Markdown: b.String()}, nil
}

func readRows(ctx context.Context, wb *excelize.File, sheet string, resultLimit int) (rows [][]string, err error) {
	iter, err := wb.Rows(sheet)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := iter.Close(); err == nil {
			err = closeErr
		}
	}()

	rowBudget, cellBudget := maxWorksheetRows, maxWorksheetCells
	if resultLimit > 0 {
		rowBudget = min(rowBudget, max(1, resultLimit/4))
		cellBudget = min(cellBudget, max(1, resultLimit/4))
	}
	cells, rawBytes := 0, 0
	for iter.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(rows) >= rowBudget {
			return nil, errors.New("worksheet row budget exceeded")
		}
		row, err := iter.Columns()
		if err != nil {
			return nil, err
		}
		cells += len(row)
		for _, cell := range row {
			rawBytes += len(cell)
		}
		if cells > cellBudget || (resultLimit > 0 && rawBytes > resultLimit) {
			return nil, errors.New("worksheet cell budget exceeded")
		}
		rows = append(rows, row)
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}
	return rows, nil
}

func xlsxResultLimitError(limit int) error {
	return fmt.Errorf("%w: xlsx result exceeds %d-byte limit", downmark.ErrResultTooLarge, limit)
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
