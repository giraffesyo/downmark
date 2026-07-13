package downmark

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeMarkdown(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"empty", "", ""},
		{"simple", "hello", "hello\n"},
		{"crlf", "a\r\nb\r", "a\nb\n"},
		{"trailing space", "a  \nb\t", "a\nb\n"},
		{"collapse newlines", "a\n\n\n\n\nb", "a\n\nb\n"},
		{"trim edges", "\n\n\na\n\n\n", "a\n"},
		{"whitespace only", "  \n\t\n ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeMarkdown(tc.in); got != tc.want {
				t.Errorf("normalizeMarkdown(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPlainTextRoundTrip(t *testing.T) {
	res, err := New().Convert(context.Background(), strings.NewReader("hello world\n"), StreamInfo{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if res.Markdown != "hello world\n" {
		t.Errorf("Markdown = %q, want %q", res.Markdown, "hello world\n")
	}
}

func TestPlainTextExtensionHint(t *testing.T) {
	res, err := New().Convert(context.Background(), strings.NewReader("# heading"), StreamInfo{Extension: ".md"})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if res.Markdown != "# heading\n" {
		t.Errorf("Markdown = %q", res.Markdown)
	}
}

func TestUnsupportedFormat(t *testing.T) {
	res, err := New().ConvertFile(context.Background(), filepath.Join("testdata", "random.bin"))
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("err = %v (res=%v), want ErrUnsupportedFormat", err, res)
	}
}

func TestConvertFileDerivesInfo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.TXT")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := New().ConvertFile(context.Background(), path)
	if err != nil {
		t.Fatalf("ConvertFile: %v", err)
	}
	if res.Markdown != "content\n" {
		t.Errorf("Markdown = %q", res.Markdown)
	}
}

func TestUTF16Decoding(t *testing.T) {
	// "hi" in UTF-16LE with BOM.
	in := []byte{0xFF, 0xFE, 'h', 0, 'i', 0}
	res, err := New().Convert(context.Background(), strings.NewReader(string(in)), StreamInfo{Extension: ".txt"})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if res.Markdown != "hi\n" {
		t.Errorf("Markdown = %q, want %q", res.Markdown, "hi\n")
	}
}

// failingConverter accepts everything and always fails.
type failingConverter struct{}

func (failingConverter) Accepts(StreamInfo) bool { return true }
func (failingConverter) Convert(context.Context, io.ReadSeeker, StreamInfo) (*Result, error) {
	return nil, errors.New("boom")
}
func (failingConverter) Name() string { return "failing" }

func TestConversionErrorAggregatesAttempts(t *testing.T) {
	e := New(WithoutBuiltins())
	e.Register(failingConverter{}, PrioritySpecific)
	_, err := e.Convert(context.Background(), strings.NewReader("x"), StreamInfo{})
	var convErr *ConversionError
	if !errors.As(err, &convErr) {
		t.Fatalf("err = %v, want *ConversionError", err)
	}
	if len(convErr.Attempts) != 1 || convErr.Attempts[0].Converter != "failing" {
		t.Errorf("Attempts = %+v", convErr.Attempts)
	}
}

// upperConverter uppercases text; used to test registration shadowing.
type upperConverter struct{}

func (upperConverter) Accepts(info StreamInfo) bool { return info.Charset != "" }
func (upperConverter) Convert(_ context.Context, in io.ReadSeeker, _ StreamInfo) (*Result, error) {
	data, err := io.ReadAll(in)
	if err != nil {
		return nil, err
	}
	return &Result{Markdown: strings.ToUpper(string(data))}, nil
}

func TestUserConverterShadowsBuiltin(t *testing.T) {
	e := New()
	e.Register(upperConverter{}, PriorityGeneric)
	res, err := e.Convert(context.Background(), strings.NewReader("abc"), StreamInfo{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if res.Markdown != "ABC\n" {
		t.Errorf("Markdown = %q, want %q (user converter should win the tie)", res.Markdown, "ABC\n")
	}
}

func TestSpecificBeatsGeneric(t *testing.T) {
	// A specific-priority converter registered before a generic one must
	// still be tried first.
	e := New(WithoutBuiltins())
	e.Register(upperConverter{}, PrioritySpecific)
	e.Register(&plainTextConverter{}, PriorityGeneric)
	res, err := e.Convert(context.Background(), strings.NewReader("abc"), StreamInfo{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if res.Markdown != "ABC\n" {
		t.Errorf("Markdown = %q, want %q", res.Markdown, "ABC\n")
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New().Convert(ctx, strings.NewReader("hello"), StreamInfo{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestBuildGuessesHintsBeatSniffing(t *testing.T) {
	// Content sniffs as plain text, but the caller insists it is CSV: the
	// first guess must carry the hint MIME type.
	rs := strings.NewReader("a,b\n1,2\n")
	guesses, err := buildGuesses(rs, StreamInfo{Extension: ".csv"})
	if err != nil {
		t.Fatal(err)
	}
	if len(guesses) == 0 || guesses[0].MIMEType != "text/csv" {
		t.Errorf("guesses = %+v, want first guess MIME text/csv", guesses)
	}
	if guesses[0].Charset == "" {
		t.Errorf("expected detected charset on text-like guess, got none")
	}
}
