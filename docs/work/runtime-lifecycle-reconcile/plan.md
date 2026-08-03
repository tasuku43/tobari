# Work Plan: Make runtime preparation and reuse deterministic

- Status: Proposed
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

1. Reproduce both runtime-not-ready registration and build-after-registration
   in fresh disposable projects.
2. Prevent broken registration: before a new Workspace is created, the
   application asks the runtime adapter to validate the image selected by the
   active Context. The adapter performs no state or Docker mutation and maps
   `builtin` to the current cluster asset image before compatibility checking.
3. Declare the missing-image prerequisite in the `tobari` catalog with
   `runtime build` as the recovery action.
4. Keep existing Workspace image reuse and no-silent-refresh semantics
   unchanged. A future refresh/recreate action needs its own mutation contract.
5. Record the shell identity wrapper timeout separately rather than changing
   the fixed `/bin/bash` entry contract without a bounded reproduction.
6. Replay the complete runtime journey and promote the contract into
   docs/tests.

## Alternatives considered

### A: Always auto-refresh existing Workspaces

Rejected until a lifecycle contract proves that replacing a live image is safe;
silent refresh can surprise a user and alter the running environment.

### B: Require runtime readiness before registration

Chosen. A read-only adapter preflight gives the first command a clear typed
prerequisite and guarantees that no logical Workspace is created when its
image cannot be used. The catalog recovery is `runtime build`.

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
