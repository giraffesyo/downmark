// Lazy one-time instantiation of the downmark wasm module. wasm_exec.js is
// imported for its side effect: it defines globalThis.Go.
import "../vendor/wasm_exec.js";
import { defaultWasmSource, resolveWasmSource } from "#wasm-loader";
import type { WasmSource, WasmSourceInput } from "./wasm-source.js";
import { DownmarkError } from "./errors.js";

/** Shape of the global the Go side registers as __downmark. */
export interface DownmarkWasmApi {
  convert(
    data: Uint8Array,
    opts: Record<string, unknown>,
  ): Promise<{
    markdown: string;
    title: string;
    warnings: {
      converter: string;
      code: "incomplete" | "skipped";
      location: string;
      message: string;
    }[];
  }>;
  canConvert(opts: Record<string, unknown>): boolean;
  version: string;
}

interface GoRuntime {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

let readyPromise: Promise<DownmarkWasmApi> | null = null;

/**
 * Instantiate the wasm module once and cache the ready promise. Passing a
 * source is only allowed before the first call has started the runtime.
 */
export function initRuntime(source?: WasmSourceInput): Promise<DownmarkWasmApi> {
  if (readyPromise) {
    if (source !== undefined) {
      return Promise.reject(
        new DownmarkError(
          "downmark: runtime already initialized; call init(source) before the first convert/canConvert",
          "INTERNAL",
        ),
      );
    }
    return readyPromise;
  }
  const started = start(source);
  readyPromise = started;
  // A failed init (e.g. wasm not found) should not poison the module
  // forever; let the next call retry.
  started.catch(() => {
    if (readyPromise === started) readyPromise = null;
  });
  return started;
}

async function start(source?: WasmSourceInput): Promise<DownmarkWasmApi> {
  const g = globalThis as Record<string, unknown>;
  const GoCtor = g.Go as (new () => GoRuntime) | undefined;
  if (typeof GoCtor !== "function") {
    throw new DownmarkError(
      "downmark: wasm_exec.js did not define globalThis.Go",
      "INTERNAL",
    );
  }
  const go = new GoCtor();
  const ready = new Promise<void>((resolve) => {
    g.__downmark_ready = resolve;
  });
  const wasm = await normalizeSource(source);
  const instance = await instantiate(wasm, go.importObject);
  // Intentionally not awaited: the Go program parks forever so its exported
  // functions stay callable. It only settles on crash or exit.
  const run = go.run(instance);
  await Promise.race([
    ready,
    run.then(() => {
      throw new DownmarkError(
        "downmark: wasm module exited before signaling ready",
        "INTERNAL",
      );
    }),
  ]);
  const api = g.__downmark as DownmarkWasmApi | undefined;
  if (!api || typeof api.convert !== "function") {
    throw new DownmarkError(
      "downmark: wasm module did not register __downmark",
      "INTERNAL",
    );
  }
  return api;
}

async function normalizeSource(source?: WasmSourceInput): Promise<WasmSource> {
  if (source === undefined) return defaultWasmSource();
  if (typeof source === "string" || source instanceof URL) {
    return resolveWasmSource(source);
  }
  return source;
}

async function instantiate(
  source: WasmSource,
  imports: WebAssembly.Imports,
): Promise<WebAssembly.Instance> {
  if (source instanceof WebAssembly.Module) {
    return WebAssembly.instantiate(source, imports);
  }
  if (typeof Response !== "undefined" && source instanceof Response) {
    if (typeof WebAssembly.instantiateStreaming === "function") {
      try {
        const result = await WebAssembly.instantiateStreaming(source.clone(), imports);
        return result.instance;
      } catch {
        // Typically a server that doesn't send Content-Type:
        // application/wasm; fall back to buffering the body.
      }
    }
    const buffer = await source.arrayBuffer();
    return (await WebAssembly.instantiate(buffer, imports)).instance;
  }
  return (await WebAssembly.instantiate(source as BufferSource, imports)).instance;
}
