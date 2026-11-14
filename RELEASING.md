# Releasing Memro

A release is a `vX.Y.Z` tag on `main`. Pushing it runs
`.github/workflows/release.yml`, which:

1. checks the tag is on `main` and `CHANGELOG.md` has a `## [vX.Y.Z]` section,
2. builds the server and dashboard images for amd64 and arm64, pushes them to
   `ghcr.io/memrohq/memro` and `ghcr.io/memrohq/memro-dashboard` as `X.Y.Z`,
   `X.Y` and `latest`, and signs them with Sigstore,
3. creates the GitHub release, with that changelog section as the notes,
4. pushes `server/vX.Y.Z` and `sdk/go/vX.Y.Z` so Go modules resolve the version,
5. publishes packages whose version isn't on the registry yet: `memro` and
   `memro-mcp` to PyPI, `@memro/sdk` and `memro-cli` to npm. A registry whose
   token secret (`PYPI_API_TOKEN`, `NPM_TOKEN`) isn't set is skipped with a
   warning. Packages keep their own versions; bump one in its
   `pyproject.toml` or `package.json` to publish it with the next release.

## Steps

1. Move the `## [Unreleased]` notes in `CHANGELOG.md` under
   `## [vX.Y.Z] - YYYY-MM-DD`, leave an empty `## [Unreleased]` above it, and
   merge that to `main`. Bump package versions in the same pull request if
   they should publish.
2. From an up-to-date `main`, as an account that can push tags:
   ```bash
   git switch main && git pull
   scripts/release.sh vX.Y.Z
   ```
   It checks the working tree, `origin/main`, the tag and the changelog,
   shows the notes and asks before pushing.
3. Watch the run: `gh run list --workflow release.yml`, then check the
   release page, the images and the packages.
4. Pin Memro Cloud to the release: in memro-cloud,
   `go get github.com/memrohq/memro/server@vX.Y.Z`.

## If something fails

- **Verify failed:** fix the changelog or tag, delete the tag
  (`git push origin :refs/tags/vX.Y.Z` and `git tag -d vX.Y.Z`) and tag again.
- **An image or package job failed:** fix the cause, then re-run the failed
  jobs from the Actions page. Every step is safe to re-run: published
  packages and existing Go tags are skipped, and an existing release only has
  its notes updated.
- A version on PyPI or npm can't be published twice. Bump the package version
  rather than re-using one.
