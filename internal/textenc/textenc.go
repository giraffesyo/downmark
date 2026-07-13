// Package textenc detects and decodes legacy text encodings to UTF-8.
package textenc

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/gogs/chardet"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/ianaindex"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// DetectCharset guesses the charset of prefix (a sample of a text stream).
// It returns a lowercase IANA-style name and falls back to "utf-8".
func DetectCharset(prefix []byte) string {
	switch {
	case bytes.HasPrefix(prefix, []byte{0xEF, 0xBB, 0xBF}):
		return "utf-8"
	case bytes.HasPrefix(prefix, []byte{0xFF, 0xFE}):
		return "utf-16le"
	case bytes.HasPrefix(prefix, []byte{0xFE, 0xFF}):
		return "utf-16be"
	}
	if utf8.Valid(prefix) {
		return "utf-8"
	}
	all, err := chardet.NewTextDetector().DetectAll(prefix)
	if err != nil || len(all) == 0 {
		return "utf-8"
	}
	// chardet's statistical models routinely rank single-byte charsets like
	// windows-1252 above CJK ones on short samples, where they are almost
	// never right (every byte "decodes" in windows-1252). A multi-byte
	// candidate that round-trips the sample without replacement characters
	// and yields CJK text is far stronger evidence, so check those first.
	slices.SortStableFunc(all, func(a, b chardet.Result) int {
		if a.Confidence != b.Confidence {
			return b.Confidence - a.Confidence
		}
		return multiBytePref(a.Charset) - multiBytePref(b.Charset)
	})
	for _, cand := range all {
		if multiBytePref(cand.Charset) < len(multiBytePrefOrder) && decodesAsCJK(prefix, cand.Charset) {
			return strings.ToLower(cand.Charset)
		}
	}
	return strings.ToLower(all[0].Charset)
}

// multiBytePrefOrder breaks confidence ties between multi-byte candidates.
// Ambiguous double-byte data is rare; when it happens Shift_JIS is the most
// common real-world source of chardet ties (cp932 spreadsheets/CSVs).
var multiBytePrefOrder = []string{"shift_jis", "euc-jp", "euc-kr", "big5", "gbk", "gb18030", "gb-18030-2000"}

func multiBytePref(charset string) int {
	name := strings.ToLower(charset)
	for i, c := range multiBytePrefOrder {
		if name == c {
			return i
		}
	}
	return len(multiBytePrefOrder)
}

// decodesAsCJK reports whether prefix decodes under charset without any
// replacement characters while producing at least one CJK rune.
func decodesAsCJK(prefix []byte, charset string) bool {
	r, err := NewReader(bytes.NewReader(prefix), charset)
	if err != nil {
		return false
	}
	decoded, err := io.ReadAll(r)
	if err != nil {
		return false
	}
	sawCJK := false
	for _, ru := range string(decoded) {
		if ru == utf8.RuneError {
			return false
		}
		if ru >= 0x2E80 && ru <= 0x9FFF || ru >= 0xF900 && ru <= 0xFAFF || ru >= 0xFF00 && ru <= 0xFFEF {
			sawCJK = true
		}
	}
	return sawCJK
}

// charsetAliases maps common vendor names that the html/IANA indexes don't
// resolve to names they do.
var charsetAliases = map[string]string{
	"cp932":       "shift_jis",
	"ms932":       "shift_jis",
	"ms_kanji":    "shift_jis",
	"sjis":        "shift_jis",
	"windows-31j": "shift_jis",
	"cp936":       "gbk",
	"cp950":       "big5",
	"cp949":       "euc-kr",
	"cp1252":      "windows-1252",
	"latin1":      "windows-1252",
}

// NewReader wraps r so it yields UTF-8, decoding from the named charset.
// An empty charset is treated as UTF-8. A UTF-8 BOM is stripped either way.
func NewReader(r io.Reader, charset string) (io.Reader, error) {
	name := strings.ToLower(strings.TrimSpace(charset))
	if alias, ok := charsetAliases[name]; ok {
		name = alias
	}
	switch name {
	case "", "utf-8", "utf8", "ascii", "us-ascii":
		return transform.NewReader(r, unicode.UTF8BOM.NewDecoder()), nil
	case "utf-16", "utf16", "utf-16le", "utf16le":
		return transform.NewReader(r, unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder()), nil
	case "utf-16be", "utf16be":
		return transform.NewReader(r, unicode.UTF16(unicode.BigEndian, unicode.UseBOM).NewDecoder()), nil
	}
	enc, err := htmlindex.Get(name)
	if err != nil {
		enc, err = ianaindex.IANA.Encoding(name)
		if err != nil || enc == nil {
			return nil, fmt.Errorf("textenc: unknown charset %q", charset)
		}
	}
	return transform.NewReader(r, enc.NewDecoder()), nil
}

// DecodeAll reads all of r decoded from the named charset. If the charset is
// unknown, the raw bytes are returned as-is (best effort).
func DecodeAll(r io.Reader, charset string) (string, error) {
	dec, err := NewReader(r, charset)
	if err != nil {
		dec = r
	}
	data, err := io.ReadAll(dec)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
