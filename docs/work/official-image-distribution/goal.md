# Work Goal: Define official Tobari image distribution

- Status: Active
- Retention: temporary
- Retention reason: Design and implementation evidence for the staged official image family, package ownership, and release lifecycle.
- Governing contract: docs/00_theses.md, docs/02_architecture.md, docs/03_security_model.md, docs/04_harness.md, docs/05_public_repository.md, docs/06_release.md
- Review/delete trigger: Delete after the release/public-boundary decision is promoted and implementation completes or is explicitly deferred.
- Successor: None
- Owner: Tobari maintainers
- Target: Official base runtime and derived agent runtime images
- Related ADRs: docs/decisions/0012-own-workspace-container-lifetime.md

## Outcome

Tobari has a reviewed, reproducible publication model for a common base runtime
and a small family of derived agent runtime images. The family uses one GHCR
package with explicit base/agent composition tags, base and tool provenance, a
compatibility contract, an immutable release identity, a documented
support/update cadence, and a least-privilege publication workflow. The plan keeps image releases separate
from the CLI release train; the base development channel is published only
after its source, license, security, and public-boundary checks, while derived
images remain gated on their own review.

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
- Adding mutable `latest` behavior before the immutable release path is proven

## Acceptance criteria

- [ ] The single `tobari/runtime` package and each base/agent variant are
      explicit in source metadata and publication rules.
- [ ] Versioning, digest identity, aliases, deprecation, retention, and
      rollback rules are explicit and do not permit silent replacement of a
      published release.
- [ ] Source-of-truth files record base-image digests, agent/tool versions,
      licenses, and the derived-image relationship.
- [ ] PR validation, scheduled refresh, release approval, multi-architecture
      build, compatibility tests, SBOM/provenance, and GHCR permissions are
      separated into bounded workflows.
- [ ] Release cadence and security-update targets are explicit for the runtime
      foundation and derived images.
- [ ] The user-facing contract says official images are convenience bases and
      remain subject to local Tobari runtime validation.
- [ ] `task check`, `task security`, `task public:check`, and the dedicated
      release/image gate are required before publication.

## Governing documents

- Thesis: docs/00_theses.md, especially Theses 2, 4, and 5
- Product/architecture: docs/01_product_contract.md and docs/02_architecture.md
- Security: docs/03_security_model.md and docs/THREAT_MODEL.md
- Harness/public/release: docs/04_harness.md, docs/05_public_repository.md, and docs/06_release.md
- Existing decision: docs/decisions/0012-own-workspace-container-lifetime.md

## Completion definition

The design is complete when the package model, source of truth, release
cadence, security boundary, workflow permissions, verification gates, and
rollback policy are accepted and promoted into durable release/public-boundary
documentation or an ADR. If implementation is deferred, the packet records
the exact successor and the reason.
