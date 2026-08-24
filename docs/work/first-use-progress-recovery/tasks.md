# Work Tasks: Make first Workspace entry legible and recoverable

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Rebaseline and freeze

- [x] Create an independent worktree at exact accepted HEAD
      `c421588bb5a2938dee35be629bd81ab7d76b4308`.
- [x] Verify clean status and all fixed upstream ancestors before editing.
- [x] Read AGENTS.md, the complete add-capability skill, docs/00-04, ADR 0084,
      ADR 0085, current packet, root/Catalog/status/entry/progress/fault/help,
      and relevant tests.
- [x] Build a temporary baseline binary and observe fresh-XDG status/root
      behavior without repository edits.
- [x] Freeze evaluator E1-E7; add no row or public concept without PO review.
- [x] Rebase all four packet files from ADR 0079/Manifest assumptions to ADR
      0084/0085 and current Catalog.

## Contract tests

- [ ] Add one typed evaluator fixture and answer key for E1-E7.
- [ ] Add fresh interactive Start/Customize/Cancel and redirected zero-mutation
      tests.
- [ ] Add exact five-stage vocabulary/state/checkpoint tests.
- [ ] Add deterministic timing, heartbeat, narrow, NO_COLOR, hostile-text, and
      redirected progress tests.
- [ ] Add cancellation at every mutation/handoff boundary and exit-130 tests.
- [ ] Add exact child argv/stream/status and secondary-cleanup tests.
- [ ] Add machine-checked Catalog recovery graph tests.

## Implement

- [ ] Add bounded invocation-only first-entry progress domain values.
- [ ] Add generic final root readiness application port/use case.
- [ ] Require fresh human review before lock/state/Docker mutation.
- [ ] Compose current default-pair initialization, final `cluster up`, Context
      entry, and child handoff without duplicating their state machines.
- [ ] Thread typed prepare-Workspace and handoff progress through Context entry.
- [ ] Preserve exact-reference nondefault `context enter` behavior.
- [ ] Preserve WP03 Runtime recovery and route missing material to `review
      runtimes` without implicit repair or synthesized refs.
- [ ] Preserve WP06 typed detach guidance and launch no ambiguous child.
- [ ] Implement pre-handoff cancel settlement/130 and post-handoff child status
      ownership.
- [ ] Add only the generic recovery binding required by root; do not create a
      parallel status recovery model.

## Durable promotion

- [ ] Update README first use, direct shell/Claude/Codex/gh login, progress,
      cancel, and recovery examples to Template/Context vocabulary.
- [ ] Update docs/00-04 and docs/09 with the final checkpoint, trust, harness,
      and readiness contracts.
- [ ] Remove stale public first-use Manifest/`--manifest`/migration text without
      changing frozen private wire explanations.
- [ ] Regenerate/review architecture-site contracts through repository tasks.

## Verify

- [ ] Focused domain/application/CLI/infrastructure tests pass. Evidence:
- [ ] TTY, redirected, narrow, NO_COLOR, failure, cancellation, status, and
      readiness render from the same semantic fixtures. Evidence:
- [ ] Agent-readiness routine success needs zero external parser/source
      inspection. Evidence:
- [ ] Explicit disposable Docker/Colima context is independently verified;
      default context receives no final test access or mutation. Evidence:
- [ ] Cold/warm standard Runtime and Gateway ranges are recorded as evidence,
      with no public ETA. Evidence:
- [ ] Every created Docker resource and disposable context is removed exactly.
      Evidence:
- [ ] `task check` passes. Evidence:
- [ ] `task security` passes. Evidence:
- [ ] `task public:check` passes. Evidence:
- [ ] `task release:check` passes. Evidence:
- [ ] Exact final HEAD is clean and contains no stale packet-era reference.
      Evidence:

## Hand off

- [ ] All E1-E7 rows and acceptance criteria have evidence.
- [ ] Durable conclusions are promoted.
- [ ] Delete this temporary packet from the final tree.
- [ ] Commit final packet retirement and verify clean status.
- [ ] Report final interfaces, gate/runtime evidence, exact HEAD/status,
      retention, and cross-audit readiness as `WP10_IMPLEMENTATION_COMPLETE`,
      or a reproduced supported-flow P0/P1 blocker as
      `WP10_IMPLEMENTATION_BLOCKED`.
