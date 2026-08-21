// Package downmark converts documents to Markdown.
//
// This package is the core: the Engine, converter registry, content-type
// detection, and a plain-text converter. Format converters live in
// separate packages so binaries link only the formats they use:
//
//   - github.com/giraffesyo/downmark/convert/pdf
//   - github.com/giraffesyo/downmark/convert/doc
//   - github.com/giraffesyo/downmark/convert/docx
//   - github.com/giraffesyo/downmark/convert/xlsx
//   - github.com/giraffesyo/downmark/convert/pptx
//   - github.com/giraffesyo/downmark/convert/html
//   - github.com/giraffesyo/downmark/convert/csv
//   - github.com/giraffesyo/downmark/convert/zipfile
//
// The github.com/giraffesyo/downmark/all package wires every converter
// into one engine for consumers who want the full matrix. Standalone PDF
// text extraction lives in the separate github.com/giraffesyo/pdf module.
//
// # Picking formats
//
//	e := downmark.New()          // engine with plain-text passthrough only
//	pdf.Register(e, pdf.Options{})   // + PDF
//	docx.Register(e, docx.Options{}) // + DOCX
//
//	res, err := e.ConvertFile(ctx, "report.pdf")
//	if err != nil { ... }
//	fmt.Println(res.Markdown)
//
// Streams work too; pass whatever type hints you have and detection fills
// in the rest by sniffing content:
//
//	res, err := e.Convert(ctx, resp.Body, downmark.StreamInfo{
//		MIMEType: resp.Header.Get("Content-Type"),
//	})
//
// Derive the context with WithResultLimit to reject oversized Markdown before
// the engine performs final normalization. Converters can inspect ResultLimit
// to enforce the same budget while constructing their result.
// Converters with hard compressed-input budgets can also implement
// InputLimitConverter so non-seekable streams are bounded while the engine
// buffers them.
//
// # Custom converters
//
// Implement Converter and register it; among converters with equal
// priority the most recently registered wins, so user converters shadow
// builtins. StreamInfo.Matches is the standard Accepts building block. A
// converter may also implement interface{ Name() string } to control how
// it is reported in errors.
//
// # Errors
//
// When no converter accepts an input, errors.Is(err, ErrUnsupportedFormat)
// reports true. When converters accepted but all failed, the error is a
// *ConversionError carrying each attempt.
package downmark
