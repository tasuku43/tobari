# Work Tasks: Retire learned policy compaction before V1

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)
- Context: [context.md](context.md)
- Retirement record: [capability-retirement.md](capability-retirement.md)

## Understand and decide

- [x] Inventory public commands, references, faults, state, OPA, fixtures,
      dependencies, documentation, and generated surfaces. Evidence: read-only
      inventory on 2026-08-12 recorded in `context.md`.
- [x] Confirm the superseding exact-rule-only retirement ADR before deletion.
      Evidence: the prerequisite Context envelope is accepted in ADR 0029 and
      ADR 0030 retires command, reference, state, matching, and fallback.

## Implement

- [x] Add negative catalog, reference-flow, and persisted-state tests.
      Evidence: `TestDefaultCatalogPublishesCWDOwnedLifecycleWithoutActionIDs`
      rejects both retired paths from public lookup and internal registration;
      `TestDomainPolicyJSONContractRejectsAmbiguousSources` rejects a persisted
      `match: prefix` rule before activation.
- [x] Remove command specs, handlers, presentation, output schemas, faults,
      recovery actions, and `policy-compaction` references.
      Evidence: the catalog now has 30 public commands, the retired commands
      have no registered handler, and repository search under `internal/`
      finds no production compaction identifier, fault, or reference kind.
- [x] Remove application ports/use cases and domain compaction vocabulary.
      Evidence: the service compaction methods, task/kind constants, proposal
      and report types, ID validation, grouping, and activation code are absent.
- [x] Remove prefix learned-rule loading, storage, matching, OPA authority,
      activation, and dormant fallback.
      Evidence: domain validation admits only `exact`; the persisted domain
      reader rejects the retired shape; OPA has no prefix matcher or prefix
      scope validator, and `test_retired_learned_prefix_rule_fails_closed`
      proves the hostile old shape cannot authorize a sibling path.
- [x] Remove or update fixtures, dependencies, documentation, ledgers, and
      generated architecture data owned by this retirement.
      Evidence: code and OPA fixtures now reject the retired `authorities`,
      `methods`/`exclude_path_prefixes`, `credential_profiles`, and
      `baseline_rules` shapes; the embedded production seed-domain tree is
      removed. There is no dependency-file diff. README, governing contracts,
      capability/schema ledgers, integration flow, and architecture-site
      sources now describe exact and reviewed single-segment path-template V1
      matches while explicitly rejecting both retired command paths and
      prefix/sibling-path widening.

## Verify and integrate

- [x] Run focused Go, OPA, CLI contract, and hostile-state tests.
      Evidence: `go test ./internal/domain/tobari ./internal/app/tobaricmd
      ./internal/cli ./internal/infra/dockerruntime` passed; `mise exec -- task
      policy:test` passed with pinned OPA 1.17.0; `mise exec -- task
      gateway:test` and canonical/embedded Gateway source equality passed.
- [x] Review generated and dependency diffs.
      Evidence: no `go.mod`, `go.sum`, package lock, or generated-file diff is
      present in this lane; stale generated/public surfaces are listed above
      for the integration lane.
- [x] Record `task check`, `task security`, and `task public:check` evidence.
      Evidence: `mise exec -- task check` and `mise exec -- task security`
      passed on integration HEAD on 2026-08-12, including race tests and
      Playwright 40/40. `task public:check` passed repoguard and contractlint,
      then stopped only at the deliberate unpublished Gateway digest
      checkpoint.
- [x] Commit only this packet and integrate it before policy presets.
      Evidence: retirement commits `58f811d` and `e183fc4` precede the preset
      integration commits and contain no preset feature implementation.
- [x] Re-run the same verification on the integration branch. Evidence:
      focused Go, pinned OPA policy, Gateway, fast, and security checks passed
      after cherry-pick integration; the integration script retains explicit
      negative command canaries.
