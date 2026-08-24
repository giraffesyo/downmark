// The native path: the same conversions as convert.test.mjs, run by
// spawning the binary instead of instantiating the wasm, plus the parts
// that only exist on this side (OCR, binaryPath, the fallback rules).
//
// The binary is built by `make js-bin` into js/.bin. Without it there is
// nothing to test, so every test here skips rather than failing: `npm test`
// alone must not require a Go toolchain.
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";

import {
  binaryPath,
  convert,
  init,
  DownmarkError,
  ConversionFailedError,
} from "../dist/index.js";

const BIN = fileURLToPath(
  new URL(process.platform === "win32" ? "../.bin/downmark.exe" : "../.bin/downmark", import.meta.url),
);
const skip = existsSync(BIN) ? false : `no binary at ${BIN}; run \`make js-bin\``;

const fixture = (name) =>
  readFile(fileURLToPath(new URL(`../../testdata/${name}`, import.meta.url)));

/** Run fn with the environment pointed at the freshly built binary. */
async function withNative(fn) {
  const previous = process.env.DOWNMARK_BIN;
  process.env.DOWNMARK_BIN = BIN;
  try {
    return await fn();
  } finally {
    if (previous === undefined) delete process.env.DOWNMARK_BIN;
    else process.env.DOWNMARK_BIN = previous;
  }
}

test("binaryPath reports the binary the next convert would run", { skip }, async () => {
  assert.equal(await withNative(() => binaryPath()), BIN);
});

test("converts DOCX by spawning the binary", { skip }, async () => {
  const data = await fixture("test.docx");
  const result = await withNative(() => convert(data, { filename: "test.docx" }));
  assert.ok(result.markdown.length > 0, "markdown should be non-empty");
  assert.equal(typeof result.title, "string");
  assert.ok(Array.isArray(result.warnings), "warnings should be an array");
});

// The two implementations are the same Go code; a caller switching hosts
// between them must not see the document change.
test("native and wasm produce the same Markdown", { skip }, async () => {
  for (const name of ["test.docx", "test.pdf", "test_mskanji.csv", "test.xlsx"]) {
    const data = await fixture(name);
    const native = await withNative(() => convert(data, { filename: name }));
    const wasm = await convert(data, { filename: name });
    assert.equal(native.markdown, wasm.markdown, `${name}: markdown differs`);
    assert.equal(native.title, wasm.title, `${name}: title differs`);
    assert.deepEqual(native.warnings, wasm.warnings, `${name}: warnings differ`);
  }
});

// Hints reach the binary as flags; a charset-sniffed CSV is where getting
// the extension wrong shows up immediately.
test("hints survive the trip through the command line", { skip }, async () => {
  const data = await fixture("test_mskanji.csv");
  const result = await withNative(() =>
    convert(data, { filename: "test_mskanji.csv", charset: "shift_jis" }),
  );
  assert.ok(result.markdown.includes("|"), "csv output should be a table");
});

test("keepDataUris reaches the binary", { skip }, async () => {
  const data = await fixture("sample.html");
  const [kept, truncated] = await withNative(async () => [
    await convert(data, { extension: ".html", keepDataUris: true }),
    await convert(data, { extension: ".html" }),
  ]);
  assert.ok(kept.markdown.length >= truncated.markdown.length);
});

test("failures come back as the same typed errors as on wasm", { skip }, async () => {
  const random = await fixture("random.bin");
  await withNative(async () => {
    await assert.rejects(convert(random, { filename: "random.bin" }), (err) => {
      assert.ok(err instanceof DownmarkError);
      assert.equal(err.code, "UNSUPPORTED_FORMAT");
      return true;
    });
    await assert.rejects(convert(random, { extension: ".docx" }), (err) => {
      assert.ok(err instanceof ConversionFailedError);
      assert.ok(err.attempts.length > 0, "attempts should be non-empty");
      assert.equal(typeof err.attempts[0].converter, "string");
      return true;
    });
    const docx = await fixture("test.docx");
    await assert.rejects(
      convert(docx, { filename: "test.docx", resultLimit: 4 }),
      (err) => {
        assert.equal(err.code, "RESULT_TOO_LARGE");
        return true;
      },
    );
  });
});

// A text-bearing PDF gives the OCR engine nothing to do, so this needs no
// tesseract: it proves the flags are built correctly and the binary accepts
// them, which is what silently breaks.
test("OCR options are accepted by the binary", { skip }, async () => {
  const data = await fixture("test.pdf");
  const result = await withNative(() =>
    convert(data, {
      extension: ".pdf",
      ocr: {
        engine: "tesseract",
        lang: "eng",
        policy: "thin",
        minGlyphs: 40,
        minConfidence: 60,
        maxPages: 2,
        pageTimeoutMs: 30_000,
        timeoutMs: 60_000,
      },
    }),
  );
  assert.ok(result.markdown.length > 0);
});

test("OCR without a binary rejects instead of silently skipping pages", async () => {
  const data = await fixture("test.pdf");
  const previous = process.env.DOWNMARK_FORCE_WASM;
  process.env.DOWNMARK_FORCE_WASM = "1";
  try {
    await assert.rejects(
      convert(data, { extension: ".pdf", ocr: { engine: "tesseract" } }),
      (err) => {
        assert.ok(err instanceof DownmarkError);
        assert.equal(err.code, "NATIVE_REQUIRED");
        return true;
      },
    );
  } finally {
    if (previous === undefined) delete process.env.DOWNMARK_FORCE_WASM;
    else process.env.DOWNMARK_FORCE_WASM = previous;
  }
});

test("preferWasm ignores an installed binary", { skip }, async () => {
  await withNative(async () => {
    await init({ preferWasm: true });
    try {
      assert.equal(binaryPath(), null);
      const data = await fixture("test.docx");
      const result = await convert(data, { filename: "test.docx" });
      assert.ok(result.markdown.length > 0);
    } finally {
      await init({ preferWasm: false });
    }
  });
});

test("a DOWNMARK_BIN that points at nothing is an error, not a fallback", async () => {
  const previous = process.env.DOWNMARK_BIN;
  process.env.DOWNMARK_BIN = fileURLToPath(new URL("./no-such-binary", import.meta.url));
  try {
    assert.throws(() => binaryPath(), (err) => {
      assert.ok(err instanceof DownmarkError);
      assert.equal(err.code, "INTERNAL");
      assert.match(err.message, /DOWNMARK_BIN/);
      return true;
    });
  } finally {
    if (previous === undefined) delete process.env.DOWNMARK_BIN;
    else process.env.DOWNMARK_BIN = previous;
  }
});

// Nothing is installed in this repo, so the wrapper must land on the wasm.
test("with no platform package installed, convert falls back to the wasm", async () => {
  assert.equal(binaryPath(), null);
  const data = await fixture("test.docx");
  const result = await convert(data, { filename: "test.docx" });
  assert.ok(result.markdown.length > 0);
});
