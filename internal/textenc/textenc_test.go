package textenc

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/japanese"
)

func TestDetectCharsetUTF8(t *testing.T) {
	if got := DetectCharset([]byte("plain ascii")); got != "utf-8" {
		t.Errorf("ascii → %q", got)
	}
	if got := DetectCharset([]byte("\xEF\xBB\xBFwith bom")); got != "utf-8" {
		t.Errorf("bom → %q", got)
	}
	if got := DetectCharset([]byte("日本語のテキスト")); got != "utf-8" {
		t.Errorf("utf-8 japanese → %q", got)
	}
}

func TestDetectCharsetShiftJIS(t *testing.T) {
	enc := japanese.ShiftJIS.NewEncoder()
	sjis, err := enc.String("名前と年齢と住所、佐藤太郎は東京に住んでいます。三木英子は大阪。")
	if err != nil {
		t.Fatal(err)
	}
	got := DetectCharset([]byte(sjis))
	if got != "shift_jis" {
		t.Errorf("DetectCharset(sjis sample) = %q, want shift_jis", got)
	}
}

func TestNewReaderAliases(t *testing.T) {
	enc := japanese.ShiftJIS.NewEncoder()
	sjis, _ := enc.String("東京")
	for _, name := range []string{"cp932", "shift_jis", "Shift_JIS", "windows-31j"} {
		r, err := NewReader(strings.NewReader(sjis), name)
		if err != nil {
			t.Errorf("NewReader(%q): %v", name, err)
			continue
		}
		out, err := DecodeAll(r, "")
		if err != nil || out != "東京" {
			t.Errorf("decode via %q = %q, %v", name, out, err)
		}
	}
}

func TestNewReaderUnknown(t *testing.T) {
	if _, err := NewReader(strings.NewReader("x"), "not-a-charset"); err == nil {
		t.Error("expected error for unknown charset")
	}
}

func TestDecodeAllFallsBackOnUnknown(t *testing.T) {
	out, err := DecodeAll(strings.NewReader("raw"), "not-a-charset")
	if err != nil || out != "raw" {
		t.Errorf("DecodeAll fallback = %q, %v", out, err)
	}
}
