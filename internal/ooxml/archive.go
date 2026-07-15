// Package ooxml provides shared archive hardening for Office Open XML
// converters.
package ooxml

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/giraffesyo/downmark/internal/ctxio"
	"github.com/giraffesyo/downmark/internal/ziputil"
)

// Shared OOXML archive hard limits.
const (
	MaxArchiveBytes  = 64 * 1024 * 1024
	MaxEntries       = 1024
	MaxPartBytes     = 16 * 1024 * 1024
	MaxTotalBytes    = 128 * 1024 * 1024
	MaxXMLElements   = 500_000
	MaxXMLAttributes = 1_000_000
	MaxXMLDepth      = 256
	flagEncrypted    = 1 << 0
)

var errEncrypted = errors.New("encrypted Office archives are not supported")

// ErrXMLComplexity reports XML that exceeds the shared structural budgets.
var ErrXMLComplexity = errors.New("office XML structural complexity exceeds limit")

// Archive is a validated OOXML ZIP with cumulative decompression accounting.
type Archive struct {
	reader    *zip.Reader
	remaining int64
}

// ValidateXML rejects XML whose structural complexity would cause excessive
// allocations even when its byte size is within the per-part limit.
func ValidateXML(ctx context.Context, data []byte) error {
	dec := xml.NewDecoder(ctxio.NewReader(ctx, bytes.NewReader(data)))
	elements, attributes, depth := 0, 0, 0
	for {
		token, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch token := token.(type) {
		case xml.StartElement:
			elements++
			attributes += len(token.Attr)
			depth++
			if elements > MaxXMLElements || attributes > MaxXMLAttributes || depth > MaxXMLDepth {
				return ErrXMLComplexity
			}
		case xml.EndElement:
			depth--
		}
	}
}

// Open validates archive metadata before exposing its parts.
func Open(ctx context.Context, ra io.ReaderAt, size int64) (*Archive, error) {
	if size > MaxArchiveBytes {
		return nil, fmt.Errorf("office archive is %d bytes; limit is %d", size, MaxArchiveBytes)
	}
	if err := ziputil.PreflightEntries(ra, size, MaxEntries); err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(ra, size)
	if err != nil {
		return nil, err
	}
	var total uint64
	for _, file := range zr.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if file.Flags&flagEncrypted != 0 {
			return nil, errEncrypted
		}
		if file.UncompressedSize64 > MaxPartBytes {
			return nil, fmt.Errorf("office archive part %q exceeds %d-byte limit", file.Name, MaxPartBytes)
		}
		if file.UncompressedSize64 > math.MaxUint64-total {
			return nil, errors.New("office archive uncompressed size overflows")
		}
		total += file.UncompressedSize64
		if total > MaxTotalBytes {
			return nil, fmt.Errorf("office archive uncompressed data exceeds %d-byte limit", MaxTotalBytes)
		}
	}
	return &Archive{reader: zr, remaining: MaxTotalBytes}, nil
}

// Read returns a named part, whether it exists, and any bounded-read error.
func (a *Archive) Read(ctx context.Context, name string) ([]byte, bool, error) {
	for _, file := range a.reader.File {
		if file.Name != name {
			continue
		}
		if file.UncompressedSize64 > MaxPartBytes {
			return nil, true, fmt.Errorf("office archive part %q exceeds %d-byte limit", name, MaxPartBytes)
		}
		if a.remaining <= 0 || file.UncompressedSize64 > uint64(a.remaining) {
			return nil, true, fmt.Errorf("office archive decompression exceeds %d-byte limit", MaxTotalBytes)
		}
		r, err := file.Open()
		if err != nil {
			return nil, true, err
		}
		defer func() { _ = r.Close() }()

		limit := min(int64(MaxPartBytes), a.remaining)
		var buf bytes.Buffer
		buf.Grow(int(file.UncompressedSize64) + 1)
		_, err = buf.ReadFrom(io.LimitReader(ctxio.NewReader(ctx, r), limit+1))
		read := int64(buf.Len())
		a.remaining -= min(read, a.remaining)
		if read > limit {
			return nil, true, fmt.Errorf("office archive part %q exceeds decompression limit", name)
		}
		if err != nil {
			return nil, true, err
		}
		return buf.Bytes(), true, nil
	}
	return nil, false, nil
}
