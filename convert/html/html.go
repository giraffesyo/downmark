// Package html converts HTML documents to Markdown for the downmark
// engine, sanitizing the DOM first (scripts and styles dropped, unsafe
// link schemes unwrapped, data URIs truncated unless kept).
package html

import (
	"context"
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

func (c converter) Convert(_ context.Context, input io.ReadSeeker, info downmark.StreamInfo) (*downmark.Result, error) {
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
	md, title, err := htmlmd.Convert(r, htmlmd.Options{KeepDataURIs: c.opts.KeepDataURIs})
	if err != nil {
		return nil, err
	}
	return &downmark.Result{Markdown: md, Title: title}, nil
}
