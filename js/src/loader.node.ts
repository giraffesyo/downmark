// Node ESM loader: read downmark.wasm from disk next to the built module.
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import type { WasmSource } from "./wasm-source.js";

export async function defaultWasmSource(): Promise<WasmSource> {
  return readFile(fileURLToPath(new URL("./downmark.wasm", import.meta.url)));
}

export async function resolveWasmSource(src: string | URL): Promise<WasmSource> {
  const url = typeof src === "string" && /^[a-z][a-z0-9+.-]*:/i.test(src) ? new URL(src) : src;
  if (url instanceof URL && (url.protocol === "http:" || url.protocol === "https:")) {
    return fetch(url);
  }
  if (url instanceof URL) {
    return readFile(fileURLToPath(url));
  }
  return readFile(src as string);
}
