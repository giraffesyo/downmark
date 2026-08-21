package downmark_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/giraffesyo/downmark"
)

func TestWarningErrorFormatting(t *testing.T) {
	base := errors.New("stream would not decode")
	tests := []struct {
		name string
		w    downmark.Warning
		want string
	}{
		{
			name: "converter and location",
			w:    downmark.Warning{Converter: "pdf", Location: "page 12", Err: base},
			want: "pdf: page 12: stream would not decode",
		},
		{
			name: "document-wide",
			w:    downmark.Warning{Converter: "pdf", Err: base},
			want: "pdf: stream would not decode",
		},
		{
			name: "bare",
			w:    downmark.Warning{Err: base},
			want: "stream would not decode",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.w.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

// The point of carrying an error rather than a message: a caller can
// still reach the source library's own type, with the detail the core
// vocabulary is too coarse to express.
func TestWarningUnwrapsToSourceError(t *testing.T) {
	type sourceError struct{ error }
	src := sourceError{errors.New("page 3 truncated")}
	w := downmark.Warning{Converter: "pdf", Code: downmark.WarningIncomplete, Err: src}

	var got sourceError
	if !errors.As(w, &got) {
		t.Fatalf("errors.As(%v) did not recover the source error", w)
	}
	if got.Error() != "page 3 truncated" {
		t.Errorf("recovered %q, want the original", got.Error())
	}
}

func TestAppendWarningStopsAtBound(t *testing.T) {
	var ws []downmark.Warning
	for i := range downmark.MaxWarnings * 2 {
		ws = downmark.AppendWarning(ws, downmark.Warning{
			Converter: "pdf",
			Code:      downmark.WarningIncomplete,
			Err:       fmt.Errorf("warning %d", i),
		})
	}
	if len(ws) != downmark.MaxWarnings {
		t.Fatalf("len = %d, want the bound %d", len(ws), downmark.MaxWarnings)
	}
	// The last slot must say the list was cut, not carry one more
	// warning, or a caller cannot tell a full list from a truncated one.
	last := ws[len(ws)-1]
	if !errors.Is(last, downmark.ErrWarningsSuppressed) {
		t.Errorf("last = %v, want the suppression marker", last)
	}
	if prev := ws[len(ws)-2]; errors.Is(prev, downmark.ErrWarningsSuppressed) {
		t.Errorf("second to last = %v, want a real warning below the marker", prev)
	}
}

func TestAppendWarningsUnderBound(t *testing.T) {
	ws := downmark.AppendWarnings(nil,
		downmark.Warning{Converter: "zip", Code: downmark.WarningSkipped, Err: errors.New("a")},
		downmark.Warning{Converter: "zip", Code: downmark.WarningSkipped, Err: errors.New("b")},
	)
	if len(ws) != 2 {
		t.Fatalf("len = %d, want 2", len(ws))
	}
}
