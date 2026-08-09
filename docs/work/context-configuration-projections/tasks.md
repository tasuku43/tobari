# Work Tasks: Configure narrow Context projections

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read governing theses, product, architecture, security, and harness documents.
- [x] Operate current root, namespace, exact-command, and interactive help paths.
- [x] Reproduce bounded host Git lookup and projection precedence with synthetic data.
- [x] Record verified facts, dirty-worktree constraints, and thesis evidence.
- [x] Confirm the public outcome and non-goals with the product owner.

## Decide

- [x] Compare context-first, configuration-first, and explicit-verb namespaces.
- [x] Select `config shell` and `config git`; remove the pre-v1.0 old path.
- [x] Compare no-flag wizard, missing-field prompting, and explicit interactive flag.
- [x] Select terminal wizard only for a wholly omitted setting group.
- [x] Classify both commands as fixed-target write acts in `context.composition`.
- [x] Identify host-read, personal-data, projection, and migration boundaries.
- [x] Decide complete delivery, no pagination, two fixed Git calls, no retry, and atomic persistence.
- [x] Add and accept ADR 0021; propagate the revised narrow-projection thesis.

## Implement

- [x] Add product-shaped failing tests.
- [x] Implement domain invariants and schema compatibility.
- [x] Implement application use cases and owned ports.
- [x] Correlate wizard reads and mutation results with the exact task,
      selected Context, applied setting, and configuration-only outcome.
- [x] Implement atomic store migration and updates.
- [x] Register both commands in `cli.Catalog`, remove the old path, and update presentation.
- [x] Implement the wizard with raw-terminal and line-input behavior.
- [x] Bind wizard Apply to the Context shown and reject explicit-empty Context
      selectors before inspection or mutation.
- [x] Implement bounded host Git resolution and fallback encoding/projection.
- [x] Add structured output/error, cancellation, hostile-output, and zero-side-effect tests.
- [x] Update durable documentation and claims-to-checks enforcement.
- [x] Complete presentation evidence from frozen synthetic fixtures.

## Verify

- [x] Focused tests pass. Evidence: the domain, application, CLI, Tobari application, and Docker-runtime packages pass; the configuration-filtered CLI race suite, task/Context/setting/cluster conformance negatives, active-Context switch/explicit-empty regressions, and presentation fixture/golden contract also pass. One full `go test -count=1 ./...` run completed before a later parallel auth-catalog edit changed the shared tree again.
- [ ] `task check` passes. Evidence: a disposable scoped snapshot completed the full gate, including 13 browser tests, race, vet, and all Go tests; with the pinned Node/npm toolchain the shared worktree gate is currently stopped first by `cmd/tobari` import restrictions introduced by the parallel credential-companion change. A later parallel auth-catalog prerequisite edit also temporarily disagrees with its existing CLI test downstream of that first failure.
- [ ] `task security` passes. Evidence: the scoped snapshot passes; the latest shared-tree run is stopped only by the public guard's secret-like-field finding in the unrelated `internal/infra/credentialhost/state_test.go` fixture.
- [ ] `task public:check` passes. Evidence: the same scoped snapshot passes; the latest shared-tree run is stopped by the same parallel credential-host fixture finding.
- [x] Relevant runtime/integration tests pass. Evidence: real Git conditional-global selection and system/global/local precedence tests pass, including malicious local-include exclusion.
- [x] Wizard raw and line modes are observed with synthetic values. Evidence: a real PTY drove Git inherit through Apply successfully; deterministic raw-mode restoration and bounded English line fallback pass focused tests.
- [x] Agent-readiness discovery budget and zero external processing are recorded. Evidence: one exact scoped-help lookup exposes each complete argv contract; one direct invocation returns schema-6 task-owned state with zero external reconstruction.
- [x] Generated diff and unrelated worktree changes are understood. Evidence: the parallel public site intentionally remains on the committed product snapshot, so its authored pages and generated Catalog retain the old command until this complete product tree has a commit to pin; `sitegen --check` and the static site gate pass for that snapshot.

## Hand off

- [ ] Acceptance criteria have evidence.
- [x] Durable decisions are promoted from the packet.
- [x] Temporary probes and synthetic scratch directories are removed.
- [x] Unrelated user-owned worktree changes remain intact.
- [ ] Goal status is marked Complete immediately before this temporary packet is removed.
- [ ] Handoff summarizes outcome, decisions, checks, and residual risks.
