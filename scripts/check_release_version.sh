#!/usr/bin/env bash
# Check that vX.Y.Z is a valid next release: only the last number goes up by
# one from the previous release, and the SDK and MCP packages carry the same
# version. The previous release is the newest vX.Y.Z tag, or, before the first
# tag, the CHANGELOG.md section below this one.
#   scripts/check_release_version.sh v0.1.1
# memro-cli is versioned separately and isn't checked here.
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

want="${version#v}"
got_py=$(sed -nE 's/^version = "(.*)"/\1/p' sdk/python/pyproject.toml)
got_mcp=$(sed -nE 's/^version = "(.*)"/\1/p' mcp/pyproject.toml)
got_js=$(sed -nE 's/^  "version": "(.*)",/\1/p' sdk/js/package.json | head -1)
bad=0
for pair in "sdk/python/pyproject.toml:$got_py" "mcp/pyproject.toml:$got_mcp" "sdk/js/package.json:$got_js"; do
  [ "${pair#*:}" = "$want" ] || { echo "${pair%%:*} has version ${pair#*:}, expected $want" >&2; bad=1; }
done
[ "$bad" = 0 ] || exit 1
echo "$version follows $prev"
