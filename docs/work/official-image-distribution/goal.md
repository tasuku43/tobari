# Work Goal: Define official Tobari image distribution

- Status: Accepted
- Retention: evidence
- Retention reason: Preserve the accepted base/build-only image boundary and the explicit release-policy deferral for public agent variants until maintainer/vendor review is available.
- Governing contract: docs/00_theses.md, docs/02_architecture.md, docs/03_security_model.md, docs/04_harness.md, docs/05_public_repository.md, docs/06_release.md
- Review/delete trigger: Delete after the release/public-boundary decision is kept current in `docs/06_release.md` and a new reviewed packet accepts public agent publication or changes the boundary.
- Successor: `docs/06_release.md` publication gate; any public agent release must open a new reviewed packet.
- Owner: Tobari maintainers
- Target: Official base runtime and build-only derived agent runtime images
- Execution state: Base publication is accepted; Claude/Codex public publication,
  stable support windows, and attestation/license claims are explicitly deferred
  until their release policy is approved.
- Related ADRs: docs/decisions/0012-own-workspace-container-lifetime.md

## Outcome

Tobari has a reviewed, reproducible publication model for a common base runtime
and a small family of derived agent runtime images. The family uses one GHCR
package with explicit base/agent composition tags, base and tool provenance, a
compatibility contract, an immutable release identity, a documented
support/update cadence, and a least-privilege publication workflow. The plan keeps image releases separate
from the CLI release train; the base development channel is published only
after its source, license, security, and public-boundary checks, while derived
images remain gated on their own review. The base development channel is
publishable; Claude/Codex variants are currently build-only and are not public
release claims.

## Why now

The compatible-base model reduces custom-image friction, but publishing
Claude/Codex images creates a new public artifact and supply-chain boundary.
The repository initially built the runtime locally and published only CLI
archives from `v*` tags; the first base slice now establishes the separate
GHCR lifecycle before agent variants are added.

## Non-goals

- Publishing derived agent/tool images or stable image releases before their
  distribution rights and release policy are reviewed
- Choosing or redistributing third-party agent binaries without license and
  vendor-policy review
- Changing the Tobari runtime API or Workspace lifetime contract
- Making the CLI's `v*` release workflow push images implicitly
- Treating the moving `latest`/`main` development aliases as stable SemVer
  release identities

## Acceptance criteria

- [x] The single `tobari/runtime` package and each base/agent variant are
      explicit in source metadata and publication rules. Base publication is
      active; agent variants are build-only.
- [x] Versioning, digest identity, aliases, deprecation, retention, and
      rollback rules are explicit and do not permit silent replacement of a
      published release. Stable agent deprecation is deliberately not claimed.
- [x] Source-of-truth files record base-image digests, agent/tool versions,
      licenses/checksum inputs, and the derived-image relationship.
- [x] PR validation, main publication, release approval, multi-architecture
      build, compatibility tests, and GHCR permissions are separated into
      bounded workflows. Scheduled refresh and SBOM/provenance are explicitly
      deferred rather than implied.
- [x] Release cadence and security-update targets are explicit as a current
      boundary: no stable support/SLA is promised for the build-only derived
      images until a release-policy packet is approved.
- [x] The user-facing contract says official images are convenience bases and
      remain subject to local Tobari runtime validation.
- [x] `task check`, `task security`, `task public:check`, and the dedicated
      release/image gates are required before the implemented publication edge;
      deferred agent publication has no public push claim.

## Governing documents

- Thesis: docs/00_theses.md, especially Theses 2, 4, and 5
- Product/architecture: docs/01_product_contract.md and docs/02_architecture.md
- Security: docs/03_security_model.md and docs/THREAT_MODEL.md
- Harness/public/release: docs/04_harness.md, docs/05_public_repository.md, and docs/06_release.md
- Existing decision: docs/decisions/0012-own-workspace-container-lifetime.md

## Completion definition

The accepted design boundary is complete when the base package model, source of
truth, security boundary, workflow permissions, verification gates, and
rollback policy are promoted into durable release/public-boundary documentation
or an ADR. Public derived-agent publication is intentionally deferred; its
exact successor is the `docs/06_release.md` publication gate plus a new reviewed
packet that must resolve redistribution rights, support/SLA, licensing, and
attestation claims before any agent tag is pushed.
