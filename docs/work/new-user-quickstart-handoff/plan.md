# Work Plan: Make the first-use host handoff executable

- Status: Proposed
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Dependency order

1. Consume the policy-review packet's final recovery/cancellation wording.
2. Consume the runtime lifecycle packet's prerequisite and reuse decision.
3. Update README/help-adjacent public guidance and replay the complete journey.

## Chosen approach

Audit every command shown in the first-use path against the canonical catalog,
then edit the smallest public documentation surface. Use a real PTY replay to
prove the documented path rather than treating prose as completion evidence.

## Alternatives considered

### Add a generic `retry` command

Rejected pending a product outcome that owns deterministic retry semantics. A
stale recovery label should not create a second command surface.

### Put policy review inside the Workspace

Rejected by the current trust boundary. Make host ownership clear instead of
adding an unrestricted agent-side control path.

### Combine all cleanup commands

Deferred. Project deletion and shared-cluster shutdown have different ownership
and impact; documentation can explain the sequence without merging commands.

## Verification

- Exact catalog/help review for every documented command.
- Real 120x40 PTY replay with synthetic denial, host review, runtime path, and
  cleanup.
- `git diff --check`, `task check`, `task public:check`, and relevant release/
  integration profiles.
