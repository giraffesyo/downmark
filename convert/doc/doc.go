// Package doc converts Word 97-2003 binary documents to Markdown for the
// downmark engine. Text is reconstructed from the document's piece table;
// legacy character and paragraph formatting is not preserved.
package doc

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/internal/legacydoc"
	"github.com/giraffesyo/downmark/internal/limitbuf"
	"github.com/giraffesyo/downmark/internal/readerat"
)

const maxDocumentBytes int64 = 64 * 1024 * 1024

// New returns the legacy DOC converter.
func New() downmark.Converter { return converter{} }

// Register adds the legacy DOC converter to e at the standard priority.
func Register(e *downmark.Engine) { e.Register(New(), downmark.PrioritySpecific) }

type converter struct{}

func (converter) Name() string { return "doc" }

func (converter) InputLimit() int64 { return maxDocumentBytes }

func (converter) Accepts(info downmark.StreamInfo) bool {
	return info.Matches(
		[]string{".doc"},
		[]string{
			"application/msword",
			"application/vnd.ms-word",
			// A CFB directory can be beyond the engine's initial sniff window,
			// leaving a valid DOC identified only by its generic container.
			// Convert still requires WordDocument plus a valid FIB and CLX.
			"application/x-ole-storage",
		},
	)
}

func (converter) Convert(ctx context.Context, input io.ReadSeeker, _ downmark.StreamInfo) (res *downmark.Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			res, err = nil, fmt.Errorf("parse failure: %v", r)
		}
	}()
	ra, size, err := readerat.FromLimit(input, maxDocumentBytes)
	if errors.Is(err, readerat.ErrTooLarge) {
		return nil, fmt.Errorf("%w: legacy DOC is %d bytes; limit is %d", downmark.ErrInputTooLarge, size, maxDocumentBytes)
	}
	if err != nil {
		return nil, err
	}
	resultLimit, _ := downmark.ResultLimit(ctx)
	markdown, err := legacydoc.Convert(ctx, ra, size, resultLimit)
	if errors.Is(err, limitbuf.ErrTooLarge) {
		return nil, fmt.Errorf("%w: legacy DOC result exceeds %d-byte limit", downmark.ErrResultTooLarge, resultLimit)
	}
	if err != nil {
		return nil, err
	}
	return &downmark.Result{Markdown: markdown}, nil
}
