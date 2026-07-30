# Work Goal: Remove inherited foundry scaffolding

- Status: Draft
- Retention: temporary
- Retention reason: None
- Governing contract: Project theses, product contract, architecture, and harness
- Review/delete trigger: Delete after the cleanup is verified and durable contracts describe only Tobari
- Successor: None
- Owner: Tobari maintainers
- Target: Current main branch
- Related ADRs: ADR 0003

## Outcome

The Tobari repository contains only Tobari product capabilities and reusable
repository mechanisms that remain active. The synthetic sample CLI and the
one-shot derived-repository bootstrap are absent from code, help, tests, tasks,
Skills, and public documentation.

## Why now

The first Tobari MVP reached `main` while retaining executable and documented
scaffolding inherited from its foundry source. That residue makes the product
surface and contributor workflow harder to understand and leaves code that a
ready-profile repository cannot use.

## Non-goals

- Removing active authoring templates under `docs/work/_template`
- Removing the Homebrew Formula template used by release packaging
- Weakening catalog, architecture, security, public-boundary, or release gates
- Changing Tobari runtime behavior, persisted state, network policy, or secrets

## Acceptance criteria

- [ ] `sample list` and `sample read` are absent from catalog, dispatch,
      composition, help, recovery, and production packages.
- [ ] The `sample` namespace is rejected as an unknown command.
- [ ] Bootstrap commands, implementation, Skill, and first-run derived-project
      instructions are absent from the ready-profile repository.
- [ ] Public SECURITY, SUPPORT, harness, and repository guidance describe
      Tobari rather than a template or hypothetical derived project.
- [ ] Project identity metadata and active authoring/release templates remain
      valid.
- [ ] `task check`, `task security`, and `task public:check` pass.

## Governing documents

- Thesis: Thesis 7, claims must be executable
- Product contract section: Public vocabulary and commands
- Architecture or security invariant: Catalog is the only public command registry
- Existing ADR: ADR 0003 manual migration for existing ready repositories

## Completion definition

The work is complete when the cleanup is committed in reviewable units, all
acceptance criteria have evidence, required gates pass, and this temporary work
packet is removed.
