// Ambient declaration for the environment-specific loader module. The
// concrete implementation is chosen at bundle time via esbuild --alias:
// loader.node.ts (ESM), loader.node.cjs.ts (CJS), or loader.browser.ts.
declare module "#wasm-loader" {
  import type { WasmSource } from "./wasm-source.js";
  /** Load the downmark.wasm that ships next to the built module. */
  export function defaultWasmSource(): Promise<WasmSource>;
  /** Resolve a caller-supplied path or URL to a wasm source. */
  export function resolveWasmSource(src: string | URL): Promise<WasmSource>;
}
