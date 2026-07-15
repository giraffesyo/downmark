package pdf_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/convert/pdf"
)

func TestMalformedPDFDoesNotPanic(t *testing.T) {
	e := downmark.New(downmark.WithoutBuiltins())
	pdf.Register(e)
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
