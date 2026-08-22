import { initRuntime } from "./runtime.js";
import { DownmarkError, rehydrateError } from "./errors.js";
import { binaryPath as locateBinary, convertNative } from "#native";
import type { ConvertHints, ConvertOptions, ConvertResult } from "./types.js";
import type { WasmSourceInput } from "./wasm-source.js";

export {
  DownmarkError,
  ConversionFailedError,
  type ConversionAttempt,
  type DownmarkErrorCode,
} from "./errors.js";
export type {
  ConvertHints,
  ConvertOptions,
  ConvertResult,
  ConvertWarning,
  OcrOptions,
  WarningCode,
} from "./types.js";
export type { WasmSourceInput } from "./wasm-source.js";

/** Options for init(). */
export interface InitOptions {
  /** Where downmark.wasm comes from; same as passing the source directly. */
  wasm?: WasmSourceInput;
  /**
   * Ignore an installed native binary and run everything on the wasm, for
   * comparing the two. The environment variable DOWNMARK_FORCE_WASM does
   * the same without a code change.
   */
  preferWasm?: boolean;
}

// Set by init({preferWasm}). Read on every convert() rather than captured,
// so it takes effect whenever it is set.
let forceWasm = false;

/**
 * Load and start the wasm module. Optional: convert() and canConvert() call
 * it implicitly. Call it explicitly to override where downmark.wasm comes
 * from (e.g. a bundler asset URL), or to force the wasm path. It must then
 * run before the first convert/canConvert.
 *
 * This concerns the wasm implementation only. Where a platform package
 * installed the native binary, convert() never touches the wasm and never
 * needs this.
 */
export async function init(
  source?: WasmSourceInput | InitOptions,
): Promise<void> {
  const opts = asInitOptions(source);
  if (opts) {
    if (opts.preferWasm !== undefined) forceWasm = opts.preferWasm;
    await initRuntime(opts.wasm);
    return;
  }
  await initRuntime(source as WasmSourceInput | undefined);
}

/**
 * Distinguish an options object from the wasm sources init() has always
 * accepted. Every source is either a primitive-ish value or a known class,
 * so a bare object carrying either option can only be the new form.
 */
function asInitOptions(
  value: WasmSourceInput | InitOptions | undefined,
): InitOptions | null {
  if (value === null || typeof value !== "object") return null;
  if (value instanceof URL) return null;
  if (value instanceof ArrayBuffer || ArrayBuffer.isView(value)) return null;
  if (typeof Response !== "undefined" && value instanceof Response) return null;
  if (value instanceof WebAssembly.Module) return null;
  return "wasm" in value || "preferWasm" in value ? (value as InitOptions) : null;
}

/**
 * Path to the native binary this process would run, or null when none is
 * installed and conversions fall back to the wasm. Exported for callers
 * that would rather exec the CLI themselves.
 */
export function binaryPath(): string | null {
  return forceWasm ? null : locateBinary();
}

/**
 * Convert a document to Markdown.
 *
 * Where the per-platform package installed the native binary, this spawns
 * it; otherwise it runs the same conversion on the wasm. The result is the
 * same either way, except that `ocr` needs the binary.
 */
export async function convert(
  data: Uint8Array | ArrayBuffer,
  opts: ConvertOptions = {},
): Promise<ConvertResult> {
  validateOptions(opts);
  const u8 = data instanceof Uint8Array ? data : new Uint8Array(data);

  const bin = binaryPath();
  if (bin) return convertNative(bin, u8, opts);

  if (opts.ocr) {
    throw new DownmarkError(
      "downmark: ocr needs the native binary, and no @giraffesyo/downmark-" +
        "<platform> package is installed for this host; install it, or drop " +
        "the ocr option to convert without reading scanned pages",
      "NATIVE_REQUIRED",
    );
  }
  const api = await initRuntime();
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

/**
 * Reject options that neither implementation would accept, before either
 * one runs, so a mistake reads the same whichever path a host takes.
 */
function validateOptions(opts: ConvertOptions): void {
  if (opts.resultLimit !== undefined) {
    requirePositiveInt(opts.resultLimit, "resultLimit");
  }
  const ocr = opts.ocr;
  if (!ocr) return;
  if (ocr.engine !== "tesseract") {
    throw new DownmarkError(
      `downmark: unknown OCR engine ${String(ocr.engine)}; only "tesseract" is built in`,
      "INTERNAL",
    );
  }
  if (ocr.policy !== undefined && ocr.policy !== "textless" && ocr.policy !== "images") {
    throw new DownmarkError(
      `downmark: unknown OCR policy ${String(ocr.policy)}; use "textless" or "images"`,
      "INTERNAL",
    );
  }
  if (ocr.minConfidence !== undefined) {
    if (
      !Number.isFinite(ocr.minConfidence) ||
      ocr.minConfidence < 0 ||
      ocr.minConfidence > 100
    ) {
      throw new DownmarkError(
        "downmark: ocr.minConfidence must be between 0 and 100",
        "INTERNAL",
      );
    }
  }
  // Zero is meaningful for these three: it is how the binary spells "no
  // limit", so they are bounded below rather than required positive.
  for (const [name, value] of [
    ["maxPages", ocr.maxPages],
    ["pageTimeoutMs", ocr.pageTimeoutMs],
    ["timeoutMs", ocr.timeoutMs],
  ] as const) {
    if (value === undefined) continue;
    if (!Number.isInteger(value) || value < 0) {
      throw new DownmarkError(
        `downmark: ocr.${name} must be a non-negative integer`,
        "INTERNAL",
      );
    }
  }
}

function requirePositiveInt(value: number, name: string): void {
  if (!Number.isInteger(value) || value <= 0) {
    throw new DownmarkError(
      `downmark: ${name} must be a positive integer`,
      "INTERNAL",
    );
  }
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
  if (opts.resultLimit !== undefined) out.resultLimit = opts.resultLimit;
  return out;
}
