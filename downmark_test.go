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
		{"preserve indentation", "\n  a\n\t b\n", "  a\n\t b\n"},
		{"blank lines with whitespace", "a\n  \n\t\nb", "a\n\nb\n"},
		{"mixed newlines", "\r\na \r\n\r\nb\r", "a\n\nb\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeMarkdown(tc.in); got != tc.want {
				t.Errorf("normalizeMarkdown(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeMarkdownMatchesReference(t *testing.T) {
	alphabet := []byte{'a', ' ', '\t', '\n', '\r'}
	for length := range 7 {
		combinations := 1
		for range length {
			combinations *= len(alphabet)
		}
		for value := range combinations {
			data := make([]byte, length)
			n := value
			for i := range length {
				data[i] = alphabet[n%len(alphabet)]
				n /= len(alphabet)
			}
			input := string(data)
			if got, want := normalizeMarkdown(input), normalizeMarkdownReference(input); got != want {
				t.Fatalf("normalizeMarkdown(%q) = %q, want %q", input, got, want)
			}
		}
	}
}

func normalizeMarkdownReference(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
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

var benchmarkNormalizedMarkdown string

func BenchmarkNormalizeMarkdown(b *testing.B) {
	input := strings.Repeat("line with trailing space  \r\n\r\n\r\n  indented line\n", 4096)
	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	for b.Loop() {
		benchmarkNormalizedMarkdown = normalizeMarkdown(input)
	}
}

func BenchmarkNormalizeMarkdownReference(b *testing.B) {
	input := strings.Repeat("line with trailing space  \r\n\r\n\r\n  indented line\n", 4096)
	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	for b.Loop() {
		benchmarkNormalizedMarkdown = normalizeMarkdownReference(input)
	}
}

func TestPlainTextRoundTrip(t *testing.T) {
	res, err := New().Convert(t.Context(), strings.NewReader("hello world\n"), StreamInfo{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if res.Markdown != "hello world\n" {
		t.Errorf("Markdown = %q, want %q", res.Markdown, "hello world\n")
	}
}

func TestPlainTextExtensionHint(t *testing.T) {
	res, err := New().Convert(t.Context(), strings.NewReader("# heading"), StreamInfo{Extension: ".md"})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if res.Markdown != "# heading\n" {
		t.Errorf("Markdown = %q", res.Markdown)
	}
}

func TestUnsupportedFormat(t *testing.T) {
	res, err := New().ConvertFile(t.Context(), filepath.Join("testdata", "random.bin"))
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
	res, err := New().ConvertFile(t.Context(), path)
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
	res, err := New().Convert(t.Context(), strings.NewReader(string(in)), StreamInfo{Extension: ".txt"})
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
	_, err := e.Convert(t.Context(), strings.NewReader("x"), StreamInfo{})
	var convErr *ConversionError
	if !errors.As(err, &convErr) {
		t.Fatalf("err = %v, want *ConversionError", err)
	}
	if len(convErr.Attempts) != 1 || convErr.Attempts[0].Converter != "failing" {
		t.Errorf("Attempts = %+v", convErr.Attempts)
	}
}

type fixedResultConverter struct {
	markdown string
}

type inputLimitedConverter struct{}

func (inputLimitedConverter) Accepts(info StreamInfo) bool { return info.Extension == ".limited" }
func (inputLimitedConverter) InputLimit() int64            { return 8 }
func (inputLimitedConverter) Convert(_ context.Context, input io.ReadSeeker, _ StreamInfo) (*Result, error) {
	data, err := io.ReadAll(input)
	return &Result{Markdown: string(data)}, err
}

type readOnlyReader struct{ r io.Reader }

func (r readOnlyReader) Read(p []byte) (int, error) { return r.r.Read(p) }

func TestNonSeekableInputLimitAppliesBeforeBuffering(t *testing.T) {
	e := New(WithoutBuiltins())
	e.Register(inputLimitedConverter{}, PrioritySpecific)
	input := readOnlyReader{r: strings.NewReader("123456789")}
	res, err := e.Convert(t.Context(), input, StreamInfo{Extension: ".limited"})
	if !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("err = %v (res=%v), want ErrInputTooLarge", err, res)
	}
}

func (fixedResultConverter) Accepts(StreamInfo) bool { return true }
func (c fixedResultConverter) Convert(context.Context, io.ReadSeeker, StreamInfo) (*Result, error) {
	return &Result{Markdown: c.markdown}, nil
}

func TestResultLimit(t *testing.T) {
	e := New(WithoutBuiltins())
	e.Register(fixedResultConverter{markdown: "12345"}, PrioritySpecific)
	ctx := WithResultLimit(t.Context(), 4)
	if limit, ok := ResultLimit(WithResultLimit(ctx, 8)); !ok || limit != 4 {
		t.Fatalf("nested result limit = %d, %v; want 4, true", limit, ok)
	}
	res, err := e.Convert(ctx, strings.NewReader("input"), StreamInfo{})
	if !errors.Is(err, ErrResultTooLarge) {
		t.Fatalf("err = %v (res=%v), want ErrResultTooLarge", err, res)
	}
}

func TestResultLimitIncludesFinalNormalization(t *testing.T) {
	e := New(WithoutBuiltins())
	e.Register(fixedResultConverter{markdown: "1234"}, PrioritySpecific)
	res, err := e.Convert(WithResultLimit(t.Context(), 4), strings.NewReader("input"), StreamInfo{})
	if !errors.Is(err, ErrResultTooLarge) {
		t.Fatalf("err = %v (res=%v), want ErrResultTooLarge after final newline", err, res)
	}
}

type proactiveLimitConverter struct{}

func (proactiveLimitConverter) Accepts(StreamInfo) bool { return true }
func (proactiveLimitConverter) Convert(context.Context, io.ReadSeeker, StreamInfo) (*Result, error) {
	return nil, ErrResultTooLarge
}

func TestProactiveResultLimitDoesNotFallBack(t *testing.T) {
	e := New(WithoutBuiltins())
	e.Register(fixedResultConverter{markdown: "fallback"}, PriorityGeneric)
	e.Register(proactiveLimitConverter{}, PrioritySpecific)
	res, err := e.Convert(t.Context(), strings.NewReader("input"), StreamInfo{})
	if !errors.Is(err, ErrResultTooLarge) || res != nil {
		t.Fatalf("err = %v (res=%v), want immediate ErrResultTooLarge", err, res)
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
	res, err := e.Convert(t.Context(), strings.NewReader("abc"), StreamInfo{})
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
	res, err := e.Convert(t.Context(), strings.NewReader("abc"), StreamInfo{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if res.Markdown != "ABC\n" {
		t.Errorf("Markdown = %q, want %q", res.Markdown, "ABC\n")
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
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
