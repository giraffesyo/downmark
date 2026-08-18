//go:build js && wasm

// Command wasm is the js/wasm entrypoint for the @giraffesyo/downmark npm
// package. It exposes a single global object, __downmark, with convert and
// canConvert functions, then parks forever so exported js.Funcs stay valid.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"syscall/js"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/all"
)

// version is stamped at build time via -ldflags "-X main.version=...".
var version = "dev"

var keepDataURIsEngine = sync.OnceValue(func() *downmark.Engine {
	return all.New(all.Options{KeepDataURIs: true})
})

func main() {
	js.Global().Set("__downmark", js.ValueOf(map[string]any{
		"convert":    js.FuncOf(convert),
		"canConvert": js.FuncOf(canConvert),
		"version":    version,
	}))
	if ready := js.Global().Get("__downmark_ready"); ready.Type() == js.TypeFunction {
		ready.Invoke()
	}
	select {}
}

// optString reads a string property from a JS options object, returning ""
// for undefined/null/missing.
func optString(opts js.Value, key string) string {
	if opts.Type() != js.TypeObject {
		return ""
	}
	v := opts.Get(key)
	if v.Type() != js.TypeString {
		return ""
	}
	return v.String()
}

func optBool(opts js.Value, key string) bool {
	if opts.Type() != js.TypeObject {
		return false
	}
	v := opts.Get(key)
	return v.Type() == js.TypeBoolean && v.Bool()
}

func optInt(opts js.Value, key string) (int, bool) {
	if opts.Type() != js.TypeObject {
		return 0, false
	}
	v := opts.Get(key)
	if v.Type() != js.TypeNumber {
		return 0, false
	}
	return v.Int(), true
}

func hintsFrom(opts js.Value) downmark.StreamInfo {
	ext := optString(opts, "extension")
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return downmark.StreamInfo{
		Filename:  optString(opts, "filename"),
		MIMEType:  optString(opts, "mimeType"),
		Extension: strings.ToLower(ext),
		Charset:   optString(opts, "charset"),
	}
}

// errorObject builds the structured rejection value the JS wrapper rehydrates
// into typed error classes.
func errorObject(err error) js.Value {
	code := "INTERNAL"
	obj := map[string]any{"name": "DownmarkError"}
	var convErr *downmark.ConversionError
	switch {
	case errors.Is(err, downmark.ErrUnsupportedFormat):
		code = "UNSUPPORTED_FORMAT"
	case errors.Is(err, downmark.ErrInputTooLarge):
		code = "INPUT_TOO_LARGE"
	case errors.Is(err, downmark.ErrResultTooLarge):
		code = "RESULT_TOO_LARGE"
	case errors.As(err, &convErr):
		code = "CONVERSION_FAILED"
		attempts := make([]any, len(convErr.Attempts))
		for i, a := range convErr.Attempts {
			attempts[i] = map[string]any{
				"converter": a.Converter,
				"message":   a.Err.Error(),
			}
		}
		obj["attempts"] = attempts
	}
	obj["code"] = code
	obj["message"] = err.Error()
	return js.ValueOf(obj)
}

// convert implements __downmark.convert(u8, opts) -> Promise<{markdown, title}>.
func convert(_ js.Value, args []js.Value) any {
	// Copy the input bytes synchronously, before the promise executor's
	// goroutine runs, so the caller can reuse the buffer immediately.
	var (
		data   []byte
		argErr error
		opts   js.Value
	)
	if len(args) < 1 || args[0].Type() != js.TypeObject {
		argErr = errors.New("downmark: convert expects a Uint8Array")
	} else {
		u8 := args[0]
		data = make([]byte, u8.Get("byteLength").Int())
		js.CopyBytesToGo(data, u8)
	}
	if len(args) > 1 {
		opts = args[1]
	}
	hints := hintsFrom(opts)
	keepDataURIs := optBool(opts, "keepDataUris")
	resultLimit, hasResultLimit := optInt(opts, "resultLimit")

	executor := js.FuncOf(func(_ js.Value, promiseArgs []js.Value) any {
		resolve, reject := promiseArgs[0], promiseArgs[1]
		// Run the conversion on a fresh goroutine: blocking inside the
		// executor itself would deadlock the JS event loop.
		go func() {
			defer func() {
				if r := recover(); r != nil {
					reject.Invoke(errorObject(fmt.Errorf("downmark: internal panic: %v", r)))
				}
			}()
			if argErr != nil {
				reject.Invoke(errorObject(argErr))
				return
			}
			ctx := context.Background()
			if hasResultLimit && resultLimit > 0 {
				ctx = downmark.WithResultLimit(ctx, resultLimit)
			}
			var (
				res *downmark.Result
				err error
			)
			if keepDataURIs {
				res, err = keepDataURIsEngine().Convert(ctx, bytes.NewReader(data), hints)
			} else {
				res, err = all.Convert(ctx, bytes.NewReader(data), hints)
			}
			if err != nil {
				reject.Invoke(errorObject(err))
				return
			}
			resolve.Invoke(js.ValueOf(map[string]any{
				"markdown": res.Markdown,
				"title":    res.Title,
			}))
		}()
		return nil
	})
	promise := js.Global().Get("Promise").New(executor)
	executor.Release()
	return promise
}

// canConvert implements __downmark.canConvert(opts) -> boolean.
func canConvert(_ js.Value, args []js.Value) any {
	var opts js.Value
	if len(args) > 0 {
		opts = args[0]
	}
	return all.CanConvert(hintsFrom(opts))
}
