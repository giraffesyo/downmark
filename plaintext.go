package downmark

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/giraffesyo/downmark/internal/ctxio"
	"github.com/giraffesyo/downmark/internal/textenc"
)

// plainTextConverter is the last-resort converter: it passes decoded text
// through unchanged (the engine's normalization handles the rest).
type plainTextConverter struct{}

func (*plainTextConverter) Name() string { return "plaintext" }

func (*plainTextConverter) Accepts(info StreamInfo) bool {
	// A detected charset means the detection layer believes this is text.
	if info.Charset != "" {
		return true
	}
	return info.Matches(
		[]string{".txt", ".text", ".md", ".markdown", ".json", ".jsonl"},
		[]string{"text/", "application/json", "application/markdown"},
	)
}

func (*plainTextConverter) Convert(ctx context.Context, input io.ReadSeeker, info StreamInfo) (*Result, error) {
	limit, _ := ResultLimit(ctx)
	text, err := textenc.DecodeAllLimit(ctxio.NewReader(ctx, input), info.Charset, limit)
	if errors.Is(err, textenc.ErrTooLarge) {
		return nil, fmt.Errorf("%w: plain-text result exceeds %d-byte limit", ErrResultTooLarge, limit)
	}
	if err != nil {
		return nil, err
	}
	return &Result{Markdown: text}, nil
}
