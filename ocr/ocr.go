// Package ocr bounds and guards an OCR implementation for the PDF
// converter's OCR seam. It supplies no engine and reads no images: the
// engine is [github.com/giraffesyo/pdf/ocr/tesseract], or anything else
// implementing pdf.OCR, and what is here wraps it.
//
// Two things an engine should not have to carry itself. Limit bounds
// what a document is allowed to spend on OCR, which matters because OCR
// costs about a second a page and nothing else in a conversion does.
// RequireImages reports the pages no image-reading engine can help with,
// which is the difference between a scan worth retrying and text that
// was converted to vector outlines.
//
//	engine := &tesseract.Engine{Languages: []string{"eng"}}
//	pdf.Register(e, pdf.Options{
//		OCR: ocr.RequireImages(ocr.Limit(engine, ocr.Limits{MaxPages: 30})),
//	})
package ocr

import (
	"context"
	"errors"

	gpdf "github.com/giraffesyo/pdf"
)

// ErrNoImages is the error RequireImages reports for a page that paints
// none. It reaches the caller as a warning on that page, so match it
// with errors.Is.
//
// The message carries no package prefix: this error is only ever
// returned through the OCR seam, which labels the warning "ocr" itself,
// and a prefix here would render as "ocr: ocr: page paints no images".
var ErrNoImages = errors.New("page paints no images to read")

// RequireImages returns engine with the pages it cannot help with turned
// away before it runs, reporting ErrNoImages for each.
//
// An engine that reads a page's images has nothing to do with a page
// that paints none, and returns quietly — which is indistinguishable
// from a scan it read and found blank. The two want different things
// next: the scan is worth another engine or another language, while a
// page with no images has had its text converted to vector outlines and
// needs something that renders pages, which no OCR engine is. Saying so
// is the difference between a caller that can act on the warning and one
// that can only see that a page went missing.
//
// It returns nil for a nil engine, so it can wrap unconditionally.
func RequireImages(engine gpdf.OCR) gpdf.OCR {
	if engine == nil {
		return nil
	}
	return requireImages{engine}
}

type requireImages struct{ engine gpdf.OCR }

func (r requireImages) ExtractPage(ctx context.Context, req gpdf.OCRRequest) ([]gpdf.Glyph, error) {
	if len(req.Page.Images) == 0 {
		return nil, ErrNoImages
	}
	return r.engine.ExtractPage(ctx, req)
}
