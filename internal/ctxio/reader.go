// Package ctxio adapts readers so blocked conversion loops observe context
// cancellation between reads.
package ctxio

import (
	"context"
	"io"
)

// NewReader returns a reader that checks ctx before delegating each read.
func NewReader(ctx context.Context, r io.Reader) io.Reader {
	return reader{ctx: ctx, reader: r}
}

type reader struct {
	ctx    context.Context
	reader io.Reader
}

func (r reader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
