package ocr_test

import (
	"bytes"
	"math"
	"strings"
	"testing"

	gpdf "github.com/giraffesyo/pdf"
	"github.com/giraffesyo/pdf/pdftest"

	"github.com/giraffesyo/downmark/ocr"
)

// scannedPDF paints one 4×4 image over a known region: the content
// stream's matrix maps the image's unit square to x in [72, 272] and y
// in [600, 700], so every mapping checked below can be worked out by
// hand. Page space puts y at the bottom, so the image's top row lands at
// y = 700.
func scannedPDF() []byte {
	pixels := strings.Repeat("\x40", 4*4) // mid grey, 8 bits per component
	return pdftest.Build(1,
		pdftest.Catalog(2),
		pdftest.Pages(3),
		pdftest.Page(2, 4, "<< /XObject << /Im0 5 0 R >> >>"),
		pdftest.Stream("", "q 200 0 0 100 72 600 cm /Im0 Do Q"),
		pdftest.Stream("/Type /XObject /Subtype /Image /Width 4 /Height 4 /ColorSpace /DeviceGray /BitsPerComponent 8", pixels),
	)
}

func pageImage(t *testing.T) gpdf.Image {
	t.Helper()
	data := scannedPDF()
	doc, err := gpdf.ExtractWithOptions(t.Context(), bytes.NewReader(data), int64(len(data)), gpdf.Options{IncludeImages: true})
	if err != nil {
		t.Fatalf("ExtractWithOptions: %v", err)
	}
	if len(doc.Pages) != 1 || len(doc.Pages[0].Images) != 1 {
		t.Fatalf("fixture yielded %d pages, want one page painting one image", len(doc.Pages))
	}
	return doc.Pages[0].Images[0]
}

func near(got, want float64) bool { return math.Abs(got-want) < 1e-6 }

func TestGlyphsMapImagePixelsToPageSpace(t *testing.T) {
	im := pageImage(t)
	// The top half of the image, full width, and then a half-width box
	// in the bottom half: two lines of a scan.
	glyphs := ocr.Glyphs(im, []ocr.Word{
		{Text: "Header", Left: 0, Top: 0, Width: 4, Height: 2},
		{Text: "Body", Left: 0, Top: 2, Width: 2, Height: 2},
	})
	if len(glyphs) != 2 {
		t.Fatalf("Glyphs = %d, want 2", len(glyphs))
	}

	want := []gpdf.Glyph{
		{Text: "Header", X: 72, Y: 650, Advance: 200, Size: 50, Direction: gpdf.Point{X: 1}, Ascent: gpdf.Point{Y: 50}},
		{Text: "Body", X: 72, Y: 600, Advance: 100, Size: 50, Direction: gpdf.Point{X: 1}, Ascent: gpdf.Point{Y: 50}},
	}
	for i, w := range want {
		g := glyphs[i]
		if g.Text != w.Text {
			t.Errorf("glyph %d Text = %q, want %q", i, g.Text, w.Text)
		}
		if !near(g.X, w.X) || !near(g.Y, w.Y) {
			t.Errorf("glyph %d origin = (%g, %g), want (%g, %g)", i, g.X, g.Y, w.X, w.Y)
		}
		if !near(g.Advance, w.Advance) {
			t.Errorf("glyph %d Advance = %g, want %g", i, g.Advance, w.Advance)
		}
		if !near(g.Size, w.Size) {
			t.Errorf("glyph %d Size = %g, want %g", i, g.Size, w.Size)
		}
		if !near(g.Direction.X, w.Direction.X) || !near(g.Direction.Y, w.Direction.Y) {
			t.Errorf("glyph %d Direction = %+v, want %+v", i, g.Direction, w.Direction)
		}
		if !near(g.Ascent.X, w.Ascent.X) || !near(g.Ascent.Y, w.Ascent.Y) {
			t.Errorf("glyph %d Ascent = %+v, want %+v", i, g.Ascent, w.Ascent)
		}
	}
}

// A glyph the layout engine cannot place would sort arbitrarily into the
// page, which is worse than dropping it.
func TestGlyphsDropsWordsItCannotPlace(t *testing.T) {
	im := pageImage(t)
	glyphs := ocr.Glyphs(im, []ocr.Word{
		{Text: "", Left: 0, Top: 0, Width: 4, Height: 2},
		{Text: "   ", Left: 0, Top: 0, Width: 4, Height: 2},
		{Text: "zero width", Left: 0, Top: 0, Width: 0, Height: 2},
		{Text: "zero height", Left: 0, Top: 0, Width: 4, Height: 0},
		{Text: "kept", Left: 0, Top: 0, Width: 4, Height: 2},
	})
	if len(glyphs) != 1 || glyphs[0].Text != "kept" {
		t.Fatalf("Glyphs = %+v, want only the placeable word", glyphs)
	}
}

func TestGlyphsTrimsSurroundingSpace(t *testing.T) {
	im := pageImage(t)
	glyphs := ocr.Glyphs(im, []ocr.Word{{Text: "  padded\n", Left: 0, Top: 0, Width: 4, Height: 2}})
	if len(glyphs) != 1 || glyphs[0].Text != "padded" {
		t.Fatalf("Glyphs = %+v, want the text trimmed", glyphs)
	}
}

// Lines invents geometry, so what matters is that the order survives and
// the baselines walk down the page far enough apart to read as lines.
func TestLinesWalkDownThePageInOrder(t *testing.T) {
	im := pageImage(t)
	glyphs := ocr.Lines(im, []string{"first", "", "third"})
	if len(glyphs) != 2 {
		t.Fatalf("Lines = %d glyphs, want 2: the blank line is a gap, not a line", len(glyphs))
	}
	if glyphs[0].Text != "first" || glyphs[1].Text != "third" {
		t.Fatalf("Lines = %q then %q, want the source order", glyphs[0].Text, glyphs[1].Text)
	}
	// The blank line between them leaves a gap: the third line sits two
	// bands down, not one.
	if glyphs[0].Y <= glyphs[1].Y {
		t.Errorf("baselines = %g then %g, want the second lower down the page", glyphs[0].Y, glyphs[1].Y)
	}
	for i, g := range glyphs {
		if g.Advance <= 0 || g.Size <= 0 {
			t.Errorf("glyph %d = %+v, want a positive advance and size", i, g)
		}
		if g.X < 72 || g.X > 272 || g.Y < 600 || g.Y > 700 {
			t.Errorf("glyph %d origin = (%g, %g), want it inside the image's region", i, g.X, g.Y)
		}
	}
}

func TestLinesWithoutLines(t *testing.T) {
	if got := ocr.Lines(pageImage(t), nil); got != nil {
		t.Errorf("Lines = %+v, want nil", got)
	}
}
