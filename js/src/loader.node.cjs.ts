// Node CJS loader: same as loader.node.ts but locates the wasm via
// __dirname, since import.meta.url does not exist in a CJS bundle.
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { join } from "node:path";
import type { WasmSource } from "./wasm-source";

declare const __dirname: string;

export async function defaultWasmSource(): Promise<WasmSource> {
  return readFile(join(__dirname, "downmark.wasm"));
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
