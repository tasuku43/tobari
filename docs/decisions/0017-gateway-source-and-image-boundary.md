# ADR 0017: Separate Gateway source ownership from CLI embedding

- Status: Accepted
- Date: 2026-08-02
- Deciders: Tobari maintainers
- Scope: Architecture, security, release, and harness
- Supersedes: None
- Superseded by: None

## Context

Gateway is a trusted enforcement component, but its Python source is currently
stored below the Go CLI's embedded runtime asset tree. That layout keeps the
CLI self-contained and keeps the Compose contract close to its consumer, but it
also makes Gateway source ownership and image publication difficult to see.
The product needs a simple first startup and should not require every user to
build the trusted proxy implementation locally. At the same time, contributors
must be able to test Gateway source changes locally without waiting for a
main-push image publication and digest pin.

## Decision drivers

- Keep one editable Gateway source of truth inside the monorepo.
- Preserve a self-contained CLI and an explicit contributor
  source-development path.
- Publish a reviewed multi-architecture image without granting it policy,
  credential, project-root, or project-principal authority.
- Keep pull requests free of package-write permissions.
- Avoid baking one developer's host UID/GID into the official image.
- Make the private Gateway CA and public runtime CA volume boundary explicit.

## Considered options

### Option A: Keep Gateway only under the embedded asset tree

This minimizes file movement and keeps the current source build intact, but it
leaves an independent Gateway release unit hidden inside a CLI implementation
package. Rejected as the long-term source ownership model.

### Option B: Separate Gateway repository

This gives Gateway a clear release boundary but introduces cross-repository
version coordination, snapshot synchronization, and public-repository overhead
before the monorepo boundary has been tested. Rejected.

### Option C: Monorepo canonical source plus checked embed snapshot

Gateway source, Docker packaging, and Python tests live under `gateway/`. The
CLI retains `internal/infra/runtimeassets/assets/gateway/` as a generated,
checked snapshot for `embed.FS`. Chosen.

## Decision

`gateway/` is the canonical Gateway source. The explicit
`scripts/sync-gateway-source.sh` operation copies it into the embedded snapshot;
`scripts/check-gateway-source.sh` fails on any byte drift. Gateway unit tests
run against the canonical source, while the runtime integration continues to
exercise the snapshot actually embedded in the CLI.

The Gateway Dockerfile is host-UID-independent. Image code is root-owned and
read-only, the default image user is a non-root numeric identity, and Compose
supplies the invoking host UID/GID at runtime. The private CA named volume is
Gateway-only; the public CA named volume is mounted read-only into work
containers. The writable volume initialization directories are deliberately
separate from the read-only credential and principal mounts.

The main-only `gateway-image.yml` workflow builds Linux amd64 and arm64 from
the canonical source using the digest-pinned mitmproxy parent recorded in the
embedded versions file. It publishes `latest`, `main`, and `sha-<commit>`
development identities to GHCR. The pull-request job is cache-only and has no
package-write permission. No provenance or SBOM claim is added by this ADR.

The original decision recorded one reviewed multi-architecture Gateway digest
in embedded `versions.env`; ADR 0033 supersedes that release-binding mechanism
with a generated paired component lock consumed during routine `cluster up`.
Startup verifies the digest, API/role labels, non-root entrypoint, and Docker
Engine platform before policy tests or cluster resources. Contributor source
development now uses `task build`, which builds a source-hash Gateway tag and a
development CLI whose resolver selects that exact local tag and runs the same
compatibility preflight. Moving tags are never that authority.

## Consequences

### Positive

- Gateway source, tests, and Docker packaging have an obvious monorepo home.
- The CLI remains self-contained while contributor source changes can still be
  verified locally through an explicit development build.
- Snapshot drift is a mechanical gate rather than a code-review convention.
- The official image can be built and published independently from CLI archive
  releases without moving policy or credential authority into the image.

### Negative

- The routine path now depends on the published immutable Gateway digest;
  registry access or image-publication failures are repaired through the
  publication path rather than a public local-build option.
- The repository carries one generated embed snapshot in addition to the
  canonical source.
- The volume directory permission design must be tested on each supported
  Docker Engine environment before direct official-image startup is enabled.

## Mechanical enforcement

- `task gateway:source:check` checks canonical/snapshot byte equality.
- `task check` runs the snapshot check and Go asset tests.
- `task gateway:test` mounts `gateway/`, not the generated snapshot.
- The Gateway image workflow separates pull-request cache-only builds from
  main-push publication permissions and verifies the pinned parent reference.
- The Gateway runtime preflight verifies the immutable digest, API/role labels,
  non-root user, entrypoint, and Docker Engine platform before cluster mutation.
- `task build` builds or reuses the source-hash local Gateway tag and compiles
  the development image resolver.
- The runtime and integration gates retain the embedded snapshot and private /
  public CA volume checks.

## Validation

- `task check`
- `task security`
- `task release:check`
- `task public:check`
- `task runtime:test`
