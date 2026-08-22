// Node ESM: resolve the platform package against this module's own URL, so
// the lookup walks the consumer's dependency tree from where the package
// actually sits.
import { createRequire } from "node:module";

import { locateBinary } from "./native-locate.js";

export { convertNative } from "./native-run.js";

const require = createRequire(import.meta.url);

export function binaryPath(): string | null {
  return locateBinary(require.resolve);
}
