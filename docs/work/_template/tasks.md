# Work Tasks: Short outcome

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

Use checkboxes for atomic work and add evidence after completion. This file tracks execution; it does not override the goal, context, plan, or governing invariants.

## Understand

- [ ] Read governing theses, product, architecture, and security sections.
- [ ] Reproduce or observe current behavior.
- [ ] Record verified facts and unknowns in `context.md`.
- [ ] Record repeated decisions, friction, and potential thesis workarounds as evidence.
- [ ] Confirm the public outcome and non-goals in `goal.md`.

## Decide

- [ ] Compare credible approaches and record the selected design.
- [ ] Identify public-contract and compatibility impact.
- [ ] Classify utility/discover/act roles and opaque reference flow.
- [ ] Classify the capability as public, internal, deferred, or excluded.
- [ ] Identify effects, target, assets, and trust-boundary changes.
- [ ] Decide authentication, output delivery, collection coverage, timeout,
      retry, idempotency, and schema-drift contracts when an external API is
      involved.
- [ ] Create or update an ADR for a durable trade-off.
- [ ] Revise an incomplete thesis before adding a local code exception, then list propagation work.
- [ ] Obtain required design approval.

## Implement

- [ ] Add failing contract or negative-path tests.
- [ ] Implement domain invariants.
- [ ] Implement application use case and owned ports.
- [ ] Implement bounded infrastructure adapters.
- [ ] Register the command in `cli.Catalog` and update presentation.
- [ ] Update the capability ledger and any publishable schema-fixture manifest.
- [ ] Add producer/consumer graph and exact opaque-ID round-trip tests.
- [ ] Add structured output/error, pagination, cancellation, hostile-output, and zero-downstream-call tests in proportion to risk.
- [ ] Add or update harness enforcement.
- [ ] Update durable documentation.
- [ ] For an interpretation-sensitive presentation change, add one typed
      semantic fixture, answer key, exact next argv, negative-inference canaries,
      and same-fixture before/after evidence using `presentation-evidence.md`.
- [ ] For a removed or replaced capability, prove public-contract, dependency,
      fallback, and persisted-state retirement using
      `capability-retirement.md`.

## Verify

- [ ] Focused tests pass. Evidence:
- [ ] `task check` passes. Evidence:
- [ ] `task security` passes when required. Evidence:
- [ ] `task public:check` passes when required. Evidence:
- [ ] `task release:check` passes when required. Evidence:
- [ ] Runtime-only behavior was observed on the required platform. Evidence:
- [ ] The relevant agent-readiness scenario met its discovery-round-trip budget. Evidence:
- [ ] The routine-success external-processing count is zero for each supported
      outcome, or a deliberately raw utility documents its narrower promise.
      Evidence:
- [ ] Setup/authentication candidates have a human-handoff scorecard and safety/certainty rationale. Evidence:
- [ ] Generated diff and repository status are understood. Evidence:

## Hand off

- [ ] Acceptance criteria have evidence.
- [ ] Goal status is changed to `Complete` only after all goal and task
      checkboxes are complete; `Superseded` names a canonical successor goal.
- [ ] Durable decisions were promoted out of the work packet.
- [ ] Temporary diagnostics and sensitive artifacts were removed.
- [ ] Follow-up work is explicit and does not block this goal.
- [ ] Pull request or handoff summary explains outcome, why, checks, and risks.
