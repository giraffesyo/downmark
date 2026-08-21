# downmark

[![Go Reference](https://pkg.go.dev/badge/github.com/giraffesyo/downmark.svg)](https://pkg.go.dev/github.com/giraffesyo/downmark)
[![CI](https://github.com/giraffesyo/downmark/actions/workflows/ci.yml/badge.svg?branch=canary)](https://github.com/giraffesyo/downmark/actions/workflows/ci.yml)

Convert documents to Markdown. Pure Go — no Python, no cgo, no external
tools. One static binary; also usable as a library where **you only link
the formats you import**.

downmark is a native Go re-imagining of Microsoft's
[markitdown](https://github.com/microsoft/markitdown): the same job (get
documents into clean, LLM-friendly Markdown) without the Python runtime and
native dependencies.

## Supported formats

| Format | Package | Notes |
|---|---|---|
| PDF | `convert/pdf` | Text extraction via [github.com/giraffesyo/pdf](https://github.com/giraffesyo/pdf): Form XObjects (Google Docs exports), Identity-H composite fonts, ToUnicode CMaps, segmented content streams, inline images; hard budgets against decompression bombs. No bundled OCR engine, but scanned pages can be routed to one you supply (`pdf.Options.OCR`), or to a tesseract you have installed (`downmark -ocr tesseract`); without one they return a clear error. |
| DOC | `convert/doc` | Word 97–2003 binary documents: bounded Compound Binary parsing, CLX piece-table reconstruction, ANSI/UTF-16 text, and displayed field results. Main-document text only; legacy formatting is not preserved. |
| DOCX | `convert/docx` | Headings, bold/italic/strikethrough, sub/superscript, nested lists, tables (incl. gridSpan/vMerge), hyperlinks, image placeholders, tracked changes. Equations degrade to plain text. |
| XLSX | `convert/xlsx` | Every sheet as `## SheetName` + a Markdown table. |
| PPTX | `convert/pptx` | Slides in order with `<!-- Slide number: N -->` markers, position-based reading order, tables, chart data tables, image alt text, speaker notes. |
| HTML | `convert/html` | DOM sanitization (hidden/inert content dropped, `javascript:` links unwrapped, data URIs truncated) + GFM tables. |
| CSV | `convert/csv` | Charset-aware (incl. Shift-JIS/cp932), delimiter sniffing (`, ; \t \|`). |
| ZIP | `convert/zipfile` | Supported members are converted through the engine and emitted under per-file headings; hard limits cap compressed archives at 64 MiB, total member data at 128 MiB (10 MiB/member), conversion at 64 members, scanning at 1,024 entries, and output at 32 MiB. |
| Plain text | built into core | Charset-detected passthrough (UTF-8/16, legacy encodings). |

## Install the CLI

```sh
go install github.com/giraffesyo/downmark/cmd/downmark@latest
```

```
usage: downmark [flags] [file]      # stdin if file omitted or "-"; Markdown → stdout
  -o file           write output to file
  -x .ext           extension hint (leading dot optional)
  -m type           MIME type hint
  -c charset        charset hint, e.g. shift_jis
  -keep-data-uris   keep full data: URIs
  -q                suppress warnings
  -version          print version

  -ocr engine       read scanned PDF pages with an installed engine ("tesseract")
  -ocr-cmd command  read them by running command; {} is the image path
  -ocr-lang lang    OCR language, in tesseract's syntax, e.g. eng+deu
  -ocr-policy p     textless (default) or images, to also read scanned figures
  -ocr-max-pages n  OCR at most n pages per document
  -ocr-page-timeout d, -ocr-timeout d   bound one page, and the whole document
```

See [OCR for scanned PDFs](#ocr-for-scanned-pdfs) for what the `-ocr` flags
do and what they cost.

Exit codes: `0` success, `1` conversion failed, `2` usage error.

## Library

Pay for what you import: the core engine pulls only lightweight detection
dependencies; each format package brings its own.

```go
import (
    "github.com/giraffesyo/downmark"
    "github.com/giraffesyo/downmark/convert/pdf"
)

e := downmark.New()              // core engine (plain-text passthrough builtin)
pdf.Register(e, pdf.Options{})   // link only the PDF stack

res, err := e.ConvertFile(ctx, "report.pdf")
// res.Markdown, res.Title
```

Everything wired up (what the CLI uses):

```go
import "github.com/giraffesyo/downmark/all"

res, err := all.ConvertFile(ctx, "report.docx")
// or: e := all.New(all.Options{KeepDataURIs: true})
```

Indicative binary sizes (`-trimpath -ldflags "-s -w"`, darwin/arm64): a
PDF-only consumer ≈ 4.9 MB, CSV-only ≈ 4.5 MB, everything ≈ 7.5 MB.

## Node.js and browsers

downmark also ships as an npm package,
[`@giraffesyo/downmark`](https://www.npmjs.com/package/@giraffesyo/downmark):
the full library compiled to WebAssembly with a TypeScript API for Node ≥ 18
and browsers. See [js/README.md](js/README.md).

```js
import { convert } from "@giraffesyo/downmark";

const { markdown, title } = await convert(data, { filename: "report.docx" });
```

## MarkItDown comparison

Downmark and [MarkItDown](https://github.com/microsoft/markitdown) solve the
same end-to-end problem, so the comparison uses their CLI interfaces against
the same fixtures. Each value is the median of ten complete conversions,
including process startup and Markdown written to stdout. The runner
alternates which CLI runs first on each measured pair to reduce order bias.
Both tools returned successfully for every fixture.

Measured 2026-07-15 from a worktree based on commit `152d4a8` with
`github.com/giraffesyo/pdf` v0.2.1, using Go `1.26.4`, MarkItDown `0.1.6`,
Python `3.14.2`, and macOS `26.5.1` on an Apple M5 Pro (`darwin/arm64`).
Re-run on your target environment before making a performance decision.

| Fixture | Downmark | MarkItDown |
|---|---:|---:|
| PDF | 7.0 ms | 394.5 ms |
| PDF (Form XObject + ToUnicode) | 5.6 ms | 337.7 ms |
| DOCX | 7.2 ms | 350.0 ms |
| XLSX | 6.7 ms | 342.1 ms |
| PPTX | 7.6 ms | 338.7 ms |
| HTML | 6.5 ms | 341.2 ms |
| Shift-JIS CSV | 5.5 ms | 339.8 ms |

Markdown is allowed to differ when both renderings are valid. The comparison
runner records each output's byte count and digest so changes are visible;
Downmark's golden tests define its output contract. Run the benchmark and
inspect its raw output with the commands in
[benchmarks/README.md](benchmarks/README.md).

Custom converters implement `downmark.Converter` (use
`StreamInfo.Matches` in `Accepts`) and register with
`e.Register(conv, downmark.PrioritySpecific)`; converters registered later
at the same priority are tried first, so yours shadow the builtins.

Errors: `errors.Is(err, downmark.ErrUnsupportedFormat)` when nothing
matched; `*downmark.ConversionError` (with per-converter attempts) when
converters matched but failed.

## Warnings

A conversion can succeed and still lose something — a PDF page that would
not decode, an archive member in a format nothing converts. `Result.Warnings`
reports what was lost, and is empty when nothing was:

```go
for _, w := range res.Warnings {
	fmt.Printf("%s: %s\n", w.Code, w) // "incomplete: pdf: page 12: ..."
}
```

`Code` is deliberately coarse — `WarningIncomplete` (content the input held
is missing from the output) and `WarningSkipped` (a whole unit was never
attempted) — because it is the part every format can answer. For detail,
unwrap to the source library's own error:

```go
var pw gpdf.Warning
if errors.As(w.Err, &pw) && pw.Code == gpdf.WarningOCR {
	// this page's text is missing because OCR failed on it
}
```

Warnings describe what varies per input. Limitations every file of a format
shares — DOCX flattening nested tables, DOC dropping headers — are in
[Limitations](#limitations) instead, so that anything in `Warnings` is
something that happened to *this* document. The list is bounded at
`MaxWarnings` (256); at the bound the final entry says so rather than being
one more warning.

The `downmark` CLI prints warnings to stderr, leaving stdout to the
Markdown; `-q` suppresses them.

## PDF extraction

PDF text extraction lives in its own module,
[github.com/giraffesyo/pdf](https://github.com/giraffesyo/pdf), usable
without the converter framework — positioned glyphs, line/word
reconstruction, and hardening against malformed and hostile files.

### OCR for scanned PDFs

downmark ships no OCR engine and takes on no such dependency. It exposes
the seam instead: give the PDF converter an implementation and pages the
content streams cannot read are handed to it.

From the CLI, `-ocr tesseract` runs a tesseract you have installed:

```console
$ downmark -ocr tesseract scan.pdf
<!-- downmark: page 1 includes OCR text -->

1 Introduction
Large language models (LLMs) are becoming a crucial building block...
```

`-ocr-cmd 'engine {} --tsv'` runs anything else that takes an image path
and writes tesseract-style TSV or plain text to stdout (`{}` is the image
path, appended if you leave it out), and `-ocr-lang` picks the language.
OCR costs roughly a second a page, so `-ocr-max-pages`, `-ocr-page-timeout`
(two minutes by default) and `-ocr-timeout` bound what a large scan is
allowed to cost; pages turned away by a budget are reported once rather
than once each. `-ocr-policy images` also reads scanned figures on pages
that have text of their own.

Pages OCR filled in are marked in the Markdown with an HTML comment, as
above: OCR text is a reading of the ink rather than the document's own
characters, and a consumer that cannot tell the two apart cannot weigh
them differently.

In Go, `ocr/exec` is the same engine the CLI uses, and `ocr` holds the
parts worth reusing under a different one — mapping an engine's word
boxes onto the page, and bounding what it may spend:

```go
import (
	"github.com/giraffesyo/downmark/ocr"
	ocrexec "github.com/giraffesyo/downmark/ocr/exec"
)

engine, err := ocrexec.New(ocrexec.Tesseract("eng"))
if err != nil {
	return err // tesseract is not installed
}
pdf.Register(e, pdf.Options{
	OCR: ocr.Limit(engine, ocr.Limits{MaxPages: 30, PerPage: time.Minute}),
})
```

Neither package is linked unless you import it, and neither is reachable
from the WebAssembly build: `-ocr-cmd` runs a process, which the browser
and Node builds cannot do.

Under it all is the seam itself, which takes any implementation:

```go
import (
	gpdf "github.com/giraffesyo/pdf"
	"github.com/giraffesyo/downmark/convert/pdf"
)

pdf.Register(e, pdf.Options{
	OCR: gpdf.OCRFunc(func(ctx context.Context, req gpdf.OCRRequest) ([]gpdf.Glyph, error) {
		// req.Page.Images carries the page's images with their encoded
		// data and placement; req.Reader and req.Size are the original
		// PDF, for implementations that render the page instead.
		return myEngine.Read(ctx, req)
	}),
})
```

The glyphs you return are positioned in unrotated page space (`Image.ToPage`
maps an engine's image coordinates there) and join the page's own text
before layout, so OCR'd pages flow into the Markdown like any other.

By default only textless pages are offered — scanned pages, and pages whose
text was converted to vector outlines. Set `OCRPolicy: gpdf.OCRImagePages`
for documents that mix typeset text with scanned figures or stamps, or
`gpdf.OCRAllPages` for every page. Pages are OCR'd concurrently, so the
implementation must be safe for concurrent use.

An engine that returns an error leaves that page textless rather than
failing the whole conversion; glyphs returned alongside an error are kept,
and the failure comes back in [`Result.Warnings`](#warnings) as a
`gpdf.Warning` with code `WarningOCR`.

Once an engine is configured, a page that is *still* textless afterwards is
reported too, as a warning matching `pdf.ErrPageNoText`. That is the list
worth acting on: those pages hold no images an OCR engine could read, so
their text is vector outlines, and recovering it needs a renderer rather
than OCR. Without an engine configured these go unreported, because a
textless page is not by itself a loss — a blank separator page is a normal
thing for a document to contain.

## Limitations

- **No bundled OCR engine.** Scanned/image-only PDFs and
  text-converted-to-outlines fail with "no extractable text" rather than
  silently emitting nothing, unless you supply an OCR implementation or
  install one for `-ocr` to drive — see
  [OCR for scanned PDFs](#ocr-for-scanned-pdfs).
- **OCR reads a page's images, not a rendering of the page.** Text
  converted to vector outlines paints no image, so no OCR engine can reach
  it; that case is reported rather than guessed at. A page scanned as
  several strips is read strip by strip, which is right except where a line
  of text is split across two of them.
- DOC: Word 97–2003 main-document text is extracted, but formatting, tables,
  images, headers/footers, footnotes, comments, and text boxes are not
  reconstructed. Word 6/95 files are not supported.
- PDF: complex multi-column layouts may interleave; fonts lacking both a
  ToUnicode map and a standard encoding are dropped rather than emitted as
  garbage; borderless table-layout reconstruction is not yet implemented.
- DOCX: headers/footers, footnotes, comments, and text boxes are skipped;
  equations render as plain text; nested tables flatten to text.
- Encrypted Office files, PDFs, and ZIP archives are not decrypted.
- OOXML archives (DOCX/XLSX/PPTX) are capped at 64 MiB compressed, 128 MiB
  total uncompressed, 16 MiB per decompressed part, and 1,024 entries; XML
  structure is also bounded to prevent small parts from creating huge trees.
- ZIP archives nested inside ZIP archives are skipped (reported in
  `Result.Warnings`); archive conversion does not recurse.
- Non-seekable inputs (stdin, network streams) are buffered in memory. Matching
  ZIP and Office inputs are rejected while buffering at their 64 MiB hard
  limit; formats without an input-limit converter remain fully buffered.

## Development

```sh
go test ./...                       # unit + vector + golden tests
go test -run TestGolden -update .   # refresh golden files
golangci-lint run ./...             # lint (config in .golangci.yml)
```

Test fixtures under `testdata/` partly come from
[microsoft/markitdown](https://github.com/microsoft/markitdown) (MIT); see
`testdata/NOTICE.md`.

## License

MIT
