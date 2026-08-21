package exec_test

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"os"
	"strings"
	"testing"
	"time"

	gpdf "github.com/giraffesyo/pdf"
	"github.com/giraffesyo/pdf/pdftest"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/convert/pdf"
	"github.com/giraffesyo/downmark/ocr"
	ocrexec "github.com/giraffesyo/downmark/ocr/exec"
)

// helperEnv names the mode the helper process runs in. Re-executing the
// test binary is what makes these tests portable: a shell script would
// not run on the Windows leg of CI, and a real tesseract is not
// something a test can require.
const helperEnv = "DOWNMARK_OCR_TEST_HELPER"

// helperTSV puts two words on one baseline near the top of the image,
// with the gap between their boxes that a real engine leaves where the
// space is, so the layout engine reads them back as one line of two
// words rather than one run-together word.
const helperTSV = "level\tleft\ttop\twidth\theight\tconf\ttext\n" +
	"5\t0\t0\t40\t20\t95.2\tScanned\n" +
	"5\t50\t0\t30\t20\t44.1\tpage\n"

// TestOCRHelperProcess is not a test. It is the OCR engine the tests in
// this file run: with helperEnv set it writes what the mode names and
// exits before the testing package can print anything, so stdout holds
// only what an engine would have written.
func TestOCRHelperProcess(*testing.T) {
	mode := os.Getenv(helperEnv)
	if mode == "" {
		return
	}
	imagePath := os.Args[len(os.Args)-1]
	switch mode {
	case "tsv":
		if _, err := decodeImage(imagePath); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Print(helperTSV)
	case "text":
		fmt.Print("first line\nsecond line\n")
	case "format":
		format, err := decodeImage(imagePath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Println(format)
	case "args":
		fmt.Println(strings.Join(os.Args[1:], "\n"))
	case "fail":
		fmt.Fprintln(os.Stderr, "loading language data")
		fmt.Fprintln(os.Stderr, "could not read the image")
		os.Exit(3)
	case "hang":
		time.Sleep(time.Minute)
	}
	os.Exit(0)
}

// decodeImage reports the format the engine was handed, and fails if it
// was handed something that is not an image at all.
func decodeImage(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // the path is the one this package just wrote
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	_, format, err := image.Decode(f)
	if err != nil {
		return "", fmt.Errorf("engine was handed an unreadable image: %w", err)
	}
	return format, nil
}

// helperOptions points opts at the test binary running in the named
// mode, leaving any placeholder in opts.Args in place.
func helperOptions(t *testing.T, mode string, opts ocrexec.Options) ocrexec.Options {
	t.Helper()
	t.Setenv(helperEnv, mode)
	opts.Name = os.Args[0]
	opts.Args = append([]string{"-test.run=^TestOCRHelperProcess$"}, opts.Args...)
	return opts
}

// convert runs the fixture through an engine holding only the PDF
// converter, so nothing else can claim the input.
func convert(t *testing.T, data []byte, opts pdf.Options) (*downmark.Result, error) {
	t.Helper()
	e := downmark.New(downmark.WithoutBuiltins())
	pdf.Register(e, opts)
	return e.Convert(t.Context(), bytes.NewReader(data), downmark.StreamInfo{Extension: ".pdf"})
}

func newEngine(t *testing.T, opts ocrexec.Options) gpdf.OCR {
	t.Helper()
	engine, err := ocrexec.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return engine
}

// scannedPDF has a typeset first page and a second page painting one
// image over x in [72, 272] and y in [600, 700]. The typeset page keeps
// the conversion successful, so the second page's warnings have
// something to be attached to.
func scannedPDF(imageDict, imageData string) []byte {
	return pdftest.Build(1,
		pdftest.Catalog(2),
		pdftest.Pages(3, 4),
		pdftest.Page(2, 5, "<< /Font << /F1 8 0 R >> >>"),
		pdftest.Page(2, 6, "<< /XObject << /Im0 7 0 R >> >>"),
		pdftest.Stream("", "BT /F1 12 Tf 72 700 Td (Typeset page) Tj ET"),
		pdftest.Stream("", "q 200 0 0 100 72 600 cm /Im0 Do Q"),
		pdftest.Stream(imageDict, imageData),
		pdftest.Helvetica(),
	)
}

// grayScanPDF's image is 100×100 raw greyscale samples, which is enough
// resolution for helperTSV's boxes to describe two separated words.
func grayScanPDF() []byte {
	return scannedPDF(
		"/Type /XObject /Subtype /Image /Width 100 /Height 100 /ColorSpace /DeviceGray /BitsPerComponent 8",
		strings.Repeat("\x40", 100*100))
}

// blankPDF's second page paints nothing at all: no text and no image,
// which is the case an OCR engine cannot help with.
func blankPDF() []byte {
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

func TestEngineReadsAScannedPageThroughTheConverter(t *testing.T) {
	engine := newEngine(t, helperOptions(t, "tsv", ocrexec.Options{Format: ocrexec.FormatTSV}))
	res, err := convert(t, grayScanPDF(), pdf.Options{OCR: engine})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !strings.Contains(res.Markdown, "Typeset page") {
		t.Errorf("Markdown = %q, want the typeset page kept", res.Markdown)
	}
	// Both words share a baseline, so they come back as one line.
	if !strings.Contains(res.Markdown, "Scanned page") {
		t.Errorf("Markdown = %q, want the OCR'd line", res.Markdown)
	}
	// The reader has to be able to tell which text was read off a scan.
	if !strings.Contains(res.Markdown, "<!-- downmark: page 2 includes OCR text -->") {
		t.Errorf("Markdown = %q, want page 2 marked as OCR'd", res.Markdown)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none: the page was read", res.Warnings)
	}
}

func TestEngineDropsWordsBelowTheConfidenceFloor(t *testing.T) {
	engine := newEngine(t, helperOptions(t, "tsv", ocrexec.Options{Format: ocrexec.FormatTSV, MinConfidence: 50}))
	res, err := convert(t, grayScanPDF(), pdf.Options{OCR: engine})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !strings.Contains(res.Markdown, "Scanned") {
		t.Errorf("Markdown = %q, want the confident word", res.Markdown)
	}
	if strings.Contains(res.Markdown, "Scanned page") {
		t.Errorf("Markdown = %q, want the word below the floor dropped", res.Markdown)
	}
}

// An engine that reports text and no geometry still has to land on the
// page, in order, through ocr.Lines.
func TestEngineReadsPlainTextOutput(t *testing.T) {
	engine := newEngine(t, helperOptions(t, "text", ocrexec.Options{Format: ocrexec.FormatText}))
	res, err := convert(t, grayScanPDF(), pdf.Options{OCR: engine})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	first := strings.Index(res.Markdown, "first line")
	second := strings.Index(res.Markdown, "second line")
	if first < 0 || second < 0 {
		t.Fatalf("Markdown = %q, want both lines", res.Markdown)
	}
	if first > second {
		t.Errorf("Markdown = %q, want the lines in the order the engine reported them", res.Markdown)
	}
}

// FormatAuto has to tell the two apart on its own, because -ocr-cmd
// cannot know what the command it was given writes.
func TestEngineAutoDetectsTheOutputFormat(t *testing.T) {
	for mode, want := range map[string]string{"tsv": "Scanned page", "text": "second line"} {
		t.Run(mode, func(t *testing.T) {
			engine := newEngine(t, helperOptions(t, mode, ocrexec.Options{Format: ocrexec.FormatAuto}))
			res, err := convert(t, grayScanPDF(), pdf.Options{OCR: engine})
			if err != nil {
				t.Fatalf("Convert: %v", err)
			}
			if !strings.Contains(res.Markdown, want) {
				t.Errorf("Markdown = %q, want %q", res.Markdown, want)
			}
		})
	}
}

// A page with no images cannot be OCR'd by anything that reads images,
// and saying so is what tells a caller to reach for a renderer instead.
func TestEngineReportsAPageWithNoImages(t *testing.T) {
	engine := newEngine(t, helperOptions(t, "tsv", ocrexec.Options{Format: ocrexec.FormatTSV}))
	res, err := convert(t, blankPDF(), pdf.Options{OCR: engine})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly one", res.Warnings)
	}
	w := res.Warnings[0]
	if w.Location != "page 2" {
		t.Errorf("Warning location = %q, want %q", w.Location, "page 2")
	}
	if !errors.Is(w, ocrexec.ErrNoImages) {
		t.Errorf("Warning = %v, want it to match ErrNoImages through the chain", w)
	}
}

func TestEngineSurfacesTheCommandsOwnMessage(t *testing.T) {
	engine := newEngine(t, helperOptions(t, "fail", ocrexec.Options{Format: ocrexec.FormatTSV}))
	res, err := convert(t, grayScanPDF(), pdf.Options{OCR: engine})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly one", res.Warnings)
	}
	// The last line of stderr is where a command-line tool puts the
	// reason, and repeating it is what makes the warning actionable.
	if got := res.Warnings[0].Error(); !strings.Contains(got, "could not read the image") {
		t.Errorf("Warning = %q, want the command's own message", got)
	}
}

// An already-encoded JPEG is the bulk of a scanned page; decoding and
// re-encoding it would cost more than handing it over as it is.
func TestEnginePassesAnEncodedJPEGStraightThrough(t *testing.T) {
	engine := newEngine(t, helperOptions(t, "format", ocrexec.Options{Format: ocrexec.FormatText}))
	data := scannedPDF(
		"/Type /XObject /Subtype /Image /Width 8 /Height 8 /ColorSpace /DeviceGray /BitsPerComponent 8 /Filter /DCTDecode",
		jpegFixture(t))
	res, err := convert(t, data, pdf.Options{OCR: engine})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !strings.Contains(res.Markdown, "jpeg") {
		t.Errorf("Markdown = %q, want the engine to have been handed a JPEG, not a re-encoded PNG", res.Markdown)
	}
}

// A raw image has no encoding an engine could read, so it is the one
// that must be encoded on the way out.
func TestEngineEncodesARawImageAsPNG(t *testing.T) {
	engine := newEngine(t, helperOptions(t, "format", ocrexec.Options{Format: ocrexec.FormatText}))
	res, err := convert(t, grayScanPDF(), pdf.Options{OCR: engine})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !strings.Contains(res.Markdown, "png") {
		t.Errorf("Markdown = %q, want the raw samples encoded as PNG", res.Markdown)
	}
}

func TestEngineSubstitutesTheImagePlaceholder(t *testing.T) {
	opts := helperOptions(t, "args", ocrexec.Options{
		Format: ocrexec.FormatText,
		// Not dash-prefixed: the helper is a test binary, and the
		// testing package would reject an unknown flag before the
		// helper ran at all.
		Args: []string{"before", ocrexec.ImagePlaceholder, "after"},
	})
	engine := newEngine(t, opts)
	res, err := convert(t, grayScanPDF(), pdf.Options{OCR: engine})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	// The placeholder is replaced where it stands rather than appended,
	// so an argument after it still reaches the command.
	if !strings.Contains(res.Markdown, "after") {
		t.Errorf("Markdown = %q, want the argument following the placeholder", res.Markdown)
	}
	if !strings.Contains(res.Markdown, "downmark-ocr") {
		t.Errorf("Markdown = %q, want the image path in the place the placeholder stood", res.Markdown)
	}
}

// A budget has to stop the command, not just discard what it returns.
func TestPerPageBudgetStopsTheCommand(t *testing.T) {
	engine := newEngine(t, helperOptions(t, "hang", ocrexec.Options{Format: ocrexec.FormatTSV}))
	limited := ocr.Limit(engine, ocr.Limits{PerPage: 50 * time.Millisecond})

	start := time.Now()
	res, err := convert(t, grayScanPDF(), pdf.Options{OCR: limited})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 30*time.Second {
		t.Fatalf("took %s, want the page abandoned at its timeout", elapsed)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want the abandoned page reported", res.Warnings)
	}
	if got := res.Warnings[0].Error(); !strings.Contains(got, "deadline") {
		t.Errorf("Warning = %q, want it to name the deadline rather than a killed process", got)
	}
}

func TestNewReportsAMissingBinaryUpFront(t *testing.T) {
	if _, err := ocrexec.New(ocrexec.Options{Name: "downmark-no-such-ocr-engine"}); err == nil {
		t.Error("err = nil, want a missing binary reported before the conversion starts")
	}
	if _, err := ocrexec.New(ocrexec.Options{}); err == nil {
		t.Error("err = nil, want an unnamed command refused")
	}
}

func TestTesseractOptionsPlaceTheLanguageBeforeTheOutputName(t *testing.T) {
	opts := ocrexec.Tesseract("eng+deu")
	want := []string{"-l", "eng+deu", ocrexec.ImagePlaceholder, "stdout", "tsv"}
	if opts.Name != "tesseract" || len(opts.Args) != len(want) {
		t.Fatalf("Tesseract = %+v, want %v", opts, want)
	}
	for i, arg := range want {
		if opts.Args[i] != arg {
			t.Fatalf("Args = %v, want %v", opts.Args, want)
		}
	}
	if bare := ocrexec.Tesseract(""); bare.Args[0] != ocrexec.ImagePlaceholder {
		t.Errorf("Tesseract(\"\").Args = %v, want no -l at all", bare.Args)
	}
}

// jpegFixture encodes a small greyscale JPEG for embedding as a
// DCTDecode image stream.
func jpegFixture(t *testing.T) string {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, 8, 8))
	var level uint8
	for y := range 8 {
		for x := range 8 {
			img.SetGray(x, y, color.Gray{Y: level})
			level += 16
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encoding the JPEG fixture: %v", err)
	}
	return buf.String()
}
