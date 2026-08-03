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
- [x] Record the second Long-01 attempt as a partial blind run whose allowed
      request missed policy learning; evidence: [feedback/official/long-01-attempt-02-partial.md](feedback/official/long-01-attempt-02-partial.md).
- [x] Officially run long scenario 1 through a real pseudo-TTY and record
      parent-owned functional feedback; evidence: [feedback/official/long-01.md](feedback/official/long-01.md).
- [x] Record the first Long-02 attempt as a non-acceptance stalled/state-boundary
      run; evidence: [feedback/official/long-02-attempt-01-stalled.md](feedback/official/long-02-attempt-01-stalled.md).
- [x] Officially run long scenario 2 through a real pseudo-TTY and record
      parent-owned functional feedback; evidence: [feedback/official/long-02.md](feedback/official/long-02.md).
- [x] Record the first Medium-01 attempt as a non-acceptance stalled Inbox
      run; evidence: [feedback/official/medium-01-attempt-01-stalled.md](feedback/official/medium-01-attempt-01-stalled.md).
- [x] Record the second Medium-01 attempt as a non-acceptance stalled Inbox
      run despite corrected state injection; evidence: [feedback/official/medium-01-attempt-02-stalled.md](feedback/official/medium-01-attempt-02-stalled.md).
- [x] Prove the Medium-01 three-decision Inbox path in a parent-owned PTY
      baseline before another blind run; evidence: [feedback/official/parent-medium-01-baseline.md](feedback/official/parent-medium-01-baseline.md).
- [x] Officially run medium scenario 1 through a real pseudo-TTY and record
      parent-owned feedback; evidence: [feedback/official/medium-01.md](feedback/official/medium-01.md).
- [x] Record the first Medium-02 run as partial functional reuse/cleanup
      evidence with the cancellation branch still open; evidence: [feedback/official/medium-02-attempt-01-partial.md](feedback/official/medium-02-attempt-01-partial.md).
- [x] Prove the Medium-02 lifecycle path in a parent-owned PTY baseline before
      another blind run; evidence: [feedback/official/parent-medium-02-baseline.md](feedback/official/parent-medium-02-baseline.md).
- [x] Officially run medium scenario 2 through a real pseudo-TTY and record
      parent-owned feedback; evidence: [feedback/official/medium-02.md](feedback/official/medium-02.md).
- [x] Do not modify production code, CLI catalog, README, or unrelated packets.

## Verify

- [x] Each scenario has real-PTY evidence, readable checkpoints, visible value
      signal, and exit/cleanup result. The official child runs predate the
      parent capture helper and did not return raw digests; that limitation is
      recorded explicitly, while the parent-owned raw/checkpoint boundary is
      proven by the external integration capture described in `docs/04_harness.md`.
- [x] Each scenario has a committed readable feedback record with a visible
      value signal and exit/cleanup result; raw digest/timing remains open in
      the preceding task. Evidence: `feedback/official/*.md`.
- [x] Each scenario records discovery-round-trip observations and any
      non-routine processing, including deviations where the child did not
      return an exact numeric count. Evidence: `feedback/official/*.md`.
- [x] Each feedback record classifies blockers and command-surface candidates.
      Evidence: `feedback/official/*.md`.
- [x] Cross-scenario comparison identifies repeated friction and missing
      transitions without prematurely approving a command change. Evidence:
      [comparison.md](comparison.md).
- [x] `git diff --check` passes. Evidence: comparison/feedback commit gate.
- [x] `task check` passes. Evidence: full repository check on current `main`.
- [x] `task public:check` passes. Evidence: public-boundary check on current
      `main`.
- [x] Security/release/runtime/integration results are recorded when relevant.
      Evidence: [context.md](context.md), including the pre-existing release
      ShellCheck failure and integration PTY block.
- [x] No raw host-specific captures or sensitive output are committed; raw
      captures remain outside Git or were not returned by children. Evidence:
      [context.md](context.md) and redacted feedback files.

## Hand off

- [x] All four official feedback files are present and linked from the
      comparison; the comparison links each actionable finding to its bounded
      successor packet.
- [x] Goal status is changed to `Complete` after all four attempted E2E
      journeys and the required gates have evidence.
- [x] Follow-up implementation/docs/CLI packets are listed with explicit
      evidence and do not hide a blocked journey as success.
- [x] The final packet contains only safe redacted evidence.
