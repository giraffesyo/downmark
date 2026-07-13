// Package pptx converts PowerPoint presentations to Markdown for the
// downmark engine: slides in order with position-based reading order,
// tables, chart data, image alt text, and speaker notes.
package pptx

import (
	"context"
	"fmt"
	"io"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/internal/pptx"
	"github.com/giraffesyo/downmark/internal/readerat"
)

// New returns the PPTX converter.
func New() downmark.Converter { return converter{} }

// Register adds the PPTX converter to e at the standard priority.
func Register(e *downmark.Engine) { e.Register(New(), downmark.PrioritySpecific) }

type converter struct{}

func (converter) Name() string { return "pptx" }

func (converter) Accepts(info downmark.StreamInfo) bool {
	return info.Matches(
		[]string{".pptx"},
		[]string{"application/vnd.openxmlformats-officedocument.presentationml"},
	)
}

func (converter) Convert(_ context.Context, input io.ReadSeeker, _ downmark.StreamInfo) (res *downmark.Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			res, err = nil, fmt.Errorf("parse failure: %v", r)
		}
	}()
	ra, size, err := readerat.From(input)
	if err != nil {
		return nil, err
	}
	md, title, err := pptx.Convert(ra, size)
	if err != nil {
		return nil, err
	}
	return &downmark.Result{Markdown: md, Title: title}, nil
}
