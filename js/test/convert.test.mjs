import { test } from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";

import {
  convert,
  canConvert,
  version,
  DownmarkError,
  ConversionFailedError,
} from "../dist/index.js";

const fixture = (name) =>
  readFile(fileURLToPath(new URL(`../../testdata/${name}`, import.meta.url)));

test("converts DOCX to markdown", async () => {
  const data = await fixture("test.docx");
  const result = await convert(data, { filename: "test.docx" });
  assert.equal(typeof result.markdown, "string");
  assert.ok(result.markdown.length > 0, "markdown should be non-empty");
  assert.equal(typeof result.title, "string");
});

test("converts PDF to markdown", async () => {
  const data = await fixture("test.pdf");
  const result = await convert(data, { extension: ".pdf" });
  assert.ok(result.markdown.length > 0, "markdown should be non-empty");
});

test("converts Shift_JIS CSV to markdown (chardet canary)", async () => {
  const data = await fixture("test_mskanji.csv");
  const result = await convert(data, { filename: "test_mskanji.csv" });
  assert.ok(result.markdown.length > 0, "markdown should be non-empty");
  assert.ok(result.markdown.includes("|"), "csv output should be a table");
});

test("accepts ArrayBuffer input and extension without a leading dot", async () => {
  const data = await fixture("test.docx");
  const buf = data.buffer.slice(data.byteOffset, data.byteOffset + data.byteLength);
  const result = await convert(buf, { extension: "docx" });
  assert.ok(result.markdown.length > 0);
});

test("rejects random binary with UNSUPPORTED_FORMAT", async () => {
  const data = await fixture("random.bin");
  await assert.rejects(convert(data, { filename: "random.bin" }), (err) => {
    assert.ok(err instanceof DownmarkError, "should be a DownmarkError");
    assert.equal(err.code, "UNSUPPORTED_FORMAT");
    return true;
  });
});

test("rejects with RESULT_TOO_LARGE when resultLimit is tiny", async () => {
  const data = await fixture("test.docx");
  await assert.rejects(
    convert(data, { filename: "test.docx", resultLimit: 4 }),
    (err) => {
      assert.ok(err instanceof DownmarkError);
      assert.equal(err.code, "RESULT_TOO_LARGE");
      return true;
    },
  );
});

test("ConversionFailedError carries attempts for a corrupt document", async () => {
  // A valid-looking DOCX extension over garbage bytes: the docx converter
  // accepts and then fails, producing a ConversionError with attempts.
  const data = await fixture("random.bin");
  await assert.rejects(convert(data, { extension: ".docx" }), (err) => {
    assert.ok(err instanceof DownmarkError);
    if (err instanceof ConversionFailedError) {
      assert.ok(err.attempts.length > 0, "attempts should be non-empty");
      assert.equal(typeof err.attempts[0].converter, "string");
    }
    return true;
  });
});

test("canConvert answers from hints", async () => {
  assert.equal(await canConvert({ extension: ".docx" }), true);
  assert.equal(await canConvert({ extension: "docx" }), true);
  assert.equal(await canConvert({ extension: ".xyz" }), false);
  assert.equal(await canConvert({ mimeType: "application/pdf" }), true);
});

test("instance survives sequential converts", async () => {
  const data = await fixture("test.docx");
  const first = await convert(data, { filename: "test.docx" });
  const second = await convert(data, { filename: "test.docx" });
  assert.equal(first.markdown, second.markdown);
});

test("version reports a string", async () => {
  const v = await version();
  assert.equal(typeof v, "string");
  assert.ok(v.length > 0);
});

test("rejects invalid resultLimit without touching the wasm", async () => {
  await assert.rejects(convert(new Uint8Array(0), { resultLimit: 0 }), (err) => {
    assert.ok(err instanceof DownmarkError);
    assert.equal(err.code, "INTERNAL");
    return true;
  });
});
