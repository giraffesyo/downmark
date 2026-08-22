package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/internal/errcode"
)

// jsonResult is what -json writes on success: one object, one line. The
// field names match the npm package's ConvertResult, because the wrapper
// hands this straight to the caller.
type jsonResult struct {
	Markdown string        `json:"markdown"`
	Title    string        `json:"title"`
	Warnings []jsonWarning `json:"warnings"`
}

// jsonWarning renders a Warning for a consumer that cannot see Go errors.
// The structured fields survive; the error behind one crosses as the same
// text the human-readable mode prints.
type jsonWarning struct {
	Converter string `json:"converter"`
	Code      string `json:"code"`
	Location  string `json:"location"`
	Message   string `json:"message"`
}

// jsonFailure is what -json writes on stderr when the conversion fails,
// leaving stdout empty. It is nested under "error" so a reader can tell a
// failure from a result without consulting the exit status.
type jsonFailure struct {
	Error jsonError `json:"error"`
}

type jsonError struct {
	Code     string            `json:"code"`
	Message  string            `json:"message"`
	Attempts []errcode.Attempt `json:"attempts,omitempty"`
}

func newJSONResult(res *downmark.Result) jsonResult {
	// Always an array, never null: a caller that iterates warnings should
	// not have to special-case the common case of none.
	ws := make([]jsonWarning, 0, len(res.Warnings))
	for _, w := range res.Warnings {
		ws = append(ws, jsonWarning{
			Converter: w.Converter,
			Code:      string(w.Code),
			Location:  w.Location,
			Message:   w.Error(),
		})
	}
	return jsonResult{Markdown: res.Markdown, Title: res.Title, Warnings: ws}
}

// encodeJSON renders v without HTML escaping, so Markdown carrying <, > or
// & stays readable in the output. Encoder.Encode ends with a newline.
func encodeJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeJSONError reports a failed conversion as JSON on w. A failure to
// render the failure is itself reported, so the caller never sees silence.
func writeJSONError(w io.Writer, err error) {
	code, attempts := errcode.Of(err)
	b, encErr := encodeJSON(jsonFailure{Error: jsonError{
		Code:     code,
		Message:  err.Error(),
		Attempts: attempts,
	}})
	if encErr != nil {
		// Nothing useful is left to do if even this cannot be written.
		_, _ = fmt.Fprintf(w, "{\"error\":{\"code\":%q,\"message\":\"downmark: failed to encode the error\"}}\n", errcode.Internal)
		return
	}
	_, _ = w.Write(b)
}
