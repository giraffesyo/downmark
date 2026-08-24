#!/usr/bin/env bash
# Publish one staged package directory to npm, skipping what the registry
# already has. Run from the repository root.
#
# A release publishes seven packages one at a time, so a job that dies in the
# middle leaves some of them out. npm versions are immutable -- republishing
# one is an EPUBLISHCONFLICT, not a no-op -- so without this the second half
# of a release can never be finished by re-running the job. Skipping a
# version that is already published makes the step converge on the state it
# describes rather than on having run exactly once.
#
# Usage: npm-publish.sh <package-dir>
set -euo pipefail

dir=${1:?usage: npm-publish.sh <package-dir>}
manifest="$PWD/$dir/package.json"

name=$(node -p "require('$manifest').name")
version=$(node -p "require('$manifest').version")

# The only question that matters, and the registry is the one that answers
# it: a missing package and a missing version both exit non-zero here.
if npm view "$name@$version" version >/dev/null 2>&1; then
  echo "$name@$version is already published; skipping"
  exit 0
fi

echo "publishing $name@$version from $dir"
# --access public is what makes the first publish of a scoped package
# public; it is a no-op on every one after that.
if ! (cd "$dir" && npm publish --access public); then
  # Ask the registry again rather than matching on npm's error text. If the
  # version is there now, the check above raced a concurrent run or ran
  # against a registry that was briefly unreachable -- either way the end
  # state is the one we wanted. Anything else is a real failure.
  if npm view "$name@$version" version >/dev/null 2>&1; then
    echo "$name@$version is published despite the error above; treating as done"
    exit 0
  fi
  exit 1
fi
