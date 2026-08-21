package downmark

import (
	"errors"
	"fmt"
)

// MaxWarnings bounds the warnings a single Result carries. A badly
// damaged or hostile document can fault in every region it has; the cap
// keeps diagnostics proportional to output. The last slot reports that
// the list was cut rather than being one more warning.
const MaxWarnings = 256

// WarningCode classifies what a conversion lost. The vocabulary is
// deliberately small: it is the part every format can answer, and it is
// what a caller can route on without knowing which converter ran. For
// finer detail, match Warning.Err against the source library's own type.
type WarningCode string

const (
	// WarningIncomplete reports content the input held and the output
	// does not: a malformed region, a stream that would not decode, a
	// budget that cut the read short.
	WarningIncomplete WarningCode = "incomplete"
	// WarningSkipped reports a whole addressable unit that was never
	// attempted: an archive member in a format nothing converts, an
	// archive nested inside an archive.
	WarningSkipped WarningCode = "skipped"
)

// Warning reports a recoverable problem that left a conversion
// incomplete. A Result carrying warnings is still a successful
// conversion: the Markdown is usable, but it is not everything the input
// held.
//
// Warnings describe what varies per input. A limitation every file of a
// format shares — DOCX flattening nested tables, DOC dropping headers and
// footers — is documented rather than warned about, so that a caller who
// reads Warnings at all is reading something that happened to this
// document.
type Warning struct {
	// Converter names the converter that produced the warning, e.g.
	// "pdf". For a member converted from inside an archive it stays the
	// converter that read the member, not the archive's.
	Converter string
	// Code classifies what the output lost.
	Code WarningCode
	// Location names the affected part of the document in the format's
	// own terms — "page 12", "notes.txt", or "report.pdf: page 3" for a
	// member of an archive — and is empty when the warning covers the
	// whole document. It is meant for display; to match precisely, use
	// Err.
	Location string
	// Err is the underlying failure, carrying the source library's own
	// type so that errors.As can recover it.
	Err error
}

func (w Warning) Error() string {
	switch {
	case w.Converter != "" && w.Location != "":
		return fmt.Sprintf("%s: %s: %v", w.Converter, w.Location, w.Err)
	case w.Converter != "":
		return fmt.Sprintf("%s: %v", w.Converter, w.Err)
	default:
		return fmt.Sprint(w.Err)
	}
}

// Unwrap exposes the source library's own error, so that for example
// errors.As recovers a pdf.Warning with its page number and its own
// finer-grained code intact.
func (w Warning) Unwrap() error { return w.Err }

// ErrWarningsSuppressed is the Err of the warning that stands in for a
// warning list cut at MaxWarnings.
var ErrWarningsSuppressed = errors.New("downmark: additional warnings suppressed")

// AppendWarning appends w to ws, bounded by MaxWarnings. At the bound the
// final slot becomes ErrWarningsSuppressed, so a caller reading the list
// always learns that it is partial.
func AppendWarning(ws []Warning, w Warning) []Warning {
	switch {
	case len(ws) >= MaxWarnings:
		return ws
	case len(ws) == MaxWarnings-1:
		w = Warning{Converter: w.Converter, Code: WarningIncomplete, Err: ErrWarningsSuppressed}
	}
	return append(ws, w)
}

// AppendWarnings appends each of more to ws under the same bound.
func AppendWarnings(ws []Warning, more ...Warning) []Warning {
	for _, w := range more {
		ws = AppendWarning(ws, w)
	}
	return ws
}
