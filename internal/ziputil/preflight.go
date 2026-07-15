// Package ziputil contains bounded ZIP metadata helpers shared by archive and
// OOXML converters.
package ziputil

import (
	"archive/zip"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

const (
	directoryHeaderSignature = 0x02014b50
	directoryEndSignature    = 0x06054b50
	directory64EndSignature  = 0x06064b50
	directory64LocSignature  = 0x07064b50
	directoryHeaderLen       = 46
	directoryEndLen          = 22
	directory64EndLen        = 56
	directory64LocLen        = 20
	maxDirectoryEndSearch    = 65 * 1024
)

// EntryLimitError reports that a ZIP central directory exceeds an entry
// budget. Count is the advertised count for a valid archive, or the first
// observed count over the limit when the directory metadata is inconsistent.
type EntryLimitError struct {
	Count uint64
	Limit int
}

func (e *EntryLimitError) Error() string {
	return fmt.Sprintf("zip: archive contains %d entries; limit is %d", e.Count, e.Limit)
}

// PreflightEntries scans ZIP central-directory headers without allocating a
// zip.File per entry. It rejects oversized directories before zip.NewReader
// performs its full metadata parse.
func PreflightEntries(r io.ReaderAt, size int64, maxEntries int) error {
	if maxEntries <= 0 {
		panic("ziputil: entry limit must be positive")
	}
	endOffset, records, directorySize, err := readDirectoryEnd(r, size)
	if err != nil {
		return err
	}
	if records > uint64(maxEntries) {
		return &EntryLimitError{Count: records, Limit: maxEntries}
	}
	if endOffset < 0 {
		return zip.ErrFormat
	}
	end := uint64(endOffset)
	if directorySize > end || directorySize > math.MaxInt64 {
		return zip.ErrFormat
	}

	start := endOffset - int64(directorySize)
	for offset, count := start, uint64(0); offset < endOffset; {
		if endOffset-offset < directoryHeaderLen {
			break
		}
		var header [directoryHeaderLen]byte
		if err := readAt(r, header[:], offset); err != nil {
			return err
		}
		if binary.LittleEndian.Uint32(header[:4]) != directoryHeaderSignature {
			break
		}
		count++
		if count > uint64(maxEntries) {
			return &EntryLimitError{Count: count, Limit: maxEntries}
		}
		variableLen := uint64(binary.LittleEndian.Uint16(header[28:30])) +
			uint64(binary.LittleEndian.Uint16(header[30:32])) +
			uint64(binary.LittleEndian.Uint16(header[32:34]))
		recordLen := uint64(directoryHeaderLen) + variableLen
		remaining := endOffset - offset
		if remaining < 0 || recordLen > uint64(remaining) {
			break
		}
		offset += int64(recordLen) //nolint:gosec // recordLen is bounded by non-negative int64 remaining
	}
	return nil
}

func readDirectoryEnd(r io.ReaderAt, size int64) (offset int64, records, directorySize uint64, err error) {
	if size < directoryEndLen {
		return 0, 0, 0, zip.ErrFormat
	}
	searchLen := min(size, int64(maxDirectoryEndSearch))
	buf := make([]byte, searchLen)
	if err := readAt(r, buf, size-searchLen); err != nil {
		return 0, 0, 0, err
	}
	index := findDirectoryEnd(buf)
	if index < 0 {
		return 0, 0, 0, zip.ErrFormat
	}
	offset = size - searchLen + int64(index)
	header := buf[index : index+directoryEndLen]
	if binary.LittleEndian.Uint16(header[4:6]) != 0 ||
		binary.LittleEndian.Uint16(header[6:8]) != 0 ||
		binary.LittleEndian.Uint16(header[8:10]) != binary.LittleEndian.Uint16(header[10:12]) {
		return 0, 0, 0, zip.ErrFormat
	}
	records = uint64(binary.LittleEndian.Uint16(header[10:12]))
	directorySize = uint64(binary.LittleEndian.Uint32(header[12:16]))
	directoryOffset := binary.LittleEndian.Uint32(header[16:20])
	if records != math.MaxUint16 && directorySize != math.MaxUint32 && directoryOffset != math.MaxUint32 {
		return offset, records, directorySize, nil
	}
	return readDirectory64End(r, offset)
}

func findDirectoryEnd(buf []byte) int {
	for i := len(buf) - directoryEndLen; i >= 0; i-- {
		if binary.LittleEndian.Uint32(buf[i:i+4]) != directoryEndSignature {
			continue
		}
		commentLen := int(binary.LittleEndian.Uint16(buf[i+20 : i+22]))
		if i+directoryEndLen+commentLen <= len(buf) {
			return i
		}
	}
	return -1
}

func readDirectory64End(r io.ReaderAt, endOffset int64) (offset int64, records, directorySize uint64, err error) {
	locatorOffset := endOffset - directory64LocLen
	if locatorOffset < 0 {
		return 0, 0, 0, zip.ErrFormat
	}
	var locator [directory64LocLen]byte
	if err := readAt(r, locator[:], locatorOffset); err != nil {
		return 0, 0, 0, err
	}
	if binary.LittleEndian.Uint32(locator[:4]) != directory64LocSignature ||
		binary.LittleEndian.Uint32(locator[4:8]) != 0 ||
		binary.LittleEndian.Uint32(locator[16:20]) != 1 {
		return 0, 0, 0, zip.ErrFormat
	}
	offset64 := binary.LittleEndian.Uint64(locator[8:16])
	if offset64 > math.MaxInt64 {
		return 0, 0, 0, zip.ErrFormat
	}
	var header [directory64EndLen]byte
	if err := readAt(r, header[:], int64(offset64)); err != nil {
		return 0, 0, 0, err
	}
	if binary.LittleEndian.Uint32(header[:4]) != directory64EndSignature ||
		binary.LittleEndian.Uint32(header[16:20]) != 0 ||
		binary.LittleEndian.Uint32(header[20:24]) != 0 ||
		binary.LittleEndian.Uint64(header[24:32]) != binary.LittleEndian.Uint64(header[32:40]) {
		return 0, 0, 0, zip.ErrFormat
	}
	return int64(offset64), binary.LittleEndian.Uint64(header[32:40]), binary.LittleEndian.Uint64(header[40:48]), nil
}

func readAt(r io.ReaderAt, buf []byte, offset int64) error {
	n, err := r.ReadAt(buf, offset)
	if n != len(buf) {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return err
	}
	return nil
}
