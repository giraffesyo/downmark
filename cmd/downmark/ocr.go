package main

import (
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	gpdf "github.com/giraffesyo/pdf"

	"github.com/giraffesyo/downmark/convert/pdf"
	"github.com/giraffesyo/downmark/ocr"
	ocrexec "github.com/giraffesyo/downmark/ocr/exec"
)

// ocrFlags are the flags that turn on OCR for scanned PDF pages. OCR is
// off unless one of -ocr or -ocr-cmd names an engine: it costs about a
// second a page and needs a binary that is not downmark's to install.
type ocrFlags struct {
	engine      *string
	command     *string
	lang        *string
	policy      *string
	maxPages    *int
	pageTimeout *time.Duration
	timeout     *time.Duration
}

func registerOCRFlags() *ocrFlags {
	return &ocrFlags{
		engine: flag.String("ocr", "", "read scanned PDF pages with `engine` (only \"tesseract\", which must be installed)"),
		command: flag.String("ocr-cmd", "",
			"read scanned PDF pages by running `command`, which is given an image path and writes\n"+
				"tesseract TSV or plain text to stdout; split on spaces, with {} for the image path"),
		lang:        flag.String("ocr-lang", "", "OCR `language`, in tesseract's syntax, e.g. eng or eng+deu"),
		policy:      flag.String("ocr-policy", "textless", "pages to OCR: `textless` for pages with no text of their own, or \"images\" to also read scanned figures on pages that have text"),
		maxPages:    flag.Int("ocr-max-pages", 0, "OCR at most `n` pages per document (0 for no limit)"),
		pageTimeout: flag.Duration("ocr-page-timeout", 2*time.Minute, "give up on one page's OCR after `duration` (0 for no limit)"),
		timeout:     flag.Duration("ocr-timeout", 0, "give up on OCR for the whole document after `duration` (0 for no limit)"),
	}
}

// enabled reports whether any engine was named.
func (f *ocrFlags) enabled() bool {
	return strings.TrimSpace(*f.engine) != "" || strings.TrimSpace(*f.command) != ""
}

// pdfOptions builds the PDF converter's options from the flags,
// returning the zero value when no engine was named.
func (f *ocrFlags) pdfOptions() (pdf.Options, error) {
	if !f.enabled() {
		// The other OCR flags are inert without an engine, and silently
		// ignoring them would hide a command line that does not do what
		// it says.
		if strings.TrimSpace(*f.lang) != "" {
			return pdf.Options{}, errors.New("-ocr-lang needs -ocr or -ocr-cmd")
		}
		return pdf.Options{}, nil
	}
	if strings.TrimSpace(*f.engine) != "" && strings.TrimSpace(*f.command) != "" {
		return pdf.Options{}, errors.New("-ocr and -ocr-cmd name two engines; use one")
	}

	execOpts, err := f.execOptions()
	if err != nil {
		return pdf.Options{}, err
	}
	policy, err := parsePolicy(*f.policy)
	if err != nil {
		return pdf.Options{}, err
	}
	engine, err := ocrexec.New(execOpts)
	if err != nil {
		return pdf.Options{}, err
	}
	return pdf.Options{
		OCR: ocr.Limit(engine, ocr.Limits{
			MaxPages: *f.maxPages,
			PerPage:  *f.pageTimeout,
			Total:    *f.timeout,
		}),
		OCRPolicy: policy,
	}, nil
}

// execOptions resolves the named engine, or the raw command, into the
// command the engine runs.
func (f *ocrFlags) execOptions() (ocrexec.Options, error) {
	if cmd := strings.TrimSpace(*f.command); cmd != "" {
		if strings.TrimSpace(*f.lang) != "" {
			// A bare command's language is whatever its own arguments
			// say; guessing where to insert -l would be wrong as often
			// as right.
			return ocrexec.Options{}, errors.New("-ocr-lang works with -ocr, not -ocr-cmd; pass the language in the command itself")
		}
		fields := strings.Fields(cmd)
		return ocrexec.Options{Name: fields[0], Args: fields[1:]}, nil
	}
	switch name := strings.TrimSpace(*f.engine); name {
	case "tesseract":
		return ocrexec.Tesseract(*f.lang), nil
	default:
		return ocrexec.Options{}, fmt.Errorf("unknown OCR engine %q; use \"tesseract\", or -ocr-cmd to run something else", name)
	}
}

func parsePolicy(s string) (gpdf.OCRPolicy, error) {
	switch strings.TrimSpace(s) {
	case "textless", "":
		return gpdf.OCRTextlessPages, nil
	case "images":
		return gpdf.OCRImagePages, nil
	default:
		return 0, fmt.Errorf("unknown OCR policy %q; use \"textless\" or \"images\"", s)
	}
}
