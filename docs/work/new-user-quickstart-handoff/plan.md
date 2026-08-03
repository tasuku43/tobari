# Work Plan: Make the first-use host handoff executable

- Status: Complete
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Dependency order

1. Consume the current policy-review TTY behavior and replace the stale
   post-allow recovery label with the existing `tobari` entry path.
2. Consume commit `6094f08` and its verified runtime transcript: image
   preflight before registration, new-Workspace-only promotion, existing-image
   reuse, and Bash shell identity.
3. Update the smallest README/help-adjacent surface and replay the complete
   journey in a real 120x40 PTY.

## Chosen approach

Audit every command shown in the first-use path against the canonical catalog,
then edit the smallest public documentation surface. Use a real PTY replay to
prove the documented path rather than treating prose as completion evidence.
Keep the denial request and opaque-reference flow from `quickstart-runtime-docs`
unchanged, add the host/Workspace and Docker shared-path cues in README, and
make the post-build example delete the old synthetic Workspace before creating
a new one so it does not claim a silent image refresh.

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
- Record discovery lookups separately from execution: the long path budget is
  three lookups and the medium bootstrap/lifecycle budget is two.
- Record the exact process/state and exit code for any Docker or policy-review
  PTY blocker; do not wait indefinitely for the external handoff.
