# Work Tasks: Keep declared reads observational on first use

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Implemented

- [x] Inventory the public `EffectRead` catalog and transitive first-use effects.
- [x] Add domain synthetic/persisted/legacy observation states.
- [x] Split observation from ensure/mutation store behavior.
- [x] Move Context initialization and migration behind create/write validation.
- [x] Keep only bounded pre-existing-journal recovery on reads.
- [x] Render explicit absence with null authority fields and bumped schemas.
- [x] Add dynamic catalog, read-only XDG, concurrency, legacy, corrupt-state,
      zero-Docker-mutation, and journal-cleanup canaries.
- [x] Promote durable contracts through docs 00–04.

## Verification

- [x] Focused domain/app/infra/CLI tests pass.
- [x] `go test ./internal/...` passes.
- [x] Every public read has a clean first-use before/after canary (16/16 paths).
- [x] Read-only XDG and concurrent read fixtures pass.
- [x] No Docker mutation argv occurs on first-use reads.
- [x] `task security` passes.
- [ ] `task check` passes.
  Evidence: hygiene and archlint pass; only the forbidden-to-edit English and
  Japanese architecture-site JSON-schema tables are stale.
- [x] Repository status and overlapping unrelated authentication work are understood.

## Deferred handoff

- [ ] Obtain authorization for the required architecture-site table sync.
- [ ] Synchronize Context list/report, auth, and Workspace status schema versions.
- [ ] Rerun `task check`.
- [ ] Remove this temporary packet after the gate passes.
