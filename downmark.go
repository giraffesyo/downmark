package downmark

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Converter converts one class of documents to Markdown.
//
// Implementations must judge Accepts from the StreamInfo alone, never by
// reading the stream; content sniffing happens in the Engine's detection
// layer before Accepts is called.
type Converter interface {
	// Accepts reports whether the converter wants to try this stream.
	Accepts(info StreamInfo) bool
	// Convert reads input (positioned at offset 0) and produces Markdown.
	Convert(ctx context.Context, input io.ReadSeeker, info StreamInfo) (*Result, error)
}

// Priority orders converters: lower values are tried first.
type Priority int

const (
	// PrioritySpecific is for converters keyed to a specific file format
	// (docx, xlsx, pptx, pdf, csv).
	PrioritySpecific Priority = 0
	// PriorityGeneric is for near-catch-all converters (html, plain text).
	PriorityGeneric Priority = 100
)

type registration struct {
	conv Converter
	prio Priority
	seq  int
}

// Engine holds a converter registry and conversion options.
// Create one with New; the zero value is not usable.
type Engine struct {
	regs       []registration
	nextSeq    int
	noBuiltins bool
	httpClient *http.Client
}

// New returns an Engine with only the builtin plain-text converter
// registered (unless WithoutBuiltins is given). Register format converters
// explicitly from the convert/... packages, or use the all package for an
// engine with every format wired up.
func New(opts ...Option) *Engine {
	e := &Engine{httpClient: http.DefaultClient}
	for _, opt := range opts {
		opt(e)
	}
	if !e.noBuiltins {
		e.registerBuiltins()
	}
	return e
}

func (e *Engine) registerBuiltins() {
	e.Register(&plainTextConverter{}, PriorityGeneric)
}

// Register adds a converter at the given priority. Among converters with
// equal priority, the most recently registered is tried first, so user
// converters shadow builtins.
func (e *Engine) Register(c Converter, p Priority) {
	e.regs = append(e.regs, registration{conv: c, prio: p, seq: e.nextSeq})
	e.nextSeq++
}

// sortedRegistrations returns converters ordered by (priority ascending,
// registration order descending).
func (e *Engine) sortedRegistrations() []registration {
	regs := slices.Clone(e.regs)
	slices.SortStableFunc(regs, func(a, b registration) int {
		if a.prio != b.prio {
			return int(a.prio) - int(b.prio)
		}
		return b.seq - a.seq
	})
	return regs
}

// Convert converts an arbitrary reader to Markdown. hints may be the zero
// value. If r is not an io.ReadSeeker its content is buffered into memory;
// seekable inputs are rewound to offset 0.
func (e *Engine) Convert(ctx context.Context, r io.Reader, hints StreamInfo) (*Result, error) {
	rs, ok := r.(io.ReadSeeker)
	if ok {
		// Types like *os.File satisfy io.ReadSeeker even when the underlying
		// descriptor (a pipe, stdin) cannot seek; probe before trusting it.
		_, err := rs.Seek(0, io.SeekCurrent)
		ok = err == nil
	}
	if !ok {
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		rs = bytes.NewReader(data)
	}
	return e.convert(ctx, rs, hints)
}

// ConvertFile opens path and converts it, deriving Extension, Filename, and
// LocalPath from the path.
func (e *Engine) ConvertFile(ctx context.Context, path string) (*Result, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() // read-only handle
	info := StreamInfo{
		Extension: strings.ToLower(filepath.Ext(path)),
		Filename:  filepath.Base(path),
		LocalPath: path,
	}
	if abs, err := filepath.Abs(path); err == nil {
		info.LocalPath = abs
	}
	return e.convert(ctx, f, info)
}

func (e *Engine) convert(ctx context.Context, rs io.ReadSeeker, base StreamInfo) (*Result, error) {
	guesses, err := buildGuesses(rs, base)
	if err != nil {
		return nil, err
	}
	regs := e.sortedRegistrations()
	var attempts []*AttemptError
	for _, guess := range guesses {
		for _, reg := range regs {
			if !reg.conv.Accepts(guess) {
				continue
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if _, err := rs.Seek(0, io.SeekStart); err != nil {
				return nil, err
			}
			res, err := reg.conv.Convert(ctx, rs, guess)
			if err != nil {
				attempts = append(attempts, &AttemptError{Converter: converterName(reg.conv), Err: err})
				continue
			}
			if res == nil {
				continue
			}
			res.Markdown = normalizeMarkdown(res.Markdown)
			return res, nil
		}
	}
	if len(attempts) > 0 {
		return nil, &ConversionError{Attempts: attempts}
	}
	return nil, ErrUnsupportedFormat
}

// converterName resolves the name used in AttemptError: converters may
// implement interface{ Name() string } (the method must be exported so
// converters in other packages satisfy it); otherwise the type name is
// used.
func converterName(c Converter) string {
	if n, ok := c.(interface{ Name() string }); ok {
		return n.Name()
	}
	return fmt.Sprintf("%T", c)
}

// normalizeMarkdown applies the central output discipline: CRLF/CR to LF,
// strip trailing whitespace per line, collapse 3+ newlines to 2, trim outer
// blank lines, and end with exactly one newline.
func normalizeMarkdown(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimRight(ln, " \t")
	}
	s = strings.Join(lines, "\n")
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	s = strings.Trim(s, "\n")
	if s == "" {
		return ""
	}
	return s + "\n"
}
