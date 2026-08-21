package pdf_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	gpdf "github.com/giraffesyo/pdf"
	"github.com/giraffesyo/pdf/pdftest"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/convert/pdf"
)

func TestMalformedPDFDoesNotPanic(t *testing.T) {
	e := downmark.New(downmark.WithoutBuiltins())
	pdf.Register(e, pdf.Options{})
	garbage := "%PDF-1.7\nthis is not a real pdf body at all\n%%EOF"
	_, err := e.Convert(t.Context(), strings.NewReader(garbage), downmark.StreamInfo{Extension: ".pdf"})
	var convErr *downmark.ConversionError
	if !errors.As(err, &convErr) {
		t.Fatalf("err = %v, want *ConversionError (panic must be contained)", err)
	}
	// The friendly converter name must survive the package split: Name()
	// has to be exported for the engine to see it across packages.
	if len(convErr.Attempts) == 0 || convErr.Attempts[0].Converter != "pdf" {
		t.Errorf("Attempts = %+v, want converter name %q", convErr.Attempts, "pdf")
	}
}

// textlessPDF is a one-page document whose content stream paints nothing:
// the shape of a scanned page as far as text extraction is concerned.
func textlessPDF() []byte {
	return pdftest.Build(1,
		pdftest.Catalog(2),
		pdftest.Pages(3),
		pdftest.Page(2, 4, "<< >>"),
		pdftest.Stream("", ""),
	)
}

// ocrLine lays out one line of glyphs the way an OCR engine would report
// them: unrotated page space, left to right along the baseline at y.
func ocrLine(text string, y float64) []gpdf.Glyph {
	glyphs := make([]gpdf.Glyph, 0, len(text))
	x := 72.0
	for _, r := range text {
		glyphs = append(glyphs, gpdf.Glyph{
			Text:      string(r),
			X:         x,
			Y:         y,
			Advance:   7,
			Size:      12,
			Direction: gpdf.Point{X: 1},
			Ascent:    gpdf.Point{Y: 12},
		})
		x += 7
	}
	return glyphs
}

func TestTextlessPDFWithoutOCR(t *testing.T) {
	e := downmark.New(downmark.WithoutBuiltins())
	pdf.Register(e, pdf.Options{})
	_, err := e.Convert(t.Context(), bytes.NewReader(textlessPDF()), downmark.StreamInfo{Extension: ".pdf"})
	if err == nil {
		t.Fatal("err = nil, want the no-extractable-text failure")
	}
	if !strings.Contains(err.Error(), "no extractable text") {
		t.Errorf("err = %v, want it to mention no extractable text", err)
	}
}

func TestOCRSuppliesTextForTextlessPage(t *testing.T) {
	var pages []int
	e := downmark.New(downmark.WithoutBuiltins())
	pdf.Register(e, pdf.Options{
		OCR: gpdf.OCRFunc(func(_ context.Context, req gpdf.OCRRequest) ([]gpdf.Glyph, error) {
			pages = append(pages, req.PageNumber)
			return ocrLine("Scanned page text", 700), nil
		}),
	})
	res, err := e.Convert(t.Context(), bytes.NewReader(textlessPDF()), downmark.StreamInfo{Extension: ".pdf"})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !strings.Contains(res.Markdown, "Scanned page text") {
		t.Errorf("Markdown = %q, want the OCR'd text", res.Markdown)
	}
	if len(pages) != 1 || pages[0] != 1 {
		t.Errorf("OCR'd pages = %v, want the one textless page", pages)
	}
}

// A failing engine must not sink the conversion: the page simply stays
// textless, and the extractor records the failure as a warning.
func TestOCRFailureLeavesPageTextless(t *testing.T) {
	e := downmark.New(downmark.WithoutBuiltins())
	pdf.Register(e, pdf.Options{
		OCR: gpdf.OCRFunc(func(context.Context, gpdf.OCRRequest) ([]gpdf.Glyph, error) {
			return nil, errors.New("engine unavailable")
		}),
	})
	_, err := e.Convert(t.Context(), bytes.NewReader(textlessPDF()), downmark.StreamInfo{Extension: ".pdf"})
	if err == nil || !strings.Contains(err.Error(), "no extractable text") {
		t.Fatalf("err = %v, want the no-extractable-text failure", err)
	}
}

// mixedPDF has one typeset page and one textless page, so a failure on
// the second still leaves a successful conversion to attach warnings to.
func mixedPDF() []byte {
	return pdftest.Build(1,
		pdftest.Catalog(2),
		pdftest.Pages(3, 4),
		pdftest.Page(2, 5, "<< /Font << /F1 7 0 R >> >>"),
		pdftest.Page(2, 6, "<< >>"),
		pdftest.Stream("", "BT /F1 12 Tf 72 700 Td (Typeset page) Tj ET"),
		pdftest.Stream("", ""),
		pdftest.Helvetica(),
	)
}

func TestOCRFailureSurfacesAsWarning(t *testing.T) {
	e := downmark.New(downmark.WithoutBuiltins())
	pdf.Register(e, pdf.Options{
		OCR: gpdf.OCRFunc(func(context.Context, gpdf.OCRRequest) ([]gpdf.Glyph, error) {
			return nil, errors.New("engine unavailable")
		}),
	})
	res, err := e.Convert(t.Context(), bytes.NewReader(mixedPDF()), downmark.StreamInfo{Extension: ".pdf"})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !strings.Contains(res.Markdown, "Typeset page") {
		t.Fatalf("Markdown = %q, want the page that did convert", res.Markdown)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly the OCR failure", res.Warnings)
	}
	w := res.Warnings[0]
	if w.Converter != "pdf" || w.Code != downmark.WarningIncomplete || w.Location != "page 2" {
		t.Errorf("Warning = %+v, want pdf/incomplete on page 2", w)
	}

	// The core vocabulary is coarse on purpose; the extractor's own
	// classification has to survive the boundary for a caller that wants
	// to tell an OCR failure from a malformed page.
	var pw gpdf.Warning
	if !errors.As(w.Err, &pw) {
		t.Fatalf("Err = %v, want a pdf.Warning recoverable with errors.As", w.Err)
	}
	if pw.Code != gpdf.WarningOCR {
		t.Errorf("pdf.Warning.Code = %q, want %q", pw.Code, gpdf.WarningOCR)
	}
	if pw.Page != 2 {
		t.Errorf("pdf.Warning.Page = %d, want 2", pw.Page)
	}
}

func TestCleanPDFCarriesNoWarnings(t *testing.T) {
	e := downmark.New(downmark.WithoutBuiltins())
	pdf.Register(e, pdf.Options{})
	res, err := e.Convert(t.Context(), bytes.NewReader(mixedPDF()), downmark.StreamInfo{Extension: ".pdf"})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none: a textless page is not itself a loss", res.Warnings)
	}
}

// A textless page is reported only alongside an engine that was meant to
// fill it in: see pdf.ErrPageNoText. Without one, TestCleanPDFCarriesNoWarnings
// covers the silence; with one, the pages still empty are the list a
// caller needs in order to decide what to do next.
func TestTextlessPageIsReportedWhenOCRDidNotFillItIn(t *testing.T) {
	e := downmark.New(downmark.WithoutBuiltins())
	pdf.Register(e, pdf.Options{
		OCR: gpdf.OCRFunc(func(context.Context, gpdf.OCRRequest) ([]gpdf.Glyph, error) {
			// An engine that read the page and found nothing on it:
			// no glyphs, and no failure to report either.
			return nil, nil
		}),
	})
	res, err := e.Convert(t.Context(), bytes.NewReader(mixedPDF()), downmark.StreamInfo{Extension: ".pdf"})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want the page that stayed empty", res.Warnings)
	}
	w := res.Warnings[0]
	if w.Converter != "pdf" || w.Code != downmark.WarningIncomplete || w.Location != "page 2" {
		t.Errorf("Warning = %+v, want pdf/incomplete on page 2", w)
	}
	if !errors.Is(w, pdf.ErrPageNoText) {
		t.Errorf("Warning = %v, want it to match ErrPageNoText", w)
	}
}

// One problem, one warning: a page the engine failed on is textless
// because of that failure, and the extractor's warning says more about
// it than this package could.
func TestOCRFailureIsNotAlsoReportedAsATextlessPage(t *testing.T) {
	e := downmark.New(downmark.WithoutBuiltins())
	pdf.Register(e, pdf.Options{
		OCR: gpdf.OCRFunc(func(context.Context, gpdf.OCRRequest) ([]gpdf.Glyph, error) {
			return nil, errors.New("engine unavailable")
		}),
	})
	res, err := e.Convert(t.Context(), bytes.NewReader(mixedPDF()), downmark.StreamInfo{Extension: ".pdf"})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want only the OCR failure", res.Warnings)
	}
	if errors.Is(res.Warnings[0], pdf.ErrPageNoText) {
		t.Errorf("Warning = %v, want the OCR failure rather than a second warning for its consequence", res.Warnings[0])
	}
}

// OCR text is a reading of the ink rather than the document's own
// characters. A consumer that cannot tell the two apart cannot weigh
// them differently, so the pages it filled in are marked.
func TestOCRPagesAreMarkedInTheMarkdown(t *testing.T) {
	e := downmark.New(downmark.WithoutBuiltins())
	pdf.Register(e, pdf.Options{
		OCR: gpdf.OCRFunc(func(context.Context, gpdf.OCRRequest) ([]gpdf.Glyph, error) {
			return ocrLine("Scanned page text", 700), nil
		}),
	})
	res, err := e.Convert(t.Context(), bytes.NewReader(mixedPDF()), downmark.StreamInfo{Extension: ".pdf"})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	const marker = "<!-- downmark: page 2 includes OCR text -->"
	if !strings.Contains(res.Markdown, marker) {
		t.Errorf("Markdown = %q, want %q", res.Markdown, marker)
	}
	// The marker belongs to the page it describes, not to the typeset
	// one above it.
	if i, j := strings.Index(res.Markdown, marker), strings.Index(res.Markdown, "Typeset page"); i < j {
		t.Errorf("Markdown = %q, want the marker after the typeset page", res.Markdown)
	}
	if strings.Count(res.Markdown, "downmark:") != 1 {
		t.Errorf("Markdown = %q, want exactly one marker: only page 2 was OCR'd", res.Markdown)
	}
}

// A page nothing OCR'd carries no marker, so the convention stays out of
// the way of every PDF that did not need an engine.
func TestPagesWithoutOCRAreNotMarked(t *testing.T) {
	e := downmark.New(downmark.WithoutBuiltins())
	pdf.Register(e, pdf.Options{})
	res, err := e.Convert(t.Context(), bytes.NewReader(mixedPDF()), downmark.StreamInfo{Extension: ".pdf"})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if strings.Contains(res.Markdown, "downmark:") {
		t.Errorf("Markdown = %q, want no marker", res.Markdown)
	}
}

// The converter lifts the extractor's page number into Warning.Location,
// so leaving the extractor's own message intact underneath printed the
// pair twice: "pdf: page 2: pdf: page 2: ocr: ...".
func TestWarningDoesNotRepeatTheConverterAndPage(t *testing.T) {
	e := downmark.New(downmark.WithoutBuiltins())
	pdf.Register(e, pdf.Options{
		OCR: gpdf.OCRFunc(func(context.Context, gpdf.OCRRequest) ([]gpdf.Glyph, error) {
			return nil, errors.New("engine unavailable")
		}),
	})
	res, err := e.Convert(t.Context(), bytes.NewReader(mixedPDF()), downmark.StreamInfo{Extension: ".pdf"})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly one", res.Warnings)
	}
	got := res.Warnings[0].Error()
	const want = "pdf: page 2: ocr: engine unavailable"
	if got != want {
		t.Errorf("Warning = %q, want %q", got, want)
	}
	// Trimming the message must not cost the caller the typed warning
	// underneath it.
	var pw gpdf.Warning
	if !errors.As(res.Warnings[0], &pw) {
		t.Fatalf("errors.As did not recover the pdf.Warning from %v", res.Warnings[0])
	}
	if pw.Code != gpdf.WarningOCR || pw.Page != 2 {
		t.Errorf("recovered %+v, want the OCR warning on page 2", pw)
	}
}
