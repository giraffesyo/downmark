package main

import (
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	gpdf "github.com/giraffesyo/pdf"
	"github.com/giraffesyo/pdf/ocr/tesseract"

	"github.com/giraffesyo/downmark/convert/pdf"
	"github.com/giraffesyo/downmark/ocr"
)

// ocrFlags are the flags that turn on OCR for scanned PDF pages. OCR is
// off unless -ocr names an engine: it costs about a second a page and
// needs a binary that is not downmark's to install.
type ocrFlags struct {
	engine        *string
	binary        *string
	lang          *string
	minConfidence *float64
	policy        *string
	minGlyphs     *int
	maxPages      *int
	pageTimeout   *time.Duration
	timeout       *time.Duration
}

func registerOCRFlags() *ocrFlags {
	return &ocrFlags{
		engine:        flag.String("ocr", "", "read scanned PDF pages with `engine` (only \"tesseract\", which must be installed)"),
		binary:        flag.String("ocr-bin", "", "run this tesseract `executable` instead of the one on PATH"),
		lang:          flag.String("ocr-lang", "", "OCR `language`, in tesseract's syntax, e.g. eng or eng+deu"),
		minConfidence: flag.Float64("ocr-min-confidence", 0, "drop OCR'd words below this `confidence`, on tesseract's 0-100 scale"),
		policy:        flag.String("ocr-policy", "textless", "pages to OCR: `textless` for pages with no text of their own, \"thin\" to also read a scan under a typed header, or \"images\" for every page that paints one"),
		minGlyphs:     flag.Int("ocr-min-glyphs", 0, "with -ocr-policy thin, OCR pages with fewer than `n` glyphs of their own (0 for 50)"),
		maxPages:      flag.Int("ocr-max-pages", 0, "OCR at most `n` pages per document (0 for no limit)"),
		pageTimeout:   flag.Duration("ocr-page-timeout", 2*time.Minute, "give up on one page's OCR after `duration` (0 for no limit)"),
		timeout:       flag.Duration("ocr-timeout", 0, "give up on OCR for the whole document after `duration` (0 for no limit)"),
	}
}

// pdfOptions builds the PDF converter's options from the flags,
// returning the zero value when no engine was named.
func (f *ocrFlags) pdfOptions() (pdf.Options, error) {
	if strings.TrimSpace(*f.engine) == "" {
		// The other OCR flags are inert without an engine, and silently
		// ignoring them would hide a command line that does not do what
		// it says.
		for _, set := range []struct {
			name string
			used bool
		}{
			{"-ocr-bin", strings.TrimSpace(*f.binary) != ""},
			{"-ocr-lang", strings.TrimSpace(*f.lang) != ""},
			{"-ocr-min-confidence", *f.minConfidence != 0},
			{"-ocr-min-glyphs", *f.minGlyphs != 0},
		} {
			if set.used {
				return pdf.Options{}, fmt.Errorf("%s needs -ocr", set.name)
			}
		}
		return pdf.Options{}, nil
	}

	engine, err := f.newEngine()
	if err != nil {
		return pdf.Options{}, err
	}
	policy, thin, err := parsePolicy(*f.policy)
	if err != nil {
		return pdf.Options{}, err
	}
	minGlyphs, err := f.glyphFloor(thin)
	if err != nil {
		return pdf.Options{}, err
	}
	bounded := ocr.Limit(engine, ocr.Limits{
		MaxPages: *f.maxPages,
		PerPage:  *f.pageTimeout,
		Total:    *f.timeout,
	})
	return pdf.Options{OCR: bounded, OCRPolicy: policy, OCRMinGlyphs: minGlyphs}, nil
}

// defaultMinGlyphs is the floor -ocr-policy thin uses when
// -ocr-min-glyphs does not name one. A typed header on a scan — a date
// stamp, a routing line, a page number — runs to a few dozen characters,
// while a page with a text layer of its own runs to hundreds; fifty sits
// between them, and errs towards reading a page's ink twice rather than
// extracting a fax as its header.
const defaultMinGlyphs = 50

// glyphFloor resolves -ocr-min-glyphs. It selects pages only under the
// thin policy, so a command line that sets it under another one is
// asking for a selection it would not get.
func (f *ocrFlags) glyphFloor(thin bool) (int, error) {
	n := *f.minGlyphs
	switch {
	case n < 0:
		return 0, errors.New("-ocr-min-glyphs must not be negative")
	case !thin && n != 0:
		return 0, errors.New("-ocr-min-glyphs needs -ocr-policy thin")
	case !thin:
		return 0, nil
	case n == 0:
		return defaultMinGlyphs, nil
	default:
		return n, nil
	}
}

// newEngine resolves the named engine. The heavy lifting — running the
// binary, mapping its word boxes onto the page — belongs to the
// extractor's own reference implementation, not to downmark.
func (f *ocrFlags) newEngine() (gpdf.OCR, error) {
	switch name := strings.TrimSpace(*f.engine); name {
	case "tesseract":
		engine := &tesseract.Engine{
			Command:       strings.TrimSpace(*f.binary),
			MinConfidence: *f.minConfidence,
		}
		// Languages joins with "+", which is tesseract's own syntax, so
		// a single element carries "eng+deu" through unchanged.
		if lang := strings.TrimSpace(*f.lang); lang != "" {
			engine.Languages = []string{lang}
		}
		return engine, nil
	default:
		return nil, fmt.Errorf("unknown OCR engine %q; only \"tesseract\" is built in", name)
	}
}

// parsePolicy names the pages OCR is asked about. The thin policy is not
// one of the extractor's own: it is a floor on glyphs per page, which the
// converter states as a predicate, so it comes back as a flag rather than
// a gpdf.OCRPolicy.
func parsePolicy(s string) (policy gpdf.OCRPolicy, thin bool, err error) {
	switch strings.TrimSpace(s) {
	case "textless", "":
		return gpdf.OCRTextlessPages, false, nil
	case "thin":
		return gpdf.OCRTextlessPages, true, nil
	case "images":
		return gpdf.OCRImagePages, false, nil
	default:
		return 0, false, fmt.Errorf("unknown OCR policy %q; use \"textless\", \"thin\" or \"images\"", s)
	}
}
