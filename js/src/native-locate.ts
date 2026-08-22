// Finding the native binary. It ships inside a per-platform package that
// npm installs (and skips for every other platform) through the os/cpu
// fields on those packages, so resolving it is a plain module resolution
// against the dependency tree the installer already built — no download,
// no postinstall, nothing to stage on the host.
import { existsSync } from "node:fs";
import { join, dirname } from "node:path";

import platforms from "./platforms.json";
import { DownmarkError } from "./errors.js";

/** Resolve a module id to a file path, i.e. a require.resolve. */
export type Resolver = (id: string) => string;

/** Environment variables that steer which implementation runs. */
const FORCE_WASM = "DOWNMARK_FORCE_WASM";
const BIN_OVERRIDE = "DOWNMARK_BIN";

/** True for the values a person means as "on". */
function enabled(value: string | undefined): boolean {
  return value !== undefined && value !== "" && value !== "0" && value !== "false";
}

/** The platform package for the running host, or undefined if unsupported. */
function packageForHost(): string | undefined {
  return platforms.packages.find(
    (p) => p.os === process.platform && p.cpu === process.arch,
  )?.npm;
}

// Module resolution walks node_modules and touches the disk; the answer
// cannot change while the process runs, so it is worked out once. `null`
// is a real answer (no package installed) and is cached too, hence the
// separate `resolved` flag.
let cached: string | null = null;
let resolved = false;

/**
 * Locate the native binary, or return null when this host has no platform
 * package installed and the wasm has to stand in.
 */
export function locateBinary(resolve: Resolver): string | null {
  if (enabled(process.env[FORCE_WASM])) return null;

  // An explicit override is checked on every call: it is how tests and
  // local builds point at a binary, and it must not be frozen by whatever
  // the first call happened to see.
  const override = process.env[BIN_OVERRIDE];
  if (override) {
    if (!existsSync(override)) {
      throw new DownmarkError(
        `downmark: ${BIN_OVERRIDE}=${override} does not exist`,
        "INTERNAL",
      );
    }
    return override;
  }

  if (resolved) return cached;
  resolved = true;
  cached = resolveFromPackage(resolve);
  return cached;
}

function resolveFromPackage(resolve: Resolver): string | null {
  const pkg = packageForHost();
  if (!pkg) return null;
  let manifest: string;
  try {
    // Resolving package.json rather than the binary itself: the binary is
    // not a module, and "exports" would not let it be resolved directly.
    manifest = resolve(`${pkg}/package.json`);
  } catch {
    // Not installed. Expected whenever the installer skipped optional
    // dependencies, and on a platform the matrix does not cover.
    return null;
  }
  const bin = join(
    dirname(manifest),
    "bin",
    process.platform === "win32" ? "downmark.exe" : "downmark",
  );
  // A package that resolved but holds no binary is a broken install, not a
  // reason to silently run the slow path.
  if (!existsSync(bin)) {
    throw new DownmarkError(
      `downmark: ${pkg} is installed but ${bin} is missing; reinstall the package`,
      "INTERNAL",
    );
  }
  return bin;
}
