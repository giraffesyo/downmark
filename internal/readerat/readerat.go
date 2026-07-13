// Package readerat adapts seekable streams to random-access readers for
// converters whose underlying formats (zip, pdf) need io.ReaderAt.
package readerat

import (
	"bytes"
	"io"
)

// From returns rs as an io.ReaderAt plus its size, buffering into memory
// only when the concrete type doesn't already support ReadAt.
func From(rs io.ReadSeeker) (io.ReaderAt, int64, error) {
	size, err := rs.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, 0, err
	}
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return nil, 0, err
	}
	if ra, ok := rs.(io.ReaderAt); ok {
		return ra, size, nil
	}
	data, err := io.ReadAll(rs)
	if err != nil {
		return nil, 0, err
	}
	return bytes.NewReader(data), int64(len(data)), nil
}
