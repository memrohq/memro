#!/usr/bin/env bash
# Print the CHANGELOG.md section for a version (without its heading), e.g.
#   scripts/changelog_section.sh v0.2.0
# Exits 1 if the section is missing or empty. Used by scripts/release.sh and
# the Release workflow for the GitHub release notes.
set -euo pipefail
version="${1:?usage: changelog_section.sh vX.Y.Z}"
file="${2:-CHANGELOG.md}"
notes=$(awk -v v="$version" '
  /^## \[/ { if (on) exit; on = (index($0, "## [" v "]") == 1); next }
  on { print }
' "$file" | sed -e '/./,$!d')
[ -n "$(printf '%s' "$notes" | tr -d '[:space:]')" ] || { echo "CHANGELOG.md has no section for $version" >&2; exit 1; }
printf '%s\n' "$notes"
