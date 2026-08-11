# Work Tasks: Make doctor diagnose prerequisites without false blame

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Finish read-purity prerequisite.
- [x] Enumerate all checks, dependencies, calls, and current early returns.
- [x] Reproduce Docker missing/down and first-read policy cases in isolated XDG.
- [x] Freeze typed fixture and answer key.

## Decide

- [x] Approve finite dependency graph and blocked status.
- [x] Decide report schema compatibility.
- [x] Approve human prerequisite plus exact rerun recovery.

## Implement

- [x] Add failing dependency-matrix and full-report tests.
- [x] Add domain prerequisite/status validation.
- [x] Refactor application orchestration to continue independent checks.
- [x] Return blocked rather than speculative downstream failure.
- [x] Update renderers, catalog, help, and fault recovery.
- [x] Add zero-mutation and hostile-output tests.
- [x] Update governing documents and readiness scenario.

## Verify

- [x] Focused domain/app/infra/CLI tests pass. Evidence: isolated staged tree `go test -count=1 ./internal/...` exit 0.
- [x] Docker missing/down matrices pass. Evidence: `TestDoctorObserverDependencyMatrixAvoidsDockerFalseBlame` and `TestRunSchedulesCompleteDependencyMatrix`.
- [x] Empty-XDG no-mutation canaries pass. Evidence: `TestDoctorObserverFreshTreeIsExactlyReadOnly` and `TestDoctorObserverDoesNotMigrateLegacyContext`.
- [ ] `task check` passes. Deferred evidence: isolated staged tree hygiene, architecture, fixture digest, and normal documentation checks pass; the run stops only on protected `docs/architecture-site/**` English/Japanese schema-table drift for Doctor report schema 2 plus independently advanced auth/status/contexts/context schemas.
- [x] `task security` passes. Evidence: isolated staged tree `task security` exit 0; module verification, security guard, Go security scan, and public-documentation source verification pass.
- [x] Agent-readiness recovery needs no exploratory call. Evidence: schema-2 rows carry direct blocker and task-owned action/next command; output/catalog tests reject drift.

## Hand off

- [ ] Acceptance criteria have evidence.
- [ ] Durable decisions are promoted.
- [ ] Temporary fake-runtime artifacts are removed.
- [ ] Packet is removed after completion.
