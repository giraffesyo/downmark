// Package ocr bounds what an OCR implementation may spend on a document,
// for the PDF converter's OCR seam. It supplies no engine and reads no
// images: the engine is [github.com/giraffesyo/pdf/ocr/tesseract], or
// anything else implementing pdf.OCR, and Limit wraps it.
//
// OCR costs roughly a second a page and nothing else in a conversion
// does, which is why this is worth a package of its own rather than a
// field on an engine.
//
//	engine := &tesseract.Engine{Languages: []string{"eng"}}
//	pdf.Register(e, pdf.Options{
//		OCR: ocr.Limit(engine, ocr.Limits{MaxPages: 30}),
//	})
package ocr

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	gpdf "github.com/giraffesyo/pdf"
)

// ErrBudget is the error an engine wrapped by Limit returns for a page it
// refused to read. It reaches the caller as a pdf warning on that page,
// so match it with errors.Is to tell a budget from a failure.
//
// The message carries no package prefix: this error is only ever
// returned through the OCR seam, which labels the warning "ocr" itself,
// and a prefix here would render as "ocr: ocr: budget exhausted".
var ErrBudget = errors.New("budget exhausted")

// Limits bounds the work an OCR engine is allowed to do for one
// document. The zero value is no bound at all, which is the right choice
// only when the input is trusted and the engine is fast: OCR costs
// roughly a second a page, so a few hundred scanned pages is a few
// minutes of wall clock that nothing else interrupts.
type Limits struct {
	// MaxPages is the most pages the engine is asked to read. Pages
	// beyond it are left textless. Zero means no limit.
	MaxPages int
	// PerPage bounds one page's read, by giving the engine a context
	// that expires. An engine that ignores its context is not stopped by
	// this. Zero means no limit.
	PerPage time.Duration
	// Total bounds every page's read together, measured from the first
	// one. Zero means no limit.
	Total time.Duration
}

// Limit returns engine bounded by lim, safe for the concurrent use the
// extractor makes of it. It returns engine itself when there is nothing
// to bound, and nil for a nil engine, so it can wrap unconditionally.
//
// A page the budget refuses is reported once — the first such page
// carries an ErrBudget warning naming the bound that was hit, and the
// rest are silent, so that a 500-page scan under a 30-page budget yields
// one warning rather than 470 copies of it.
func Limit(engine gpdf.OCR, lim Limits) gpdf.OCR {
	if engine == nil || lim == (Limits{}) {
		return engine
	}
	return &limited{engine: engine, lim: lim}
}

type limited struct {
	engine gpdf.OCR
	lim    Limits

	pages     atomic.Int64
	startOnce sync.Once
	start     time.Time
	reported  atomic.Bool
}

func (l *limited) ExtractPage(ctx context.Context, req gpdf.OCRRequest) ([]gpdf.Glyph, error) {
	// The extractor hands each page the document's context; checking it
	// here is what makes a cancelled conversion stop between pages
	// rather than after the last one.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.startOnce.Do(func() { l.start = time.Now() })

	if n := l.lim.MaxPages; n > 0 && l.pages.Add(1) > int64(n) {
		return l.refuse(fmt.Errorf("%w: page budget of %d reached, later pages were not read", ErrBudget, n))
	}
	if d := l.lim.Total; d > 0 {
		deadline := l.start.Add(d)
		if !time.Now().Before(deadline) {
			return l.refuse(fmt.Errorf("%w: total budget of %s spent, later pages were not read", ErrBudget, d))
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadline)
		defer cancel()
	}
	if d := l.lim.PerPage; d > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}
	return l.engine.ExtractPage(ctx, req)
}

// refuse reports err for the first page the budget turned away and stays
// quiet for the rest.
func (l *limited) refuse(err error) ([]gpdf.Glyph, error) {
	if l.reported.CompareAndSwap(false, true) {
		return nil, err
	}
	return nil, nil
}
