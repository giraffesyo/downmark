// Package ocr adapts external OCR engines to the PDF converter's OCR
// seam, so that supplying one is a few lines rather than an
// implementation of positioned glyph geometry.
//
// The seam wants []pdf.Glyph positioned in unrotated page space. Engines
// report word boxes in the pixel space of the image they were given, so
// the work is a coordinate mapping, which Glyphs does through
// pdf.Image.ToPage. An engine that reports no boxes at all is served by
// Lines, which lays its text down the image's own region.
//
// Nothing here execs anything or links an engine: this package is
// stdlib-only, and ocr/exec drives a command-line engine on top of it.
package ocr

import (
	"math"
	"strings"
	"unicode/utf8"

	gpdf "github.com/giraffesyo/pdf"
)

// Word is one unit of text an OCR engine recognized, with its bounding
// box in the pixel space of the image the engine read: Left and Top
// measured from the image's top-left sample, x running right and y
// running down. That is what tesseract's TSV, hOCR's bbox, and most
// other engines report, so a caller usually passes the numbers through
// unchanged.
type Word struct {
	// Text is the recognized text. A Word with no text, or with only
	// space, is dropped rather than placed.
	Text string
	// Left, Top, Width and Height are the box, in image pixels.
	Left, Top, Width, Height float64
	// Conf is the engine's confidence, on whatever scale it reports
	// (tesseract uses 0 to 100, and -1 for a row that is not a word).
	// Nothing here filters on it; it is carried so a caller can.
	Conf float64
}

// Glyphs maps word boxes an engine reported over im into glyphs
// positioned in unrotated page space, ready to return from an
// implementation of pdf.OCR.
//
// Each word becomes one glyph cluster whose baseline runs along the
// bottom edge of its box, so rotation and skew in the way the image was
// painted are preserved rather than assumed away: a page scanned
// sideways lands sideways, and the layout engine reads it in the right
// order. Words with an empty box or no text are dropped, so the result
// can be shorter than words.
func Glyphs(im gpdf.Image, words []Word) []gpdf.Glyph {
	out := make([]gpdf.Glyph, 0, len(words))
	for _, w := range words {
		text := strings.TrimSpace(w.Text)
		if text == "" || w.Width <= 0 || w.Height <= 0 {
			continue
		}
		// The baseline is the box's bottom edge and the ascent reaches
		// its top edge; both are mapped through the image's placement
		// rather than computed in page units, so a rotated or mirrored
		// image needs no special case.
		origin := im.ToPage(w.Left, w.Top+w.Height)
		end := im.ToPage(w.Left+w.Width, w.Top+w.Height)
		top := im.ToPage(w.Left, w.Top)

		dx, dy := end.X-origin.X, end.Y-origin.Y
		advance := math.Hypot(dx, dy)
		ax, ay := top.X-origin.X, top.Y-origin.Y
		size := math.Hypot(ax, ay)
		if advance == 0 || size == 0 {
			// A box that maps to nothing carries no position, and a
			// glyph without one would sort arbitrarily into the page.
			continue
		}
		out = append(out, gpdf.Glyph{
			Text:      text,
			X:         origin.X,
			Y:         origin.Y,
			Advance:   advance,
			Size:      size,
			Direction: gpdf.Point{X: dx / advance, Y: dy / advance},
			Ascent:    gpdf.Point{X: ax, Y: ay},
		})
	}
	return out
}

// Lines places text an engine reported without boxes, by dividing the
// image's region into one evenly spaced band per line. Use it only for
// an engine that reports no geometry: the placement is invented, so the
// lines come out in order and on the right page, but their glyphs do not
// sit where the ink does and column structure is lost.
//
// Empty lines are kept, so that blank space between paragraphs survives
// as a gap rather than closing up.
func Lines(im gpdf.Image, lines []string) []gpdf.Glyph {
	if len(lines) == 0 {
		return nil
	}
	band := float64(max(im.Height, 1)) / float64(len(lines))
	words := make([]Word, 0, len(lines))
	for i, line := range lines {
		text := strings.TrimSpace(line)
		if text == "" {
			continue
		}
		// Leave a fifth of the band as leading so consecutive baselines
		// stay far enough apart for the layout engine to read them as
		// separate lines.
		height := band * 0.8
		// Without metrics the width is a guess; half the line height per
		// character is close enough for proportional text, and clamping
		// keeps the run inside the image it came from.
		width := math.Min(height*0.5*float64(utf8.RuneCountInString(text)), float64(max(im.Width, 1)))
		words = append(words, Word{
			Text:   text,
			Left:   0,
			Top:    float64(i)*band + band*0.2,
			Width:  width,
			Height: height,
		})
	}
	return Glyphs(im, words)
}
