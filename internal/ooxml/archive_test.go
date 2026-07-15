package ooxml

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestValidateXMLDepthLimit(t *testing.T) {
	data := []byte(strings.Repeat("<x>", MaxXMLDepth+1) + strings.Repeat("</x>", MaxXMLDepth+1))
	if err := ValidateXML(t.Context(), data); err == nil || !strings.Contains(err.Error(), "complexity") {
		t.Fatalf("err = %v, want structural-complexity error", err)
	}
}

func TestValidateXMLCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := ValidateXML(ctx, []byte("<x/>")); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
