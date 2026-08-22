// Browsers have no processes to spawn, so the native path does not exist
// there and the wasm is the only implementation. Nothing here pulls in a
// node: builtin, which is what keeps the browser bundle buildable.
import { DownmarkError } from "./errors.js";
import type { ConvertOptions, ConvertResult } from "./types.js";

export function binaryPath(): null {
  return null;
}

export function convertNative(
  _bin: string,
  _data: Uint8Array,
  _opts: ConvertOptions,
): Promise<ConvertResult> {
  return Promise.reject(
    new DownmarkError(
      "downmark: the native binary cannot run in a browser",
      "NATIVE_REQUIRED",
    ),
  );
}
