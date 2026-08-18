import { initRuntime } from "./runtime.js";
import { DownmarkError, rehydrateError } from "./errors.js";
import type { WasmSourceInput } from "./wasm-source.js";

export {
  DownmarkError,
  ConversionFailedError,
  type ConversionAttempt,
  type DownmarkErrorCode,
} from "./errors.js";
export type { WasmSourceInput } from "./wasm-source.js";

/** Hints about the input; every field is optional but the more the better. */
export interface ConvertHints {
  /** Base name of the source file, e.g. "report.docx". */
  filename?: string;
  /** Media type without parameters, e.g. "application/pdf". */
  mimeType?: string;
  /** File extension; normalized to lowercase with a leading dot. */
  extension?: string;
  /** IANA charset name for text inputs, e.g. "shift_jis". */
  charset?: string;
}

/** Options for convert(). */
export interface ConvertOptions extends ConvertHints {
  /**
   * Preserve full data: URIs in output (HTML) and embed images as data URIs
   * (DOCX) instead of short placeholders.
   */
  keepDataUris?: boolean;
  /** Reject results larger than this many bytes with RESULT_TOO_LARGE. */
  resultLimit?: number;
}

/** The outcome of a successful conversion. */
export interface ConvertResult {
  /** Normalized Markdown output. */
  markdown: string;
  /** Document title if the format provides one, else "". */
  title: string;
}

/**
 * Load and start the wasm module. Optional: convert() and canConvert() call
 * it implicitly. Call it explicitly to override where downmark.wasm comes
 * from (e.g. a bundler asset URL) — it must then run before the first
 * convert/canConvert.
 */
export async function init(source?: WasmSourceInput): Promise<void> {
  await initRuntime(source);
}

/** Convert a document to Markdown. */
export async function convert(
  data: Uint8Array | ArrayBuffer,
  opts: ConvertOptions = {},
): Promise<ConvertResult> {
  const api = await initRuntime();
  const u8 = data instanceof Uint8Array ? data : new Uint8Array(data);
  try {
    return await api.convert(u8, marshalOptions(opts));
  } catch (err) {
    throw rehydrateError(err);
  }
}

/**
 * Report whether a format-specific converter claims the format described by
 * the hints (judged from the hints alone; no content is sniffed). False does
 * not mean convert() must fail: it may still classify the input by its bytes
 * or fall back to plain text.
 */
export async function canConvert(hints: ConvertHints): Promise<boolean> {
  const api = await initRuntime();
  return api.canConvert(marshalOptions(hints));
}

/** The downmark version the wasm module was built from. */
export async function version(): Promise<string> {
  const api = await initRuntime();
  return api.version;
}

function marshalOptions(opts: ConvertOptions): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  if (opts.filename) out.filename = opts.filename;
  if (opts.mimeType) out.mimeType = opts.mimeType;
  if (opts.extension) {
    let ext = opts.extension.toLowerCase();
    if (!ext.startsWith(".")) ext = "." + ext;
    out.extension = ext;
  }
  if (opts.charset) out.charset = opts.charset;
  if (opts.keepDataUris) out.keepDataUris = true;
  if (opts.resultLimit !== undefined) {
    if (!Number.isInteger(opts.resultLimit) || opts.resultLimit <= 0) {
      throw new DownmarkError(
        "downmark: resultLimit must be a positive integer",
        "INTERNAL",
      );
    }
    out.resultLimit = opts.resultLimit;
  }
  return out;
}
