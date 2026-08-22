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
	"github.com/giraffesyo/downmark/internal/errcode"
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
// warningValues renders the conversion's warnings for JS. The structured
// fields survive the boundary; the Go error behind each one cannot, so it
// crosses as its message.
func warningValues(ws []downmark.Warning) []any {
	out := make([]any, len(ws))
	for i, w := range ws {
		out[i] = map[string]any{
			"converter": w.Converter,
			"code":      string(w.Code),
			"location":  w.Location,
			"message":   w.Error(),
		}
	}
	return out
}

func errorObject(err error) js.Value {
	// Classification lives in internal/errcode so that this and the CLI's
	// -json mode report the same code for the same failure; the wrapper
	// rehydrates either one into the same typed error.
	code, attempts := errcode.Of(err)
	obj := map[string]any{
		"name":    "DownmarkError",
		"code":    code,
		"message": err.Error(),
	}
	if attempts != nil {
		vals := make([]any, len(attempts))
		for i, a := range attempts {
			vals[i] = map[string]any{"converter": a.Converter, "message": a.Message}
		}
		obj["attempts"] = vals
	}
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
				"warnings": warningValues(res.Warnings),
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
