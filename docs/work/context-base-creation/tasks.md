# Work Tasks: Create a Context from an explicit Base

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

Use checkboxes for atomic work and add evidence after completion. This file
tracks execution; it does not override the goal, context, plan, or governing
invariants.

## Understand

- [ ] Read governing theses, product, architecture, security, harness, ADR 0071,
      and agent-readiness sections.
- [ ] Reproduce zero-, one-, and multiple-Context creation from a clean catalog.
- [ ] Audit atomic creation storage for Boundary, Runtime, shell, Git, and
      Workspace bootstrap.
- [ ] Audit Advanced-mode policy copying and exact Runtime readiness behavior.
- [ ] Resolve every unknown in `context.md` with code or contract evidence.
- [ ] Preserve and separately classify the unrelated review-command edits.

## Decide

- [x] Keep one canonical `context create` action.
- [x] Use Base, `--base`, and Base chooser vocabulary; reject `--from`.
- [x] Put Base selection before name and individual settings.
- [x] Skip the chooser only when no persisted Context exists or `--base` is
      explicit.
- [x] Initially select the current Context without mutating current selection.
- [x] Treat Base as a draft initializer, not inheritance, parent authority, or
      persisted lineage.
- [x] Replace the complete draft after confirmed Base change; do not merge
      hidden overrides.
- [ ] Complete the catalog target-binding and semantic-dimension audit.
- [ ] Decide the exact copyable composition after storage audit.
- [ ] Decide whether ADR 0071 is revised or succeeded.

## Implement

- [ ] Add failing domain Base/draft invariants and isolation tests.
- [ ] Add application ordering, override, cancellation, and zero-mutation tests.
- [ ] Add or extend atomic infrastructure composition without partial Contexts.
- [ ] Add catalog `--base`, dependency/default/completion/fault metadata, and
      negative `--from` tests.
- [ ] Add raw and line Base chooser flows with current preselection.
- [ ] Seed the existing review/customization flow from the selected Base.
- [ ] Add confirmed Base-reset behavior after edits.
- [ ] Preserve existing direct complete creation and first-use behavior.
- [ ] Add typed semantic fixture, answer key, same-fixture before/after goldens,
      exact next argv, and negative-inference canaries.
- [ ] Update capability documentation, ADR, security claims, harness, and agent
      readiness.

## Verify

- [ ] Focused domain/application/infrastructure/CLI tests pass. Evidence:
- [ ] Catalog, contractlint, completion, and agent-help checks pass. Evidence:
- [ ] `task check` passes. Evidence:
- [ ] `task security` passes. Evidence:
- [ ] `task public:check` passes when required. Evidence:
- [ ] `task release:check` is confirmed unnecessary or passes. Evidence:
- [ ] Zero-, one-, and multiple-Context terminal behavior is observed. Evidence:
- [ ] Agent-readiness discovery budget and zero external processing pass. Evidence:
- [ ] Generated diff and repository status are understood. Evidence:

## Hand off

- [ ] Acceptance criteria have evidence.
- [ ] Durable decisions are promoted out of this temporary packet.
- [ ] Temporary packet and diagnostics are removed in the completion handoff.
- [ ] The atomic implementation commit contains no unrelated review-command
      changes.
- [ ] Handoff states user outcome, exact checks, compatibility, and residual
      non-goals.
