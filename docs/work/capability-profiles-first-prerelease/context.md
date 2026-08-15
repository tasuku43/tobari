# Work Context: Capability profiles and first prerelease

## Observed starting behavior

- `task build` and `task build:dev` both compile with only `tobari_dev`; their provider surface is identical.
- The compiled reviewed login order includes GitHub, AWS, Datadog, OpenAI, and Anthropic.
- `go run ./tools/releaseversion v0.1.0-dev.1` reports `stable=false`.
- The Release workflow marks non-stable tags as GitHub prereleases and skips the stable-only Homebrew job.
- The generated component lock binds Gateway and Auth Broker. The accepted
  local-base decision makes that the complete release lock rather than an
  omission.
- Anonymous GHCR inspection on 2026-08-16 found existing public `tobari/runtime`, `tobari/gateway`, `tobari/auth-broker`, and `tobari-runtime` packages. All four were permanently deleted through GitHub's authenticated package settings. The owner package page then showed zero packages and anonymous GHCR token requests returned HTTP 403 for all four names.
- Standalone Gateway and Auth Broker workflows still contain protected manual publish jobs.

## Relevant structure

- Entry point: `Taskfile.yml` and `.github/workflows/release.yml`
- Domain rule: `internal/domain/authbroker/provider_registry.go`, `internal/domain/componentlock`
- Application use case: existing authentication and lifecycle use cases remain unchanged
- Infrastructure boundary: `internal/infra/authproviders`, `internal/infra/dockerruntime`
- CLI catalog or presentation: `internal/cli/auth_catalog.go`, `internal/cli/version.go`
- Existing tests and harness checks: `scripts/check.sh`, `scripts/lint-release.sh`

## Constraints

- Profiles are compile-time authority, not a runtime escape hatch.
- Standard capabilities cannot depend on experimental capabilities.
- The release lock is generated from actual published digests and is never committed.
- Gateway and Auth Broker must share one exact source revision and Linux platform set. The agent-ready base is source-bound through embedded bytes, a source-derived local tag, and local compatibility validation.
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
- [x] Public documentation source snapshot advanced to implementation commit `475e201703579d4c639c37f8051cbf6d80b22a52`; generated evidence awaits the publication revision commit.

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
  -f tag=v0.1.0-dev.1 \
  -f revision="$revision" \
  -f publish=false
```

After the protected environment exists, publish that same revision:

```sh
revision=$(git rev-parse HEAD)
gh workflow run release.yml --ref main \
  -f tag=v0.1.0-dev.1 \
  -f revision="$revision" \
  -f publish=true
```

The release workflow revalidates the full revision and publishes only Gateway
and Auth Broker to GHCR. The agent-ready base remains local-build-only.

## Security and public-boundary notes

- Assets and side effects involved: two public OCI packages, one GitHub prerelease, and release assets.
- Credentials or confidential data involved: GitHub publication authority only; no provider credential material.
- New dependencies, destinations, files, processes, or generated content: no dependency; first-use Docker build downloads pinned agent artifacts from their official sources.
- Publication and licensing concerns: Tobari does not publish the combined agent image; native Anthropic account-login terms remain a separate manual product/legal release review.

## Glossary

- Standard profile: supported capabilities compiled into normal and release builds.
- Experimental profile: standard capabilities plus development-only capabilities.
- Component lock: generated release authority binding exact OCI digests to one source revision.
