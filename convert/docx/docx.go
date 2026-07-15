// Package docx converts Word documents to Markdown for the downmark
// engine: headings, emphasis, lists, tables, hyperlinks, images, and
// tracked changes, via an intermediate-HTML stage shared with the HTML
// converter.
package docx

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/internal/docx"
	"github.com/giraffesyo/downmark/internal/htmlmd"
	"github.com/giraffesyo/downmark/internal/limitbuf"
	"github.com/giraffesyo/downmark/internal/ooxml"
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

func (converter) InputLimit() int64 { return ooxml.MaxArchiveBytes }

func (converter) Accepts(info downmark.StreamInfo) bool {
	return info.Matches(
		[]string{".docx"},
		[]string{"application/vnd.openxmlformats-officedocument.wordprocessingml"},
	)
}

func (c converter) Convert(ctx context.Context, input io.ReadSeeker, _ downmark.StreamInfo) (res *downmark.Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			res, err = nil, fmt.Errorf("parse failure: %v", r)
		}
	}()
	ra, size, err := readerat.FromLimit(input, ooxml.MaxArchiveBytes)
	if errors.Is(err, readerat.ErrTooLarge) {
		return nil, fmt.Errorf("%w: docx archive is %d bytes; limit is %d", downmark.ErrInputTooLarge, size, ooxml.MaxArchiveBytes)
	}
	if err != nil {
		return nil, err
	}
	keep := c.opts.KeepDataURIs
	resultLimit, _ := downmark.ResultLimit(ctx)
	intermediate, title, err := docx.Convert(ctx, ra, size, docx.Options{
		KeepDataURIs: keep,
		OutputLimit:  resultLimit,
	})
	if errors.Is(err, limitbuf.ErrTooLarge) {
		return nil, docxResultLimitError(resultLimit)
	}
	if err != nil {
		return nil, err
	}
	md, _, err := htmlmd.ConvertString(ctx, intermediate, htmlmd.Options{KeepDataURIs: keep})
	if err != nil {
		return nil, err
	}
	if resultLimit > 0 && len(md) > resultLimit {
		return nil, docxResultLimitError(resultLimit)
	}
	return &downmark.Result{Markdown: md, Title: title}, nil
}

func docxResultLimitError(limit int) error {
	return fmt.Errorf("%w: docx result exceeds %d-byte limit", downmark.ErrResultTooLarge, limit)
}
