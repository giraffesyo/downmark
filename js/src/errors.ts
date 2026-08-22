/** Error codes surfaced by downmark. */
export type DownmarkErrorCode =
  | "UNSUPPORTED_FORMAT"
  | "INPUT_TOO_LARGE"
  | "RESULT_TOO_LARGE"
  | "CONVERSION_FAILED"
  /**
   * The call needs the native binary and this host has none installed: OCR
   * on the wasm fallback, or any convert() in a browser with `ocr` set.
   */
  | "NATIVE_REQUIRED"
  | "INTERNAL";

/** Base error for every failure reported by downmark. */
export class DownmarkError extends Error {
  readonly code: DownmarkErrorCode;

  constructor(message: string, code: DownmarkErrorCode) {
    super(message);
    this.name = "DownmarkError";
    this.code = code;
  }
}

/** One converter's failed attempt inside a ConversionFailedError. */
export interface ConversionAttempt {
  /** Converter name, e.g. "pdf". */
  converter: string;
  /** The error message that converter reported. */
  message: string;
}

/**
 * Thrown when at least one converter accepted the input but every attempt
 * failed. `attempts` lists each failure in the order tried.
 */
export class ConversionFailedError extends DownmarkError {
  readonly attempts: ConversionAttempt[];

  constructor(message: string, attempts: ConversionAttempt[]) {
    super(message, "CONVERSION_FAILED");
    this.name = "ConversionFailedError";
    this.attempts = attempts;
  }
}

interface WireError {
  code?: unknown;
  message?: unknown;
  attempts?: unknown;
}

// The vocabulary Go reports, shared by the wasm module and the binary's
// -json mode. NATIVE_REQUIRED is deliberately absent: it is decided on
// this side of the boundary, so it can never arrive over one.
const KNOWN_CODES: ReadonlySet<string> = new Set([
  "UNSUPPORTED_FORMAT",
  "INPUT_TOO_LARGE",
  "RESULT_TOO_LARGE",
  "CONVERSION_FAILED",
  "INTERNAL",
]);

/**
 * Rehydrate a structured failure from Go ({code, message, attempts?}) into
 * a typed error class. Both implementations report in this shape — the
 * wasm module as a rejection value, the binary as JSON on stderr. Unknown
 * shapes become INTERNAL.
 */
export function rehydrateError(value: unknown): DownmarkError {
  if (value instanceof DownmarkError) return value;
  const wire = (value ?? {}) as WireError;
  const message =
    typeof wire.message === "string"
      ? wire.message
      : value instanceof Error
        ? value.message
        : String(value);
  const code =
    typeof wire.code === "string" && KNOWN_CODES.has(wire.code)
      ? (wire.code as DownmarkErrorCode)
      : "INTERNAL";
  if (code === "CONVERSION_FAILED") {
    const attempts: ConversionAttempt[] = Array.isArray(wire.attempts)
      ? wire.attempts.map((a: { converter?: unknown; message?: unknown }) => ({
          converter: typeof a?.converter === "string" ? a.converter : "",
          message: typeof a?.message === "string" ? a.message : "",
        }))
      : [];
    return new ConversionFailedError(message, attempts);
  }
  return new DownmarkError(message, code);
}
