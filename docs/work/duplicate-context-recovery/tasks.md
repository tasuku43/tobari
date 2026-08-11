# Work Tasks: Route duplicate Context recovery to the right collection

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Reproduce the recovery against a non-active duplicate.
- [x] Confirm application and catalog currently agree on the misleading command.
- [x] Confirm `context list` is the exact, read-only, target-inclusive alternative.

## Decide

- [x] Preserve the fault kind/code/retry semantics.
- [x] Avoid synthesized selector argv.
- [x] Limit the change to application and catalog recovery metadata/tests.

## Implement

- [x] Add a failing recovery test.
- [x] Align runtime fault and catalog metadata on `context list`.

## Verify

- [x] Focused tests pass. Evidence: `go test ./internal/app/contextcmd ./internal/cli -count=1`.
- [x] Non-active duplicate replay passes. Evidence: `TestContextExistsCatalogRecoveryRoutesToListContainingNonActiveDuplicate` routes `tobari context list` and renders non-active `review`.
- [x] Required repository gate passes. Evidence: `task check:fast`.
- [x] Repository status is understood. Evidence: only this packet's app, catalog, tests, and work-packet files are included; `docs/work/transparent-http-routing/` remains unrelated and untouched.

## Hand off

- [x] Acceptance criteria have evidence.
- [ ] Temporary packet is removed after completion.
- [x] Commit contains only this concern.
