// Package docx converts Word documents to Markdown for the downmark
// engine: headings, emphasis, lists, tables, hyperlinks, images, and
// tracked changes, via an intermediate-HTML stage shared with the HTML
// converter.
package docx

import (
	"context"
	"fmt"
	"io"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/internal/docx"
	"github.com/giraffesyo/downmark/internal/htmlmd"
	"github.com/giraffesyo/downmark/internal/readerat"
)

// Options configures the DOCX converter.
type Options struct {
	// KeepDataURIs embeds images as full data: URIs instead of filename
	// placeholders.
	KeepDataURIs bool
}

// New returns the DOCX converter.
func New(opts Options) downmark.Converter { return converter{opts: opts} }

// Register adds the DOCX converter to e at the standard priority.
func Register(e *downmark.Engine, opts Options) {
	e.Register(New(opts), downmark.PrioritySpecific)
}

type converter struct {
	opts Options
}

func (converter) Name() string { return "docx" }

func (converter) Accepts(info downmark.StreamInfo) bool {
	return info.Matches(
		[]string{".docx"},
		[]string{"application/vnd.openxmlformats-officedocument.wordprocessingml"},
	)
}

func (c converter) Convert(_ context.Context, input io.ReadSeeker, _ downmark.StreamInfo) (res *downmark.Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			res, err = nil, fmt.Errorf("parse failure: %v", r)
		}
	}()
	ra, size, err := readerat.From(input)
	if err != nil {
		return nil, err
	}
	keep := c.opts.KeepDataURIs
	intermediate, title, err := docx.Convert(ra, size, docx.Options{KeepDataURIs: keep})
	if err != nil {
		return nil, err
	}
	md, _, err := htmlmd.ConvertString(intermediate, htmlmd.Options{KeepDataURIs: keep})
	if err != nil {
		return nil, err
	}
	return &downmark.Result{Markdown: md, Title: title}, nil
}
