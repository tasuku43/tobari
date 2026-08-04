# Release Model

Tobari follows Semantic Versioning and initially supports source builds on
Linux and macOS with Docker Engine. Colima is the documented macOS runtime;
Docker Desktop-specific behavior is not required.

## Artifacts

The Go CLI remains a pure-Go binary for Linux and macOS on amd64 and arm64.
Windows CLI archives are buildable by the inherited packaging harness but the
MVP runtime is not supported on Windows because bind mounts, Unix ownership,
TTY behavior, and container networking have not been validated there.

Runtime assets are embedded in the binary and materialized into versioned state
before Docker builds. The embedded Tobari, Gateway, OPA policy, and compose
inputs are therefore bound to the CLI source revision. Container base images
are pinned to reviewed immutable versions or digests.

The canonical base image definition is maintained under `runtimes/base` and
its Dockerfile/bootstrap snapshot is checked against the embedded runtime
assets. The first official image workflow supports Linux amd64 and arm64.
The canonical Gateway image definition is maintained under `gateway` and its
Dockerfile, addon, entrypoint, and tests are checked against the embedded
`internal/infra/runtimeassets/assets/gateway` snapshot. The main-only Gateway
workflow publishes `ghcr.io/<owner>/tobari/gateway:latest`, `:main`, and an
immutable `:sha-<commit>` tag for Linux amd64 and arm64. The CLI records one
reviewed multi-architecture manifest digest in `versions.env` and uses that
digest for routine startup; `cluster up --gateway-source` is the explicit
source-development/recovery path.

## Compatibility

Before v1.0, breaking changes require release notes but not a deprecation
window. The stable boundaries are command paths, exit meanings, Docker labels,
state schema, configuration keys, OPA input/decision schemas, audit fields, and
preservation of each Tobari home volume by default.

## Publication

Tags use `vMAJOR.MINOR.PATCH` for CLI releases. Image releases are independent:
the base uses `ghcr.io/<owner>/tobari/runtime:<base-version>`, while a derived
agent uses `<agent>.<agent-version>-base.<base-version>-r<revision>`, for example
`claude.2.1.34-base.0.1.0-r1`. A push to `main` additionally runs the base-image
workflow, which publishes `ghcr.io/<owner>/tobari/runtime:latest`, `:main`, and
an immutable `sha-<commit>` tag for the exact main revision. `latest` and
`main` are moving development channels, not stable image releases. Pull
requests never receive package-write permission.

The Gateway workflow follows the same moving-versus-immutable tag rule. Its
pull-request job has no package-write permission and uses a cache-only
multi-architecture build; only the main-push job can publish to GHCR.

The first Claude and Codex agent-image slices are build-only: their
pull-request and main-push workflows validate the pinned parent, agent release
checksums, multi-architecture build, and inherited runtime contract without
publishing an agent tag. Public agent publication remains blocked until the
agent redistribution terms and image-layer license review are recorded.

The current publication boundary therefore has one supported runtime family
edge: the base image may use its reviewed moving development channels and
immutable commit tag, while Claude and Codex variants are local/CI build
artifacts only. The repository does not claim a public agent image, stable
support window, SBOM/attestation, or redistribution approval until a new
release decision accepts those claims. The Gateway source and image checks are
implemented; anonymous package visibility is the remaining owner-side
publication handoff recorded in the active
[Gateway image packet](work/gateway-official-image/goal.md).

Tobari does not yet claim code signing, notarization, SBOM attestation, or
externally verifiable build provenance. Checksums protect selected artifact
integrity but do not identify the builder.

## Ownership and security

The repository owner performs release and confidentiality review. Vulnerability
reports use GitHub private vulnerability reporting as documented in
[Security Policy](../SECURITY.md). A broken or vulnerable release is replaced
by a new version; assets for an existing tag are not overwritten.

## Required gates

Before tagging:

```sh
task check
task security
task release:check
task public:check
task policy:test
task gateway:test
task integration:test
```

The first public release also requires a clean-environment Colima or Linux
Quick Start run and a human review of history, dependencies, licenses, and
generated artifacts.
