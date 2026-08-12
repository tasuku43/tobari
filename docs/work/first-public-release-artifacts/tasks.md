# Work Tasks: Prepare first public V1 release artifacts

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)
- Context: [context.md](context.md)

## Understand and decide

- [x] Inventory release, image, formula, gate, and generated-site machinery.
      Evidence: read-only audit on 2026-08-12 recorded in `context.md`.
- [x] Revise the release contract for checksums, SBOMs, and provenance before
      implementing those claims. Evidence: `docs/06_release.md` now defines
      exact archive-subject SPDX coverage and unsigned provenance limits.
- [x] Define the explicit publication approval checkpoint and exact operator
      handoff. Evidence: local create-only preparation precedes approval;
      manual workflow dispatch, protected publication environment, image
      publication/pinning, tag, Release, and tap update remain explicit steps.

## Prepare locally

- [ ] Finalize canonical/embedded Gateway and Auth Broker source snapshots only
      after auth/policy integration.
- [ ] Add deterministic component image metadata, SBOM, and provenance
      generation with reviewed permissions/dependencies.
- [x] Add CLI archive SBOM/provenance generation and subject checks. Evidence:
      standard-library `tools/releaseartifacts` creates and verifies exact
      SHA-256, SPDX 2.3, and unsigned in-toto/SLSA metadata with separate
      builder/invocation identities and hostile inventory tests.
- [x] Make release/formula workflows consume exact verified subjects and avoid
      implicit publication during preparation. Evidence: manual dispatch,
      exact pre-upload and pre-publish inventories, protected environment,
      no tag trigger, and no Formula branch/PR/tap mutation.
- [ ] Regenerate architecture site, catalog, schema/capability ledgers, and
      component metadata from the final integrated source.
- [x] Run a synthetic no-network/no-publish artifact and formula dry run.
      Evidence: `./scripts/lint-release.sh` passed with offline metadata
      generation, two archive/metadata matrices, Formula syntax/audit, and no
      external mutation.

## Verify and stop

- [x] Run focused release lint, packaging, archive, SBOM/provenance, formula,
      and action checks. Evidence: `./scripts/lint-release.sh`, focused Go
      tests, and pinned actionlint v1.7.7 passed; snapshot/generated checks
      remain correctly deferred to final integrated source.
- [x] Review workflow permissions, dependency, generated, and artifact diffs.
      Evidence: no module or Action was added, privileged publication remains
      one environment-gated `contents: write` job, all other permissions are
      read-only, and exact final asset inventory rejects extras and symlinks.
- [ ] Run `task check` and `task security` locally.
- [ ] Record the intentional pre-publication result of `task public:check` and
      `task release:check` if unpublished image pins remain.
- [x] Commit preparation only; perform no push, tag, OCI publication, GitHub
      Release, or Homebrew tap mutation. Evidence: the packet-scoped local diff
      contains only release tooling, workflow, contract, tests, and evidence;
      no external operation was invoked.
- [ ] Present exact publication commands and wait for explicit maintainer
      approval before the first external operation.
