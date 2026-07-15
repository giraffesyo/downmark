package downmark

import (
	"errors"
	"fmt"
	"strings"
)

// ErrUnsupportedFormat is returned when no registered converter accepted the
// input under any detected interpretation of its type.
var ErrUnsupportedFormat = errors.New("downmark: unsupported format")

// ErrInputTooLarge is returned when a converter's hard input budget is
// exceeded, including while the engine buffers a non-seekable stream.
var ErrInputTooLarge = errors.New("downmark: input exceeds size limit")

// ErrResultTooLarge is returned when a conversion result exceeds the maximum
// size carried by its context via WithResultLimit.
var ErrResultTooLarge = errors.New("downmark: result exceeds size limit")

// AttemptError records a single converter's failed conversion attempt.
type AttemptError struct {
	// Converter is the name of the converter that failed, e.g. "pdf".
	Converter string
	// Err is the error the converter returned.
	Err error
}

func (e *AttemptError) Error() string {
	return fmt.Sprintf("%s: %v", e.Converter, e.Err)
}

func (e *AttemptError) Unwrap() error { return e.Err }

// ConversionError is returned when at least one converter accepted the input
// but every attempt failed. Attempts lists each failure in the order tried.
type ConversionError struct {
	Attempts []*AttemptError
}

func (e *ConversionError) Error() string {
	if len(e.Attempts) == 0 {
		return "downmark: conversion failed"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "downmark: conversion failed after %d attempt(s):", len(e.Attempts))
	for _, a := range e.Attempts {
		b.WriteString("\n  ")
		b.WriteString(a.Error())
	}
	return b.String()
}

func (e *ConversionError) Unwrap() []error {
	errs := make([]error, len(e.Attempts))
	for i, a := range e.Attempts {
		errs[i] = a
	}
	return errs
}
