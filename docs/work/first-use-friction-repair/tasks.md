# Work Tasks: First-use friction repair

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

Use checkboxes for atomic work and add evidence after completion. This file tracks execution; it does not override the goal, context, plan, or governing invariants.

## Understand

- [x] Read governing theses, product, architecture, and security sections. Evidence: `docs/00_theses.md` through `docs/04_harness.md` read before code changes.
- [x] Reproduce or observe current behavior. Evidence: first-use replay captured in `context.md`.
- [x] Record verified facts and unknowns in `context.md`.
- [x] Record repeated decisions, friction, and potential thesis workarounds as evidence.
- [x] Confirm the public outcome and non-goals in `goal.md`.

## Decide

- [x] Compare credible approaches and record the selected design. Evidence: `plan.md`.
- [x] Identify public-contract and compatibility impact. Evidence: no command or JSON schema role change; human/Gateway guidance and README text changed.
- [x] Classify utility/discover/act roles and opaque reference flow. Evidence: policy review remains discover, policy allow/deny remain reference-bound acts.
- [x] Classify the capability as public, internal, deferred, or excluded. Evidence: existing public workflow repaired; pinned image publication deferred to the existing release-boundary packet.
- [x] Identify effects, target, assets, and trust-boundary changes. Evidence: read-only doctor diagnostic, host-side runtime build guidance, Gateway audit projection; no new credential or mutation boundary.
- [x] Decide output delivery, collection coverage, timeout, retry, idempotency, and schema-drift contracts. Evidence: policy candidate/denial discovery remains bounded-window complete output; policy writes remain opaque-ID-bound.
- [x] Create or update an ADR for a durable trade-off, or record why none is needed. Evidence: no new architectural trade-off; the source/pinned image boundary is governed by ADR 0017 and `gateway-image-refresh-e2e`.
- [x] Revise an incomplete thesis before adding a local code exception, then list propagation work. Evidence: no thesis revision required; tests and docs make the existing thesis executable.

## Implement

- [x] Add failing contract or negative-path tests. Evidence: Gateway HTTPS normalization tests, Rego README quick-start learnability test, doctor policy data test, CLI rendering tests.
- [x] Implement domain invariants. Evidence: no domain invariant change required.
- [x] Implement application use case and owned ports. Evidence: existing use cases retained; doctor now surfaces policy-data safety through runtime diagnostic composition.
- [x] Implement bounded infrastructure adapters. Evidence: Gateway policy input normalization and Docker runtime doctor diagnostic.
- [x] Register the command in `cli.Catalog` and update presentation. Evidence: no new catalog command; existing runtime/cluster human renderers updated.
- [x] Update the capability ledger and any publishable schema-fixture manifest. Evidence: not applicable; no new capability or schema fixture.
- [x] Add producer/consumer graph and exact opaque-ID round-trip tests. Evidence: policy candidate flow unchanged; Rego/Gateway/manual replay confirms existing producer path.
- [x] Add structured output/error, cancellation, hostile-output, and zero-downstream-call tests in proportion to risk. Evidence: focused structured review/manual JSON and rendering tests cover changed output.
- [x] Add or update harness enforcement. Evidence: focused tests cover the repaired first-use route; no new harness rule required.
- [x] Update durable documentation. Evidence: README and external API contract denial-message guidance updated.

## Verify

- [x] Focused tests pass. Evidence: `go test ./internal/infra/dockerruntime ./internal/cli ./internal/app/tobaricmd ./internal/domain/tobari`; `task policy:test`; `task gateway:test`; `task gateway:source:check`.
- [x] `task check` passes. Evidence: `task check`.
- [x] `task security` passes when required. Evidence: `task security`.
- [x] Runtime-only behavior was observed on the required platform. Evidence: clean source-Gateway replay and the previous explicit source-Gateway integration comparison returned `integration: OK`; the local comparison path is now `task build:dev` plus `bin/tobari-dev`.
- [x] The relevant agent-readiness scenario met its discovery-round-trip budget. Evidence: source-Gateway denial produced one `policy review --format json` candidate without source inspection.
- [x] The routine-success external-processing count is zero for each supported outcome. Evidence: supported review route used Tobari's Gateway JSON and `policy review`; no external parser or provider discovery was needed.
- [x] Generated diff and repository status are understood. Evidence: only code, tests, public docs, and temporary work-packet updates remain modified; verification directories and Tobari containers were removed.

## Hand off

- [ ] Acceptance criteria have evidence. Remaining evidence depends on the pinned Gateway image refresh.
- [x] Durable decisions were promoted out of the work packet. Evidence: README, external API contract, Rego, Gateway, CLI, and doctor tests.
- [x] Temporary diagnostics and sensitive artifacts were removed. Evidence: `.tobari-repro`, `.tobari-source`, `repro-project`, and `source-project` were deleted; no Tobari containers remained.
- [x] Follow-up work is explicit and does not block the source-level repair. Evidence: pinned image publication and digest update remain in `docs/work/gateway-image-refresh-e2e`.
- [x] Pull request or handoff summary explains outcome, why, checks, and risks. Evidence: final response.
