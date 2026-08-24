// Driving the native binary. One spawn per conversion, the document on
// stdin, the CLI's -json object on stdout: about 5-10 ms of process
// overhead, in exchange for the concurrency, the OCR, and the event loop
// the wasm build cannot give.
import { spawn } from "node:child_process";

import { DownmarkError, rehydrateError } from "./errors.js";
import type { ConvertOptions, ConvertResult } from "./types.js";

/** Build the command line for one conversion. Options are pre-validated. */
export function buildArgs(opts: ConvertOptions): string[] {
  const args = ["-json"];
  // The binary derives the extension from a path it opened itself, and it
  // is reading stdin here, so the filename hint has to arrive as -x. This
  // is what the wasm side does with `filename` internally.
  const ext = normalizeExtension(opts.extension ?? extensionOf(opts.filename));
  if (ext) args.push("-x", ext);
  if (opts.mimeType) args.push("-m", opts.mimeType);
  if (opts.charset) args.push("-c", opts.charset);
  if (opts.keepDataUris) args.push("-keep-data-uris");
  if (opts.resultLimit !== undefined) args.push("-result-limit", String(opts.resultLimit));

  const ocr = opts.ocr;
  if (ocr) {
    args.push("-ocr", ocr.engine);
    if (ocr.bin) args.push("-ocr-bin", ocr.bin);
    if (ocr.lang) args.push("-ocr-lang", ocr.lang);
    if (ocr.minConfidence !== undefined) {
      args.push("-ocr-min-confidence", String(ocr.minConfidence));
    }
    if (ocr.policy) args.push("-ocr-policy", ocr.policy);
    if (ocr.minGlyphs !== undefined) args.push("-ocr-min-glyphs", String(ocr.minGlyphs));
    if (ocr.maxPages !== undefined) args.push("-ocr-max-pages", String(ocr.maxPages));
    // Go parses durations from a unit suffix; milliseconds is the unit a
    // JS caller already thinks in.
    if (ocr.pageTimeoutMs !== undefined) {
      args.push("-ocr-page-timeout", `${ocr.pageTimeoutMs}ms`);
    }
    if (ocr.timeoutMs !== undefined) args.push("-ocr-timeout", `${ocr.timeoutMs}ms`);
  }
  return args;
}

function extensionOf(filename: string | undefined): string | undefined {
  if (!filename) return undefined;
  // Deliberately not node:path.extname: the hint is a name, not a path on
  // this host, and a Windows-style name has to work on Linux too.
  const match = /\.[^./\\]+$/.exec(filename);
  return match ? match[0] : undefined;
}

function normalizeExtension(ext: string | undefined): string | undefined {
  if (!ext) return undefined;
  const lower = ext.toLowerCase();
  return lower.startsWith(".") ? lower : `.${lower}`;
}

/** Convert `data` by running the binary at `bin`. */
export async function convertNative(
  bin: string,
  data: Uint8Array,
  opts: ConvertOptions,
): Promise<ConvertResult> {
  const { status, signal, stdout, stderr } = await run(bin, buildArgs(opts), data);
  if (signal) {
    throw new DownmarkError(
      `downmark: the binary was killed by ${signal}`,
      "INTERNAL",
    );
  }
  if (status !== 0) throw failure(status, stderr);
  try {
    return JSON.parse(stdout.toString("utf8")) as ConvertResult;
  } catch {
    throw new DownmarkError(
      `downmark: the binary did not produce JSON${detail(stderr)}`,
      "INTERNAL",
    );
  }
}

/**
 * Turn a non-zero exit into the same typed error the wasm path throws. The
 * binary writes the structured form for a failed conversion; anything else
 * (a rejected flag, a crash) arrives as text and is reported as INTERNAL.
 */
function failure(status: number | null, stderr: Buffer): DownmarkError {
  let wire: unknown;
  try {
    wire = JSON.parse(stderr.toString("utf8"));
  } catch {
    return new DownmarkError(
      `downmark: the binary exited with status ${status}${detail(stderr)}`,
      "INTERNAL",
    );
  }
  const body = (wire as { error?: unknown })?.error;
  if (!body || typeof body !== "object") {
    return new DownmarkError(
      `downmark: the binary exited with status ${status}${detail(stderr)}`,
      "INTERNAL",
    );
  }
  return rehydrateError(body);
}

function detail(stderr: Buffer): string {
  const text = stderr.toString("utf8").trim();
  return text ? `: ${text}` : "";
}

interface Exit {
  status: number | null;
  signal: NodeJS.Signals | null;
  stdout: Buffer;
  stderr: Buffer;
}

function run(bin: string, args: string[], data: Uint8Array): Promise<Exit> {
  return new Promise<Exit>((resolve, reject) => {
    const child = spawn(bin, args, { stdio: ["pipe", "pipe", "pipe"] });
    const stdout: Buffer[] = [];
    const stderr: Buffer[] = [];
    let settled = false;

    const fail = (err: Error) => {
      if (settled) return;
      settled = true;
      reject(
        new DownmarkError(`downmark: running ${bin}: ${err.message}`, "INTERNAL"),
      );
    };

    child.on("error", fail);
    child.stdout.on("data", (chunk: Buffer) => stdout.push(chunk));
    child.stderr.on("data", (chunk: Buffer) => stderr.push(chunk));
    child.stdout.on("error", fail);
    child.stderr.on("error", fail);
    // A binary that rejects its flags exits before reading the document,
    // which lands here as EPIPE. The exit status and stderr say what went
    // wrong, so let 'close' report it rather than this.
    child.stdin.on("error", () => {});
    child.stdin.end(data);

    // 'close' rather than 'exit': it fires once the pipes are drained, so
    // the output is whole.
    child.on("close", (status, signal) => {
      if (settled) return;
      settled = true;
      resolve({
        status,
        signal,
        stdout: Buffer.concat(stdout),
        stderr: Buffer.concat(stderr),
      });
    });
  });
}
