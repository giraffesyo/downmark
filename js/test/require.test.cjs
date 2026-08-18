const { test } = require("node:test");
const assert = require("node:assert/strict");
const { readFile } = require("node:fs/promises");
const path = require("node:path");

const { convert, canConvert, DownmarkError } = require("../dist/index.cjs");

test("CJS entry converts DOCX to markdown", async () => {
  const data = await readFile(path.join(__dirname, "..", "..", "testdata", "test.docx"));
  const result = await convert(data, { filename: "test.docx" });
  assert.ok(result.markdown.length > 0, "markdown should be non-empty");
});

test("CJS entry exposes canConvert and typed errors", async () => {
  assert.equal(await canConvert({ extension: ".pdf" }), true);
  assert.equal(typeof DownmarkError, "function");
});
