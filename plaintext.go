package downmark

import (
	"context"
	"io"

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

func (*plainTextConverter) Convert(_ context.Context, input io.ReadSeeker, info StreamInfo) (*Result, error) {
	text, err := textenc.DecodeAll(input, info.Charset)
	if err != nil {
		return nil, err
	}
	return &Result{Markdown: text}, nil
}
