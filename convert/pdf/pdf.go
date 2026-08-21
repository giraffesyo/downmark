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

// Options configures the PDF converter.
type Options struct {
	// OCR supplies text for pages the content streams cannot: scanned
	// pages, and pages whose text was converted to vector outlines.
	// Without one, such pages contribute nothing and a document made
	// only of them fails with a "no extractable text" error.
	//
	// downmark ships no engine; implement gpdf.OCR (or wrap a function
	// in gpdf.OCRFunc) over the renderer or OCR service you already
	// have. Each request carries the page's images, with their encoded
	// data and placement, alongside a reader over the original PDF.
	// Pages are OCR'd concurrently, so the implementation must be safe
	// for concurrent use.
	OCR gpdf.OCR

	// OCRPolicy selects the pages OCR is asked about. The zero value,
	// gpdf.OCRTextlessPages, asks only about pages that produced no text
	// of their own; gpdf.OCRImagePages also covers pages that mix
	// typeset text with scanned figures or stamps. Ignored when OCR is
	// nil.
	OCRPolicy gpdf.OCRPolicy
}

// New returns the PDF converter.
func New(opts Options) downmark.Converter { return converter{opts: opts} }

// Register adds the PDF converter to e at the standard priority.
func Register(e *downmark.Engine, opts Options) {
	e.Register(New(opts), downmark.PrioritySpecific)
}

type converter struct {
	opts Options
}

func (converter) Name() string { return "pdf" }

func (converter) Accepts(info downmark.StreamInfo) bool {
	return info.Matches([]string{".pdf"}, []string{"application/pdf", "application/x-pdf"})
}

var errNoText = errors.New("no extractable text; the PDF may be scanned images or use unsupported fonts")

func (c converter) Convert(ctx context.Context, input io.ReadSeeker, _ downmark.StreamInfo) (*downmark.Result, error) {
	ra, size, err := readerat.From(input)
	if err != nil {
		return nil, err
	}
	// The policy only means something alongside an implementation, and
	// extraction rejects an out-of-range one, so leave both zero without.
	var extract gpdf.Options
	if c.opts.OCR != nil {
		extract.OCR = c.opts.OCR
		extract.OCRPolicy = c.opts.OCRPolicy
	}
	doc, err := gpdf.ExtractWithOptions(ctx, ra, size, extract)
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
