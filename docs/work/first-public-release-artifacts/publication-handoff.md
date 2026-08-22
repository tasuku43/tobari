# Publication Handoff: First public V1

Do not run this handoff without explicit maintainer approval. The release tag
remains a maintainer-owned input. Stable distribution uses the fixed reviewed
`tasuku43/homebrew-tap` destination.

## 1. Confirm the immutable source and external authorities

```sh
cd "$(git rev-parse --show-toplevel)"
test -z "$(git status --porcelain)"
test "$(git branch --show-current)" = agent/reset-pre-public-contracts-v1
source_revision=$(git rev-parse HEAD)
release_tag=${RELEASE_TAG:?set the approved vMAJOR.MINOR.PATCH tag}
repo=tasuku43/tobari
branch=agent/reset-pre-public-contracts-v1
```

## 2. Push and validate the integrated release candidate

The first command below is the first external mutation.

```sh
git push -u origin "$branch"
```

Wait for pull-request CI to pass. It runs full, security, public, runtime, and
release packaging evidence as independent parallel jobs. Confirm no component lock or OCI
artifact is present. Release preparation occurs only after the exact reviewed
revision reaches `main` and its main-push CI succeeds.

## 3. Keep release outputs out of source

Do not add Gateway or Auth Broker digests to `versions.env`. Pin the generated
site to the already reviewed source revision and regenerate it before the final
release commit:

```sh
# Apply a reviewed patch setting docs/architecture-site/source-snapshot.txt
# to exactly $source_revision.
npm --prefix docs/architecture-site run generate
git add docs/architecture-site/source-snapshot.txt docs/architecture-site/src/generated
git commit -m 'docs: publish reviewed V1 source identity'
release_revision=$(git rev-parse HEAD)
```

## 4. Run every final gate and manual release scenario

Run on a healthy clean Linux or Colima Docker environment:

```sh
mise exec -- task check
mise exec -- task security
mise exec -- task public:check
mise exec -- task release:check
mise exec -- task policy:test
mise exec -- task gateway:test
mise exec -- task authbroker:test
mise exec -- task integration:test
```

Also complete the documented clean Quick Start, disposable GitHub trusted-host
login/static-import scorecards, deny/review/allow/manual-retry journey, and
history/dependency/license/generated-artifact review without retaining secrets.

## 5. Prepare the exact GitHub Release candidate once

```sh
git push origin "$branch"
# Merge the reviewed branch, then bind preparation to the exact main revision.
git fetch origin main
release_revision=$(git rev-parse origin/main)
gh workflow run release.yml --repo "$repo" --ref main \
  -f operation=prepare -f tag="$release_tag" -f revision="$release_revision"
```

Wait for the complete release asset artifact, download it, and independently
verify the five archives, `checksums.txt`, SPDX SBOM,
unsigned provenance, and stable Formula before creating a tag. Record the
successful Release preparation run ID as `prepared_run_id`; its artifact expires
after seven days.

## 6. Publish only after a second synchronous approval

```sh
git tag -a "$release_tag" "$release_revision" -m "Tobari $release_tag"
git push origin "refs/tags/$release_tag"
gh workflow run release.yml --repo "$repo" --ref main \
  -f operation=publish -f tag="$release_tag" -f revision="$release_revision" \
  -f prepared_run_id="$prepared_run_id"
```

The protected `release-publication` environment must approve the final jobs.
After the immutable Release succeeds, confirm the workflow-created Formula-only
pull request in `tasuku43/homebrew-tap` passes its checks and merges. Confirm it
contains the exact released `tobari.rb`; do not create a second manual Formula.
On a clean host, run:

```sh
brew tap tasuku43/tap
brew install tasuku43/tap/tobari
tobari version
tobari doctor
brew uninstall tobari
```

Do not delete or overwrite an existing tag, Release, or Release asset. A
correction uses a new reviewed source revision and version.
