// Package exec drives a command-line OCR engine over the PDF converter's
// OCR seam: it writes each selected page's images to a temporary file,
// runs the command on them, and maps what the command reports back onto
// the page.
//
// tesseract is the engine this is shaped around — Tesseract returns
// options that drive it — but any command that reads an image path and
// writes tesseract-style TSV or plain text to stdout works.
//
// This package is not part of downmark's default engine and nothing
// imports it: OCR is a choice a caller makes, and making it links an
// external binary into the deployment.
package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"

	gpdf "github.com/giraffesyo/pdf"

	"github.com/giraffesyo/downmark/ocr"
)

// ImagePlaceholder is replaced in Options.Args with the path to the page
// image. Args carrying no placeholder get the path appended instead, so
// the common `engine <image>` shape needs no ceremony.
const ImagePlaceholder = "{}"

// ErrNoImages is returned for a page the policy selected that paints no
// images. Nothing can be read from it here: its text was almost
// certainly converted to vector outlines, which needs a renderer rather
// than an OCR engine. It reaches the caller as a warning on that page,
// so match it with errors.Is to tell that case apart from a scan this
// engine merely failed on.
// The message carries no package prefix for the reason ocr.ErrBudget's
// does not: the warning wrapping it is labelled "ocr" already.
var ErrNoImages = errors.New("page paints no images to read")

// Format says how to read the command's stdout.
type Format int

const (
	// FormatAuto reads TSV when stdout looks like tesseract's TSV, and
	// plain text otherwise.
	FormatAuto Format = iota
	// FormatTSV reads tesseract's TSV, and fails on anything else.
	FormatTSV
	// FormatText reads plain text. Its lines are placed evenly down the
	// image rather than where the ink is: see ocr.Lines.
	FormatText
)

// Options configures the command an engine runs.
type Options struct {
	// Name is the executable, looked up in PATH by New.
	Name string
	// Args are its arguments. See ImagePlaceholder.
	Args []string
	// Format says how to read stdout.
	Format Format
	// MinConfidence drops words the engine was less sure of than this,
	// on the engine's own scale (tesseract reports 0 to 100). It applies
	// only to a format that carries confidence, so FormatText ignores
	// it. Zero keeps everything.
	MinConfidence float64
}

// Tesseract returns Options driving the tesseract binary, reading TSV
// from stdout. lang is passed to -l and may be empty for tesseract's
// default; it accepts tesseract's own syntax, so "eng+deu" works.
func Tesseract(lang string) Options {
	args := []string{ImagePlaceholder, "stdout", "tsv"}
	if lang = strings.TrimSpace(lang); lang != "" {
		// -l precedes the positional arguments: tesseract rejects
		// configuration options placed after the output base name.
		args = append([]string{"-l", lang}, args...)
	}
	return Options{Name: "tesseract", Args: args, Format: FormatTSV}
}

// New returns an OCR engine running the configured command, after
// checking that it exists: a missing binary is worth reporting before a
// conversion starts rather than once per scanned page.
func New(opts Options) (gpdf.OCR, error) {
	if strings.TrimSpace(opts.Name) == "" {
		return nil, errors.New("ocr/exec: no command named")
	}
	path, err := osexec.LookPath(opts.Name)
	if err != nil {
		return nil, fmt.Errorf("ocr/exec: %w", err)
	}
	opts.Name = path
	return engine{opts: opts}, nil
}

type engine struct {
	opts Options
}

// ExtractPage runs the command once per image the page paints and
// returns every word they yielded, positioned on the page.
//
// Images are read independently, so a page scanned as several strips
// still comes out in reading order — the glyphs carry page coordinates,
// and the layout engine sorts them. A line of text split across two
// strips is the case that loses, coming out as two lines.
//
// Glyphs and an error are returned together when some images were read
// and others were not, which the seam keeps: the page gets the text that
// was recovered and a warning about the rest.
func (e engine) ExtractPage(ctx context.Context, req gpdf.OCRRequest) ([]gpdf.Glyph, error) {
	if len(req.Page.Images) == 0 {
		return nil, ErrNoImages
	}
	var (
		glyphs []gpdf.Glyph
		errs   []error
	)
	for i, im := range req.Page.Images {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		read, err := e.readImage(ctx, im)
		if err != nil {
			errs = append(errs, fmt.Errorf("image %d: %w", i+1, err))
			continue
		}
		glyphs = append(glyphs, read...)
	}
	return glyphs, errors.Join(errs...)
}

// readImage writes im where the command can reach it, runs the command,
// and maps what it recognized onto the page.
func (e engine) readImage(ctx context.Context, im gpdf.Image) ([]gpdf.Glyph, error) {
	path, cleanup, err := writeImage(im)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	out, err := e.run(ctx, path)
	if err != nil {
		return nil, err
	}
	words, positioned, err := e.parse(out)
	if err != nil {
		return nil, err
	}
	if !positioned {
		// The command reported text and no geometry, so the only
		// placement available is the invented one.
		lines := make([]string, len(words))
		for i, w := range words {
			lines[i] = w.Text
		}
		return ocr.Lines(im, lines), nil
	}
	if e.opts.MinConfidence > 0 {
		kept := words[:0]
		for _, w := range words {
			if w.Conf >= e.opts.MinConfidence {
				kept = append(kept, w)
			}
		}
		words = kept
	}
	return ocr.Glyphs(im, words), nil
}

// run executes the command against the image at path and returns its
// standard output.
func (e engine) run(ctx context.Context, path string) ([]byte, error) {
	args := make([]string, 0, len(e.opts.Args)+1)
	substituted := false
	for _, arg := range e.opts.Args {
		if strings.Contains(arg, ImagePlaceholder) {
			arg = strings.ReplaceAll(arg, ImagePlaceholder, path)
			substituted = true
		}
		args = append(args, arg)
	}
	if !substituted {
		args = append(args, path)
	}

	// Running the command the caller named is what this package is for;
	// the name reached it from a flag or an Options literal, both of
	// which are the caller's own input rather than the document's.
	cmd := osexec.CommandContext(ctx, e.opts.Name, args...) //nolint:gosec // the command is the caller's, never the input's
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		// The context's own error is the useful one when a budget
		// stopped the command: "signal: killed" says nothing.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if detail := lastLine(stderr.String()); detail != "" {
			return nil, fmt.Errorf("%s: %w: %s", filepath.Base(e.opts.Name), err, detail)
		}
		return nil, fmt.Errorf("%s: %w", filepath.Base(e.opts.Name), err)
	}
	return stdout.Bytes(), nil
}

// parse reads the command's output in the configured format, reporting
// whether the words it returns carry boxes.
func (e engine) parse(out []byte) (words []ocr.Word, positioned bool, err error) {
	switch e.opts.Format {
	case FormatTSV:
		words, err = ocr.ParseTesseractTSV(bytes.NewReader(out))
		return words, true, err
	case FormatText:
		return textWords(out), false, nil
	default:
		if words, err = ocr.ParseTesseractTSV(bytes.NewReader(out)); err == nil {
			return words, true, nil
		}
		return textWords(out), false, nil
	}
}

// textWords carries plain-text lines through as words with no boxes;
// placing them is left to ocr.Lines, which needs the image.
func textWords(out []byte) []ocr.Word {
	lines := strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
	words := make([]ocr.Word, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			words = append(words, ocr.Word{Text: line})
		}
	}
	return words
}

// writeImage puts the image somewhere the command can open it, passing
// an already-encoded JPEG through untouched rather than decoding and
// re-encoding a scan that is usually the largest thing on the page.
func writeImage(im gpdf.Image) (path string, cleanup func(), err error) {
	ext, data := ".png", []byte(nil)
	if im.Filter == "DCTDecode" {
		ext, data = ".jpg", im.Data
	}
	f, err := os.CreateTemp("", "downmark-ocr-*"+ext)
	if err != nil {
		return "", nil, err
	}
	path = f.Name()
	cleanup = func() {
		_ = f.Close() // already closed on the success path
		_ = os.Remove(path)
	}
	defer func() {
		if err != nil {
			cleanup()
			path, cleanup = "", nil
		}
	}()

	if data != nil {
		if _, err = f.Write(data); err != nil {
			return "", nil, err
		}
		return path, cleanup, f.Close()
	}
	img, err := im.Decode()
	if err != nil {
		return "", nil, fmt.Errorf("decoding page image: %w", err)
	}
	if err = png.Encode(f, img); err != nil {
		return "", nil, err
	}
	return path, cleanup, f.Close()
}

// lastLine returns the final non-empty line of s, which is where a
// command-line tool usually puts the reason it failed.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}
