package legacydoc

import (
	"errors"
	"strings"
	"testing"

	"github.com/giraffesyo/downmark/internal/limitbuf"
)

func TestTextWriterFiltersNestedFieldsAcrossPieces(t *testing.T) {
	w := newTextWriter(0)
	parts := [][]byte{
		[]byte("before \x13HYPERLINK \"https://example.com\""),
		[]byte("\x14label \x13REF hidden\x14nested\x15 end\x15 after\r"),
	}
	for _, part := range parts {
		if err := w.writePiece(t.Context(), part, true); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := w.String(), "before label nested end after\n"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if strings.Contains(w.String(), "HYPERLINK") || strings.Contains(w.String(), "REF hidden") {
		t.Errorf("field instructions leaked into text: %q", w.String())
	}
}

func TestTextWriterHonorsLimit(t *testing.T) {
	w := newTextWriter(4)
	err := w.writePiece(t.Context(), []byte("12345"), true)
	if !errors.Is(err, limitbuf.ErrTooLarge) {
		t.Fatalf("err = %v, want limitbuf.ErrTooLarge", err)
	}
}

func TestFindPlcPcdSkipsPropertyRecords(t *testing.T) {
	clx := []byte{0x01, 0x02, 0x00, 0xAA, 0xBB, 0x02, 0x04, 0x00, 0x00, 0x00, 1, 2, 3, 4}
	got, err := findPlcPcd(clx)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string([]byte{1, 2, 3, 4}) {
		t.Errorf("PlcPcd = %v", got)
	}
}

func TestFindPlcPcdRejectsTruncatedRecord(t *testing.T) {
	if _, err := findPlcPcd([]byte{0x01, 0x05, 0x00, 0xAA}); err == nil {
		t.Fatal("findPlcPcd accepted a truncated property record")
	}
}
