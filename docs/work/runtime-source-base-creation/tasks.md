# Work Tasks: Create a Runtime source from an explicit Base

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

Use checkboxes for atomic work and add evidence after completion. This file
tracks execution; it does not override the goal, context, plan, or governing
invariants.

## Understand

- [x] Read governing theses, product, architecture, security, harness, and
      agent-readiness Runtime sections.
- [x] Observe the existing Runtime create/source/build storage path.
- [x] Confirm that immutable revision snapshots freeze regular files to `0400`
      and retain no original per-file mode inventory.
- [x] Confirm that the current managed editable source preserves the mode facts
      needed for a faithful Base copy.
- [x] Record current non-interactive standard-starter semantics.
- [x] Audit unrelated worktree changes and keep this packet disjoint.

## Decide

- [x] Keep one canonical `runtime create` action.
- [x] Use Base and `--base`; reject `--from` and `name@ordinal` Base values.
- [x] Define `standard` as the built-in starter source and a managed Runtime
      name as its current editable `source/` tree.
- [x] Make Base a creation initializer only, with no persisted lineage.
- [x] Give the new Runtime a fresh ID, empty history, and independent source.
- [x] Keep build and Context selection as later explicit operations.
- [x] Skip the interactive chooser when standard is the only Base; otherwise
      show a standard-first Base chooser.
- [x] Preserve non-interactive omission as implicit standard with no prompt.
- [x] Preserve the fixed-target Runtime-catalog create contract; `--base` is
      not a reference or target binding.
- [ ] Decide selector-versus-combined-review presentation after frozen-fixture
      comparison.
- [ ] Decide whether existing fault codes fully express Base disappearance and
      drift.
- [ ] Decide whether an ADR is required for the editable-source Base decision.

## Implement

- [ ] Add failing domain Base-selector, standalone-target, and no-lineage tests.
- [ ] Extend the application create port and result correlation with an exact
      Base source selector.
- [ ] Refactor one shared bounded source inventory/copy primitive without
      changing current build digest/freeze behavior.
- [ ] Implement standard starter generation and managed editable-source copy
      into a private atomic target stage.
- [ ] Preserve exact relative paths, bytes, and owner modes including execute.
- [ ] Add source drift, cleanup, collision, cancellation, and zero-partial-target tests.
- [ ] Add Catalog `--base`, completion, help, structured faults, and negative
      ordinal/`--from` tests.
- [ ] Add raw and line standard-first Base chooser flows and standard-only skip.
- [ ] Preserve redirected/JSON omission and existing Runtime report schemas.
- [ ] Add the typed semantic fixture, answer key, same-fixture before/after
      goldens, exact next argv, and negative-inference canaries described in
      `presentation-evidence.md`.
- [ ] Update docs 00–04, ADR if required, capability/help fixtures, and
      agent-readiness.

## Verify

- [ ] Focused domain/application/infrastructure/CLI tests pass. Evidence:
- [ ] Catalog, contractlint, completion, and agent-help checks pass. Evidence:
- [ ] `task check` passes. Evidence:
- [ ] `task security` passes. Evidence:
- [ ] `task public:check` passes. Evidence:
- [ ] `task release:check` is confirmed unnecessary or passes. Evidence:
- [ ] Standard-only, multiple-source, explicit, redirected, JSON, and cancel
      behavior is observed. Evidence:
- [ ] Executable-mode preservation and Base/target independence are observed. Evidence:
- [ ] Agent-readiness discovery budget and zero external processing pass. Evidence:
- [ ] Generated diff and repository status are understood. Evidence:

## Hand off

- [ ] Acceptance criteria have evidence.
- [ ] Durable decisions are promoted out of this temporary packet.
- [ ] Temporary packet and diagnostics are removed in the completion handoff.
- [ ] The atomic implementation commit contains no unrelated protocol-derived-intent changes.
- [ ] Handoff states user outcome, exact checks, compatibility, and residual non-goals.
