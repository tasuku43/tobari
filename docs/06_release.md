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
before Docker builds. The embedded Tobari, Gateway, Auth Broker, OPA policy, and compose
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
digest for routine startup. Contributor source-development uses `task
build:dev` and a `tobari_dev` binary; the public `cluster up` command does not
build Gateway source.

The canonical Auth Broker image definition is maintained under `authbroker/`.
Its package, Dockerfile, entrypoints, tests, pinned GitHub CLI checksums, MIT
license, and third-party notice are byte-checked against
`internal/infra/runtimeassets/assets/authbroker/`. The main-only Auth Broker
workflow publishes `ghcr.io/<owner>/tobari/auth-broker:latest`, `:main`, and an
immutable `:sha-<commit>` tag for Linux amd64 and arm64. The CLI records one
reviewed multi-architecture manifest digest in `versions.env` and uses that
digest for routine startup. The first public index and its anonymous registry
access, platform members, image labels, non-root user, entrypoint, and source
revision were reviewed before the digest replaced the bootstrap state.
Contributor source development uses
`tobari-auth-broker:dev` through `task build:dev`; public `cluster up` never
builds broker source.

## Compatibility

Before v1.0, breaking changes require release notes but not a deprecation
window. The stable boundaries are command paths, exit meanings, Docker labels,
state schema, configuration keys, OPA input/decision schemas, audit fields,
provider/broker/vault schemas, root-key backend identifiers, handle prefix,
Unix socket paths, Gateway/Auth Broker image labels, and preservation of each
Tobari home volume by default. `cluster down` and `cluster down --purge` also
preserve encrypted Context vaults and the installation root key; purge adds
only shared CA-volume removal.
This slice advances Gateway OPA input to schema 5, cluster status JSON to
schema 3, and Context report JSON to schema 4; auth command JSON, provider
manifests/projection, broker protocols, and encrypted vaults start at schema 1.
Release notes must identify those changes and the source-schema-4 plus
legacy-source-3-to-runtime-5 projection bridge.
Public auth backend values are exactly `macos_keychain|xdg_file`, while cluster
status may additionally use `unavailable`. The `linux_xdg_file` string is an
infrastructure/doctor diagnostic label, not a public JSON enum. The release
review uses the complete canonical schema/path/backend table in
[Authentication handling](07_authentication.md#canonical-schemas-paths-and-backend-identifiers).

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

The Auth Broker workflow follows that same permission and tag split. Before
building it verifies canonical/snapshot equality, image metadata, GitHub CLI
2.96.0 archive checksums for both supported architectures, and the bundled MIT
license/notice. Pull requests run the Python tests and cache-only
multi-architecture build without package-write permission. The main-push job
alone publishes the GHCR manifest. No login, credential, account fixture,
device code, handle, root key, vault, or authenticated output is a release
artifact.

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
release decision accepts those claims. The Gateway and Auth Broker source/image
checks are implemented, and the current Gateway and Auth Broker indexes are
public and digest-pinned. Each future registry visibility or immutable digest
promotion remains an explicit owner-side publication handoff; a moving tag or
successful workflow does not make a digest reviewed runtime authority by
itself.

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
task authbroker:test
task integration:test
```

Auth Broker changes additionally require the canonical source and image checks
used by `task check` and `task runtime:test`. The required reproducible
synthetic Auth Broker proof is delegated explicitly to `task integration:test`;
the manual transcript does not duplicate that synthetic manipulation. Release
also requires the trusted-host GitHub scenario in
[Agent Readiness Validation](09_agent_readiness_validation.md), including the
no-print assertion that `gh auth token --hostname github.com` equals the exact
projected `GH_TOKEN` handle before the allowed API call. That scenario records
only secret-free pass/fail outcomes and never becomes a repository fixture.

The first public release also requires a clean-environment Colima or Linux
Quick Start run and a human review of history, dependencies, licenses, and
generated artifacts.
