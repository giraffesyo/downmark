// Ambient declaration for the environment-specific native module. The
// concrete implementation is chosen at bundle time via esbuild --alias:
// native.node.ts (ESM), native.node.cjs.ts (CJS), or native.browser.ts.
declare module "#native" {
  import type { ConvertOptions, ConvertResult } from "./types.js";
  /** Path to the platform package's binary, or null if none is installed. */
  export function binaryPath(): string | null;
  /** Convert by spawning the binary at `bin`. */
  export function convertNative(
    bin: string,
    data: Uint8Array,
    opts: ConvertOptions,
  ): Promise<ConvertResult>;
}
