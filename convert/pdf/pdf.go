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

// ErrPageNoText is the error behind the warning reported for a page that
// produced no text and so is absent from the Markdown entirely.
//
// It is reported only when Options.OCR is set. A textless page is not by
// itself a loss — a blank separator page, or the back of a duplex scan,
// is a normal thing for a document to hold, and reporting every one of
// them would bury the warnings that matter. Supplying an engine is the
// caller saying they want such pages recovered, which makes the ones
// still empty afterwards worth naming: they are where an OCR engine did
// not help and a renderer might, because their text is vector outlines
// rather than ink. Match with errors.Is.
var ErrPageNoText = errors.New("page produced no text; it may be a scan, or text converted to vector outlines")

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
	ws := warnings(doc.Warnings)
	// See ErrPageNoText for why a textless page is worth reporting only
	// alongside an engine that was meant to fill it in. Pages the
	// extractor already reported an OCR failure for are left to that
	// warning, so that one problem is reported once.
	reportTextless := c.opts.OCR != nil
	ocrFailed := ocrFailures(doc.Warnings)
	for _, page := range doc.Pages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		text := page.Text()
		if text == "" {
			if reportTextless && !ocrFailed[page.Number] {
				ws = downmark.AppendWarning(ws, downmark.Warning{
					Converter: "pdf",
					Code:      downmark.WarningIncomplete,
					Location:  fmt.Sprintf("page %d", page.Number),
					Err:       ErrPageNoText,
				})
			}
			continue
		}
		if b.Len() > 0 {
			if _, err := b.WriteString("\n\n"); err != nil {
				return nil, pdfResultLimitError(limit)
			}
		}
		if page.OCRGlyphs > 0 {
			// OCR text is a reading of the ink rather than the
			// document's own characters, and a consumer that cannot
			// tell the two apart cannot weigh them differently. The
			// marker covers a page OCR filled in entirely and one where
			// it supplied only a scanned figure's text, because both
			// are pages holding text nothing typeset.
			if _, err := fmt.Fprintf(b, "<!-- downmark: page %d includes OCR text -->\n\n", page.Number); err != nil {
				return nil, pdfResultLimitError(limit)
			}
		}
		if _, err := b.WriteString(text); err != nil {
			return nil, pdfResultLimitError(limit)
		}
	}
	text := b.String()
	if strings.TrimSpace(text) == "" {
		return nil, noTextError(ws)
	}
	return &downmark.Result{Markdown: text, Warnings: ws}, nil
}

// warnings translates the extractor's warnings into the engine's. Every
// one of them means the same thing at this altitude — text the document
// held is missing from the output — so they all map to
// WarningIncomplete, and callers who want the extractor's finer
// distinction reach it with errors.As on Err.
func warnings(in []gpdf.Warning) []downmark.Warning {
	var out []downmark.Warning
	for _, w := range in {
		var location string
		if w.Page > 0 {
			location = fmt.Sprintf("page %d", w.Page)
		}
		out = downmark.AppendWarning(out, downmark.Warning{
			Converter: "pdf",
			Code:      downmark.WarningIncomplete,
			Location:  location,
			Err:       extractorWarning{w},
		})
	}
	return out
}

// extractorWarning states an extractor warning without the converter
// name and page number that the surrounding downmark.Warning prints from
// its own fields. gpdf.Warning names both in its message, which is right
// when it is read on its own and wrong once those fields have been
// lifted out of it: printed unchanged, the pair reads "pdf: page 3: pdf:
// page 3: ocr: ...".
//
// What is left is the extractor's own finer code and the failure under
// it, which is the part downmark.Warning does not already say. The typed
// warning stays one Unwrap below, so errors.As still recovers it with
// its page and code intact.
type extractorWarning struct{ w gpdf.Warning }

func (e extractorWarning) Error() string {
	return fmt.Sprintf("%s: %v", e.w.Code, e.w.Err)
}

func (e extractorWarning) Unwrap() error { return e.w }

// noTextError explains a conversion that produced nothing. Warnings
// reach a caller on a Result, and a failed conversion has none, so
// whatever the pages did report has to travel in the error itself —
// otherwise an OCR engine that timed out on every page is reported as a
// PDF that might be scanned.
func noTextError(ws []downmark.Warning) error {
	if len(ws) == 0 {
		return errNoText
	}
	// The warning goes in as text rather than wrapped: which one is
	// first is an accident of page order, and matching on it with
	// errors.Is would be matching on that accident.
	return fmt.Errorf("%w; %d warning(s) along the way, the first: %s", errNoText, len(ws), ws[0].Error())
}

// ocrFailures indexes the pages the extractor already reported an OCR
// failure for. Those pages are textless for a reason that warning
// already gives, in more detail than this package could.
func ocrFailures(in []gpdf.Warning) map[int]bool {
	var out map[int]bool
	for _, w := range in {
		if w.Code == gpdf.WarningOCR && w.Page > 0 {
			if out == nil {
				out = make(map[int]bool, 1)
			}
			out[w.Page] = true
		}
	}
	return out
}

func pdfResultLimitError(limit int) error {
	return fmt.Errorf("%w: PDF result exceeds %d-byte limit", downmark.ErrResultTooLarge, limit)
}
