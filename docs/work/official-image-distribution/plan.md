# Work Plan: Define official Tobari image distribution

- Status: Active
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Propose independent package and version lifecycles for a small GHCR image
family. Start with a minimal `tobari-runtime` foundation and derived
`tobari-toolbox`, `tobari-claude`, and `tobari-codex` packages only after each
has a reviewed source/license story. Keep immutable SemVer tags and digests as
the supported identity; do not make `latest` part of the first contract.

Keep image publication in a dedicated release workflow, separate from the
CLI's `v*` archive workflow. Pull requests build and test without push. A
protected, manually approved image release builds the exact source revision for
the supported architectures, pushes once, records the digest, verifies the
runtime contract and tools, and produces SBOM/provenance evidence. A scheduled
refresh creates update PRs; it does not publish directly.

The initial cadence proposal is: monthly foundation refreshes, immediate
security patches within a defined SLA, and derived-image releases when their
agent/tool version or foundation digest changes. The exact SLA, architecture
matrix, package names, and alias policy remain decisions for review.

## Repository structure candidate

Use `runtimes/` as the source tree for user-facing work images, with one
directory per published image:

```text
runtimes/
  base/
    Dockerfile
    entrypoint.sh
    runtime.json
    runtime.lock.json
  toolbox/
    Dockerfile
    runtime.json
    runtime.lock.json
  claude/
    Dockerfile
    runtime.json
    runtime.lock.json
  codex/
    Dockerfile
    runtime.json
    runtime.lock.json
  manifest.json
```

`runtime.json` owns the image identity, base relationship, supported
architectures, runtime API, source/license metadata, and publication metadata.
`runtime.lock.json` should own downloaded artifact versions, source URLs,
checksums/signature identifiers, and package-manager inputs. A short-term
`versions.env` compatibility layer is acceptable for the existing toolbox, but
it must not become a second source of truth. `manifest.json` records the tested
family relationship but does not replace digest pinning. JSON keeps the
metadata executable with the Go standard library and avoids adding a YAML
parser solely for release tooling.

### Provisional layer contract

The proposed public family is intentionally a single-parent chain:

```text
tobari-runtime (runtimes/base)
  -> tobari-toolbox (runtimes/toolbox)
       -> tobari-claude (runtimes/claude)
       -> tobari-codex (runtimes/codex)
```

`base` is a runtime foundation, not a general developer image. It should
contain the pinned Debian root, `bash` (plus the inherited POSIX shell and
coreutils used by the bootstrap), `ca-certificates`, `tini`, the `tobari` user
and home, the Tobari entrypoint, the runtime API/lifetime labels, and the
inherited `sleep infinity` default. The current `git`, `python3`, and `curl`
packages are convenience tools rather than lifecycle requirements; the
provisional recommendation is to move them to `toolbox`. The base must not
contain an agent CLI, cloud CLI, Kubernetes CLI, credentials, host config, or a
user-owned `CMD`/`ENTRYPOINT` replacement.

`toolbox` is the neutral work-image layer. It should contain `git`, `curl`,
`python3`, `gh`, AWS CLI, `kubectl`, TWG, `jq`, `openssh-client`, `rsync`,
`dnsutils`, `less`, and `groff`, subject to each artifact's redistribution and
integrity review. It inherits the base lifecycle contract and must not replace
the entrypoint or command. The initial toolbox should remain deliberately
small; Terraform, Helm, language toolchains, Docker CLI, and browser tooling
are candidates for later derived images rather than an unbounded toolbox.
The current integration suite assumes `python3` and `curl` are in the minimal
image, so moving them is not a silent Dockerfile edit: the runtime fixture and
its probes must be updated in the same implementation slice.

Each agent image should be `FROM toolbox` and add only its agent CLI and
agent-specific runtime dependencies. Claude and Codex images must not bake in
API keys, user settings, host CLI configuration, or an agent command as the
image `CMD`. If an agent needs Node, Python, or another runtime that is not
already a neutral toolbox dependency, that runtime belongs in the agent layer
and is recorded in that image's lock metadata.

This preserves a useful default for custom images: users who want only the
Tobari lifecycle can start from `tobari-runtime`; users who want common work
tools can start from `tobari-toolbox`; users who want an agent can start from
the corresponding agent image. A smaller agent-specific image can be added
only when image size or pull-time evidence justifies a second branch.

The base Dockerfile and bootstrap must have one canonical source. The current
embedded `internal/infra/runtimeassets/assets/tobari` is coupled to Go's
`embed.FS`; it should either become a generated, checksum-verified snapshot of
`runtimes/base`, or remain the sole base source while `runtimes/base` is only a
release wrapper. A second independently edited Dockerfile is not acceptable.

The main-branch workflow should trigger on `runtimes/**` and shared build
inputs, compute the dependency fan-out, and build/test changed images without
publishing. A base change expands to every derived image; a leaf change builds
only that leaf; a manifest, contract checker, or shared script change expands
to the full family. GHCR publication remains a separate protected release
workflow.

The parent reference used by an official build should identify the exact
published parent digest. Local and pull-request builds may override that
reference with a locally built parent tag, but the canonical release metadata
must retain the public parent package and digest. This keeps the one-level-up
`FROM` relationship visible to both humans and dependency tooling while still
allowing offline/local verification.

### First implemented slice

The first slice makes `runtimes/base` canonical, synchronizes its Dockerfile
and bootstrap into the embedded CLI snapshot, and publishes only the base
image on pushes to `main`. The workflow publishes the moving `main` channel and
an immutable `sha-<commit>` tag for Linux amd64 and arm64 with job-scoped
`packages: write`. It performs no pull-request or local-startup publication.
The GHCR package must still receive its intended public visibility through the
maintainer's package settings before users can consume it as an official public
image. Toolbox is the next implementation slice and is not part of this
workflow.

### Update strategy

Use a hybrid update path with one review and CI contract:

- Dependabot `docker` entries monitor the Dockerfile directories and update
  external `FROM` image references, especially the pinned Debian base and
  published Tobari parent images. Keep the existing Go module and GitHub
  Actions entries.
- If an agent is distributed through a standard package manager, its manifest
  and lock file lives in that agent directory so the corresponding Dependabot
  ecosystem can update it. This is the preferred path for Claude/Codex when
  their distribution format permits it.
- Downloaded release assets such as the current GitHub CLI, AWS CLI, kubectl,
  and TWG flow through a Tobari runtime refresh script. The script checks the
  upstream release, updates the version and checksum/signature data together,
  and opens a normal dependency PR. This is not a second review path: the
  resulting PR runs the same image build, integrity, license, and runtime
  contract checks.
- A scheduled refresh may propose updates weekly; security fixes can bypass
  the normal cadence. Neither Dependabot nor the refresh job publishes GHCR
  images directly. Main-branch merges verify the dependency fan-out, while a
  protected image release publishes the reviewed revision.

The distinction matters because the current `versions.env` values and custom
download/checksum shell are not a standard package manifest. Treating them as
if the Docker Dependabot manager owned them would leave version and integrity
updates out of sync. The lock schema and updater should make that mismatch
impossible before publication.

## Alternatives considered

### Alternative A: Publish all images from the CLI `v*` release workflow

This keeps one visible release event but couples the CLI, OS foundation, and
third-party agent updates. It also gives a package-publishing job a wider
blast radius than the current archive workflow. Reject for the initial family.

### Alternative B: One monolithic image containing every agent and tool

This minimizes package management but increases image size, license scope,
pull cost, update coupling, and user choice friction. It also makes a single
agent update require a full-family rebuild. Reject.

### Alternative C: Independent package releases with a small family manifest

This lets the runtime foundation and each tool image update on its own cadence
while a manifest records a tested set of package digests. Choose this approach;
the manifest is a compatibility convenience, not a replacement for digest
pinning.

## Design

### Public contract

Proposed package names are `ghcr.io/tasuku43/tobari-runtime`,
`ghcr.io/tasuku43/tobari-toolbox`, `ghcr.io/tasuku43/tobari-claude`, and
`ghcr.io/tasuku43/tobari-codex`, subject to naming review. Each package is a
compatible environment image, not a separate Tobari authority boundary. Users
may select a version or digest locally; Tobari still performs the same local
runtime validation and never pulls implicitly.

Each published image should expose OCI metadata for package version, source
revision, license, base image name/digest, runtime API, lifetime capability,
tool versions, and supported architectures. The image's `CMD` and entrypoint
remain inherited from the foundation unless a future ADR explicitly changes
the runtime contract.

### Layer changes

- Domain: no new CLI task; preserve the current runtime API and compatibility
  labels. Add image-manifest vocabulary only if a consumer needs it.
- Application: no new use case for publication; publication is maintainer-side
  release automation.
- Infrastructure: add per-image Docker build contexts, lock/metadata files,
  contract inspectors, multi-architecture build and verification scripts.
- CLI and catalog: no command change in the first publication slice.
- Release/public boundary: add a dedicated image workflow, package permissions,
  provenance/SBOM gates, public docs, and release checks.

### Data and control flow

```text
reviewed Dockerfile + pinned base/tool metadata
  -> PR build (no push)
  -> runtime contract/tool/license tests
  -> scheduled update PR or protected release request
  -> exact source revision + multi-arch build
  -> GHCR manifest/digest
  -> SBOM/provenance attestation
  -> immutable release record + family manifest
```

### Error and cancellation behavior

Pull-request and scheduled workflows must fail before publication on a missing
digest, unsupported architecture, runtime-contract drift, tool verification
failure, license/provenance gap, or package permission mismatch. A release job
must refuse to overwrite an existing version tag or publish a different digest
under an existing release identity. A cancelled pre-push build has no public
side effect; cancellation after a successful push is reconciled by digest and
does not permit a retry to replace the artifact.

### Security and public boundary

Use a separate publish job with only `contents: read`, `packages: write`, and
the explicitly required attestation permissions. Never expose package-write or
signing permissions to pull-request code. Build contexts must contain no
credentials, and downloaded installers must be checksum/signature verified.
Public image layers and SBOMs are reviewed for secrets, private URLs, license
obligations, and accidental host configuration. Official status describes
source and review, not additional runtime authority.

## Implementation slices

1. Decide package names, version/alias policy, support window, architectures,
   update SLA, and vendor redistribution eligibility.
2. Add image metadata/lock schema and contract validation without publishing.
3. Add PR-only build/test and scheduled update workflows.
4. Add protected multi-architecture publication with digest, SBOM, provenance,
   and immutable-release checks.
5. Add release/publication documentation, manifests, and clean-environment
   consumer verification.

## Verification

- Unit and contract tests: metadata/lock schema, package matrix, runtime API,
  architecture, label, user, entrypoint, and tool checks.
- Negative side-effect tests: PR and untrusted paths cannot push; release
  refuses duplicate tag/digest mismatch and missing provenance.
- Opaque-reference and complete-pagination tests: not applicable; no CLI task.
- Structured output, hostile-output, and recovery tests: bounded build logs,
  secret scans, failed-build cleanup, and publish retry reconciliation.
- Agent-readiness scenario and discovery-round-trip count: unchanged; no public
  CLI capability.
- Human-handoff scorecard for setup/authentication candidates: not applicable;
  maintainer release UX still requires a protected approval record.
- Manual observation: pull each published digest on amd64 and arm64, inspect
  OCI labels/config, run a terminating-`CMD` compatibility fixture, and verify
  the attestation/SBOM against the exact digest.
- Required profiles: `task check`, `task security`, `task public:check`,
  `task release:check`, and the dedicated image gate.
- Generated-diff or artifact checks: digest manifest, SBOM, provenance, and
  package metadata are tied to the exact source revision.

## Rollout and rollback

Publish the foundation first, then derived images that reference its immutable
digest. Keep the local embedded runtime and `tobari-runtime:local` workflow
unchanged. A bad image is retired by marking its version affected and
publishing a new version; pinned consumers are not silently moved. If a moving
alias is eventually added, it points only to an already published digest and
is excluded from the immutable identity.

## Documentation promotion

- Update `docs/05_public_repository.md` with image layer/license/history review.
- Update `docs/06_release.md` with package names, versioning, cadence, gates,
  attestations, retention, and rollback.
- Update `docs/02_architecture.md` and `docs/03_security_model.md` with the
  package/publication boundary and no-added-authority rule.
- Add a release ADR if independent image trains, aliases, or vendor
  redistribution become durable trade-offs.
