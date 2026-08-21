package main

import (
	"os"
	"strings"
	"testing"
	"time"

	gpdf "github.com/giraffesyo/pdf"
)

// flagsFor builds the parsed flags directly, so that these tests do not
// have to register anything on the global command line.
func flagsFor(engine, command, lang, policy string) *ocrFlags {
	maxPages, pageTimeout, timeout := 0, 2*time.Minute, time.Duration(0)
	return &ocrFlags{
		engine:      &engine,
		command:     &command,
		lang:        &lang,
		policy:      &policy,
		maxPages:    &maxPages,
		pageTimeout: &pageTimeout,
		timeout:     &timeout,
	}
}

// Without an engine the PDF converter has to be left exactly as it was,
// or every conversion pays for a feature nobody asked for.
func TestOCRIsOffUnlessAnEngineIsNamed(t *testing.T) {
	opts, err := flagsFor("", "", "", "textless").pdfOptions()
	if err != nil {
		t.Fatalf("pdfOptions: %v", err)
	}
	if opts.OCR != nil {
		t.Errorf("OCR = %v, want nil", opts.OCR)
	}
}

func TestOCRCommandIsRunnable(t *testing.T) {
	// The test binary is a command that exists on every platform CI
	// runs; nothing here executes it.
	opts, err := flagsFor("", os.Args[0], "", "images").pdfOptions()
	if err != nil {
		t.Fatalf("pdfOptions: %v", err)
	}
	if opts.OCR == nil {
		t.Fatal("OCR = nil, want the named command wired up")
	}
	if opts.OCRPolicy != gpdf.OCRImagePages {
		t.Errorf("OCRPolicy = %v, want the images policy", opts.OCRPolicy)
	}
}

func TestOCRFlagsAreRefusedWhenTheyContradict(t *testing.T) {
	for name, tc := range map[string]struct {
		flags *ocrFlags
		want  string
	}{
		"two engines":          {flagsFor("tesseract", "some-command", "", "textless"), "use one"},
		"unknown engine":       {flagsFor("gocr", "", "", "textless"), "unknown OCR engine"},
		"unknown policy":       {flagsFor("tesseract", "", "", "everything"), "unknown OCR policy"},
		"language without ocr": {flagsFor("", "", "eng", "textless"), "-ocr-lang needs"},
		"language with cmd":    {flagsFor("", os.Args[0], "eng", "textless"), "-ocr-lang works with -ocr"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := tc.flags.pdfOptions()
			if err == nil {
				t.Fatalf("err = nil, want %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestOCRCommandIsSplitOnSpaces(t *testing.T) {
	opts, err := flagsFor("", "  "+os.Args[0]+"  --flag  {}  ", "", "textless").execOptions()
	if err != nil {
		t.Fatalf("execOptions: %v", err)
	}
	if opts.Name != os.Args[0] {
		t.Errorf("Name = %q, want %q", opts.Name, os.Args[0])
	}
	want := []string{"--flag", "{}"}
	if len(opts.Args) != len(want) || opts.Args[0] != want[0] || opts.Args[1] != want[1] {
		t.Errorf("Args = %v, want %v", opts.Args, want)
	}
}

func TestOCRPolicyDefaultsToTextlessPages(t *testing.T) {
	for _, s := range []string{"", "textless", "  textless  "} {
		got, err := parsePolicy(s)
		if err != nil {
			t.Fatalf("parsePolicy(%q): %v", s, err)
		}
		if got != gpdf.OCRTextlessPages {
			t.Errorf("parsePolicy(%q) = %v, want the textless policy", s, got)
		}
	}
}
