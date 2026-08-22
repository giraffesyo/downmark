#!/usr/bin/env bash
# Build the per-platform npm packages from goreleaser's output and check
# that what came out is publishable. Run after `goreleaser build --snapshot`,
# from the repository root.
#
# This is the release's packaging step run for real, on a throwaway version:
# the same script, the same dist directory, the same manifests. What it
# catches is a build matrix that stopped matching src/platforms.json, a
# goreleaser output layout that moved, and a binary that lost its executable
# bit on the way into the tarball.
set -euo pipefail

VERSION=0.0.0-verify

node js/scripts/stage-npm-release.mjs --version "$VERSION"

expected=$(node -p 'require("./js/src/platforms.json").packages.length')
declared=$(node -p 'Object.keys(require("./js/package.json").optionalDependencies || {}).length')
if [ "$declared" != "$expected" ]; then
  echo "the wrapper declares $declared optionalDependencies, want $expected" >&2
  exit 1
fi

staged=0
for dir in js/npm/*/; do
  name=$(node -p "require('./$dir/package.json').name")
  bin=$dir$(node -p "require('./$dir/package.json').downmark.binary")
  if [ ! -x "$bin" ]; then
    echo "$name: $bin is missing or not executable" >&2
    exit 1
  fi
  # npm's own view of the manifest: it rejects here rather than mid-release.
  (cd "$dir" && npm pack --dry-run >/dev/null)
  staged=$((staged + 1))
done

if [ "$staged" != "$expected" ]; then
  echo "staged $staged packages, want $expected" >&2
  exit 1
fi

# The runner's own platform package holds a binary it can execute, so run it:
# a truncated or mismatched copy passes every check above and none of this.
host="js/npm/downmark-linux-x64/bin/downmark"
if [ -x "$host" ] && [ "$(uname -s)-$(uname -m)" = "Linux-x86_64" ]; then
  "$host" -version
fi

echo "staged $staged platform packages at $VERSION"
