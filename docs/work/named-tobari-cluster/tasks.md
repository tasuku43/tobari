# Work Tasks: Named Tobari cluster

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read governing theses, product, architecture, security, and harness sections.
- [x] Observe current singleton lifecycle and policy reload behavior.
- [x] Record verified facts, unknowns, and thesis evidence.
- [x] Confirm the public outcome and non-goals.

## Decide

- [x] Compare shared-network, dedicated-network, read-write, and watch approaches.
- [x] Identify the pre-v1 compatibility impact.
- [x] Classify fixed cluster operations, discovery, and reference-bound actions.
- [x] Identify effects, targets, assets, and trust-boundary changes.
- [x] Revise the incomplete singleton thesis before implementation.

## Implement

- [ ] Update durable contracts and capability ledger.
- [ ] Add domain invariants and tests.
- [ ] Add application use cases and ports.
- [ ] Implement shared cluster and per-Tobari runtime operations.
- [ ] Register catalog commands and presentation.
- [ ] Add opaque-ID round-trip and negative-path tests.
- [ ] Update integration tests and README.

## Verify

- [ ] Focused tests pass. Evidence:
- [ ] `task check` passes. Evidence:
- [ ] `task security` passes. Evidence:
- [ ] `task public:check` passes. Evidence:
- [ ] Runtime profile passes. Evidence:
- [ ] Two-Tobari and live-policy behavior observed. Evidence:
- [ ] Agent-readiness workflow remains within budget. Evidence:

## Hand off

- [ ] Acceptance criteria have evidence.
- [ ] Durable decisions are promoted and this temporary packet is removed.
- [ ] Repository status and commits are understood.
