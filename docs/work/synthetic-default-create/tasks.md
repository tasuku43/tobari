# Work Tasks: Make first Context creation atomic and actionable

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read governing theses, product, architecture, security, and harness documents.
- [x] Reproduce current behavior in an isolated XDG environment.
- [x] Record verified facts, constraints, and non-goals.

## Decide

- [x] Preserve the command, fixed-target effect, JSON schema, and observational read contract.
- [x] Keep Docker reconciliation outside `context create`.
- [x] Select lock-scoped infrastructure ordering plus exact CLI recovery.

## Implement

- [x] Add failing runtime tests for first explicit `default` creation and duplicate preservation.
- [x] Implement atomic first creation without weakening legacy migration.
- [x] Add CLI recovery coverage for a successful create with an absent cluster.

## Verify

- [x] Focused tests pass. Evidence: `go test ./internal/infra/dockerruntime ./internal/app/contextcmd ./internal/cli`.
- [x] Isolated-XDG runtime replay passes. Evidence: a temporary built CLI returned exit 0 for the first `context create --name default`, rendered exact `Next: run \`tobari cluster up\``, returned exit 10 with `context_exists` on the second create, and retained the same manifest SHA-256.
- [x] `task check` passes. Evidence: full profile exit 0, including 40 browser tests and the race-enabled Go suite.
- [x] Repository status contains only understood changes. Evidence: four implementation/test files plus this packet; unrelated untracked `docs/work/transparent-http-routing/` remains untouched. Generated `scripts/__pycache__/` was removed after the gate.

## Hand off

- [x] Acceptance criteria have evidence.
- [ ] Temporary packet is removed after completion.
- [x] Commit contains only this concern.
