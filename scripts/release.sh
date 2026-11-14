#!/usr/bin/env bash
# Cut a release: checks main, the changelog and the tag, then pushes vX.Y.Z.
# The tag starts .github/workflows/release.yml (images, GitHub release, Go
# module tags, packages). See RELEASING.md.
#   scripts/release.sh v0.2.0
set -euo pipefail
version="${1:?usage: scripts/release.sh vX.Y.Z}"
cd "$(git rev-parse --show-toplevel)"

[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "Version must look like v1.2.3" >&2; exit 1; }
git fetch --quiet origin main --tags
[ -z "$(git status --porcelain --untracked-files=no)" ] || { echo "Commit or discard your changes first" >&2; exit 1; }
head=$(git rev-parse HEAD)
[ "$head" = "$(git rev-parse origin/main)" ] || { echo "HEAD must be origin/main (git switch main && git pull)" >&2; exit 1; }
! git rev-parse -q --verify "refs/tags/$version" >/dev/null || { echo "Tag $version already exists" >&2; exit 1; }
scripts/changelog_section.sh "$version" >/dev/null

echo "Releasing $version at $(git log -1 --format='%h %s')"
scripts/changelog_section.sh "$version"
read -r -p "Push tag $version? [y/N] " ok
[ "$ok" = "y" ] || { echo "Cancelled"; exit 1; }
git tag -a "$version" -m "Memro $version"
git push origin "$version"
echo "Pushed. Follow the Release workflow: gh run list --workflow release.yml"
