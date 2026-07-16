// Package legacydoc extracts text from Word 97-2003 binary (.doc) files.
package legacydoc

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"

	"github.com/richardlehane/mscfb"
	"golang.org/x/text/encoding/charmap"

	"github.com/giraffesyo/downmark/internal/limitbuf"
)

const (
	maxFIBBytes = 4096
	clxPair     = 33 // zero-based FibRgFcLcb97 fcClx/lcbClx pair
	pcdSize     = 8
)

var errNoText = errors.New("legacy DOC contains no extractable text")

// Convert extracts the main-document text from a Word binary file. ra is
// bounded to size before it is handed to the compound-file reader.
func Convert(ctx context.Context, ra io.ReaderAt, size int64, outputLimit int) (string, error) {
	if size < 512 {
		return "", errors.New("legacy DOC: compound file is too small")
	}
	cfb, err := mscfb.New(io.NewSectionReader(ra, 0, size))
	if err != nil {
		return "", fmt.Errorf("legacy DOC: opening compound file: %w", err)
	}

	word := findRootStream(cfb, "WordDocument")
	if word == nil {
		return "", errors.New("legacy DOC: WordDocument stream is missing")
	}
	fibData, err := readStreamRange(word, 0, min(word.Size, maxFIBBytes))
	if err != nil {
		return "", fmt.Errorf("legacy DOC: reading FIB: %w", err)
	}
	fib, err := parseFIB(fibData)
	if err != nil {
		return "", fmt.Errorf("legacy DOC: %w", err)
	}
	if fib.encrypted {
		return "", errors.New("legacy DOC: encrypted documents are not supported")
	}

	table := findRootStream(cfb, fib.tableName)
	if table == nil {
		return "", fmt.Errorf("legacy DOC: %s stream is missing", fib.tableName)
	}
	clx, err := readStreamRange(table, int64(fib.fcClx), int64(fib.lcbClx))
	if err != nil {
		return "", fmt.Errorf("legacy DOC: reading CLX: %w", err)
	}
	plc, err := findPlcPcd(clx)
	if err != nil {
		return "", fmt.Errorf("legacy DOC: %w", err)
	}

	meaningfulBytes := word.Size
	if fib.cbMac != 0 {
		if int64(fib.cbMac) > word.Size {
			return "", fmt.Errorf("legacy DOC: FIB cbMac %d exceeds WordDocument stream size %d", fib.cbMac, word.Size)
		}
		meaningfulBytes = int64(fib.cbMac)
	}
	text, err := extractPieces(ctx, word, meaningfulBytes, plc, fib.ccpText, outputLimit)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(text) == "" {
		return "", errNoText
	}
	return text, nil
}

type fib struct {
	tableName string
	encrypted bool
	cbMac     uint32
	ccpText   int64
	fcClx     uint32
	lcbClx    uint32
}

func parseFIB(data []byte) (fib, error) {
	if len(data) < 32 {
		return fib{}, errors.New("truncated FIB base")
	}
	if got := binary.LittleEndian.Uint16(data[0:2]); got != 0xA5EC {
		return fib{}, fmt.Errorf("invalid FIB signature %#04x", got)
	}

	flags := binary.LittleEndian.Uint16(data[10:12])
	f := fib{
		tableName: "0Table",
		encrypted: flags&(1<<8) != 0,
	}
	if flags&(1<<9) != 0 {
		f.tableName = "1Table"
	}

	off := 32
	csw, next, err := countedSection(data, off, 2)
	if err != nil {
		return fib{}, fmt.Errorf("invalid FibRgW: %w", err)
	}
	off = next
	if csw < 14 {
		return fib{}, fmt.Errorf("FibRgW has %d values; need at least 14", csw)
	}

	cslw, lwStart, next, err := countedSectionStart(data, off, 4)
	if err != nil {
		return fib{}, fmt.Errorf("invalid FibRgLw: %w", err)
	}
	if cslw < 4 {
		return fib{}, fmt.Errorf("FibRgLw has %d values; need at least 4", cslw)
	}
	f.cbMac = binary.LittleEndian.Uint32(data[lwStart : lwStart+4])
	ccpText := int32(binary.LittleEndian.Uint32(data[lwStart+12 : lwStart+16]))
	if ccpText < 0 {
		return fib{}, fmt.Errorf("negative main-document character count %d", ccpText)
	}
	f.ccpText = int64(ccpText)
	off = next

	pairs, fcLcbStart, _, err := countedSectionStart(data, off, 8)
	if err != nil {
		return fib{}, fmt.Errorf("invalid FibRgFcLcb: %w", err)
	}
	if pairs <= clxPair {
		return fib{}, fmt.Errorf("FibRgFcLcb has %d pairs; fcClx is unavailable", pairs)
	}
	clxOff := fcLcbStart + clxPair*8
	f.fcClx = binary.LittleEndian.Uint32(data[clxOff : clxOff+4])
	f.lcbClx = binary.LittleEndian.Uint32(data[clxOff+4 : clxOff+8])
	if f.lcbClx == 0 {
		return fib{}, errors.New("CLX is empty")
	}
	return f, nil
}

func countedSection(data []byte, off, width int) (count, next int, err error) {
	count, _, next, err = countedSectionStart(data, off, width)
	return count, next, err
}

func countedSectionStart(data []byte, off, width int) (count, start, next int, err error) {
	if off < 0 || off+2 > len(data) {
		return 0, 0, 0, io.ErrUnexpectedEOF
	}
	count = int(binary.LittleEndian.Uint16(data[off : off+2]))
	start = off + 2
	if count > (len(data)-start)/width {
		return 0, 0, 0, io.ErrUnexpectedEOF
	}
	next = start + count*width
	return count, start, next, nil
}

func findPlcPcd(clx []byte) ([]byte, error) {
	for off := 0; off < len(clx); {
		switch clx[off] {
		case 0x01: // Prc: clxt, cbGrpprl, grpprl
			if off+3 > len(clx) {
				return nil, errors.New("truncated CLX property record")
			}
			size := int(binary.LittleEndian.Uint16(clx[off+1 : off+3]))
			if size > len(clx)-(off+3) {
				return nil, errors.New("CLX property record exceeds stream")
			}
			off += 3 + size
		case 0x02: // Pcdt: clxt, lcb, PlcPcd
			if off+5 > len(clx) {
				return nil, errors.New("truncated Pcdt header")
			}
			size := int64(binary.LittleEndian.Uint32(clx[off+1 : off+5]))
			if size > int64(len(clx)-(off+5)) {
				return nil, errors.New("PlcPcd exceeds CLX stream")
			}
			return clx[off+5 : off+5+int(size)], nil
		default:
			return nil, fmt.Errorf("invalid CLX record type %#02x", clx[off])
		}
	}
	return nil, errors.New("CLX contains no Pcdt")
}

func extractPieces(ctx context.Context, word *mscfb.File, meaningfulBytes int64, plc []byte, ccpText int64, outputLimit int) (string, error) {
	if len(plc) < 4 || (len(plc)-4)%(4+pcdSize) != 0 {
		return "", fmt.Errorf("legacy DOC: invalid PlcPcd size %d", len(plc))
	}
	pieceCount := (len(plc) - 4) / (4 + pcdSize)
	cpBytes := (pieceCount + 1) * 4
	if cpBytes > len(plc) {
		return "", errors.New("legacy DOC: truncated PlcPcd character positions")
	}
	if ccpText == 0 {
		return "", errNoText
	}

	firstCP, err := readCP(plc, 0)
	if err != nil || firstCP != 0 {
		return "", fmt.Errorf("legacy DOC: PlcPcd begins at character position %d; want 0", firstCP)
	}
	lastCP, err := readCP(plc, pieceCount)
	if err != nil || lastCP < ccpText {
		return "", fmt.Errorf("legacy DOC: PlcPcd ends at character position %d before main document ends at %d", lastCP, ccpText)
	}

	w := newTextWriter(outputLimit)
	previousCP := int64(-1)
	for i := 0; i < pieceCount; i++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		startCP, err := readCP(plc, i)
		if err != nil {
			return "", fmt.Errorf("legacy DOC: reading piece %d start: %w", i, err)
		}
		endCP, err := readCP(plc, i+1)
		if err != nil {
			return "", fmt.Errorf("legacy DOC: reading piece %d end: %w", i, err)
		}
		if startCP <= previousCP || endCP <= startCP {
			return "", fmt.Errorf("legacy DOC: piece %d has invalid character range %d..%d", i, startCP, endCP)
		}
		previousCP = startCP
		if startCP >= ccpText {
			break
		}
		endCP = min(endCP, ccpText)
		charCount := endCP - startCP

		pcdOff := cpBytes + i*pcdSize
		fcCompressed := binary.LittleEndian.Uint32(plc[pcdOff+2 : pcdOff+6])
		compressed := fcCompressed&(1<<30) != 0
		fc := int64(fcCompressed & 0x3FFFFFFF)
		bytesPerChar := int64(2)
		if compressed {
			if fc&1 != 0 {
				return "", fmt.Errorf("legacy DOC: piece %d has odd compressed offset %d", i, fc)
			}
			fc /= 2
			bytesPerChar = 1
		}
		if charCount > (meaningfulBytes-fc)/bytesPerChar || fc < 0 || fc > meaningfulBytes {
			return "", fmt.Errorf("legacy DOC: piece %d text range exceeds WordDocument stream", i)
		}
		piece, err := readStreamRange(word, fc, charCount*bytesPerChar)
		if err != nil {
			return "", fmt.Errorf("legacy DOC: reading piece %d: %w", i, err)
		}
		if err := w.writePiece(ctx, piece, compressed); err != nil {
			if errors.Is(err, limitbuf.ErrTooLarge) {
				return "", err
			}
			return "", fmt.Errorf("legacy DOC: decoding piece %d: %w", i, err)
		}
	}
	return w.String(), nil
}

func readCP(plc []byte, index int) (int64, error) {
	off := index * 4
	if index < 0 || off+4 > len(plc) {
		return 0, io.ErrUnexpectedEOF
	}
	cp := int32(binary.LittleEndian.Uint32(plc[off : off+4]))
	if cp < 0 {
		return 0, fmt.Errorf("negative character position %d", cp)
	}
	return int64(cp), nil
}

type textWriter struct {
	out          *limitbuf.Buffer
	fields       []bool // false while reading instructions, true in displayed result
	hiddenFields int
}

func newTextWriter(limit int) *textWriter {
	return &textWriter{out: limitbuf.New(limit)}
}

func (w *textWriter) String() string { return w.out.String() }

func (w *textWriter) writePiece(ctx context.Context, data []byte, compressed bool) error {
	if compressed {
		for i, b := range data {
			if i&0xFFF == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			if err := w.writeRune(charmap.Windows1252.DecodeByte(b)); err != nil {
				return err
			}
		}
		return nil
	}
	if len(data)&1 != 0 {
		return errors.New("odd-length UTF-16 text")
	}
	for i := 0; i < len(data); i += 2 {
		if i&0x1FFF == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		r := rune(binary.LittleEndian.Uint16(data[i : i+2]))
		if utf16.IsSurrogate(r) {
			if i+4 > len(data) {
				r = unicodeReplacement
			} else {
				next := rune(binary.LittleEndian.Uint16(data[i+2 : i+4]))
				decoded := utf16.DecodeRune(r, next)
				if decoded == unicodeReplacement {
					r = unicodeReplacement
				} else {
					r = decoded
					i += 2
				}
			}
		}
		if err := w.writeRune(r); err != nil {
			return err
		}
	}
	return nil
}

const unicodeReplacement = '\uFFFD'

func (w *textWriter) writeRune(r rune) error {
	switch r {
	case 0x13: // field begin; instructions follow
		w.fields = append(w.fields, false)
		w.hiddenFields++
		return nil
	case 0x14: // field separator; displayed result follows
		if n := len(w.fields); n > 0 && !w.fields[n-1] {
			w.fields[n-1] = true
			w.hiddenFields--
		}
		return nil
	case 0x15: // field end
		if n := len(w.fields); n > 0 {
			if !w.fields[n-1] {
				w.hiddenFields--
			}
			w.fields = w.fields[:n-1]
		}
		return nil
	}
	if w.hiddenFields > 0 {
		return nil
	}

	switch r {
	case '\r', '\n', 0x0B, 0x0E:
		return w.out.WriteByte('\n')
	case 0x0C:
		_, err := w.out.WriteString("\n\n")
		return err
	case '\t', 0x07:
		return w.out.WriteByte('\t')
	case 0x1E: // nonbreaking hyphen
		return w.out.WriteByte('-')
	case 0x1F: // optional hyphen
		return nil
	case 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x08:
		return nil
	}
	if r < 0x20 || r == 0x7F || (r >= 0x80 && r <= 0x9F) {
		return nil
	}
	_, err := w.out.WriteRune(r)
	return err
}

func findRootStream(cfb *mscfb.Reader, name string) *mscfb.File {
	for _, file := range cfb.File {
		if len(file.Path) == 0 && !file.FileInfo().IsDir() && strings.EqualFold(file.Name, name) {
			return file
		}
	}
	return nil
}

func readStreamRange(stream *mscfb.File, off, size int64) ([]byte, error) {
	if off < 0 || size < 0 || off > stream.Size || size > stream.Size-off {
		return nil, io.ErrUnexpectedEOF
	}
	if size > int64(int(^uint(0)>>1)) {
		return nil, errors.New("stream range is too large")
	}
	buf := make([]byte, int(size))
	if len(buf) == 0 {
		return buf, nil
	}
	n, err := stream.ReadAt(buf, off)
	if n != len(buf) {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return buf, nil
}
