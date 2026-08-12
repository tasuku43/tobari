# Work Goal: Prepare first public V1 release artifacts

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: Public repository and release contracts
- Review/delete trigger: Delete after the first public release is verified and the parent closes
- Successor: None
- Owner: Tobari maintainers
- Target: Publication approval checkpoint
- Related ADRs: ADR 0027 and the final V1 scope decisions
- Parent: [First public release core](../first-public-release-core/goal.md)
- Prerequisites: source access, policy compaction retirement, policy presets, authentication narrowing, and integrated security verification

## Outcome

The integrated V1 source has deterministic, locally verified automation for
canonical component images, CLI archives, checksums, SBOMs, provenance,
GitHub Release assets, and the Homebrew formula. Work stops before any external
push, tag, OCI publication, GitHub Release, or tap update and asks the
maintainer for synchronous publication approval.

## Non-goals

- Publishing from a partially integrated source tree.
- Claiming a candidate image digest is published or anonymously retrievable.
- Code signing/notarization or reproducible-build claims unless separately
  decided and proven.
- Adding release-unrelated runtimes, providers, VM layers, or capabilities.

## Acceptance criteria

- [x] Gateway/Auth Broker canonical source and embedded snapshots are final and
      byte-equal after integrated auth/policy changes. Evidence: canonical
      source checks pass after the complete Docker policy/auth journey and the
      GraphQL fail-closed correction.
- [x] Workflows build Linux amd64/arm64 component indexes and supported CLI
      archives, emit checksums, SPDX/CycloneDX SBOMs, and CI provenance with
      pinned permissions and reviewed dependencies. Evidence: workflow and
      release lint verify the exact subjects, pinned Actions, permissions, and
      protected publication environment without adding a dependency.
- [x] Synthetic local dry runs validate archive, checksum, SBOM, provenance,
      and formula generation without mutating external state. Evidence:
      `scripts/lint-release.sh` creates and independently verifies the complete
      synthetic artifact inventory with networking disabled.
- [x] Publication is split into explicit, auditable steps whose first external
      mutation requires maintainer approval. Evidence:
      `publication-handoff.md` names branch push as the first mutation and a
      second synchronous approval before tag and Release publication.
- [x] `task check`, `task security`, `task public:check`, and
      `task release:check` validate paired generated component authority
      without a digest-pin commit; before approval candidate digests are not
      described as published authority.

## Completion definition

This preparation packet reaches the approval checkpoint with local commits and
synthetic evidence. The parent release remains Active until approved external
publication, generated digest locking, final gates, release creation, and install
verification complete synchronously with the maintainer.
