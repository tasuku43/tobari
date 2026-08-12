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
      manual workflow dispatch, protected publication environment, paired image
      publication, tag, Release, and stable tap pull request remain explicit
      ordered steps.

## Prepare locally

- [x] Finalize canonical/embedded Gateway and Auth Broker source snapshots only
      after auth/policy integration. Evidence: dynamic auth and broad policy
      retirement integrated first; `check-gateway-source.sh` and
      `check-authbroker-source.sh` pass byte equality for the final static/
      exact runtime sources.
- [x] Add deterministic component image metadata, SBOM, and provenance
      generation with reviewed permissions/dependencies. Evidence: Gateway and
      Auth Broker publication are manual exact-revision workflows behind the
      protected `release-publication` environment; BuildKit attaches SBOM and
      max-mode provenance to both-platform OCI indexes, the workflows inspect
      the immutable digest and upload validated metadata/SBOM/provenance as one
      component evidence artifact, and no Action/dependency was added.
- [x] Add CLI archive SBOM/provenance generation and subject checks. Evidence:
      standard-library `tools/releaseartifacts` creates and verifies exact
      SHA-256, SPDX 2.3, and unsigned in-toto/SLSA metadata with separate
      builder/invocation identities and hostile inventory tests.
- [x] Make release/formula workflows consume exact verified subjects and avoid
      implicit publication during preparation. Evidence: manual dispatch,
      exact pre-upload and pre-publish inventories, protected environment, no
      tag trigger, and a stable-only post-Release Formula pull request to the
      fixed reviewed tap.
- [x] Regenerate architecture site, catalog, schema/capability ledgers, and
      component metadata from the final integrated source. Evidence:
      `source-snapshot.txt` pins committed integrated source
      `ab158bde6f0d6ba9e5b3c99aebd5e4ed07b510c6`; sitegen regenerated the
      34-command catalog, exact denied-scheme fields, retained brokered-auth
      wording, and component/schema data. Every EN/JA source-evidence link was
      advanced to that same commit; `generate:check` and `check:source` pass.
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
- [x] Run `task check` and `task security` locally. Evidence: `mise exec -- task
      check` and `mise exec -- task security` pass on integrated V1 HEAD on
      2026-08-12; full includes race tests, both site builds, and Playwright
      40/40. After Colima recovered, `mise exec -- task integration:test`
      completed the explicit V1 journey, including custom preset snapshot
      immutability, exact review, terminal guardrails, static Broker rotation,
      logout, and re-entry.
- [x] Record the pre-publication result of `task public:check` and
      `task release:check`. The revised gates validate paired generated locks
      and reject any committed owned-image output; no gate is weakened.
- [x] Commit preparation only; perform no push, tag, OCI publication, GitHub
      Release, or Homebrew tap mutation. Evidence: the packet-scoped local diff
      contains only release tooling, workflow, contract, tests, and evidence;
      no external operation was invoked.
- [x] Present exact publication commands and wait for explicit maintainer
      approval before the first external operation. Evidence:
      `publication-handoff.md` fixes the source-image-lock-gate-tag-Release-tap
      order, fixes the tap authority and GitHub App scope, and names the
      branch push as the first external mutation. No handoff command was run.
