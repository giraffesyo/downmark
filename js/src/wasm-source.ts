/** Forms the wasm binary can arrive in before instantiation. */
export type WasmSource = BufferSource | Response | WebAssembly.Module;

/** What callers may pass to init() to override where the wasm comes from. */
export type WasmSourceInput =
  | string
  | URL
  | ArrayBuffer
  | Uint8Array
  | Response
  | WebAssembly.Module;
