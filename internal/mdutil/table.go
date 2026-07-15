// Package mdutil holds small shared Markdown emission helpers.
package mdutil

import (
	"io"
	"strings"
)

// Table renders rows as a GFM pipe table; the first row is the header.
// Ragged rows are padded to the widest row. Returns "" for no rows.
func Table(rows [][]string) string {
	var b strings.Builder
	_ = WriteTable(&b, rows)
	return b.String()
}

// WriteTable renders rows as a GFM pipe table into w. The first row is the
// header, ragged rows are padded to the widest row, and no output is written
// for an empty or zero-width grid.
func WriteTable(w io.StringWriter, rows [][]string) error {
	if len(rows) == 0 {
		return nil
	}
	width := 0
	for _, row := range rows {
		width = max(width, len(row))
	}
	if width == 0 {
		return nil
	}
	write := func(s string) error {
		_, err := w.WriteString(s)
		return err
	}
	writeRow := func(row []string) error {
		if err := write("|"); err != nil {
			return err
		}
		for i := range width {
			cell := ""
			if i < len(row) {
				cell = EscapeCell(row[i])
			}
			for _, part := range []string{" ", cell, " |"} {
				if err := write(part); err != nil {
					return err
				}
			}
		}
		return write("\n")
	}
	if err := writeRow(rows[0]); err != nil {
		return err
	}
	if err := write("|"); err != nil {
		return err
	}
	for range width {
		if err := write(" --- |"); err != nil {
			return err
		}
	}
	if err := write("\n"); err != nil {
		return err
	}
	for _, row := range rows[1:] {
		if err := writeRow(row); err != nil {
			return err
		}
	}
	return nil
}

// EscapeCell makes s safe inside a pipe-table cell: newlines become spaces,
// pipes are escaped, and surrounding whitespace is trimmed.
func EscapeCell(s string) string {
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
	s = strings.ReplaceAll(s, "|", `\|`)
	return strings.TrimSpace(s)
}
