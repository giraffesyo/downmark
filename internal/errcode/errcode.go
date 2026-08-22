// Package errcode classifies a conversion failure into the small, stable
// vocabulary the JS wrapper rehydrates into typed error classes.
//
// It exists so the two machine-readable entrypoints — the CLI's -json mode
// and the js/wasm build — cannot drift: a Node caller sees the same code for
// the same failure whichever one ran.
package errcode

import (
	"errors"

	"github.com/giraffesyo/downmark"
)

// The codes. Every failure maps to one of these; Internal is the fallback
// for anything the engine did not classify.
const (
	UnsupportedFormat = "UNSUPPORTED_FORMAT"
	InputTooLarge     = "INPUT_TOO_LARGE"
	ResultTooLarge    = "RESULT_TOO_LARGE"
	ConversionFailed  = "CONVERSION_FAILED"
	Internal          = "INTERNAL"
)

// Attempt is one converter's failed try, carried by a ConversionFailed
// error. The Go error behind it cannot cross the boundary, so it crosses
// as its message.
type Attempt struct {
	Converter string `json:"converter"`
	Message   string `json:"message"`
}

// Of classifies err. Attempts is non-nil only for ConversionFailed, where
// it lists each converter that accepted the input and then failed, in the
// order they were tried.
func Of(err error) (code string, attempts []Attempt) {
	var convErr *downmark.ConversionError
	switch {
	case errors.Is(err, downmark.ErrUnsupportedFormat):
		return UnsupportedFormat, nil
	case errors.Is(err, downmark.ErrInputTooLarge):
		return InputTooLarge, nil
	case errors.Is(err, downmark.ErrResultTooLarge):
		return ResultTooLarge, nil
	case errors.As(err, &convErr):
		attempts = make([]Attempt, len(convErr.Attempts))
		for i, a := range convErr.Attempts {
			attempts[i] = Attempt{Converter: a.Converter, Message: a.Err.Error()}
		}
		return ConversionFailed, attempts
	default:
		return Internal, nil
	}
}
