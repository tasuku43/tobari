# Work Tasks: CWD-owned Tobari lifecycle

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Contract

- [x] Read governing theses, product, architecture, security, and harness documents.
- [x] Record the named-lifecycle conflict and selected CWD lifecycle design.
- [x] Retire named commands and promote durable contract changes.

## Implementation

- [x] Implement canonical CWD lookup and stable XDG records.
- [x] Implement atomic persistence, lock handling, and state validation.
- [x] Implement ensure/reconcile/enter for one logical Tobari.
- [x] Implement CWD-local status, list, and destructive delete.
- [x] Implement read-only shared profiles and isolated mutable state.
- [x] Add focused unit, contract, and integration coverage.

## Verify

- [x] `task check` passes. Evidence: `task check` passed with Go 1.26.5 selected.
- [x] `task security` passes. Evidence: `task security` passed with no vulnerabilities after bounding runtime-owned directory opens.
- [x] `task public:check` passes. Evidence: `./scripts/check.sh public` passed with Go 1.26.5 selected.
- [x] Agent-readiness journey is recorded. Evidence: `docs/09_agent_readiness_validation.md` now uses CWD resolution and delete recovery.
