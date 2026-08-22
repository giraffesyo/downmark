// Command downmark converts documents (PDF, DOC, DOCX, XLSX, PPTX, HTML, CSV, ZIP,
// and plain text) to Markdown.
//
// Usage:
//
//	downmark [flags] [file]
//
// Reads file, or stdin if file is omitted or "-", and writes Markdown to
// stdout (or to -o file). With -json it writes one JSON object carrying the
// Markdown, the title, and the warnings instead, which is the form the
// @giraffesyo/downmark npm package drives the binary through.
//
// Scanned PDF pages hold no text to extract. Pass -ocr tesseract (or
// -ocr-cmd, for another engine) to read them with an OCR engine; pages
// it filled in are marked in the Markdown, and -ocr-max-pages and the
// timeout flags bound what a large scan is allowed to cost.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/all"
)

// version is overridden at release time via -ldflags "-X main.version=...".
var version = ""

func main() {
	os.Exit(run())
}

func run() int {
	flag.Usage = func() {
		// Nothing useful can be done if writing usage text fails.
		_, _ = fmt.Fprintf(flag.CommandLine.Output(),
			"usage: downmark [flags] [file]\n\nReads <file>, or stdin if omitted or \"-\", and writes Markdown to stdout.\n\n")
		flag.PrintDefaults()
	}
	outPath := flag.String("o", "", "write output to `file` instead of stdout")
	extHint := flag.String("x", "", "extension hint, e.g. `.docx` (leading dot optional)")
	mimeHint := flag.String("m", "", "MIME type hint, e.g. `application/pdf`")
	charsetHint := flag.String("c", "", "charset hint, e.g. `shift_jis`")
	keepDataURIs := flag.Bool("keep-data-uris", false, "keep full data: URIs in output instead of truncating")
	asJSON := flag.Bool("json", false, "write one JSON object {markdown, title, warnings} instead of Markdown")
	resultLimit := flag.Int("result-limit", 0, "reject results larger than `bytes` (0 for no limit)")
	quiet := flag.Bool("q", false, "suppress the warnings reporting what the conversion lost")
	showVersion := flag.Bool("version", false, "print version and exit")
	ocrOpts := registerOCRFlags()
	flag.Parse()

	if *showVersion {
		fmt.Println("downmark", resolveVersion())
		return 0
	}
	if flag.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "downmark: too many arguments")
		flag.Usage()
		return 2
	}

	hints := downmark.StreamInfo{
		Charset: strings.TrimSpace(*charsetHint),
	}
	if e := strings.TrimSpace(*extHint); e != "" {
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		hints.Extension = strings.ToLower(e)
	}
	if m := strings.TrimSpace(*mimeHint); m != "" {
		if strings.Count(m, "/") != 1 {
			fmt.Fprintf(os.Stderr, "downmark: invalid MIME type %q\n", m)
			return 2
		}
		hints.MIMEType = m
	}

	if *resultLimit < 0 {
		fmt.Fprintln(os.Stderr, "downmark: -result-limit cannot be negative")
		return 2
	}

	pdfOpts, err := ocrOpts.pdfOptions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "downmark: %v\n", err)
		return 2
	}
	engine := all.New(all.Options{KeepDataURIs: *keepDataURIs, PDF: pdfOpts})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if *resultLimit > 0 {
		ctx = downmark.WithResultLimit(ctx, *resultLimit)
	}

	var res *downmark.Result
	if path := flag.Arg(0); path == "" || path == "-" {
		res, err = engine.Convert(ctx, os.Stdin, hints)
	} else {
		var f *os.File
		f, err = os.Open(filepath.Clean(path))
		if err != nil {
			fmt.Fprintf(os.Stderr, "downmark: %v\n", err)
			return 2
		}
		defer func() { _ = f.Close() }() // read-only handle
		info := hints
		if info.Extension == "" {
			info.Extension = strings.ToLower(filepath.Ext(path))
		}
		info.Filename = filepath.Base(path)
		info.LocalPath = path
		res, err = engine.Convert(ctx, f, info)
	}
	if err != nil {
		if *asJSON {
			writeJSONError(os.Stderr, err)
		} else {
			printConversionError(err)
		}
		return 1
	}

	out := res.Markdown
	if *asJSON {
		// The warnings ride inside the object, so -q is already satisfied:
		// nothing reaches stderr on a successful -json run either way.
		b, err := encodeJSON(newJSONResult(res))
		if err != nil {
			writeJSONError(os.Stderr, fmt.Errorf("downmark: encoding the result: %w", err))
			return 1
		}
		out = string(b)
	} else if !*quiet {
		printWarnings(res.Warnings)
	}

	if *outPath != "" {
		if err := writeOutput(*outPath, out); err != nil {
			fmt.Fprintf(os.Stderr, "downmark: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := os.Stdout.WriteString(out); err != nil {
		fmt.Fprintf(os.Stderr, "downmark: %v\n", err)
		return 1
	}
	return 0
}

// printWarnings reports what the conversion lost on stderr, leaving stdout
// to the Markdown alone. Conversion succeeded, so this never changes the
// exit status.
func printWarnings(ws []downmark.Warning) {
	for _, w := range ws {
		fmt.Fprintf(os.Stderr, "downmark: %s: %v\n", w.Code, w)
	}
}

// writeOutput creates the output file with umask-derived permissions and
// surfaces write and close errors (close reports write-back failures).
func writeOutput(path, content string) error {
	f, err := os.Create(filepath.Clean(path))
	if err != nil {
		return err
	}
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func printConversionError(err error) {
	var convErr *downmark.ConversionError
	switch {
	case errors.As(err, &convErr):
		fmt.Fprintln(os.Stderr, "downmark: conversion failed:")
		for _, a := range convErr.Attempts {
			fmt.Fprintf(os.Stderr, "  %s\n", a.Error())
		}
	case errors.Is(err, downmark.ErrUnsupportedFormat):
		fmt.Fprintln(os.Stderr, "downmark: unsupported format: no converter accepted the input (try -x or -m to hint the type)")
	default:
		fmt.Fprintf(os.Stderr, "downmark: %v\n", err)
	}
}

func resolveVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "devel"
}
