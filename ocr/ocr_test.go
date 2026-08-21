package ocr_test

import (
	"errors"
	"testing"

	gpdf "github.com/giraffesyo/pdf"

	"github.com/giraffesyo/downmark/ocr"
)

func TestRequireImagesTurnsAwayAPageWithNone(t *testing.T) {
	engine := &countingEngine{}
	guarded := ocr.RequireImages(engine)

	glyphs, err := guarded.ExtractPage(t.Context(), gpdf.OCRRequest{PageNumber: 1})
	if !errors.Is(err, ocr.ErrNoImages) {
		t.Fatalf("err = %v, want ErrNoImages", err)
	}
	if len(glyphs) != 0 {
		t.Errorf("glyphs = %v, want none", glyphs)
	}
	// An engine that reads images has nothing to do here, and running it
	// would cost a process to learn that.
	if engine.count() != 0 {
		t.Errorf("engine calls = %d, want none", engine.count())
	}
}

func TestRequireImagesPassesAPageWithThem(t *testing.T) {
	engine := &countingEngine{}
	req := gpdf.OCRRequest{PageNumber: 1, Page: gpdf.Page{Images: []gpdf.Image{{}}}}

	glyphs, err := ocr.RequireImages(engine).ExtractPage(t.Context(), req)
	if err != nil {
		t.Fatalf("ExtractPage: %v", err)
	}
	if len(glyphs) == 0 {
		t.Error("glyphs = none, want the engine's own result")
	}
	if engine.count() != 1 {
		t.Errorf("engine calls = %d, want 1", engine.count())
	}
}

func TestRequireImagesOfNil(t *testing.T) {
	if got := ocr.RequireImages(nil); got != nil {
		t.Errorf("RequireImages(nil) = %v, want nil", got)
	}
}

// The guard belongs outside the budget: a page no engine can read should
// not spend one of the pages the budget allows.
func TestRequireImagesOutsideLimitDoesNotSpendBudget(t *testing.T) {
	engine := &countingEngine{}
	guarded := ocr.RequireImages(ocr.Limit(engine, ocr.Limits{MaxPages: 1}))

	if _, err := guarded.ExtractPage(t.Context(), gpdf.OCRRequest{PageNumber: 1}); !errors.Is(err, ocr.ErrNoImages) {
		t.Fatalf("page 1: err = %v, want ErrNoImages", err)
	}
	withImages := gpdf.OCRRequest{PageNumber: 2, Page: gpdf.Page{Images: []gpdf.Image{{}}}}
	if _, err := guarded.ExtractPage(t.Context(), withImages); err != nil {
		t.Fatalf("page 2: err = %v, want the budget still unspent", err)
	}
	if engine.count() != 1 {
		t.Errorf("engine calls = %d, want the one page that had images", engine.count())
	}
}
