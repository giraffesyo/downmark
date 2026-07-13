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
| PDF | `convert/pdf` | Text extraction via [github.com/giraffesyo/pdf](https://github.com/giraffesyo/pdf): Form XObjects (Google Docs exports), Identity-H composite fonts, ToUnicode CMaps, segmented content streams, inline images; hard budgets against decompression bombs. No OCR: scanned PDFs return a clear error. |
| DOCX | `convert/docx` | Headings, bold/italic/strikethrough, sub/superscript, nested lists, tables (incl. gridSpan/vMerge), hyperlinks, image placeholders, tracked changes. Equations degrade to plain text. |
| XLSX | `convert/xlsx` | Every sheet as `## SheetName` + a Markdown table. |
| PPTX | `convert/pptx` | Slides in order with `<!-- Slide number: N -->` markers, position-based reading order, tables, chart data tables, image alt text, speaker notes. |
| HTML | `convert/html` | DOM sanitization (scripts/styles dropped, `javascript:` links unwrapped, data URIs truncated) + GFM tables. |
| CSV | `convert/csv` | Charset-aware (incl. Shift-JIS/cp932), delimiter sniffing (`, ; \t \|`). |
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
  -version          print version
```

Exit codes: `0` success, `1` conversion failed, `2` usage error.

## Library

Pay for what you import: the core engine pulls only lightweight detection
dependencies; each format package brings its own.

```go
import (
    "github.com/giraffesyo/downmark"
    "github.com/giraffesyo/downmark/convert/pdf"
)

e := downmark.New() // core engine (plain-text passthrough builtin)
pdf.Register(e)     // link only the PDF stack

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

Custom converters implement `downmark.Converter` (use
`StreamInfo.Matches` in `Accepts`) and register with
`e.Register(conv, downmark.PrioritySpecific)`; converters registered later
at the same priority are tried first, so yours shadow the builtins.

Errors: `errors.Is(err, downmark.ErrUnsupportedFormat)` when nothing
matched; `*downmark.ConversionError` (with per-converter attempts) when
converters matched but failed.

## PDF extraction

PDF text extraction lives in its own module,
[github.com/giraffesyo/pdf](https://github.com/giraffesyo/pdf), usable
without the converter framework — positioned glyphs, line/word
reconstruction, and hardening against malformed and hostile files.

## Limitations

- **No OCR.** Scanned/image-only PDFs and text-converted-to-outlines fail
  with "no extractable text" rather than silently emitting nothing.
- PDF: complex multi-column layouts may interleave; fonts lacking both a
  ToUnicode map and a standard encoding are dropped rather than emitted as
  garbage; borderless table-layout reconstruction is not yet implemented.
- DOCX: headers/footers, footnotes, comments, and text boxes are skipped;
  equations render as plain text; nested tables flatten to text.
- Encrypted Office files and PDFs are not decrypted.
- Non-seekable inputs (stdin, network streams) are buffered fully in memory.

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
