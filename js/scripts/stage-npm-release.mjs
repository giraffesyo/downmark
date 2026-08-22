// Stage everything npm publish needs for one release:
//
//   1. js/npm/<platform>/ — one package per platform, holding nothing but
//      the binary goreleaser built for it and a manifest whose os/cpu
//      fields tell an installer to skip the other five.
//   2. js/package.json — the same list written in as optionalDependencies,
//      pinned to this version.
//
// Step 2 happens here rather than in the committed manifest because npm ci
// validates package.json against package-lock.json: a checked-in
// optionalDependency on a version that is not published yet (which is
// exactly what a release commit has) makes every clean install fail. The
// dependencies are generated, so they are generated at the one moment they
// can be correct.
//
// Nothing here is committed; js/npm is ignored and js/package.json is
// restored by the checkout of the next job.
//
// Usage:
//   node scripts/stage-npm-release.mjs --version 1.2.3 [--dist ../dist] [--out npm]

import { chmod, copyFile, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { readdirSync, statSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const jsRoot = resolve(here, "..");
const repoRoot = resolve(jsRoot, "..");

const args = parseArgs(process.argv.slice(2));
const version = args.version;
if (!version) fail("--version is required");
// A leading v is what the git tag carries and what npm rejects.
if (version.startsWith("v")) fail(`--version must not start with "v": ${version}`);

const distDir = resolve(repoRoot, args.dist ?? "dist");
const outDir = resolve(jsRoot, args.out ?? "npm");

const manifest = JSON.parse(await readFile(join(jsRoot, "src", "platforms.json"), "utf8"));
const root = JSON.parse(await readFile(join(jsRoot, "package.json"), "utf8"));

await rm(outDir, { recursive: true, force: true });

const optional = {};
for (const platform of manifest.packages) {
  const binName = platform.os === "win32" ? "downmark.exe" : "downmark";
  const source = findBinary(distDir, platform, binName);
  const pkgDir = join(outDir, platform.npm.replace(/^@[^/]+\//, ""));

  await mkdir(join(pkgDir, "bin"), { recursive: true });
  await copyFile(source, join(pkgDir, "bin", binName));
  // npm tarballs preserve the mode, and a binary nobody can execute is the
  // whole feature broken.
  await chmod(join(pkgDir, "bin", binName), 0o755);
  await writeFile(
    join(pkgDir, "package.json"),
    JSON.stringify(platformManifest(platform, binName), null, 2) + "\n",
  );
  await writeFile(join(pkgDir, "README.md"), platformReadme(platform));

  optional[platform.npm] = version;
  console.log(`staged ${platform.npm}  <- ${source}`);
}

root.version = version;
root.optionalDependencies = optional;
await writeFile(join(jsRoot, "package.json"), JSON.stringify(root, null, 2) + "\n");
console.log(`wrote ${Object.keys(optional).length} optionalDependencies at ${version}`);
console.log("js/package.json now carries generated fields; do not commit it.");

function platformManifest(platform, binName) {
  return {
    name: platform.npm,
    version,
    description: `The ${platform.os}-${platform.cpu} binary for downmark.`,
    license: root.license,
    // The wrapper's repository, minus its "directory": these packages are
    // built at release time and have no directory in the tree.
    repository: { type: root.repository.type, url: root.repository.url },
    homepage: root.homepage,
    bugs: root.bugs,
    engines: root.engines,
    // What makes an installer fetch exactly one of these six.
    os: [platform.os],
    cpu: [platform.cpu],
    files: ["bin", "README.md"],
    // Yarn PnP keeps packages zipped; this one has to be a real file on
    // disk for the wrapper to spawn it.
    preferUnplugged: true,
    // Recorded for whoever has to match a package back to a build target.
    downmark: { goos: platform.goos, goarch: platform.goarch, binary: `bin/${binName}` },
  };
}

function platformReadme(platform) {
  return `# ${platform.npm}

The ${platform.os}-${platform.cpu} build of the [downmark](${root.homepage}) binary.

Do not depend on this package directly. Install
[\`${root.name}\`](https://www.npmjs.com/package/${root.name}): it declares
this one as an optional dependency, and npm installs only the package that
matches the host. The wrapper falls back to its bundled WebAssembly build
wherever no platform package applies.
`;
}

/**
 * Find the binary goreleaser built for one platform. Its output directories
 * are named downmark_<goos>_<goarch>, sometimes with a microarchitecture
 * suffix (downmark_linux_amd64_v1), so the prefix is what can be matched.
 */
function findBinary(dir, platform, binName) {
  const prefix = `downmark_${platform.goos}_${platform.goarch}`;
  let entries;
  try {
    entries = readdirSync(dir);
  } catch (err) {
    fail(`cannot read the goreleaser output at ${dir}: ${err.message}`);
  }
  const matches = entries
    .filter((name) => name === prefix || name.startsWith(`${prefix}_`))
    .map((name) => join(dir, name, binName))
    .filter((path) => {
      try {
        return statSync(path).isFile();
      } catch {
        return false;
      }
    });
  if (matches.length === 0) {
    fail(`no ${binName} for ${platform.npm} under ${join(dir, prefix)}*`);
  }
  if (matches.length > 1) {
    // Two candidates means two microarchitecture variants; picking one at
    // random would ship a binary nobody chose.
    fail(`ambiguous binaries for ${platform.npm}:\n  ${matches.join("\n  ")}`);
  }
  return matches[0];
}

function parseArgs(argv) {
  const out = {};
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (!arg.startsWith("--")) fail(`unexpected argument ${arg}`);
    const eq = arg.indexOf("=");
    if (eq !== -1) out[arg.slice(2, eq)] = arg.slice(eq + 1);
    else out[arg.slice(2)] = argv[++i];
  }
  return out;
}

function fail(message) {
  console.error(`stage-npm-release: ${message}`);
  process.exit(1);
}
