# Work Plan: Make runtime preparation and reuse deterministic

- Status: Proposed
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Resolve the lifecycle contract before changing mechanics:

1. Reproduce both runtime-not-ready registration and build-after-registration
   in fresh disposable projects.
2. Decide whether prevention, reconcile, or new-Workspace-only semantics best
   preserve CWD ownership, explicit host authority, and safe lifecycle effects.
3. Reproduce the shell prompt identity in a clean PTY and classify it.
4. Implement the smallest bounded change, if the decision requires one.
5. Replay the full runtime journey and promote the contract into docs/tests.

## Alternatives considered

### A: Always auto-refresh existing Workspaces

Rejected until a lifecycle contract proves that replacing a live image is safe;
silent refresh can surprise a user and alter the running environment.

### B: Require runtime readiness before registration

Viable if the first command gives a clear typed prerequisite and no broken
instance is created. It keeps the lifecycle explicit but may require a better
recovery path.

### C: Add an explicit reconcile/recreate action

Viable if it binds one exact current Workspace and declares its destructive or
reversible impact. It must not become a generic Docker escape hatch.

## Verification

- Focused runtime/domain/adapter tests for the selected invariant.
- Real 120x40 PTY E2E from fresh project through build, entry/re-entry, and
  cleanup.
- Negative case proving a failed build or canceled lifecycle action does not
  silently replace the prior runtime.
- `task runtime:test`, `task check`, `task security`, and `task public:check`.
- Integration replay after the existing policy-review PTY blocker is resolved;
  record the exact blocker if it remains external.
