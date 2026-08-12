# Work Tasks: Retire learned policy compaction before V1

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)
- Context: [context.md](context.md)
- Retirement record: [capability-retirement.md](capability-retirement.md)

## Understand and decide

- [x] Inventory public commands, references, faults, state, OPA, fixtures,
      dependencies, documentation, and generated surfaces. Evidence: read-only
      inventory on 2026-08-12 recorded in `context.md`.
- [ ] Confirm the superseding exact-rule-only ADR and Context envelope are
      accepted before deletion.

## Implement

- [ ] Add negative catalog, reference-flow, and persisted-state tests.
- [ ] Remove command specs, handlers, presentation, output schemas, faults,
      recovery actions, and `policy-compaction` references.
- [ ] Remove application ports/use cases and domain compaction vocabulary.
- [ ] Remove prefix learned-rule loading, storage, matching, OPA authority,
      activation, and dormant fallback.
- [ ] Remove or update fixtures, dependencies, documentation, ledgers, and
      generated architecture data owned by this retirement.

## Verify and integrate

- [ ] Run focused Go, OPA, CLI contract, and hostile-state tests.
- [ ] Review generated and dependency diffs.
- [ ] Record `task check`, `task security`, and `task public:check` evidence.
- [ ] Commit only this packet and integrate it before policy presets.
- [ ] Re-run the same verification on the integration branch.
