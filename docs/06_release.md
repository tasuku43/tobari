# Release Model

Tobari follows Semantic Versioning and initially supports source builds on
Linux and macOS with Docker Engine. Colima is the documented macOS runtime;
Docker Desktop-specific behavior is not required.

## Artifacts

The Go CLI remains a pure-Go binary for Linux and macOS on amd64 and arm64.
Windows CLI archives are buildable by the inherited packaging harness but the
MVP runtime is not supported on Windows because bind mounts, Unix ownership,
TTY behavior, and container networking have not been validated there.
Provider homes, credentials, handles, root keys, and vault state are never
packaged. V1 has no resident credential companion or second executable.

Every CLI artifact exposes build identity through `version` text and schema-1
JSON. A release archive embeds the validated SemVer and full source commit,
uses the published resolver channel, names source-required and selected
Gateway/Auth Broker APIs, and leaves contributor command fields empty. A
repository build retains version `dev`, embeds only the exact HEAD commit, and
selects content-addressed local component images. `bin/tobari-dev` is retained
only as a compatibility-named development output, not a release candidate.

One complete CLI archive matrix is accompanied by `component-lock.json` and
three repository-generated metadata files. The strict schema-1 lock binds the
source revision, immutable Gateway and Auth Broker indexes, APIs, and exact
Linux platform set. `checksums.txt` binds the five sorted archives and the lock
to SHA-256 digests. `sbom.spdx.json` is an SPDX 2.3 package inventory of those
same release subjects, their release URLs, versions, declared project license,
and SHA-256 digests; `filesAnalyzed: false` states its deliberate archive-level
coverage instead of implying a file or dependency analysis it did not perform.
It is not a dependency, container-layer, or vulnerability inventory.
`provenance.intoto.jsonl` is one unsigned in-toto Statement v1 with a SLSA
provenance v1 predicate that binds the same subjects to the requested tag,
reviewed source revision, target matrix, stable builder/workflow identity, and
separate concrete local or CI invocation identity.
The repository-owned `tools/releaseartifacts` command creates these files
without network access or overwrite and verifies them by exact deterministic
regeneration. Its normalized SPDX creation timestamp is reproducibility
metadata, not publication time.

Runtime assets are embedded in the binary and materialized into versioned state
before Docker builds. The embedded Tobari, Gateway, Auth Broker, OPA policy, and compose
inputs are therefore bound to the CLI source revision. Container base images
are pinned to reviewed immutable versions or digests.

The canonical base image definition is maintained under `runtimes/base` and
its Dockerfile/bootstrap snapshot is checked against the embedded runtime
assets. This capability leaves the published base at its pre-change GitHub CLI
and AWS CLI baseline and preserves their checked-in per-platform integrity and
redistribution checks. `kubectl`, `cwk`, `pup`, and TWG are artifacts of the
explicit user-triggered local toolbox only; that build does not create a public
image or change the base metadata/snapshot. The official image workflow builds
Linux amd64 and arm64 and performs the existing offline native command smoke
check for the workflow host's published platform.
The canonical Gateway image definition is maintained under `gateway` and its
Dockerfile, addon, entrypoint, and tests are checked against the embedded
`internal/infra/runtimeassets/assets/gateway` snapshot. Release assembly builds
and, after approval, publishes its immutable `:sha-<commit>` Linux amd64/arm64
index. The generated component lock supplies the digest used by routine
startup. Contributor source development uses the source-hash tag built by
`task build`; public `cluster up` does not build Gateway source.

The canonical Auth Broker image definition is maintained under `authbroker/`.
Its package, Dockerfile, entrypoints, bridge/protocol, tests, and provider-CLI
absence are byte-checked against
`internal/infra/runtimeassets/assets/authbroker/`. Release assembly builds and,
after approval, publishes its immutable `:sha-<commit>` Linux amd64/arm64 index
beside Gateway. The generated component lock supplies the digest used by
routine startup. Its anonymous registry access, platform members, image labels,
non-root user, entrypoint, and source revision are reviewed before CLI release.
Contributor source development uses a source-hash local tag through `task
build`; public `cluster up` never builds broker source.

## Pre-public V1 contract

All Tobari-owned command outputs, persisted state, configuration, OPA input and
decisions, audits, provider/projection/vault records, private protocols, and
Gateway/Auth Broker component APIs use V1. Readers accept exactly V1 and reject
every other version. Before the first public release, development snapshots
receive no deprecation window, migration, compatibility reader, retired command
alias, or old-state interpretation; local state must be removed and recreated
when the contract changes.

The V1 boundaries include command paths, exit meanings, Docker labels,
configuration keys, root-key backend identifiers, the handle prefix, Unix
socket paths, and preservation of each Tobari home volume by default. `cluster
down` and `cluster down --purge` also preserve encrypted Context vaults and the
installation root key; purge adds only shared CA-volume removal.
Public auth backend values are exactly `macos_keychain|xdg_file`, while cluster
status may additionally use `unavailable`. The `linux_xdg_file` string is an
infrastructure/doctor diagnostic label, not a public JSON enum. The release
review uses the complete canonical schema/path/backend table in
[Authentication handling](07_authentication.md#canonical-schemas-paths-and-backend-identifiers).

Every public JSON envelope uses schema 1 and exact catalog-owned recursive
fields. Explicit `null`, empty collections, zero, false, and finite unavailable
states retain their declared meaning; presentation must not replace them with
sentinels or infer missing authority.

## Publication

Tags use `vMAJOR.MINOR.PATCH` for CLI releases. Gateway and Auth Broker are
internal artifacts of that release; the base remains independently versioned
as `ghcr.io/<owner>/tobari/runtime:<base-version>`, while a derived
agent uses `<agent>.<agent-version>-base.<base-version>-r<revision>`, for example
`claude.2.1.34-base.0.1.0-r1`. A push to `main` additionally runs the base-image
workflow, which publishes `ghcr.io/<owner>/tobari/runtime:latest`, `:main`, and
an immutable `sha-<commit>` tag for the exact main revision. `latest` and
`main` are moving development channels, not stable image releases. Pull
requests never receive package-write permission.

Pull-request component workflows remain validation-only and have no package
write permission. The manual release workflow owns paired publication after
protected-environment approval. It verifies canonical/snapshot equality, image
metadata, provider-CLI absence, and the closed reviewed-plan protocol suite,
then creates the component lock before CLI packaging. No login, credential,
account fixture, device or authorization code, token, handle, root key, vault,
or authenticated output is a release artifact.

The first Claude and Codex agent-image slices are build-only: their
pull-request and main-push workflows validate the pinned parent, agent release
checksums, multi-architecture build, and inherited runtime contract without
publishing an agent tag. Their OCI/runtime metadata uses `NOASSERTION` while
the lock records `license_review: pending`; it must not imply that the bundled
agent layer is MIT-licensed. Public agent publication remains blocked until
the agent redistribution terms and image-layer license review are recorded.

The current publication boundary therefore has one supported runtime family
edge: the base image may use its reviewed moving development channels and
immutable commit tag, while Claude and Codex variants are local/CI build
artifacts only. The repository does not claim a public agent image, stable
support window, image SBOM/attestation, or redistribution approval until a new
release decision accepts those claims. Gateway and Auth Broker source records
contain no release-output digest. A moving tag or standalone successful
workflow does not make a digest runtime authority; only the paired lock
generated during the reviewed release flow is injected into published
archives. Build the applicable runtime separately with the
`task runtime:codex:build` or `task runtime:claude:build` command. Moving tags
and development images are not release authority. Codex and Claude runtime variants
remain local/CI-only pending their separate redistribution and image-layer
license decisions.

Tobari does not claim code signing, notarization, an SBOM attestation, or
externally verifiable build provenance. Checksums protect selected artifact
integrity but do not identify the builder. The SPDX document describes the
archive packages but does not sign them, inventory OCI layers, or prove that
another build reproduces them. The unsigned provenance statement records its
declared source, parameters, subjects, and builder-run URI; without a trusted
signature or transparency service, it remains auditable release metadata, not
independent proof of builder identity.

## Publication approval checkpoint

Artifact preparation is a local, create-only operation. Before the first
external mutation, the maintainer validates paired component candidate builds,
builds two independent archive matrices from the candidate lock, regenerates
and verifies checksum/SBOM/provenance subjects, renders and audits the stable
Formula, completes the required gates and manual reviews, and stops for
explicit approval. A local preparation command never pushes a branch or tag,
publishes an OCI image, creates a GitHub Release, or updates a Homebrew tap.

After approval, component images are published and independently inspected
first. Their real immutable indexes form one generated component lock; no
follow-up source commit is created. The exact lock is then injected into every
CLI archive before the SemVer release is assembled. The Release
workflow is manual `workflow_dispatch`, never a tag-push trigger. Its caller
must supply the exact tag and full reviewed revision. `publish: false` performs
only CI assembly; `publish: true` also requires approval through the protected
`release-publication` environment, revalidates that the existing tag points to
the requested revision, and creates a Release only when none exists. The
workflow has no overwrite path.

For a stable release, the audited checksum-pinned Formula is one Release asset.
The workflow does not change `main`, create a Formula pull request, or mutate a
tap. After the Release assets are independently verified and installed, the
maintainer updates the Homebrew tap as a separate explicit external operation.

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

`task release:check` verifies that release packaging has no fallback component
authority and that its generated lock is paired, source-bound, API-compatible,
and propagated into build identity. A repository binary remains development
only; only release packaging supplies published image authority.

Auth Broker changes additionally require the canonical source, image, static
protocol, GitHub host-driver, and topology checks used by `task check`
and `task runtime:test`. The required reproducible synthetic Auth Broker proof
is delegated explicitly to `task integration:test`;
the manual transcript does not duplicate that synthetic manipulation. Release
also requires the trusted-host GitHub scenario in
[Agent Readiness Validation](09_agent_readiness_validation.md), including the
no-print assertion that `gh auth token --hostname github.com` equals the exact
projected `GH_TOKEN` handle. The scenario records only secret-free pass/fail
outcomes and never becomes a repository fixture; tokens,
authorization/device codes, provider credential files, handles, and raw
authenticated transcripts are forbidden fixtures. An
implementation handoff may report the reviewed image evidence, but release
completion still requires that manual trusted-host scenario and every
release gate; image publication alone is insufficient.

The first public release also requires a clean-environment Colima or Linux
Quick Start run and a human review of history, dependencies, licenses, and
generated artifacts.
