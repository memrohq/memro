# Releasing Memro

A release is a `vX.Y.Z` tag on `main`. **Each release raises only the last
number by one** (v0.1.0, v0.1.1, v0.1.2, …); the release script and the
workflow refuse anything else. Packages keep their own versions, as listed in
each release's notes (see `docs/releases/v0.1.0.md`).

Pushing the tag runs
`.github/workflows/release.yml`, which:

1. checks the tag is on `main`, follows the previous release with only the
   last number raised, and has a `## [vX.Y.Z]` section in `CHANGELOG.md`,
2. builds the server and dashboard images for amd64 and arm64, pushes them to
   `ghcr.io/memrohq/memro` and `ghcr.io/memrohq/memro-dashboard` as `X.Y.Z`,
   `X.Y` and `latest`, and signs them with Sigstore,
3. creates the GitHub release, with that changelog section as the notes,
4. pushes `server/vX.Y.Z` and `sdk/go/vX.Y.Z` so Go modules resolve the version,
5. publishes packages whose version isn't on the registry yet: `memro` and
   `memro-mcp` to PyPI, `@memro/sdk` and `memro-cli` to npm. A registry whose
   token secret (`PYPI_API_TOKEN`, `NPM_TOKEN`) isn't set is skipped with a
   warning. To publish a package, raise its version in its `pyproject.toml`
   or `package.json` before the release.

## Steps

1. In one pull request to `main`:
   - move the `## [Unreleased]` notes in `CHANGELOG.md` under
     `## [vX.Y.Z] - YYYY-MM-DD` (the previous version with the last number
     plus one), leaving an empty `## [Unreleased]` above it;
   - raise the version of any package that should publish (its
     `pyproject.toml` and `__init__.py`, or `package.json`);
   - check the release number with `scripts/check_release_version.sh vX.Y.Z`.
2. After it's merged, from an up-to-date `main`, as an account that can push
   tags:
   ```bash
   git switch main && git pull
   scripts/release.sh
   ```
   It takes the version from the changelog, checks it, shows the notes and
   asks before pushing the tag.
3. Watch the run: `gh run list --workflow release.yml`, then check the
   release page, the images and the packages.
4. Pin Memro Cloud to the release: in memro-cloud,
   `go get github.com/memrohq/memro/server@vX.Y.Z`.

## If something fails

- **Verify failed:** fix the changelog in a pull request, delete the tag
  (`git push origin :refs/tags/vX.Y.Z` and `git tag -d vX.Y.Z`) and tag again.
- **An image or package job failed:** fix the cause, then re-run the failed
  jobs from the Actions page. Every step is safe to re-run: published
  packages and existing Go tags are skipped, and an existing release only has
  its notes updated.
- A version on PyPI or npm can't be published twice. Bump the package version
  rather than re-using one.
