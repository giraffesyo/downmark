// Package csv converts delimiter-separated values to Markdown tables for
// the downmark engine. The delimiter (comma, semicolon, tab, or pipe) is
// sniffed from the content; legacy charsets are decoded automatically.
package csv

import (
	"context"
	stdcsv "encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/internal/ctxio"
	"github.com/giraffesyo/downmark/internal/limitbuf"
	"github.com/giraffesyo/downmark/internal/mdutil"
	"github.com/giraffesyo/downmark/internal/textenc"
)

// New returns the CSV converter.
func New() downmark.Converter { return converter{} }

// Register adds the CSV converter to e at the standard priority.
func Register(e *downmark.Engine) { e.Register(New(), downmark.PrioritySpecific) }

type converter struct{}

func (converter) Name() string { return "csv" }

func (converter) Accepts(info downmark.StreamInfo) bool {
	return info.Matches([]string{".csv"}, []string{"text/csv", "application/csv"})
}

func (converter) Convert(ctx context.Context, input io.ReadSeeker, info downmark.StreamInfo) (*downmark.Result, error) {
	text, err := textenc.DecodeAll(ctxio.NewReader(ctx, input), info.Charset)
	if err != nil {
		return nil, err
	}
	text = strings.TrimPrefix(text, "\uFEFF")

	r := stdcsv.NewReader(strings.NewReader(text))
	r.Comma = sniffDelimiter(text)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return &downmark.Result{}, nil
	}

	// The first row is the header; pad or truncate every other row to its
	// width.
	width := len(rows[0])
	for i, row := range rows[1:] {
		if len(row) > width {
			rows[i+1] = row[:width]
		}
		for len(rows[i+1]) < width {
			rows[i+1] = append(rows[i+1], "")
		}
	}
	limit, _ := downmark.ResultLimit(ctx)
	b := limitbuf.New(limit)
	if err := mdutil.WriteTable(b, rows); err != nil {
		if errors.Is(err, limitbuf.ErrTooLarge) {
			return nil, fmt.Errorf("%w: CSV result exceeds %d-byte limit", downmark.ErrResultTooLarge, limit)
		}
		return nil, err
	}
	return &downmark.Result{Markdown: b.String()}, nil
}

// sniffDelimiter picks the delimiter that yields the most columns with a
// consistent count across the first rows of the sample. Comma wins ties.
func sniffDelimiter(sample string) rune {
	const maxRows = 10
	best, bestCols := ',', 1
	for _, cand := range []rune{',', ';', '\t', '|'} {
		r := stdcsv.NewReader(strings.NewReader(sample))
		r.Comma = cand
		r.FieldsPerRecord = 0 // require consistency
		r.LazyQuotes = true
		cols, rows := 0, 0
		for rows < maxRows {
			rec, err := r.Read()
			if err != nil {
				if rows == 0 || err != io.EOF {
					rows = 0
				}
				break
			}
			cols = len(rec)
			rows++
		}
		if rows > 0 && cols > bestCols {
			best, bestCols = cand, cols
		}
	}
	return best
}
