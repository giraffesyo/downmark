// Package mdutil holds small shared Markdown emission helpers.
package mdutil

import "strings"

// Table renders rows as a GFM pipe table; the first row is the header.
// Ragged rows are padded to the widest row. Returns "" for no rows.
func Table(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	width := 0
	for _, row := range rows {
		width = max(width, len(row))
	}
	if width == 0 {
		return ""
	}
	var b strings.Builder
	writeRow := func(row []string) {
		b.WriteString("|")
		for i := range width {
			cell := ""
			if i < len(row) {
				cell = EscapeCell(row[i])
			}
			b.WriteString(" ")
			b.WriteString(cell)
			b.WriteString(" |")
		}
		b.WriteString("\n")
	}
	writeRow(rows[0])
	b.WriteString("|")
	for range width {
		b.WriteString(" --- |")
	}
	b.WriteString("\n")
	for _, row := range rows[1:] {
		writeRow(row)
	}
	return b.String()
}

// EscapeCell makes s safe inside a pipe-table cell: newlines become spaces,
// pipes are escaped, and surrounding whitespace is trimmed.
func EscapeCell(s string) string {
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
	s = strings.ReplaceAll(s, "|", `\|`)
	return strings.TrimSpace(s)
}
