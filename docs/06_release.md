# Release Model

Tobari follows Semantic Versioning and initially supports source builds on
Linux and macOS with Docker Engine. Colima is the documented macOS runtime;
Docker Desktop-specific behavior is not required.

## Artifacts

The Go CLI remains a pure-Go binary for Linux and macOS on amd64 and arm64.
Windows CLI archives are buildable by the inherited packaging harness but the
MVP runtime is not supported on Windows because bind mounts, Unix ownership,
TTY behavior, and container networking have not been validated there.
The resident credential companion is a private same-binary process identity,
not a second executable or public Catalog command, so archives add no sibling
binary. Runtime epoch/session keys, provider homes, and credential state are
never packaged.

Every CLI artifact exposes build identity through `version` text and schema-1
JSON. A release archive embeds the validated SemVer and full source commit,
uses the published resolver channel, names source-required and selected
Gateway/Auth Broker APIs, and leaves contributor command fields empty. A
repository build retains version `dev` and embeds only the exact HEAD commit;
the `tobari_dev` build is a distinct resolver artifact named
`bin/tobari-dev`, not a release candidate.

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
`internal/infra/runtimeassets/assets/gateway` snapshot. The main-only Gateway
workflow publishes `ghcr.io/<owner>/tobari/gateway:latest`, `:main`, and an
immutable `:sha-<commit>` tag for Linux amd64 and arm64. The CLI records one
reviewed multi-architecture manifest digest in `versions.env` and uses that
digest for routine startup. Contributor source-development uses `task
build:dev` and a `tobari_dev` binary; the public `cluster up` command does not
build Gateway source.

The canonical Auth Broker image definition is maintained under `authbroker/`.
Its package, Dockerfile, entrypoints, bridge/protocol, tests, and provider-CLI
absence are byte-checked against
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
The current source contract uses Gateway image API label 4 and Auth Broker
image API label 3, Gateway OPA input
schema 5, cluster status JSON schema 6 (including nullable unconfigured
resources and always-present
`credential_companion_state`), Context report JSON schema 7, and auth command
JSON schema 2. Broker protocols and private companion epoch/frames remain
schema 1. Owner static provider
manifests remain schema 1; reviewed built-ins and the normalized projection
support schema 2. The encrypted vault keeps its schema-1 envelope and migrates
valid static payloads into encrypted payload schema 2. Release notes must
identify those changes and the source-schema-4 plus legacy-source-3-to-runtime-5
projection bridge.
Public auth backend values are exactly `macos_keychain|xdg_file`, while cluster
status may additionally use `unavailable`. The `linux_xdg_file` string is an
infrastructure/doctor diagnostic label, not a public JSON enum. The release
review uses the complete canonical schema/path/backend table in
[Authentication handling](07_authentication.md#canonical-schemas-paths-and-backend-identifiers).

The complete public CLI compatibility set is structured error 1, agent help 9,
version 1, doctor 1, Context list 3, Context report 7, cluster status 5,
cluster denials 4, policy candidates/review 5, policy rules 3, policy
compactions 3, auth result/status 2, Workspace list 2, and Workspace status 3.
The envelope names and exact recursive fields remain catalog-owned.

This pre-v1 contract-closure release corrects earlier documentation that named
cluster denials 3, policy candidates/review 4, policy rules 2, or Workspace
status 2 even though the executable contracts had already advanced. Context
report advances from 6 to 7 and auth result/status advances from 1 to 2 because
absent credential revisions and account labels now serialize as JSON `null`
instead of changing their type to the empty-string sentinel. Agent help advances
from 8 to 9 because scoped help now publishes recursive output facts and exact
machine invocations. Existing pre-v1 consumers must select by exact envelope and
schema, treat newly explicit `null` and finite unavailable states according to
the published field contract. Cluster status advances from 4 to 5 because
unconfigured resources now serialize as JSON `null`. Consumers must not depend
on empty-string sentinels or the internal interactive completion path.

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
building it verifies canonical/snapshot equality, image metadata,
provider-CLI absence, and the private bridge/protocol suite. Pull requests run
the Python tests and cache-only
multi-architecture build without package-write permission. The main-push job
alone publishes the GHCR manifest. No login, credential, account fixture,
device or authorization code, SSO client/token state, Codex `auth.json`, Claude
setup token, OpenAI ID/access/refresh token, role credential, signed
authorization field, handle, root key, vault, or authenticated output is a
release artifact.

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
support window, SBOM/attestation, or redistribution approval until a new
release decision accepts those claims. The Gateway and Auth Broker source/image
checks are implemented. The reviewed Gateway API-3 index was built from source
revision `328196221c5be2861b67ec51339d0184b04c6b31`; the compatible Auth Broker
API-2 index was built from source revision
`a3fedb66ad5a72c19d6721f3f8da49852882ced8`. Anonymous access, Linux
amd64/arm64 members, API/role metadata, non-root `1000:1000` users,
entrypoints, source, revision, and license metadata were independently
inspected. `versions.env` records Gateway
`sha256:44a84576266617c78eae433ea53d60e199226dc7bc275b2aaa6c728875c91878`
and the historical Auth Broker
`sha256:a2df8169fd1b28ab67d42c83c5181714ce5373ab74fe9931e84ab4542dc97fb1`.
A moving tag or successful workflow does not make a digest reviewed runtime
authority by itself.

Those reviewed indexes and pins remain historical API-3/API-2 publication
facts; they predate and are incompatible with the current Gateway API-5/Auth
Broker API-3 source contract. Standard startup from this source must reject the
old pins. `versions.env` records `GATEWAY_IMAGE_API=3` and
`AUTH_BROKER_IMAGE_API=2` beside those digests; `task release:check` derives
the canonical Dockerfile APIs and fails until both API authorities and digests
advance together. The closed OpenAI Codex and Anthropic Claude plans are usable only
through the explicit `task build:dev` development-image path until maintainers
publish and independently review new immutable Linux amd64/arm64 indexes and
advance `versions.env`. Build the applicable runtime separately with the
`task runtime:codex:build` or `task runtime:claude:build` command. Moving tags
and development images are not release authority. Codex and Claude runtime variants
remain local/CI-only pending their separate redistribution and image-layer
license decisions.

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

`task release:check` also requires the release artifact build identity to be
complete and compatible. The current API-3/API-2 immutable pins make that gate
fail intentionally against API-5/API-3 source; version diagnostics explain the
same state but cannot override it.

Auth Broker and companion changes additionally require the canonical source,
image, private protocol, host-driver, and topology checks used by `task check`
and `task runtime:test`. The required reproducible synthetic Auth Broker proof
is delegated explicitly to `task integration:test`;
the manual transcript does not duplicate that synthetic manipulation. Release
also requires the trusted-host GitHub, OpenAI Codex, Anthropic Claude, Datadog,
and both AWS login-method scenarios in
[Agent Readiness Validation](09_agent_readiness_validation.md), including the
no-print assertion that `gh auth token --hostname github.com` equals the exact
projected `GH_TOKEN` handle, that console mode rejects AWS CLI older than 2.32
before provider login, and that the three AWS credential variables equal
one handle before the allowed API calls. OpenAI and Anthropic validation
requires exact trusted-host Codex 0.146.0 and Claude Code 2.1.220 executables,
respectively, an interactive terminal, and deliberate completion of each
provider's browser flow. Those scenarios record only secret-free pass/fail
outcomes and never become repository fixtures; OAuth tokens, setup tokens,
authorization/device codes, provider credential files, handles, and raw
authenticated transcripts are forbidden fixtures. An
implementation handoff may report the reviewed image evidence, but release
completion still requires those manual trusted-host scenarios and every
release gate; image publication alone is insufficient.

The first public release also requires a clean-environment Colima or Linux
Quick Start run and a human review of history, dependencies, licenses, and
generated artifacts.
