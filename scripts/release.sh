#!/usr/bin/env bash
# Cut the next release. The version is the newest "## [vX.Y.Z]" section in
# CHANGELOG.md; it must be the previous release with only the last number
# raised by one. Checks main, then pushes the tag, which starts .github/workflows/release.yml. See RELEASING.md.
#   scripts/release.sh
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

version=$(grep -Eo '^## \[v[0-9]+\.[0-9]+\.[0-9]+\]' CHANGELOG.md | head -1 | tr -d '#[] ')
[ -n "$version" ] || { echo "CHANGELOG.md has no ## [vX.Y.Z] section" >&2; exit 1; }
git fetch --quiet origin main --tags
[ -z "$(git status --porcelain --untracked-files=no)" ] || { echo "Commit or discard your changes first" >&2; exit 1; }
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] || { echo "HEAD must be origin/main (git switch main && git pull)" >&2; exit 1; }
! git rev-parse -q --verify "refs/tags/$version" >/dev/null || { echo "$version is already released; add the next version to CHANGELOG.md" >&2; exit 1; }
scripts/check_release_version.sh "$version"
scripts/changelog_section.sh "$version" >/dev/null

echo "Releasing $version at $(git log -1 --format='%h %s')"
scripts/changelog_section.sh "$version"
read -r -p "Push tag $version? [y/N] " ok
[ "$ok" = "y" ] || { echo "Cancelled"; exit 1; }
git tag -a "$version" -m "Memro $version"
git push origin "$version"
echo "Pushed. Follow the Release workflow: gh run list --workflow release.yml"
