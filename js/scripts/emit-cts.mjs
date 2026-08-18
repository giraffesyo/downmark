// tsc emits only .d.ts, and because package.json sets "type": "module"
// those are ESM-flavored declarations. CommonJS consumers resolving through
// the "require" condition need .d.cts alongside them, or TypeScript reports
// the package as masquerading as ESM. Copy the reachable declaration graph,
// rewriting relative specifiers to the .cjs extension.
import { readFile, writeFile } from "node:fs/promises";

const MODULES = ["index", "errors", "wasm-source"];

for (const name of MODULES) {
  const src = await readFile(`dist/${name}.d.ts`, "utf8");
  const out = src.replace(/(from\s+")\.\/([\w.-]+)\.js(")/g, "$1./$2.cjs$3");
  await writeFile(`dist/${name}.d.cts`, out);
}
