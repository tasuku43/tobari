# Work Tasks: Align CWD-owned lifecycle boundaries

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read the review specification and governing documents.
- [x] Inspect current application, infrastructure, catalog, tests, and integration harness.
- [ ] Record verified facts and unresolved questions in `context.md`.

## Decide

- [ ] Confirm the shared cluster/project lifecycle boundary and public fault contract.
- [ ] Decide journal shape and recovery phases.
- [ ] Decide exact protected-root and spec-hash rules.
- [ ] Decide legacy implementation retirement scope.

## Implement

- [ ] Remove implicit cluster mutation from `tobari`.
- [ ] Add protected-root validation and active-policy separation.
- [ ] Reconcile explicit cluster operations from CWD project indexes.
- [ ] Add project and cluster durable mutation recovery.
- [ ] Make concurrent enter use the latest immutable state.
- [ ] Add runtime readiness and spec drift reconciliation.
- [ ] Make partial delete idempotent.
- [ ] Remove or isolate legacy named authority.
- [ ] Update contracts, docs, and integration coverage.

## Verify

- [ ] Focused tests pass.
- [ ] `task check` passes.
- [ ] `task security` passes.
- [ ] `task public:check` passes.
- [ ] `task integration:test` passes.
- [ ] Final requirement-by-requirement audit is complete.

## Hand off

- [ ] Durable decisions are promoted out of the work packet.
- [ ] Temporary diagnostics and the work packet are removed.
- [ ] Commit history is grouped by concern.
