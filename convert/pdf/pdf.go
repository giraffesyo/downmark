// Package pdf converts PDF documents to Markdown for the downmark engine.
// Text extraction is provided by github.com/giraffesyo/pdf.
package pdf

import (
	"context"
	"errors"
	"io"
	"strings"

	gpdf "github.com/giraffesyo/pdf"

	"github.com/giraffesyo/downmark"
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
	text := doc.Text()
	if strings.TrimSpace(text) == "" {
		return nil, errNoText
	}
	return &downmark.Result{Markdown: text}, nil
}
