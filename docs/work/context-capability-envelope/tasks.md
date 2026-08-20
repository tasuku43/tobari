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
- [x] Retire the public preset packet under [ADR 0066](../../decisions/0066-context-owned-policy-replaces-presets.md).
- [x] Integrate both axes into one exact Context manifest/report validation.
      Evidence: manifest creation requires validated source access, preset
      origin/revision, and normalized snapshot; list/show/create reports and
      strict old-state readers bind the same immutable facts.
- [x] Update `context create`, `context list`, `context show`, scoped help, and
      agent-readiness fixtures. Evidence: catalog-derived agent help and the
      readiness scenario expose both selectors/defaults and every report field.
- [x] Update capability/schema/claim ledgers and generated architecture data.
      Evidence: policy-presets capability and public schema tables pass
      contractlint; generated catalog/component data is pinned to committed
      integrated V1 source.
- [x] Update durable theses, product, architecture, security, harness, and ADR
      documents. Evidence: ADR 0029 and the governing-document propagation in
      the Context decision commit.

## Verify

- [ ] Focused Context, runtime, policy, catalog, and integration tests pass.
      Evidence: focused Go/OPA/Gateway/Auth Broker and fast/security gates pass;
      the Docker integration script passes static/preflight validation but the
      local Colima engine blocked before starting its new fixture container.
- [ ] One root can have independently reported read-only/offline and
      read-write/reviewed Context-bound Workspaces.
      Evidence pending runtime execution: store/spec tests and the integration
      journey create and inspect opposite same-root Context envelopes.
- [x] `task check` passes. Evidence: `mise exec -- task check` passed on
      integrated V1 HEAD on 2026-08-12, including race tests, catalog/source/
      generated checks, both site builds, and Playwright 40/40.
- [x] `task security` passes. Evidence: `mise exec -- task security` passed on
      integrated V1 HEAD on 2026-08-12.
- [ ] `task public:check` passes. Evidence: repoguard and contractlint passed;
      the intentional unpublished Gateway digest checkpoint stopped the gate.
- [x] Agent readiness meets the scoped-help and zero external-processing
      budgets. Evidence: the regenerated exact-command/namespace help carries
      typed selectors, defaults, result fields, faults, and recovery; the
      readiness journey consumes them without source inspection or joins.

## Hand off

- [ ] Both child packets have executable evidence. Evidence: both have unit,
      catalog, generated, and integration-script evidence; supported-platform
      Docker execution remains blocked by the local engine.
- [x] Acceptance criteria and ADR consequences agree. Evidence: governing
      contracts, implementation, help, readiness matrices, and architecture
      pages use the exact ADR 0029 fields/defaults/precedence and reject mutable
      source access or live preset references.
- [ ] Durable decisions are promoted and this temporary packet is removed with
      the child packets in the completion handoff.
