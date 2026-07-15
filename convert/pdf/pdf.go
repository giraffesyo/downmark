// Package pdf converts PDF documents to Markdown for the downmark engine.
// Text extraction is provided by github.com/giraffesyo/pdf.
package pdf

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	gpdf "github.com/giraffesyo/pdf"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/internal/limitbuf"
	"github.com/giraffesyo/downmark/internal/readerat"
)

// New returns the PDF converter.
func New() downmark.Converter { return converter{} }

// Register adds the PDF converter to e at the standard priority.
func Register(e *downmark.Engine) { e.Register(New(), downmark.PrioritySpecific) }

type converter struct{}

func (converter) Name() string { return "pdf" }

func (converter) Accepts(info downmark.StreamInfo) bool {
	return info.Matches([]string{".pdf"}, []string{"application/pdf", "application/x-pdf"})
}

var errNoText = errors.New("no extractable text; the PDF may be scanned images or use unsupported fonts")

func (converter) Convert(ctx context.Context, input io.ReadSeeker, _ downmark.StreamInfo) (*downmark.Result, error) {
	ra, size, err := readerat.From(input)
	if err != nil {
		return nil, err
	}
	doc, err := gpdf.Extract(ctx, ra, size)
	if err != nil {
		return nil, err
	}
	limit, _ := downmark.ResultLimit(ctx)
	b := limitbuf.New(limit)
	for _, page := range doc.Pages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		text := page.Text()
		if text == "" {
			continue
		}
		if b.Len() > 0 {
			if _, err := b.WriteString("\n\n"); err != nil {
				return nil, pdfResultLimitError(limit)
			}
		}
		if _, err := b.WriteString(text); err != nil {
			return nil, pdfResultLimitError(limit)
		}
	}
	text := b.String()
	if strings.TrimSpace(text) == "" {
		return nil, errNoText
	}
	return &downmark.Result{Markdown: text}, nil
}

func pdfResultLimitError(limit int) error {
	return fmt.Errorf("%w: PDF result exceeds %d-byte limit", downmark.ErrResultTooLarge, limit)
}
