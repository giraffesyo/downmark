package main

import (
	"strings"
	"testing"
	"time"

	gpdf "github.com/giraffesyo/pdf"
	"github.com/giraffesyo/pdf/ocr/tesseract"
)

// flagsFor builds the parsed flags directly, so that these tests do not
// have to register anything on the global command line.
func flagsFor(engine, binary, lang, policy string, minConfidence float64) *ocrFlags {
	maxPages, pageTimeout, timeout := 0, 2*time.Minute, time.Duration(0)
	minGlyphs := 0
	return &ocrFlags{
		engine:        &engine,
		binary:        &binary,
		lang:          &lang,
		minConfidence: &minConfidence,
		policy:        &policy,
		minGlyphs:     &minGlyphs,
		maxPages:      &maxPages,
		pageTimeout:   &pageTimeout,
		timeout:       &timeout,
	}
}

// withMinGlyphs sets -ocr-min-glyphs on flags flagsFor built, which
// leaves it at its "not given" zero.
func withMinGlyphs(f *ocrFlags, n int) *ocrFlags {
	f.minGlyphs = &n
	return f
}

// Without an engine the PDF converter has to be left exactly as it was,
// or every conversion pays for a feature nobody asked for.
func TestOCRIsOffUnlessAnEngineIsNamed(t *testing.T) {
	opts, err := flagsFor("", "", "", "textless", 0).pdfOptions()
	if err != nil {
		t.Fatalf("pdfOptions: %v", err)
	}
	if opts.OCR != nil {
		t.Errorf("OCR = %v, want nil", opts.OCR)
	}
}

// Nothing is executed here: building the engine only describes the
// command, so these tests do not need tesseract installed.
func TestOCRTesseractIsWiredUp(t *testing.T) {
	opts, err := flagsFor("tesseract", "", "eng+deu", "images", 60).pdfOptions()
	if err != nil {
		t.Fatalf("pdfOptions: %v", err)
	}
	if opts.OCR == nil {
		t.Fatal("OCR = nil, want the engine wired up")
	}
	if opts.OCRPolicy != gpdf.OCRImagePages {
		t.Errorf("OCRPolicy = %v, want the images policy", opts.OCRPolicy)
	}
	if opts.OCRMinGlyphs != 0 {
		t.Errorf("OCRMinGlyphs = %d, want none outside the thin policy", opts.OCRMinGlyphs)
	}
}

// The thin policy is a glyph floor rather than one of the extractor's
// own policies, so what it has to reach the converter as is a count.
func TestOCRThinPolicyCarriesItsGlyphFloor(t *testing.T) {
	for name, tc := range map[string]struct {
		flags *ocrFlags
		want  int
	}{
		"floor given":   {withMinGlyphs(flagsFor("tesseract", "", "", "thin", 0), 120), 120},
		"floor omitted": {flagsFor("tesseract", "", "", "thin", 0), defaultMinGlyphs},
	} {
		t.Run(name, func(t *testing.T) {
			opts, err := tc.flags.pdfOptions()
			if err != nil {
				t.Fatalf("pdfOptions: %v", err)
			}
			if opts.OCRMinGlyphs != tc.want {
				t.Errorf("OCRMinGlyphs = %d, want %d", opts.OCRMinGlyphs, tc.want)
			}
		})
	}
}

func TestOCRFlagsAreRefusedWhenTheyContradict(t *testing.T) {
	for name, tc := range map[string]struct {
		flags *ocrFlags
		want  string
	}{
		"unknown engine":         {flagsFor("gocr", "", "", "textless", 0), "unknown OCR engine"},
		"unknown policy":         {flagsFor("tesseract", "", "", "everything", 0), "unknown OCR policy"},
		"binary without ocr":     {flagsFor("", "/usr/bin/tesseract", "", "textless", 0), "-ocr-bin needs -ocr"},
		"language without ocr":   {flagsFor("", "", "eng", "textless", 0), "-ocr-lang needs -ocr"},
		"confidence without ocr": {flagsFor("", "", "", "textless", 60), "-ocr-min-confidence needs -ocr"},
		"glyph floor without ocr": {
			withMinGlyphs(flagsFor("", "", "", "textless", 0), 40),
			"-ocr-min-glyphs needs -ocr",
		},
		// Setting a floor the policy will not consult is a command line
		// that does not do what it says, the same as the flags above.
		"glyph floor without the thin policy": {
			withMinGlyphs(flagsFor("tesseract", "", "", "images", 0), 40),
			"-ocr-min-glyphs needs -ocr-policy thin",
		},
		"negative glyph floor": {
			withMinGlyphs(flagsFor("tesseract", "", "", "thin", 0), -1),
			"-ocr-min-glyphs must not be negative",
		},
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

func TestOCRPolicyDefaultsToTextlessPages(t *testing.T) {
	for _, s := range []string{"", "textless", "  textless  "} {
		got, thin, err := parsePolicy(s)
		if err != nil {
			t.Fatalf("parsePolicy(%q): %v", s, err)
		}
		if got != gpdf.OCRTextlessPages || thin {
			t.Errorf("parsePolicy(%q) = %v, thin %v, want the textless policy", s, got, thin)
		}
	}
}

// tesseract's own syntax for several languages is "eng+deu", and
// Engine.Languages joins its elements with the same "+", so passing the
// flag through as one element has to come out unchanged.
func TestOCRLanguagePassesThroughUnchanged(t *testing.T) {
	engine, err := flagsFor("tesseract", "", "eng+deu", "textless", 0).newEngine()
	if err != nil {
		t.Fatalf("newEngine: %v", err)
	}
	tess, ok := engine.(*tesseract.Engine)
	if !ok {
		t.Fatalf("engine = %T, want *tesseract.Engine", engine)
	}
	if len(tess.Languages) != 1 || tess.Languages[0] != "eng+deu" {
		t.Errorf("Languages = %v, want [eng+deu]", tess.Languages)
	}
}
