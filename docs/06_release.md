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
uses the embedded resolver channel and standard capability profile, names
source-required and selected Gateway APIs, and leaves contributor command fields empty. A
repository build retains version `dev`, embeds only the exact HEAD commit, and
selects content-addressed local component images. `task build` compiles the
standard profile; `task build:dev` compiles the experimental profile as
`bin/tobari-dev`. Neither repository binary is a release candidate.

One complete five-archive CLI matrix is accompanied by three
repository-generated metadata files. `checksums.txt` binds the five sorted
archives to SHA-256 digests. `sbom.spdx.json` is an SPDX 2.3 package inventory
of those same release subjects, their release URLs, versions, declared project
license, and SHA-256 digests; `filesAnalyzed: false` states its deliberate archive-level
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
before Docker builds. The standard embedded Tobari, Gateway, OPA policy, and compose
inputs are therefore bound to the CLI source revision. Container base images
are pinned to reviewed immutable versions or digests.

The canonical base image definition is maintained under `runtimes/base` and
its Dockerfile/bootstrap snapshot is checked against the embedded runtime
assets. The source includes GitHub CLI, AWS CLI, Claude Code 2.1.220, and Codex
0.147.0 and preserves checked-in per-platform integrity metadata for each.
`kubectl`, `cwk`, `pup`, and TWG are not part of the canonical base artifact
inventory. A custom Context image does not change base metadata or publication
authority. The base workflow always validates Linux amd64 and arm64 with
cache-only output. The released CLI builds this base on the user's Docker host
from its embedded pinned recipe; the protected Release workflow has no runtime
registry path.
The canonical Gateway image definition is maintained under `gateway` and its
Dockerfile, addon, entrypoint, and tests are checked against the embedded
`internal/infra/runtimeassets/assets/gateway` snapshot. The released CLI builds
the pinned snapshot on the user's Docker host under a source-derived local tag
when it is absent, then applies the ordinary compatibility preflight.
Contributor source development uses its own source-hash tag built by
`task build`.

The experimental Auth Broker image definition is maintained under `authbroker/`.
Its package, Dockerfile, entrypoints, bridge/protocol, tests, and provider-CLI
absence are byte-checked against
`internal/infra/runtimeassets/assets/authbroker/`. `task build:dev` builds it
locally together with the experimental Gateway layer. Release assembly never
publishes it, and standard `cluster up` has no Broker startup path.

## Pre-public V1 contract

All Tobari-owned command outputs, persisted state, configuration, OPA input and
decisions, audits, and Gateway component API use V1. Experimental Broker state
and protocols also use V1. Readers accept exactly V1 and reject every other
version. Before the first public release, development snapshots receive no
deprecation window, compatibility reader, retired command alias, or implicit
old-state interpretation. The explicit `migrate apply` command is the sole
exception: it accepts only ADR 0070's enumerated unpublished
Context-policy/Runtime predecessor and emits current V1 state. Every other
development snapshot must be removed and recreated when the contract changes.

The standard V1 boundaries include command paths, exit meanings, Docker labels,
configuration keys, and preservation of each Workspace home by default. Broker
root-key identifiers, handles, sockets, and vault preservation belong only to
the experimental profile.

Every public JSON envelope uses schema 1 and exact catalog-owned recursive
fields. Explicit `null`, empty collections, zero, false, and finite unavailable
states retain their declared meaning; presentation must not replace them with
sentinels or infer missing authority.

## Publication

Tags use SemVer `vMAJOR.MINOR.PATCH` for stable releases and SemVer prerelease
identifiers such as `v0.1.0-dev.1` for development releases. Tobari publishes
no OCI images. Main, pull-request, and standalone image workflows are
validation-only, use cache-only output, and receive no package-write
permission. The manual release workflow packages only CLI archives and their
repository-generated metadata. No login, credential,
account fixture, device or authorization code, token, handle, root key, vault,
or authenticated output is a release artifact.
Each CLI archive includes root `THIRD_PARTY_NOTICES` so the pinned
`github.com/creack/pty` v1.1.24 MIT notice ships with the binary that uses it.

The combined agent-ready base and the retained focused Claude/Codex child
fixtures have validation-only workflows. They validate release checksums,
multi-architecture construction, and the runtime contract without publishing.
The combined OCI/runtime metadata uses `NOASSERTION` while both locks record
`license_review: pending`; it must not imply that either bundled agent is
MIT-licensed. Tobari permanently omits public base publication and instead
ships its pinned recipes and integrity checks inside the CLI. Gateway source
records contain no release-output digest. Contributor development builds
`tobari-runtime:dev`; focused child commands remain integrity checks only.

Standard native Anthropic login is executed and stored by Claude Code inside
the Workspace; Tobari ships no Anthropic acquisition or refresh adapter. The
experimental Broker integration is not a release artifact.

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
external mutation, the maintainer validates the Gateway and local base
construction, builds two independent archive matrices, regenerates
and verifies checksum/SBOM/provenance subjects, renders and audits the stable
Formula, completes the required gates and manual reviews, and stops for
explicit approval. A local preparation command never pushes a branch or tag,
creates a GitHub Release, or updates a Homebrew tap.

After approval, the Release workflow assembles the CLI matrix directly from
the reviewed source revision. It is manual `workflow_dispatch`, never a
tag-push trigger. Its caller
must supply the exact tag and full reviewed revision. `publish: false` performs
only CI assembly; `publish: true` also requires approval through the protected
`release-publication` environment, revalidates that the existing tag points to
the requested revision, and creates a Release only when none exists. The
workflow has no overwrite path. Non-stable SemVer tags create a GitHub
prerelease; they do not render a Formula, obtain the Homebrew App token, or
mutate the tap.

For a stable release, the audited checksum-pinned Formula is one Release asset.
After the immutable GitHub Release succeeds, the same protected workflow
downloads that exact published `tobari.rb`, obtains a GitHub-App token scoped
only to `tasuku43/homebrew-tap`, and opens a Formula-only pull request that
updates `Formula/tobari.rb`. It never pushes tap `main` directly. The tap's own
checks and trusted-bot policy decide merge. A dry run or prerelease has no tap
write path. Stable publication is complete only when that tap pull request has
been created successfully; installation availability follows its merge.

The `release-publication` environment supplies `HOMEBREW_APP_ID` and
`HOMEBREW_APP_KEY`. That GitHub App must be installed only where needed and
have repository contents and pull-request write permission for
`tasuku43/homebrew-tap`; the generated installation token names only that
repository. The release repository's `GITHUB_TOKEN` is never used to push the
tap. The shared tap's trusted-bot Formula-only checks remain an independent
merge boundary.

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

`task release:check` verifies that release packaging has no component lock,
Tobari GHCR reference, package-write permission, registry login, image push,
or link-injected image authority. A repository binary remains development
only; release archives use the embedded resolver and source-selected APIs.

Auth Broker changes additionally require the canonical source, image, static
protocol, GitHub host-driver, and topology checks used by `task check`
and `task runtime:test`. The required reproducible synthetic Auth Broker proof
is delegated explicitly to `task integration:test`. The experimental `auth`
namespace and Broker runtime are absent from standard release archives, so a
live Broker-backed provider login is not a standard publication prerequisite.
Maintainers may record a secret-free pass/fail compatibility observation for
the experimental profile, but it grants no release evidence and never becomes
a repository fixture.

The first public release also requires a clean-environment Colima or Linux
Quick Start run and a human review of history, dependencies, licenses, and
generated artifacts. That review confirms the standard archives contain no
experimental provider acquisition implementation, Broker runtime, provider
credential state, or bundled Claude/Codex binary, and separately reviews the
native integration recipes and browser/callback behavior against applicable
provider terms. That terms review does not require a live account login.
