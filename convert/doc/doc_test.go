package doc_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/all"
	"github.com/giraffesyo/downmark/convert/doc"
)

func TestConvertSyntheticDOCWithoutHints(t *testing.T) {
	res, err := all.Convert(t.Context(), bytes.NewReader(buildSyntheticDOC(false)), downmark.StreamInfo{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	for _, want := range []string{
		"Heading",
		"Hello “legacy” Word",
		"Example link",
		"Unicode 🚀",
	} {
		if !strings.Contains(res.Markdown, want) {
			t.Errorf("Markdown missing %q:\n%s", want, res.Markdown)
		}
	}
	for _, bad := range []string{"HYPERLINK", "https://example.com"} {
		if strings.Contains(res.Markdown, bad) {
			t.Errorf("Markdown contains hidden field instruction %q:\n%s", bad, res.Markdown)
		}
	}
}

func TestConvertGenericOLEWithoutHints(t *testing.T) {
	data := buildSyntheticDOC(false)
	// Without the root CLSID the initial MIME result is generic OLE storage,
	// as it is when a real CFB directory lies beyond the sniff window.
	clear(data[sectorSize+80 : sectorSize+96])
	res, err := all.Convert(t.Context(), bytes.NewReader(data), downmark.StreamInfo{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !strings.Contains(res.Markdown, "Hello “legacy” Word") {
		t.Errorf("Markdown = %q", res.Markdown)
	}
}

func TestConvertSyntheticDOCWithHints(t *testing.T) {
	e := downmark.New()
	doc.Register(e)
	res, err := e.Convert(t.Context(), bytes.NewReader(buildSyntheticDOC(false)), downmark.StreamInfo{
		Extension: ".doc",
		MIMEType:  "application/msword",
	})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !strings.Contains(res.Markdown, "Unicode 🚀") {
		t.Errorf("Markdown = %q", res.Markdown)
	}
}

func TestEncryptedDOCIsRejected(t *testing.T) {
	_, err := all.Convert(t.Context(), bytes.NewReader(buildSyntheticDOC(true)), downmark.StreamInfo{})
	if err == nil || !strings.Contains(err.Error(), "encrypted documents are not supported") {
		t.Fatalf("err = %v, want encrypted-document error", err)
	}
}

func TestDOCResultLimit(t *testing.T) {
	ctx := downmark.WithResultLimit(t.Context(), 16)
	res, err := all.Convert(ctx, bytes.NewReader(buildSyntheticDOC(false)), downmark.StreamInfo{})
	if !errors.Is(err, downmark.ErrResultTooLarge) || res != nil {
		t.Fatalf("err = %v (res=%v), want ErrResultTooLarge", err, res)
	}
}

var benchmarkMarkdown string

func BenchmarkConvertSyntheticDOC(b *testing.B) {
	data := buildSyntheticDOC(false)
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		res, err := all.Convert(b.Context(), bytes.NewReader(data), downmark.StreamInfo{})
		if err != nil {
			b.Fatal(err)
		}
		benchmarkMarkdown = res.Markdown
	}
}

const (
	sectorSize = 512
	endOfChain = uint32(0xFFFFFFFE)
	freeSector = uint32(0xFFFFFFFF)
	fatSector  = uint32(0xFFFFFFFD)
)

// buildSyntheticDOC creates a minimal Word 97 compound file with one ANSI
// piece, one UTF-16 piece, a CLX property record, and a hyperlink field. The
// two streams are deliberately 4096 bytes so the fixture does not need a CFB
// mini-stream.
func buildSyntheticDOC(encrypted bool) []byte {
	const compressedText = "Heading\rHello \x93legacy\x94 Word\r\x13HYPERLINK \"https://example.com\"\x14Example link\x15\r"
	compressed := []byte(compressedText)
	unicodeRunes := utf16.Encode([]rune("Unicode 🚀\r"))
	unicodeBytes := make([]byte, len(unicodeRunes)*2)
	for i, r := range unicodeRunes {
		binary.LittleEndian.PutUint16(unicodeBytes[i*2:], r)
	}

	const (
		wordStartSector  = 1
		tableStartSector = 9
		fatSectorNumber  = 17
		compressedOff    = 1024
		unicodeOff       = 1536
	)
	word := make([]byte, 8*sectorSize)
	copy(word[compressedOff:], compressed)
	copy(word[unicodeOff:], unicodeBytes)

	const (
		compressedCPs = len(compressedText)
		plcSize       = 4*3 + 8*2
		clxSize       = 1 + 2 + 2 + 1 + 4 + plcSize
	)
	cp1 := uint32(compressedCPs)
	cp2 := cp1
	for range unicodeRunes {
		cp2++
	}
	plc := make([]byte, plcSize)
	binary.LittleEndian.PutUint32(plc[0:4], 0)
	binary.LittleEndian.PutUint32(plc[4:8], cp1)
	binary.LittleEndian.PutUint32(plc[8:12], cp2)
	// Pcd.fc stores compressed byte offsets doubled with bit 30 set.
	binary.LittleEndian.PutUint32(plc[14:18], uint32(compressedOff*2)|(1<<30))
	binary.LittleEndian.PutUint32(plc[22:26], unicodeOff)

	clx := []byte{0x01, 0x02, 0x00, 0xAA, 0xBB, 0x02}
	clx = binary.LittleEndian.AppendUint32(clx, plcSize)
	clx = append(clx, plc...)
	table := make([]byte, 8*sectorSize)
	copy(table, clx)

	// FIB base and counted sections.
	binary.LittleEndian.PutUint16(word[0:2], 0xA5EC)
	binary.LittleEndian.PutUint16(word[2:4], 0x00C1)
	binary.LittleEndian.PutUint16(word[6:8], 0x0409)
	flags := uint16((1 << 2) | (1 << 9) | (1 << 12)) // complex, 1Table, extended chars
	if encrypted {
		flags |= 1 << 8
	}
	binary.LittleEndian.PutUint16(word[10:12], flags)
	binary.LittleEndian.PutUint16(word[12:14], 0x00BF)
	binary.LittleEndian.PutUint32(word[24:28], compressedOff)
	binary.LittleEndian.PutUint32(word[28:32], 2048)
	binary.LittleEndian.PutUint16(word[32:34], 14)
	binary.LittleEndian.PutUint16(word[62:64], 22)
	binary.LittleEndian.PutUint32(word[64:68], 2048) // cbMac
	binary.LittleEndian.PutUint32(word[76:80], cp2)  // ccpText
	binary.LittleEndian.PutUint16(word[152:154], 93)
	const fcClxOffset = 154 + 33*8
	binary.LittleEndian.PutUint32(word[fcClxOffset:fcClxOffset+4], 0)
	binary.LittleEndian.PutUint32(word[fcClxOffset+4:fcClxOffset+8], clxSize)

	data := make([]byte, sectorSize*(1+18))
	writeCFBHeader(data[:sectorSize], fatSectorNumber)
	directory := data[sectorSize : 2*sectorSize]
	writeDirectoryEntry(directory[0:128], "Root Entry", 5, freeSector, freeSector, 1, endOfChain, 0)
	copy(directory[80:96], []byte{
		0x06, 0x09, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46,
	}) // Word.Document.8 CLSID, used for content sniffing
	writeDirectoryEntry(directory[128:256], "WordDocument", 2, freeSector, 2, freeSector, wordStartSector, uint64(len(word)))
	writeDirectoryEntry(directory[256:384], "1Table", 2, freeSector, freeSector, freeSector, tableStartSector, uint64(len(table)))
	copy(data[(wordStartSector+1)*sectorSize:], word)
	copy(data[(tableStartSector+1)*sectorSize:], table)

	fat := data[(fatSectorNumber+1)*sectorSize:]
	for i := range sectorSize / 4 {
		binary.LittleEndian.PutUint32(fat[i*4:], freeSector)
	}
	binary.LittleEndian.PutUint32(fat[0*4:], endOfChain)
	for i := uint32(1); i < 8; i++ {
		binary.LittleEndian.PutUint32(fat[i*4:], i+1)
	}
	binary.LittleEndian.PutUint32(fat[8*4:], endOfChain)
	for i := uint32(9); i < 16; i++ {
		binary.LittleEndian.PutUint32(fat[i*4:], i+1)
	}
	binary.LittleEndian.PutUint32(fat[16*4:], endOfChain)
	binary.LittleEndian.PutUint32(fat[fatSectorNumber*4:], fatSector)
	return data
}

func writeCFBHeader(header []byte, fatSectorNumber uint32) {
	copy(header[0:8], []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1})
	binary.LittleEndian.PutUint16(header[24:26], 0x003E)
	binary.LittleEndian.PutUint16(header[26:28], 3)
	binary.LittleEndian.PutUint16(header[28:30], 0xFFFE)
	binary.LittleEndian.PutUint16(header[30:32], 9)
	binary.LittleEndian.PutUint16(header[32:34], 6)
	binary.LittleEndian.PutUint32(header[44:48], 1)
	binary.LittleEndian.PutUint32(header[48:52], 0)
	binary.LittleEndian.PutUint32(header[56:60], 4096)
	binary.LittleEndian.PutUint32(header[60:64], endOfChain)
	binary.LittleEndian.PutUint32(header[68:72], endOfChain)
	for off := 76; off < sectorSize; off += 4 {
		binary.LittleEndian.PutUint32(header[off:], freeSector)
	}
	binary.LittleEndian.PutUint32(header[76:80], fatSectorNumber)
}

func writeDirectoryEntry(entry []byte, name string, objectType byte, left, right, child, start uint32, size uint64) {
	encodedName := utf16.Encode([]rune(name + "\x00"))
	if len(encodedName) > 32 {
		panic("synthetic CFB directory name is too long")
	}
	var nameBytes uint16
	for i, r := range encodedName {
		binary.LittleEndian.PutUint16(entry[i*2:], r)
		nameBytes += 2
	}
	binary.LittleEndian.PutUint16(entry[64:66], nameBytes)
	entry[66] = objectType
	entry[67] = 1
	binary.LittleEndian.PutUint32(entry[68:72], left)
	binary.LittleEndian.PutUint32(entry[72:76], right)
	binary.LittleEndian.PutUint32(entry[76:80], child)
	binary.LittleEndian.PutUint32(entry[116:120], start)
	binary.LittleEndian.PutUint64(entry[120:128], size)
}
