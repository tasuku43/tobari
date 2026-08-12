# Work Tasks: Make Context the explicit capability envelope

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read governing theses, product, architecture, security, ADR 0013, ADR
      0010, ADR 0026, ADR 0027, and ADR 0028.
- [x] Inspect Context manifest/report, create catalog, store initialization,
      project root mount, and aggregate policy flow.
- [x] Record current facts and product-owner direction in `context.md`.

## Decide

- [x] Select immutable Context composition over live preset references or
      per-Workspace flags. Evidence: product-owner approval 2026-08-12.
- [x] Select preset guardrail precedence over guided, learned, and Advanced
      allows.
- [x] Select `read-write` and `builtin/reviewed-exact` omission defaults.
- [x] Review and accept the exact CLI/report field names. Evidence: ADR 0029
      fixes `source_access`, `policy_preset_origin`,
      `policy_preset_revision`, and the task-owned `policy_guardrail` report.
- [x] Add and accept the durable capability-envelope ADR. Evidence:
      `docs/decisions/0029-context-capability-envelope.md` accepted on
      2026-08-12.
- [x] Propagate the thesis before completing mechanism changes. Evidence:
      theses, product, architecture, security, threat model, harness,
      readiness validation, and repository add-capability Skill updated with
      the creation-time immutable envelope decision.

## Implement

- [ ] Complete every task in `../context-source-access/tasks.md`.
- [ ] Complete every task in `../policy-presets/tasks.md`.
- [ ] Integrate both axes into one exact Context manifest/report validation.
- [ ] Update `context create`, `context list`, `context show`, scoped help, and
      agent-readiness fixtures.
- [ ] Update capability/schema/claim ledgers and generated architecture data.
- [x] Update durable theses, product, architecture, security, harness, and ADR
      documents. Evidence: ADR 0029 and the governing-document propagation in
      the Context decision commit.

## Verify

- [ ] Focused Context, runtime, policy, catalog, and integration tests pass.
- [ ] One root can have independently reported read-only/offline and
      read-write/reviewed Context-bound Workspaces.
- [ ] `task check` passes. Evidence:
- [ ] `task security` passes. Evidence:
- [ ] `task public:check` passes. Evidence:
- [ ] Agent readiness meets the scoped-help and zero external-processing
      budgets. Evidence:

## Hand off

- [ ] Both child packets have executable evidence.
- [ ] Acceptance criteria and ADR consequences agree.
- [ ] Durable decisions are promoted and this temporary packet is removed with
      the child packets in the completion handoff.
