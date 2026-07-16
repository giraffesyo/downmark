package legacydoc

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/giraffesyo/downmark/internal/limitbuf"
)

func TestParseFIBSelectsTableStream(t *testing.T) {
	tests := []struct {
		name      string
		flags     uint16
		wantTable string
	}{
		{name: "zero table", wantTable: "0Table"},
		{name: "one table", flags: 1 << 9, wantTable: "1Table"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := minimalFIB(tt.flags, 1)
			got, err := parseFIB(data)
			if err != nil {
				t.Fatal(err)
			}
			if got.tableName != tt.wantTable {
				t.Errorf("tableName = %q, want %q", got.tableName, tt.wantTable)
			}
		})
	}
}

func TestParseFIBRejectsNegativeCharacterCount(t *testing.T) {
	if _, err := parseFIB(minimalFIB(0, ^uint32(0))); err == nil {
		t.Fatal("parseFIB accepted a negative main-document character count")
	}
}

func minimalFIB(flags uint16, ccpText uint32) []byte {
	const (
		fibRgFcLcbPairs = clxPair + 1
		fcLcbStart      = 32 + 2 + 14*2 + 2 + 22*4 + 2
	)
	data := make([]byte, fcLcbStart+fibRgFcLcbPairs*8)
	binary.LittleEndian.PutUint16(data[0:2], 0xA5EC)
	binary.LittleEndian.PutUint16(data[10:12], flags)
	binary.LittleEndian.PutUint16(data[32:34], 14)
	binary.LittleEndian.PutUint16(data[62:64], 22)
	binary.LittleEndian.PutUint32(data[76:80], ccpText)
	binary.LittleEndian.PutUint16(data[152:154], fibRgFcLcbPairs)
	binary.LittleEndian.PutUint32(data[fcLcbStart+clxPair*8+4:], 1)
	return data
}

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

func TestTextWriterUsesDOCCompressedCharacterMapping(t *testing.T) {
	w := newTextWriter(0)
	if err := w.writePiece(t.Context(), []byte{'A', 0x80, 0x93, 'B'}, true); err != nil {
		t.Fatal(err)
	}
	if got, want := w.String(), "A\u201cB"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestTextWriterDecodesSurrogateAcrossPieces(t *testing.T) {
	w := newTextWriter(0)
	if err := w.writePiece(t.Context(), []byte{0x3D, 0xD8}, false); err != nil {
		t.Fatal(err)
	}
	if err := w.writePiece(t.Context(), []byte{0x80, 0xDE}, false); err != nil {
		t.Fatal(err)
	}
	if err := w.finish(); err != nil {
		t.Fatal(err)
	}
	if got, want := w.String(), "🚀"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestTextWriterLimitsFieldNesting(t *testing.T) {
	w := newTextWriter(0)
	fields := make([]byte, maxFieldNesting+1)
	for i := range fields {
		fields[i] = 0x13
	}
	if err := w.writePiece(t.Context(), fields, true); err == nil {
		t.Fatal("writePiece accepted excessive field nesting")
	}
}

func TestExtractPiecesLimitsFragmentation(t *testing.T) {
	plc := make([]byte, 4+12*(maxPieces+1))
	if _, err := extractPieces(t.Context(), nil, 0, plc, 1, 0); err == nil {
		t.Fatal("extractPieces accepted excessive piece-table fragmentation")
	}
}

func TestExtractPiecesAllowsEmptyDocument(t *testing.T) {
	text, err := extractPieces(t.Context(), nil, 0, make([]byte, 4), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if text != "" {
		t.Errorf("text = %q, want empty", text)
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
