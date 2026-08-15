# ADR 0043: Build the agent-ready base locally

- Status: Accepted
- Date: 2026-08-16
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, release, public boundary, and harness
- Revises: The runtime-publication portions of ADR 0033 and ADR 0039
- Superseded by: None

## Context

Tobari's default Context needs Codex and Claude Code from first use. Publishing
their combined image from GHCR would also make Tobari the distributor of those
third-party binaries. Claude Code's published license is not an open-source
redistribution grant, and its artifact lock correctly records that review as
pending. Keeping a dormant runtime-push path behind a review flag makes the
release model harder to understand and leaves an unnecessary privileged path.

The released CLI already embeds the canonical base Dockerfile, entrypoint,
public key, exact versions, and artifact checksums. Docker can therefore build
the same reviewed environment directly for the user without a Tobari-published
agent image.

## Decision

The agent-ready base is permanently local-build-only. Tobari does not publish
it to GHCR. A released CLI selects a source-derived local image tag and, when
that image is absent, builds it from the embedded canonical recipe before
validating the runtime API, user, entrypoint, and lifetime contract. The build
downloads pinned agent artifacts from their official sources and verifies the
reviewed checksums. Custom Context recipes extend that local base.

The protected Release workflow publishes exactly two internal service images:
Gateway and Auth Broker. Its generated schema-1 component lock binds only
those immutable multi-architecture indexes, their APIs, and their common source
revision. CLI packaging injects those service authorities and a deterministic
local-base tag; no runtime image digest enters the release lock.

Validation workflows continue to build the base for both supported Linux
architectures with cache-only output. The pending agent license-review fields
remain honest integrity metadata, but they no longer gate a publication path
because no agent-ready image publication path exists.

## Consequences

- First use may spend several minutes and requires Docker build support plus
  network access to the pinned official artifact sources.
- Tobari distributes the recipe and integrity policy, not the combined agent
  binary image.
- Gateway and Auth Broker remain common reviewed bytes obtained by immutable
  digest, while the Workspace environment is built on the user's Docker host.
- A CLI upgrade may select a new source-derived local tag; existing Context
  authority remains `builtin` and resolves through that installed CLI.
- Release publication has one fewer privileged artifact and no dormant runtime
  registry mutation.

## Mechanical enforcement

- The component-lock schema rejects a `runtime` field and accepts exactly the
  Gateway and Auth Broker authorities.
- Release scripts and workflow tests reject the runtime GHCR repository,
  runtime registry push, and redistribution-gate flag.
- The official resolver never pulls a runtime image. Missing-base tests require
  one build from the embedded `tobari/` asset tree, followed by the ordinary
  compatibility inspection; existing-image tests require zero build calls.
- Runtime source synchronization, checksum locks, multi-architecture cache-only
  builds, and agent version smokes remain required gates.
