// Package readerat adapts seekable streams to random-access readers for
// converters whose underlying formats (zip, pdf) need io.ReaderAt.
package readerat

import (
	"bytes"
	"errors"
	"io"
)

// ErrTooLarge is returned by FromLimit when the stream exceeds its limit.
var ErrTooLarge = errors.New("readerat: stream exceeds size limit")

// From returns rs as an io.ReaderAt plus its size, buffering into memory
// only when the concrete type doesn't already support ReadAt.
func From(rs io.ReadSeeker) (io.ReaderAt, int64, error) {
	return from(rs, 0)
}

// FromLimit is like From but returns ErrTooLarge before buffering when the
// seekable stream exceeds maxSize. maxSize must be positive.
func FromLimit(rs io.ReadSeeker, maxSize int64) (io.ReaderAt, int64, error) {
	if maxSize <= 0 {
		panic("readerat: size limit must be positive")
	}
	return from(rs, maxSize)
}

func from(rs io.ReadSeeker, maxSize int64) (io.ReaderAt, int64, error) {
	size, err := rs.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, 0, err
	}
	if maxSize > 0 && size > maxSize {
		return nil, size, ErrTooLarge
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
