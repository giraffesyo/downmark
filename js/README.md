# @giraffesyo/downmark

Convert documents to Markdown in Node.js and the browser: PDF, DOCX, XLSX, PPTX, DOC (Word 97–2003), HTML, CSV, ZIP archives, and plaintext. This is the pure-Go [downmark](https://github.com/giraffesyo/downmark) library, wrapped in a small TypeScript API. Everything runs locally, no network calls.

## Install

```sh
npm install @giraffesyo/downmark
```

Requires Node ≥ 18 or any modern browser.

## Two implementations, one API

`convert()` runs the same Go code either way, but there are two builds of it
and the package picks between them per host:

- **The native binary**, which ships inside a per-platform package
  (`@giraffesyo/downmark-linux-x64` and five siblings) that this package
  declares as an optional dependency. npm installs only the one matching the
  host's `os`/`cpu` and skips the rest. There is no download step and no
  postinstall script: the binary is inside the tarball, so `npm install`
  works behind a proxy and on an air-gapped cluster, `pnpm deploy` carries
  it, and the lockfile pins it.
- **The WebAssembly build**, bundled in this package, used wherever no
  platform package applies — browsers, an unlisted platform, an installer
  run with `--omit=optional`.

They return the same `{ markdown, title, warnings }` for the same input. The
native path is faster, runs the conversion in another process instead of on
the event loop, and is the only one that can do [OCR](#ocr-for-scanned-pdfs).
Nothing else about your code changes.

```js
import { binaryPath } from "@giraffesyo/downmark";

binaryPath(); // "/…/node_modules/@giraffesyo/downmark-darwin-arm64/bin/downmark", or null on wasm
```

Supported platforms: linux, darwin, and win32 on x64 and arm64. The Go
binaries are static, so the linux packages cover glibc and musl (Alpine)
alike.

Set `DOWNMARK_FORCE_WASM=1`, or call `init({ preferWasm: true })`, to ignore
an installed binary — useful for comparing the two. `DOWNMARK_BIN=/path/to/downmark`
points at a binary of your own; if it does not exist the call throws rather
than quietly falling back.

## Usage (Node)

```js
import { readFile } from "node:fs/promises";
import { convert } from "@giraffesyo/downmark";

const data = await readFile("report.docx");
const { markdown, title } = await convert(data, { filename: "report.docx" });
console.log(markdown);
```

CommonJS works too:

```js
const { convert } = require("@giraffesyo/downmark");
```

On the native path each conversion spawns the binary (about 5–10 ms of
process overhead) and nothing is loaded into your process. On the wasm path
the module is loaded lazily on the first call and cached, so subsequent
calls reuse the same instance.

## Usage (browser)

```js
import { convert } from "@giraffesyo/downmark";

const file = input.files[0];
const data = new Uint8Array(await file.arrayBuffer());
const { markdown } = await convert(data, { filename: file.name });
```

Browsers have no process to spawn, so this is always the wasm build.

The browser build fetches `downmark.wasm` relative to the module via `new URL("./downmark.wasm", import.meta.url)`. Bundlers that understand that pattern (Vite, webpack 5, Rollup with the url plugin) copy the wasm into the build output automatically.

If your bundler doesn't, point `init()` at the wasm explicitly before the first call:

```js
// Vite
import wasmUrl from "@giraffesyo/downmark/downmark.wasm?url";
import { init, convert } from "@giraffesyo/downmark";

await init(wasmUrl);
```

`init()` accepts a string URL/path, a `URL`, an `ArrayBuffer`/`Uint8Array`, a `Response`, or a precompiled `WebAssembly.Module`.

No bundler at all? See [`example/browser.html`](https://github.com/giraffesyo/downmark/blob/canary/js/example/browser.html) for a plain `<script type="module">` page.

**Serving tip:** the wasm is ~12 MB uncompressed. Serve it with `Content-Encoding: br` or `gzip` (≈4–6 MB over the wire) and a `Content-Type: application/wasm` header so `WebAssembly.instantiateStreaming` kicks in (the package falls back to buffering if the MIME type is wrong).

## API

### `convert(data, options?) → Promise<{ markdown, title, warnings }>`

`data` is a `Uint8Array` or `ArrayBuffer`. All options are optional, but the more hints you give the better the format detection:

| Option        | Type      | Meaning                                                                 |
| ------------- | --------- | ----------------------------------------------------------------------- |
| `filename`    | `string`  | Base name of the source file, e.g. `"report.docx"`                      |
| `mimeType`    | `string`  | Media type without parameters, e.g. `"application/pdf"`                 |
| `extension`   | `string`  | File extension (normalized to lowercase with a leading dot)             |
| `charset`     | `string`  | IANA charset name for text inputs, e.g. `"shift_jis"`                   |
| `keepDataUris`| `boolean` | Keep full `data:` URIs (HTML) / embed images as data URIs (DOCX)        |
| `resultLimit` | `number`  | Reject results larger than this many bytes with `RESULT_TOO_LARGE`      |
| `ocr`         | `object`  | Read scanned PDF pages with an OCR engine ([below](#ocr-for-scanned-pdfs)); native binary only |

`warnings` reports what the conversion lost, and is empty when it lost
nothing. A conversion that returns warnings still succeeded — the Markdown is
usable, it just is not everything the input held.

```js
const { markdown, warnings } = await convert(data, { filename: "scan.pdf" });
for (const w of warnings) {
  // { converter: "pdf", code: "incomplete", location: "page 12", message: "..." }
  console.warn(`${w.code}: ${w.message}`);
}
```

`code` is either `"incomplete"` (content the input held is missing from the
output) or `"skipped"` (a whole unit was never attempted).

### OCR for scanned PDFs

Scanned pages hold images, not text, so there is nothing to extract from
them. Pass `ocr` to hand those pages to an OCR engine instead:

```js
const { markdown } = await convert(data, {
  filename: "scan.pdf",
  ocr: { engine: "tesseract", lang: "eng", maxPages: 30 },
});
```

| Field           | Type                        | Meaning                                                       |
| --------------- | --------------------------- | ------------------------------------------------------------- |
| `engine`        | `"tesseract"`               | Required. The engine to run; it must be installed             |
| `bin`           | `string`                    | Run this executable instead of the one on `PATH`              |
| `lang`          | `string`                    | Language in tesseract's syntax, e.g. `"eng"` or `"eng+deu"`   |
| `minConfidence` | `number`                    | Drop words below this, on tesseract's 0–100 scale             |
| `policy`        | `"textless"` \| `"images"`  | Pages with no text of their own (default), or also scanned figures on pages that have text |
| `maxPages`      | `number`                    | OCR at most this many pages per document; `0` for no limit    |
| `pageTimeoutMs` | `number`                    | Give up on one page after this long                           |
| `timeoutMs`     | `number`                    | Give up on the whole document's OCR after this long           |

downmark ships no OCR engine: `engine: "tesseract"` runs a
[tesseract](https://github.com/tesseract-ocr/tesseract) you installed
yourself. OCR costs roughly a second a page, which is why it is off unless
asked for and why the budgets are there.

Pages OCR filled in are marked in the Markdown with an HTML comment
(`<!-- downmark: page 1 includes OCR text -->`): OCR text is a reading of the
ink rather than the document's own characters, and a consumer that cannot
tell the two apart cannot weigh them differently.

This needs the native binary — wasm cannot execute a program. Where none is
installed, passing `ocr` rejects with `NATIVE_REQUIRED` rather than
returning a document quietly missing its scanned pages.

### `binaryPath() → string | null`

The native binary the next `convert()` would run, or `null` when this host
falls back to the wasm. For callers who would rather exec the CLI
themselves — it accepts `-json` and writes the same object this package
returns.

### `canConvert(hints) → Promise<boolean>`

Whether a format-specific converter claims the format described by the hints (judged from hints alone, with no content sniffing). `false` doesn't mean `convert()` must fail; it may still classify the input by its bytes or fall back to plaintext.

### `init(source?) → Promise<void>`

Optional, and about the wasm implementation only. `convert()`/`canConvert()`
initialize implicitly; call `init()` to override where the wasm comes from
(see the bundler recipe above), in which case it must run before the first
conversion. On a host using the native binary, nothing needs it.

It also accepts an options object:

```js
await init({ wasm: wasmUrl, preferWasm: true });
```

`preferWasm: true` makes every conversion use the wasm even where a platform
package is installed.

### `version() → Promise<string>`

The downmark version the wasm was built from.

`canConvert()` and `version()` answer from the wasm even where the native
binary is installed, and so pay for loading it. A caller on the native path
that wants neither can skip both: `convert()` classifies the input itself,
and `binaryPath()` names a binary that answers `-version`.

### Errors

All failures reject with a `DownmarkError` carrying a `code`:

| Code                 | Meaning                                                        |
| -------------------- | -------------------------------------------------------------- |
| `UNSUPPORTED_FORMAT` | No converter accepted the input under any interpretation       |
| `INPUT_TOO_LARGE`    | A converter's hard input budget was exceeded                   |
| `RESULT_TOO_LARGE`   | The result exceeded `resultLimit`                              |
| `CONVERSION_FAILED`  | Converters accepted the input but every attempt failed         |
| `NATIVE_REQUIRED`    | The call needs the native binary and this host has none        |
| `INTERNAL`           | Bad arguments or an unexpected internal failure                |

`CONVERSION_FAILED` rejections are `ConversionFailedError` instances with an `attempts` array listing each converter that tried and why it failed:

```js
import { convert, ConversionFailedError } from "@giraffesyo/downmark";

try {
  await convert(data, { filename: "broken.pdf" });
} catch (err) {
  if (err instanceof ConversionFailedError) {
    for (const a of err.attempts) console.error(`${a.converter}: ${a.message}`);
  }
}
```

## Notes and limitations

- **On wasm, conversion is CPU-bound and single-threaded.** A big PDF blocks the thread it runs on, so in a browser UI, run conversions inside a Web Worker to keep the page responsive. There is no `timeout` option because a busy wasm instance can't be preempted. The native path does not have this problem: the work happens in another process, and the event loop stays free.
- **Very large XLSX parts:** a workbook containing any single internal part larger than 16 MiB triggers the underlying Excel library's spill-to-disk path, which fails on wasm (no filesystem). Such files reject with `CONVERSION_FAILED`.
- The wasm instance is shared per process/page. Conversions are safe to issue concurrently, but they execute one at a time. Native conversions are separate processes and do run in parallel.
- **The platform packages are an implementation detail.** Depend on `@giraffesyo/downmark`; never on `@giraffesyo/downmark-<platform>` directly.

## Development

Built from the repo root:

```sh
make js-test   # build wasm + TS + the binary, run the Node test suite
```

`make wasm` compiles `wasm/main.go` to `js/dist/downmark.wasm` and refreshes `js/vendor/wasm_exec.js` from the local Go toolchain. `make js-bin` builds `cmd/downmark` into `js/.bin`, which is what the native tests run; without it they skip.

The platform packages are built at release time, not committed:

```sh
goreleaser build --snapshot --clean            # dist/downmark_<goos>_<goarch>*/
node js/scripts/stage-npm-release.mjs --version 1.2.3
```

That writes `js/npm/<platform>/` from those binaries and stamps the version
and the six `optionalDependencies` into `js/package.json`. The dependencies
are generated rather than committed because `npm ci` validates
`package.json` against the lockfile, and a release commit names versions
that are not published yet — so every clean install would fail. Neither
output is meant to be committed; `.github/scripts/check-npm-packages.sh`
runs the same script in CI to keep it honest.
