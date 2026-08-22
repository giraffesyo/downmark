// The public option and result types, in their own module so that both
// implementations — the wasm runtime and the native binary — can name them
// without importing the entry point that routes between them.

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

/**
 * How to read scanned PDF pages, which hold no text to extract. OCR runs
 * an engine that is not downmark's and costs about a second a page, so it
 * is off unless an engine is named.
 *
 * It needs the native binary: the wasm build cannot execute one. On a host
 * with no platform package installed, passing this rejects with
 * NATIVE_REQUIRED rather than quietly returning pages of nothing.
 */
export interface OcrOptions {
  /** The engine to run. Only tesseract is built in, and it must be installed. */
  engine: "tesseract";
  /** Run this executable instead of the one on PATH. */
  bin?: string;
  /** Language in tesseract's own syntax, e.g. "eng" or "eng+deu". */
  lang?: string;
  /** Drop OCR'd words below this confidence, on tesseract's 0-100 scale. */
  minConfidence?: number;
  /**
   * Which pages to read: "textless" (the default) for pages with no text of
   * their own, or "images" to also read scanned figures on pages that have
   * text.
   */
  policy?: "textless" | "images";
  /** OCR at most this many pages per document; omit for no limit. */
  maxPages?: number;
  /** Give up on one page's OCR after this many milliseconds. */
  pageTimeoutMs?: number;
  /** Give up on OCR for the whole document after this many milliseconds. */
  timeoutMs?: number;
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
  /** Read scanned PDF pages with an OCR engine. Native binary only. */
  ocr?: OcrOptions;
}

/** What a conversion classifies itself as having lost. */
export type WarningCode = "incomplete" | "skipped";

/**
 * A recoverable problem that left a conversion incomplete. Its presence
 * does not mean the conversion failed: the Markdown is usable, but it is
 * not everything the input held.
 */
export interface ConvertWarning {
  /** Converter that produced it, e.g. "pdf". */
  converter: string;
  /** What the output lost. */
  code: WarningCode;
  /** Affected part in the format's own terms ("page 12"), or "". */
  location: string;
  /** Human-readable rendering of the underlying failure. */
  message: string;
}

/** The outcome of a successful conversion. */
export interface ConvertResult {
  /** Normalized Markdown output. */
  markdown: string;
  /** Document title if the format provides one, else "". */
  title: string;
  /** What the conversion lost; empty when it lost nothing. */
  warnings: ConvertWarning[];
}
