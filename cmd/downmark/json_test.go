package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/giraffesyo/downmark"
)

// The npm wrapper reads this object field by field, so the names and the
// shape are the contract, not an implementation detail.
func TestJSONResultShape(t *testing.T) {
	res := &downmark.Result{
		Markdown: "# Hello <b> & co",
		Title:    "Hello",
		Warnings: []downmark.Warning{{
			Converter: "pdf",
			Code:      downmark.WarningIncomplete,
			Location:  "page 12",
			Err:       errors.New("stream would not decode"),
		}},
	}
	b, err := encodeJSON(newJSONResult(res))
	if err != nil {
		t.Fatalf("encodeJSON: %v", err)
	}
	// Markdown is full of <, > and &; escaping them would make the output
	// unreadable for no gain, since JSON does not require it.
	if bytes.Contains(b, []byte(`\u003c`)) {
		t.Errorf("output escapes HTML: %s", b)
	}
	var got jsonResult
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal %s: %v", b, err)
	}
	if got.Markdown != res.Markdown || got.Title != res.Title {
		t.Errorf("round trip = %+v, want markdown/title from %+v", got, res)
	}
	if len(got.Warnings) != 1 {
		t.Fatalf("warnings = %v, want 1", got.Warnings)
	}
	want := jsonWarning{
		Converter: "pdf",
		Code:      "incomplete",
		Location:  "page 12",
		Message:   "pdf: page 12: stream would not decode",
	}
	if got.Warnings[0] != want {
		t.Errorf("warning = %+v, want %+v", got.Warnings[0], want)
	}
}

// A caller that loops over warnings should not have to special-case the
// usual outcome, so the field is an empty array rather than null.
func TestJSONResultWarningsAreNeverNull(t *testing.T) {
	b, err := encodeJSON(newJSONResult(&downmark.Result{Markdown: "x"}))
	if err != nil {
		t.Fatalf("encodeJSON: %v", err)
	}
	if !bytes.Contains(b, []byte(`"warnings":[]`)) {
		t.Errorf("output = %s, want an empty warnings array", b)
	}
}

func TestJSONErrorCarriesCodeAndAttempts(t *testing.T) {
	cases := []struct {
		name         string
		err          error
		wantCode     string
		wantAttempts int
	}{
		{"unsupported", fmt.Errorf("wrapped: %w", downmark.ErrUnsupportedFormat), "UNSUPPORTED_FORMAT", 0},
		{"too large", downmark.ErrResultTooLarge, "RESULT_TOO_LARGE", 0},
		{"anything else", errors.New("disk on fire"), "INTERNAL", 0},
		{"conversion", &downmark.ConversionError{Attempts: []*downmark.AttemptError{
			{Converter: "docx", Err: errors.New("not a zip")},
		}}, "CONVERSION_FAILED", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			writeJSONError(&buf, tc.err)
			var got jsonFailure
			if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
				t.Fatalf("unmarshal %s: %v", buf.Bytes(), err)
			}
			if got.Error.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", got.Error.Code, tc.wantCode)
			}
			if len(got.Error.Attempts) != tc.wantAttempts {
				t.Errorf("attempts = %v, want %d", got.Error.Attempts, tc.wantAttempts)
			}
			if !strings.Contains(got.Error.Message, tc.err.Error()) {
				t.Errorf("message = %q, want it to carry %q", got.Error.Message, tc.err)
			}
		})
	}
}
