// Package html converts HTML documents to Markdown for the downmark
// engine, sanitizing the DOM first (hidden and inert content dropped, unsafe
// link schemes unwrapped, data URIs truncated unless kept).
package html

import (
	"context"
	"fmt"
	"io"

	xcharset "golang.org/x/net/html/charset"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/internal/htmlmd"
	"github.com/giraffesyo/downmark/internal/textenc"
)

// Options configures the HTML converter.
type Options struct {
	// KeepDataURIs preserves full data: URIs in output instead of
	// truncating them to a short placeholder.
	KeepDataURIs bool
}

// New returns the HTML converter.
func New(opts Options) downmark.Converter { return converter{opts: opts} }

// Register adds the HTML converter to e at the generic priority, so
// format-specific converters win when both accept a stream.
func Register(e *downmark.Engine, opts Options) {
	e.Register(New(opts), downmark.PriorityGeneric)
}

type converter struct {
	opts Options
}

func (converter) Name() string { return "html" }

func (converter) Accepts(info downmark.StreamInfo) bool {
	return info.Matches(
		[]string{".html", ".htm", ".xhtml"},
		[]string{"text/html", "application/xhtml"},
	)
}

func (c converter) Convert(ctx context.Context, input io.ReadSeeker, info downmark.StreamInfo) (*downmark.Result, error) {
	var r io.Reader
	if info.Charset != "" {
		dec, err := textenc.NewReader(input, info.Charset)
		if err != nil {
			dec, _ = xcharset.NewReader(input, "text/html")
		}
		r = dec
	} else {
		// No charset known: let the HTML prescan find <meta charset>.
		dec, err := xcharset.NewReader(input, "text/html")
		if err != nil {
			return nil, err
		}
		r = dec
	}
	md, title, err := htmlmd.Convert(ctx, r, htmlmd.Options{KeepDataURIs: c.opts.KeepDataURIs})
	if err != nil {
		return nil, err
	}
	if limit, ok := downmark.ResultLimit(ctx); ok && len(md) > limit {
		return nil, fmt.Errorf("%w: HTML result exceeds %d-byte limit", downmark.ErrResultTooLarge, limit)
	}
	return &downmark.Result{Markdown: md, Title: title}, nil
}
