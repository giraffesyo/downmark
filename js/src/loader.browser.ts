// Browser loader: fetch downmark.wasm relative to the built module. Bundlers
// that rewrite new URL(..., import.meta.url) (Vite, webpack 5, etc.) will
// copy the wasm into their output automatically; otherwise use init() with
// an explicit URL (e.g. Vite's `?url` import).
import type { WasmSource } from "./wasm-source.js";

export async function defaultWasmSource(): Promise<WasmSource> {
  return fetch(new URL("./downmark.wasm", import.meta.url));
}

export async function resolveWasmSource(src: string | URL): Promise<WasmSource> {
  return fetch(src);
}
