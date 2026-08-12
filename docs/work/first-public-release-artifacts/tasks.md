# Work Tasks: Prepare first public V1 release artifacts

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)
- Context: [context.md](context.md)

## Understand and decide

- [x] Inventory release, image, formula, gate, and generated-site machinery.
      Evidence: read-only audit on 2026-08-12 recorded in `context.md`.
- [ ] Revise the release contract for checksums, SBOMs, and provenance before
      implementing those claims.
- [ ] Define the explicit publication approval checkpoint and exact operator
      handoff.

## Prepare locally

- [ ] Finalize canonical/embedded Gateway and Auth Broker source snapshots only
      after auth/policy integration.
- [ ] Add deterministic component image metadata, SBOM, and provenance
      generation with reviewed permissions/dependencies.
- [ ] Add CLI archive SBOM/provenance generation and subject checks.
- [ ] Make release/formula workflows consume exact verified subjects and avoid
      implicit publication during preparation.
- [ ] Regenerate architecture site, catalog, schema/capability ledgers, and
      component metadata from the final integrated source.
- [ ] Run a synthetic no-network/no-publish artifact and formula dry run.

## Verify and stop

- [ ] Run focused release lint, packaging, archive, SBOM/provenance, formula,
      action, snapshot, and generated-data checks.
- [ ] Review workflow permissions, dependency, generated, and artifact diffs.
- [ ] Run `task check` and `task security` locally.
- [ ] Record the intentional pre-publication result of `task public:check` and
      `task release:check` if unpublished image pins remain.
- [ ] Commit preparation only; perform no push, tag, OCI publication, GitHub
      Release, or Homebrew tap mutation.
- [ ] Present exact publication commands and wait for explicit maintainer
      approval before the first external operation.
