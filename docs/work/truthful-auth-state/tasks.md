# Work Tasks: Report authentication state and change truthfully

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Close the active auth selector prerequisite and preserve its dirty-tree work.
- [x] Map provider grant and Workspace projection revision authority.
- [x] Capture current/missing/stale/zero/unavailable/locked/mixed and mutation fixtures.
- [x] Freeze semantic fixture and answer key without secrets.

## Decide

- [x] Approve finite provider/activation/no-change states.
- [x] Approve exhaustive/unavailable/not-applicable coverage and schema 4.
- [x] Approve selector rotation warning and no-op receipt wording.
- [x] Approve infra observation versus app/domain semantic ownership.

## Implement

- [x] Add domain/application state and authority-correlation tables.
- [x] Add bounded authoritative projection observation.
- [x] Compute activation state without label/presentation inference.
- [x] Classify absent-provider logout as no change.
- [x] Mark configured providers and warn before rotation.
- [x] Update status/receipt/help/fault presentation and schema.
- [x] Add no-secret, hostile-output, exact-action, and zero-I/O tests.
- [x] Promote durable contracts and readiness evidence.

## Verify

- [x] Focused domain/app/infra/CLI tests pass. Evidence: `go test -count=1
      ./internal/domain/authbroker ./internal/app/authcmd
      ./internal/infra/dockerruntime ./internal/cli`.
- [x] Typed corpus fixture and answer key pass. Evidence:
      `TestTruthfulAuthStateTypedCorpusClosesInterpretationBoundaries`.
- [x] Login/re-entry/Ready integration passes. Evidence: `task integration:test`
      asserts exhaustive activation coverage, a Ready target Workspace, a
      current provider projection, and no re-entry action.
- [x] No-op logout before/after state equality passes. Evidence: the same
      integration profile asserts `no_change`, `not_applicable`, no Workspace
      rows, and byte-identical auth status before and after logout.
- [x] Secret/handle/vault/transcript canaries pass. Evidence: pinned corpus and
      existing CLI/runtime secret canaries in focused suites.
- [ ] `task check` passes. Evidence: the profile reaches contract lint and fails
      only on committed-base stale JSON-schema versions in the explicitly
      excluded English and Japanese `docs/architecture-site/**` Pages tables;
      no file in that subtree is changed by this packet.
- [x] `task security` and `task authbroker:test` pass. Evidence: security profile
      passes repository, vulnerability, and public-source guards; Auth Broker
      suite passes 82 tests.
- [x] Relevant integration/readiness scenarios pass. Evidence:
      `task integration:test` reports `integration: OK` after the new truthful
      status and no-op scenarios.

## Hand off

- [ ] Acceptance criteria have final gate evidence.
- [x] Durable decisions are promoted.
- [ ] Temporary packet is removed after all required gates pass.
