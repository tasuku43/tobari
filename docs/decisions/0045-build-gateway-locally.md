# ADR 0045: Build Gateway locally from embedded pinned source

- Status: Accepted
- Date: 2026-08-16
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, release, public boundary, and harness
- Revises: The publication portions of ADR 0017, ADR 0033, ADR 0043, and ADR 0044
- Superseded by: None

## Context

Gateway was the last Tobari-owned OCI artifact in the release. Publishing it
required package-write authority, a protected registry job, a generated
component lock, link-time digest injection, and a second artifact family even
though `cluster up` already ensures larger local runtime images. Gateway's
complete Docker build inputs and immutable third-party parent are already
embedded in the CLI and byte-checked against canonical source.

That publication machinery did not strengthen the network boundary. Runtime
metadata validation remains necessary whether the image came from a registry
or the local Docker store, while a source-derived local tag gives one reusable
build per installed source snapshot.

## Decision

Tobari publishes no OCI images. A released CLI embeds the pinned Gateway build
inputs, derives a local image tag from their content identity, and makes
`cluster up` build the image with Docker BuildKit only when that tag is absent.
The same API, role, architecture, entrypoint, and non-root-user validation runs
before cluster mutation. Contributor builds use the same model with their
development source identity.

The agent-ready base and experimental Auth Broker remain local-only. Custom
Context images may still be explicit user-owned OCI inputs, but they are not
Tobari release artifacts or authorities.

Release archives use the `embedded` resolver channel. Release packaging injects
only version, source commit, and the source-derived local runtime selector; it
does not consume or emit a component lock. The protected Release workflow
publishes five CLI archives and their checksum, SPDX, and unsigned provenance
metadata to GitHub Releases. Stable releases may additionally open the existing
Homebrew Formula pull request. The workflow has no package-write, registry
login, image push, or GHCR path.

## Consequences

- First use can spend time building Gateway and the agent-ready base, and needs
  Docker BuildKit plus access to their pinned public inputs.
- Repeated startup reuses the content-addressed local images until a CLI/source
  upgrade selects new identities.
- CLI and container publication become one GitHub Release flow with no
  cross-registry transaction or generated digest handoff.
- Runtime metadata checks remain compatibility checks, not signatures or
  provenance claims.

## Mechanical enforcement

- The official resolver is `embedded`, derives the Gateway tag from embedded
  bytes, and requests a build only when local inspection reports it absent.
- Missing, reuse, build-failure, and post-build compatibility tests cover the
  local ensure path.
- Release lint rejects package-write permission, GHCR references, registry
  login/push, component locks, and link-injected Gateway authorities.
- Release artifact tests bind exactly five CLI archives plus checksums, SPDX,
  and unsigned provenance metadata.
- Canonical/snapshot equality, pinned parent identity, multi-architecture
  validation, and Gateway unit/integration tests remain required.
