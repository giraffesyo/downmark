// Package pptx converts PowerPoint presentations to Markdown for the
// downmark engine: slides in order with position-based reading order,
// tables, chart data, image alt text, and speaker notes.
package pptx

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/internal/limitbuf"
	"github.com/giraffesyo/downmark/internal/ooxml"
	"github.com/giraffesyo/downmark/internal/pptx"
	"github.com/giraffesyo/downmark/internal/readerat"
)

// New returns the PPTX converter.
func New() downmark.Converter { return converter{} }

// Register adds the PPTX converter to e at the standard priority.
func Register(e *downmark.Engine) { e.Register(New(), downmark.PrioritySpecific) }

type converter struct{}

func (converter) Name() string { return "pptx" }

func (converter) InputLimit() int64 { return ooxml.MaxArchiveBytes }

func (converter) Accepts(info downmark.StreamInfo) bool {
	return info.Matches(
		[]string{".pptx"},
		[]string{"application/vnd.openxmlformats-officedocument.presentationml"},
	)
}

func (converter) Convert(ctx context.Context, input io.ReadSeeker, _ downmark.StreamInfo) (res *downmark.Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			res, err = nil, fmt.Errorf("parse failure: %v", r)
		}
	}()
	ra, size, err := readerat.FromLimit(input, ooxml.MaxArchiveBytes)
	if errors.Is(err, readerat.ErrTooLarge) {
		return nil, fmt.Errorf("%w: pptx archive is %d bytes; limit is %d", downmark.ErrInputTooLarge, size, ooxml.MaxArchiveBytes)
	}
	if err != nil {
		return nil, err
	}
	resultLimit, _ := downmark.ResultLimit(ctx)
	md, title, err := pptx.Convert(ctx, ra, size, resultLimit)
	if errors.Is(err, limitbuf.ErrTooLarge) {
		return nil, fmt.Errorf("%w: pptx result exceeds %d-byte limit", downmark.ErrResultTooLarge, resultLimit)
	}
	if err != nil {
		return nil, err
	}
	return &downmark.Result{Markdown: md, Title: title}, nil
}
