# Work Context: Capability profiles and first prerelease

## Observed starting behavior

- `task build` and `task build:dev` both compile with only `tobari_dev`; their provider surface is identical.
- The compiled reviewed login order includes GitHub, AWS, Datadog, OpenAI, and Anthropic.
- `go run ./tools/releaseversion v0.1.0-dev.1` reports `stable=false`.
- The Release workflow marks non-stable tags as GitHub prereleases and skips the stable-only Homebrew job.
- The accepted ADR 0045 decision removes the component lock and every
  Tobari-owned OCI publication path. Released CLIs build pinned local Gateway
  and runtime images from embedded source.
- Anonymous GHCR inspection on 2026-08-16 found existing public `tobari/runtime`, `tobari/gateway`, `tobari/auth-broker`, and `tobari-runtime` packages. All four were permanently deleted through GitHub's authenticated package settings. The owner package page then showed zero packages and anonymous GHCR token requests returned HTTP 403 for all four names.
- Standalone image workflows are validation-only and the Release workflow has
  no package-write, registry login, or image push path.

## Relevant structure

- Entry point: `Taskfile.yml` and `.github/workflows/release.yml`
- Domain rule: `internal/domain/authbroker/provider_registry.go`, `internal/domain/buildidentity`
- Application use case: existing authentication and lifecycle use cases remain unchanged
- Infrastructure boundary: `internal/infra/authproviders`, `internal/infra/dockerruntime`
- CLI catalog or presentation: `internal/cli/auth_catalog.go`, `internal/cli/version.go`
- Existing tests and harness checks: `scripts/check.sh`, `scripts/lint-release.sh`

## Constraints

- Profiles are compile-time authority, not a runtime escape hatch.
- Standard capabilities cannot depend on experimental capabilities.
- Gateway, Auth Broker, and the agent-ready base are source-bound through
  embedded bytes, source-derived local tags, and local compatibility validation.
- The first intentional publication must occur only after existing packages are removed and rechecked.
- Credentials and live provider material never enter release fixtures or logs.

## External facts

- GitHub REST package deletion requires package read/delete authority; the current local `gh` token lacks `read:packages` and `delete:packages`.
- Anonymous OCI registry reads are sufficient to verify current public tags and post-delete absence.
- The official Codex repository and package metadata identify Codex CLI as
  Apache-2.0. The Claude Code 2.1.220 native npm package instead states
  `All rights reserved` and points to Anthropic legal agreements; no explicit
  redistribution grant was found. Publication therefore remains fail-closed.

## Unknowns

- [ ] Confirm the protected `release-publication` environment is configured before the publish run.
- [x] Public documentation source snapshot advanced to implementation commit
  `381835cda72885f78f5a3c9c69868e25df9a15fd`; generated evidence is committed
  in `459170f`.

## Thesis evidence

- Repeated design decision or point of agent confusion: development-only behavior has repeatedly needed exclusion from normal builds.
- User outcome or friction observed in the minimal slice: AWS authentication should remain usable for development without becoming a first-release promise.
- Code workaround or exception being considered: a provider-specific `aws` build tag.
- Current thesis that resolves it, or proposed thesis revision: declare reusable standard and experimental capability profiles instead of feature-specific build identities.
- Downstream product, architecture, security, Skill, catalog, and harness impact: build identity, provider projection, help, release locks, workflows, and matrix tests.

## Reproduction or observation

```sh
go run ./tools/releaseversion v0.1.0-dev.1
curl -H "Authorization: Bearer $ANONYMOUS_PULL_TOKEN" \
  https://ghcr.io/v2/tasuku43/tobari/runtime/tags/list
```

After the reviewed revision is committed and pushed to `main`, prepare the
exact prerelease without publication:

```sh
revision=$(git rev-parse HEAD)
gh workflow run release.yml --ref main \
  -f operation=prepare \
  -f tag=v0.1.0-dev.1 \
  -f revision="$revision"
```

After the protected environment exists, publish that same revision:

```sh
revision=$(git rev-parse HEAD)
gh workflow run release.yml --ref main \
  -f operation=publish \
  -f tag=v0.1.0-dev.1 \
  -f revision="$revision" \
  -f prepared_run_id="$prepared_run_id"
```

The release workflow revalidates the successful preparation run, full revision,
tag, provenance, and inventory, then publishes only those prepared CLI archives
and metadata to GitHub Releases. All Tobari-owned images remain local.

## Security and public-boundary notes

- Assets and side effects involved: one GitHub prerelease and its release assets.
- Credentials or confidential data involved: GitHub publication authority only; no provider credential material.
- New dependencies, destinations, files, processes, or generated content: no dependency; first-use Docker build downloads pinned agent artifacts from their official sources.
- Publication and licensing concerns: Tobari does not publish the combined agent image; native Anthropic account-login terms remain a separate manual product/legal release review.

## Glossary

- Standard profile: supported capabilities compiled into normal and release builds.
- Experimental profile: standard capabilities plus development-only capabilities.
- Embedded resolver: released-CLI authority binding local image identities to
  the CLI's pinned source bytes.
