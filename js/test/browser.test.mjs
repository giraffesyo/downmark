// Browser-side smoke tests. The Node tests cover the same API, but they run
// the Node bundle: a different wasm loader (fs.readFile rather than fetch +
// instantiateStreaming) and a different esbuild output. This is the only
// coverage of the bundle that actually ships to browsers.
import { test, before, after } from "node:test";
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { join, extname, normalize } from "node:path";
import { chromium } from "playwright";

const ROOT = fileURLToPath(new URL("..", import.meta.url));
const FIXTURES = fileURLToPath(new URL("../../testdata/", import.meta.url));

const MIME = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  // Served correctly on purpose: this is what lets the bundle take the
  // instantiateStreaming path instead of its arrayBuffer fallback.
  ".wasm": "application/wasm",
};

let server;
let browser;
let page;
let origin;

before(async () => {
  server = createServer(async (req, res) => {
    try {
      const rel = normalize(decodeURIComponent(new URL(req.url, "http://x").pathname)).replace(
        /^(\.\.[/\\])+/,
        "",
      );
      const body = await readFile(join(ROOT, rel));
      res.writeHead(200, { "content-type": MIME[extname(rel)] ?? "application/octet-stream" });
      res.end(body);
    } catch {
      res.writeHead(404).end("not found");
    }
  });
  await new Promise((r) => server.listen(0, "127.0.0.1", r));
  origin = `http://127.0.0.1:${server.address().port}`;

  browser = await chromium.launch();
  page = await browser.newPage();
  const failures = [];
  page.on("pageerror", (e) => failures.push(String(e)));
  await page.goto(`${origin}/test/browser-fixture.html`);
  await page.waitForFunction(() => window.downmarkReady === true, null, { timeout: 60_000 });
  assert.deepEqual(failures, [], "no uncaught page errors while loading the bundle");
});

after(async () => {
  await browser?.close();
  await new Promise((r) => server.close(r));
});

const fixture = async (name) => (await readFile(join(FIXTURES, name))).toString("base64");

test("browser bundle converts DOCX", async () => {
  const b64 = await fixture("test.docx");
  const res = await page.evaluate(
    ([data]) => window.downmark.convert(data, { filename: "test.docx" }),
    [b64],
  );
  assert.ok(res.markdown.length > 0, "markdown should be non-empty");
  assert.match(res.title, /AutoGen/);
});

test("browser bundle converts XLSX to tables", async () => {
  const b64 = await fixture("test.xlsx");
  const res = await page.evaluate(
    ([data]) => window.downmark.convert(data, { filename: "test.xlsx" }),
    [b64],
  );
  assert.match(res.markdown, /\| --- \|/, "xlsx output should contain a table");
});

test("browser bundle reports typed errors", async () => {
  const b64 = await fixture("random.bin");
  const err = await page.evaluate(
    ([data]) => window.downmark.convertExpectingError(data, { filename: "random.bin" }),
    [b64],
  );
  assert.ok(err, "convert should have rejected");
  assert.equal(err.code, "UNSUPPORTED_FORMAT");
});

test("browser bundle exposes canConvert and version", async () => {
  const [docx, xyz, ver] = await page.evaluate(async () => [
    await window.downmark.canConvert({ extension: ".docx" }),
    await window.downmark.canConvert({ extension: ".xyz" }),
    await window.downmark.version(),
  ]);
  assert.equal(docx, true);
  assert.equal(xyz, false);
  assert.ok(ver.length > 0);
});
