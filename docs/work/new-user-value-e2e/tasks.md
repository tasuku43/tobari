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
- [x] Confirm the scenario order and shared-resource isolation before each
      protocol-v1 pilot; the official rerun uses parent-prepared project
      copies.

## Implement

- [x] Create the parent-authored scenario definitions and feedback template.
- [x] Record the incomplete protocol-v1 pilot attempts without counting them as
      official success; evidence: the three pilot feedback files.
- [x] Prove the core value loop in a parent-owned PTY baseline before blind
      delegation; evidence: [feedback/official/parent-baseline.md](feedback/official/parent-baseline.md).
- [x] Record the first official Long-01 attempt as an environment-blocked,
      non-acceptance run; evidence: [feedback/official/long-01-attempt-01-environment-blocked.md](feedback/official/long-01-attempt-01-environment-blocked.md).
- [ ] Officially run long scenario 1 through a real pseudo-TTY and record
      parent-owned feedback.
- [ ] Officially run long scenario 2 through a real pseudo-TTY and record
      parent-owned feedback.
- [ ] Officially run medium scenario 1 through a real pseudo-TTY and record
      parent-owned feedback.
- [ ] Officially run medium scenario 2 through a real pseudo-TTY and record
      parent-owned feedback.
- [x] Do not modify production code, CLI catalog, README, or unrelated packets.

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
