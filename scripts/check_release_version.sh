#!/usr/bin/env bash
# Check that vX.Y.Z is a valid next release: only the last number goes up by
# one from the previous release (the newest vX.Y.Z tag, or, before the first
# tag, the CHANGELOG.md section below this one). Packages keep their own
# versions (see docs/releases/), so they aren't checked here.
#   scripts/check_release_version.sh v0.1.1
set -euo pipefail
version="${1:?usage: check_release_version.sh vX.Y.Z}"
cd "$(dirname "$0")/.."
[[ "$version" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]] || { echo "Version must look like v1.2.3" >&2; exit 1; }

prev=$(git tag --list 'v[0-9]*.[0-9]*.[0-9]*' | grep -Ex 'v[0-9]+\.[0-9]+\.[0-9]+' | grep -vx "$version" | sort -V | tail -1 || true)
if [ -z "$prev" ]; then
  prev=$(grep -Eo '^## \[v[0-9]+\.[0-9]+\.[0-9]+\]' CHANGELOG.md | tr -d '#[] ' | grep -vx "$version" | head -1 || true)
fi
[ -n "$prev" ] || { echo "No previous release found" >&2; exit 1; }
IFS=. read -r maj min pat <<<"${prev#v}"
expected="v$maj.$min.$((pat + 1))"
[ "$version" = "$expected" ] || { echo "Next release after $prev is $expected (only the last number changes), not $version" >&2; exit 1; }

echo "$version follows $prev"
