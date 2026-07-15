// Package limitbuf provides a sticky-error string builder with a hard byte
// limit. It is useful in renderers whose WriteString errors are intentionally
// ignored at individual call sites.
package limitbuf

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// ErrTooLarge reports that a write would exceed a Buffer's byte limit.
var ErrTooLarge = errors.New("buffer exceeds size limit")

// Buffer accumulates text up to limit bytes. A non-positive limit is
// unlimited. Once a write would exceed the limit, all later writes return the
// same sticky error and no partial write is retained.
type Buffer struct {
	b     strings.Builder
	limit int
	err   error
}

// New returns an empty Buffer with the supplied byte limit.
func New(limit int) *Buffer { return &Buffer{limit: limit} }

func (b *Buffer) Write(p []byte) (int, error) {
	if err := b.reserve(len(p)); err != nil {
		return 0, err
	}
	return b.b.Write(p)
}

// WriteString appends s if it fits within the byte limit.
func (b *Buffer) WriteString(s string) (int, error) {
	if err := b.reserve(len(s)); err != nil {
		return 0, err
	}
	return b.b.WriteString(s)
}

// WriteByte appends c if it fits within the byte limit.
func (b *Buffer) WriteByte(c byte) error {
	if err := b.reserve(1); err != nil {
		return err
	}
	return b.b.WriteByte(c)
}

// WriteRune appends r if its UTF-8 encoding fits within the byte limit.
func (b *Buffer) WriteRune(r rune) (int, error) {
	size := utf8.RuneLen(r)
	if size < 0 {
		size = len(string(r))
	}
	if err := b.reserve(size); err != nil {
		return 0, err
	}
	return b.b.WriteRune(r)
}

// Len returns the number of retained bytes.
func (b *Buffer) Len() int { return b.b.Len() }

func (b *Buffer) String() string { return b.b.String() }

// Err returns the first size error encountered by the Buffer.
func (b *Buffer) Err() error { return b.err }

func (b *Buffer) reserve(size int) error {
	if b.err != nil {
		return b.err
	}
	if b.limit > 0 && size > b.limit-b.b.Len() {
		b.err = ErrTooLarge
		return b.err
	}
	return nil
}
