// Node CJS: same as native.node.ts, anchored on __filename because
// import.meta does not exist in a CJS bundle.
import { createRequire } from "node:module";

import { locateBinary } from "./native-locate.js";

export { convertNative } from "./native-run.js";

declare const __filename: string;

const require = createRequire(__filename);

export function binaryPath(): string | null {
  return locateBinary(require.resolve);
}
