# Publication Handoff: First public V1

Do not run this handoff without explicit maintainer approval. The release tag
and Homebrew tap repository remain maintainer-owned inputs; do not infer them.

## 1. Confirm the immutable source and external authorities

```sh
cd "$(git rev-parse --show-toplevel)"
test -z "$(git status --porcelain)"
test "$(git branch --show-current)" = agent/reset-pre-public-contracts-v1
source_revision=$(git rev-parse HEAD)
release_tag=${RELEASE_TAG:?set the approved vMAJOR.MINOR.PATCH tag}
tap_repo=${HOMEBREW_TAP_REPO:?set the approved owner/repository tap}
repo=tasuku43/tobari
branch=agent/reset-pre-public-contracts-v1
```

## 2. Push and validate both component workflows

The first command below is the first external mutation.

```sh
git push -u origin "$branch"
gh workflow run gateway-image.yml --repo "$repo" --ref "$branch" \
  -f revision="$source_revision" -f publish=false
gh workflow run authbroker-image.yml --repo "$repo" --ref "$branch" \
  -f revision="$source_revision" -f publish=false
```

Wait for both validate runs to pass. Then request the protected-environment
approval and publish the exact same revision:

```sh
gh workflow run gateway-image.yml --repo "$repo" --ref "$branch" \
  -f revision="$source_revision" -f publish=true
gh workflow run authbroker-image.yml --repo "$repo" --ref "$branch" \
  -f revision="$source_revision" -f publish=true
```

Record the two publish run IDs as `gateway_run_id` and `authbroker_run_id`,
then independently download and validate their evidence:

```sh
evidence_dir=$(mktemp -d)
gh run download "$gateway_run_id" --repo "$repo" \
  -n "gateway-component-$source_revision" -D "$evidence_dir/gateway"
gh run download "$authbroker_run_id" --repo "$repo" \
  -n "auth-broker-component-$source_revision" -D "$evidence_dir/authbroker"
test "$(jq -er .revision "$evidence_dir/gateway/gateway.component.json")" = "$source_revision"
test "$(jq -er .revision "$evidence_dir/authbroker/auth-broker.component.json")" = "$source_revision"
gateway_digest=$(jq -er .digest "$evidence_dir/gateway/gateway.component.json")
authbroker_digest=$(jq -er .digest "$evidence_dir/authbroker/auth-broker.component.json")
docker buildx imagetools inspect "ghcr.io/tasuku43/tobari/gateway@$gateway_digest"
docker buildx imagetools inspect "ghcr.io/tasuku43/tobari/auth-broker@$authbroker_digest"
```

## 3. Pin reviewed component identities and regenerate public data

Apply one reviewed patch to `internal/infra/runtimeassets/assets/versions.env`:

```text
GATEWAY_IMAGE=ghcr.io/tasuku43/tobari/gateway@<gateway_digest>
AUTH_BROKER_IMAGE=ghcr.io/tasuku43/tobari/auth-broker@<authbroker_digest>
```

Keep both API values at `1`, commit both pins atomically, then pin the generated
site to that commit and regenerate:

```sh
git add internal/infra/runtimeassets/assets/versions.env
git commit -m 'build: pin reviewed V1 component images'
pin_revision=$(git rev-parse HEAD)
# Apply a reviewed patch setting docs/architecture-site/source-snapshot.txt
# to exactly $pin_revision.
npm --prefix docs/architecture-site run generate
git add docs/architecture-site/source-snapshot.txt docs/architecture-site/src/generated
git commit -m 'docs: publish reviewed V1 component identities'
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

## 5. Prepare the exact GitHub Release without publication

```sh
git push origin "$branch"
gh workflow run release.yml --repo "$repo" --ref "$branch" \
  -f tag="$release_tag" -f revision="$release_revision" -f publish=false
```

Wait for the complete release asset artifact, download it, and independently
verify the five archives, `checksums.txt`, SPDX SBOM, unsigned provenance, and
stable Formula before creating a tag.

## 6. Publish only after a second synchronous approval

```sh
git tag -a "$release_tag" "$release_revision" -m "Tobari $release_tag"
git push origin "refs/tags/$release_tag"
gh workflow run release.yml --repo "$repo" --ref "$branch" \
  -f tag="$release_tag" -f revision="$release_revision" -f publish=true
```

The protected `release-publication` environment must approve the final job.
After the immutable Release is verified, clone the approved `$tap_repo`, copy
the exact released `tobari.rb` asset to `Formula/tobari.rb`, review and push
that tap commit as a separate external operation. On a clean host, run:

```sh
brew tap "$tap_repo"
brew install "$tap_repo/tobari"
tobari version
tobari doctor
brew uninstall tobari
```

Do not delete or overwrite an existing tag, component immutable tag, Release,
or Release asset. A correction uses a new reviewed source revision and version.
