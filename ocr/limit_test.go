package ocr_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	gpdf "github.com/giraffesyo/pdf"

	"github.com/giraffesyo/downmark/ocr"
)

// countingEngine records the calls it received and returns one glyph a
// page, so a test can tell a page that was read from one that was not.
type countingEngine struct {
	mu    sync.Mutex
	calls int
	block time.Duration
}

func (e *countingEngine) ExtractPage(ctx context.Context, _ gpdf.OCRRequest) ([]gpdf.Glyph, error) {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()
	if e.block > 0 {
		select {
		case <-time.After(e.block):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return []gpdf.Glyph{{Text: "x", Advance: 1, Size: 1}}, nil
}

func (e *countingEngine) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

func TestLimitWithoutBoundsReturnsTheEngineItself(t *testing.T) {
	engine := &countingEngine{}
	if got := ocr.Limit(engine, ocr.Limits{}); got != gpdf.OCR(engine) {
		t.Errorf("Limit = %v, want the engine unwrapped", got)
	}
	if got := ocr.Limit(nil, ocr.Limits{MaxPages: 1}); got != nil {
		t.Errorf("Limit(nil) = %v, want nil", got)
	}
}

func TestLimitStopsAtThePageBudgetAndReportsOnce(t *testing.T) {
	engine := &countingEngine{}
	limited := ocr.Limit(engine, ocr.Limits{MaxPages: 2})

	var refusals []error
	for page := 1; page <= 5; page++ {
		glyphs, err := limited.ExtractPage(t.Context(), gpdf.OCRRequest{PageNumber: page})
		switch {
		case page <= 2:
			if err != nil || len(glyphs) == 0 {
				t.Fatalf("page %d: glyphs = %v, err = %v, want it read", page, glyphs, err)
			}
		default:
			if len(glyphs) != 0 {
				t.Errorf("page %d: glyphs = %v, want none past the budget", page, glyphs)
			}
			if err != nil {
				refusals = append(refusals, err)
			}
		}
	}
	if engine.count() != 2 {
		t.Errorf("engine calls = %d, want 2: the budget must stop the work, not just the result", engine.count())
	}
	if len(refusals) != 1 {
		t.Fatalf("refusals reported = %d, want exactly 1: a long scan must not bury its other warnings", len(refusals))
	}
	if !errors.Is(refusals[0], ocr.ErrBudget) {
		t.Errorf("refusal = %v, want it to match ErrBudget", refusals[0])
	}
	if !strings.Contains(refusals[0].Error(), "2") {
		t.Errorf("refusal = %v, want it to name the budget that was hit", refusals[0])
	}
}

func TestLimitPerPageTimeoutStopsTheEngine(t *testing.T) {
	engine := &countingEngine{block: time.Minute}
	limited := ocr.Limit(engine, ocr.Limits{PerPage: 10 * time.Millisecond})

	start := time.Now()
	_, err := limited.ExtractPage(t.Context(), gpdf.OCRRequest{PageNumber: 1})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the deadline", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("took %s, want the page abandoned at its timeout", elapsed)
	}
}

// The extractor hands every page the document's context, so an
// interrupted conversion has to stop between pages rather than run the
// engine over the rest of the scan.
func TestLimitHonorsACancelledContext(t *testing.T) {
	engine := &countingEngine{}
	limited := ocr.Limit(engine, ocr.Limits{MaxPages: 10})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := limited.ExtractPage(ctx, gpdf.OCRRequest{PageNumber: 1}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the cancellation", err)
	}
	if engine.count() != 0 {
		t.Errorf("engine calls = %d, want none after cancellation", engine.count())
	}
}

// The extractor OCRs pages concurrently, so the budget has to hold
// across goroutines.
func TestLimitIsSafeForConcurrentPages(t *testing.T) {
	engine := &countingEngine{}
	limited := ocr.Limit(engine, ocr.Limits{MaxPages: 4})

	var wg sync.WaitGroup
	for page := 1; page <= 32; page++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = limited.ExtractPage(t.Context(), gpdf.OCRRequest{PageNumber: page})
		}()
	}
	wg.Wait()
	if engine.count() != 4 {
		t.Errorf("engine calls = %d, want exactly the 4 the budget allows", engine.count())
	}
}
