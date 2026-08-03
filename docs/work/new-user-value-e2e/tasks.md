# Work Tasks: New-user value journeys through a real pseudo-TTY

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read `AGENTS.md`, the governing theses/contracts, and the existing
      Quick Start/integration/PTY harness.
- [x] Confirm that this packet is evidence-only and does not authorize CLI or
      command-surface changes.
- [x] Record the current TTY, policy, runtime, Workspace, and cleanup facts in
      [context.md](context.md).
- [x] Define the common human-paced pseudo-TTY protocol and evidence format.

## Decide

- [x] Select exactly two long and two medium journeys in [scenarios.md](scenarios.md).
- [x] Define value signals, success/failure criteria, cleanup, and discovery
      budgets before delegating execution.
- [x] Define the command-surface observation rubric: keep, integrate, narrow,
      deprecate candidate, or docs-only, with catalog/recovery evidence.
- [ ] Confirm the scenario order and shared-resource isolation before each run.

## Implement

- [x] Create the parent-authored scenario definitions and feedback template.
- [ ] Run long scenario 1 through a real pseudo-TTY and record feedback.
- [ ] Run long scenario 2 through a real pseudo-TTY and record feedback.
- [ ] Run medium scenario 1 through a real pseudo-TTY and record feedback.
- [ ] Run medium scenario 2 through a real pseudo-TTY and record feedback.
- [ ] Do not modify production code, CLI catalog, README, or unrelated packets.

## Verify

- [ ] Each scenario has raw-PTY evidence, readable checkpoints, typed-key
      timing, visible value signal, and exit/cleanup result. Evidence:
- [ ] Each scenario records discovery-round-trip count and any non-routine
      processing. Evidence:
- [ ] Each feedback record classifies blockers and command-surface candidates.
      Evidence:
- [ ] Cross-scenario comparison identifies repeated friction and missing
      transitions without prematurely approving a command change. Evidence:
- [ ] `git diff --check` passes. Evidence:
- [ ] `task check` passes. Evidence:
- [ ] `task public:check` passes. Evidence:
- [ ] Security/release/runtime/integration results are recorded when relevant.
      Evidence:
- [ ] Raw host-specific captures are outside Git and no sensitive output is in
      the packet. Evidence:

## Hand off

- [ ] All four feedback files are present and linked from the comparison.
- [ ] Goal status is changed to `Complete` only after all four attempted E2E
      journeys and the required gates have evidence.
- [ ] Follow-up implementation/docs/CLI packets are listed with explicit
      evidence and do not hide a blocked journey as success.
- [ ] The final packet contains only safe redacted evidence.
