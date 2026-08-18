# @giraffesyo/downmark

Convert documents to Markdown in Node.js and the browser: PDF, DOCX, XLSX, PPTX, DOC (Word 97–2003), HTML, CSV, ZIP archives, and plaintext. This is the pure-Go [downmark](https://github.com/giraffesyo/downmark) library compiled to WebAssembly and wrapped in a small TypeScript API. Zero runtime dependencies; everything runs locally, no network calls.

## Install

```sh
npm install @giraffesyo/downmark
```

Requires Node ≥ 18 or any modern browser. The package ships a ~12 MB `.wasm` binary (npm compresses the tarball for download).

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

The wasm module is loaded lazily on the first call and cached, so subsequent calls reuse the same instance.

## Usage (browser)

```js
import { convert } from "@giraffesyo/downmark";

const file = input.files[0];
const data = new Uint8Array(await file.arrayBuffer());
const { markdown } = await convert(data, { filename: file.name });
```

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

### `convert(data, options?) → Promise<{ markdown, title }>`

`data` is a `Uint8Array` or `ArrayBuffer`. All options are optional, but the more hints you give the better the format detection:

| Option        | Type      | Meaning                                                                 |
| ------------- | --------- | ----------------------------------------------------------------------- |
| `filename`    | `string`  | Base name of the source file, e.g. `"report.docx"`                      |
| `mimeType`    | `string`  | Media type without parameters, e.g. `"application/pdf"`                 |
| `extension`   | `string`  | File extension (normalized to lowercase with a leading dot)             |
| `charset`     | `string`  | IANA charset name for text inputs, e.g. `"shift_jis"`                   |
| `keepDataUris`| `boolean` | Keep full `data:` URIs (HTML) / embed images as data URIs (DOCX)        |
| `resultLimit` | `number`  | Reject results larger than this many bytes with `RESULT_TOO_LARGE`      |

### `canConvert(hints) → Promise<boolean>`

Whether a format-specific converter claims the format described by the hints (judged from hints alone, with no content sniffing). `false` doesn't mean `convert()` must fail; it may still classify the input by its bytes or fall back to plaintext.

### `init(source?) → Promise<void>`

Optional. `convert()`/`canConvert()` initialize implicitly; call `init()` only to override where the wasm comes from (see the bundler recipe above). Must run before the first conversion.

### `version() → Promise<string>`

The downmark version the wasm was built from.

### Errors

All failures reject with a `DownmarkError` carrying a `code`:

| Code                 | Meaning                                                        |
| -------------------- | -------------------------------------------------------------- |
| `UNSUPPORTED_FORMAT` | No converter accepted the input under any interpretation       |
| `INPUT_TOO_LARGE`    | A converter's hard input budget was exceeded                   |
| `RESULT_TOO_LARGE`   | The result exceeded `resultLimit`                              |
| `CONVERSION_FAILED`  | Converters accepted the input but every attempt failed         |
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

- **Conversion is CPU-bound and single-threaded.** A big PDF blocks the thread it runs on, so in a browser UI, run conversions inside a Web Worker to keep the page responsive. There is no `timeout` option because a busy wasm instance can't be preempted.
- **Very large XLSX parts:** a workbook containing any single internal part larger than 16 MiB triggers the underlying Excel library's spill-to-disk path, which fails on wasm (no filesystem). Such files reject with `CONVERSION_FAILED`.
- The wasm instance is shared per process/page. Conversions are safe to issue concurrently, but they execute one at a time.

## Development

Built from the repo root:

```sh
make js-test   # build wasm + TS, run the Node test suite
```

`make wasm` compiles `wasm/main.go` to `js/dist/downmark.wasm` and refreshes `js/vendor/wasm_exec.js` from the local Go toolchain.
