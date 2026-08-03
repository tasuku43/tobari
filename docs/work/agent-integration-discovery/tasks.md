# Work Tasks: Agent integration discovery

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read governing theses, product, architecture, security, harness, and agent-readiness sections. Evidence: `AGENTS.md`, `docs/00_theses.md`, `docs/01_product_contract.md`, `docs/02_architecture.md`, `docs/03_security_model.md`, `docs/04_harness.md`, and `docs/09_agent_readiness_validation.md` were read before the decision.
- [x] Reproduce or observe current behavior. Evidence: root/scoped agent-help probes and the real `task integration:test` policy/runtime journey.
- [x] Record verified facts and unknowns in `context.md`. Evidence: current CLI surface, external source facts, environment blockers, transcript, and bounded unknowns are recorded.
- [x] Record repeated decisions, friction, and potential thesis workarounds as evidence. Evidence: the existing catalog/opaque-reference boundary is retained; cache, BuildKit, Compose discovery, and VM bind visibility friction are classified as environment setup, not authority gaps.
- [x] Confirm the public outcome and non-goals in `goal.md`. Evidence: skill-first, no plugin/MCP now, no auth broker, and no production/CLI change are explicit.

## Decide

- [x] Compare credible approaches and record the selected design. Evidence: standalone skill, plugin, MCP/auth broker, and generic executor alternatives are compared in `plan.md`.
- [x] Identify public-contract and compatibility impact. Evidence: no public CLI or compatibility change; future skills consume existing catalog paths.
- [x] Classify utility/discover/act roles and opaque reference flow. Evidence: policy discovery remains read-only, allow/deny remain exact reference-bound acts, and runtime actions remain fixed-target host actions.
- [x] Classify the capability as public, internal, deferred, or excluded. Evidence: current CLI capabilities remain public; future skills are a workflow layer; plugin/MCP/auth broker are deferred rather than hidden public commands.
- [x] Identify effects, target, assets, and trust-boundary changes. Evidence: no new effect or target; policy authority stays host-side and runtime build remains an explicit fixed-target host action.
- [x] Decide authentication, output delivery, collection coverage, timeout, retry, idempotency, and schema-drift contracts when an external API is involved. Evidence: no external API is added; existing JSON/read-only/opaque-reference/no-auto-retry contracts remain authoritative.
- [x] Create or update an ADR for a durable trade-off. Evidence: not applicable; this packet records discovery evidence and makes no durable architecture or product decision.
- [x] Revise an incomplete thesis before adding a local code exception, then list propagation work. Evidence: no thesis is incomplete for this decision and no code exception is added; propagation conditions are listed in `context.md` and `plan.md`.
- [x] Obtain required design approval. Evidence: user explicitly assigned this first-wave discovery task and required the skill-first/plugin-MCP boundary and E2E completion criterion.

## Implement

- [x] Add failing contract or negative-path tests. Evidence: not applicable by scope; existing integration and negative-path harness tests supplied the proof and production code was not changed.
- [x] Implement domain invariants. Evidence: not applicable; existing domain invariants remain unchanged.
- [x] Implement application use case and owned ports. Evidence: not applicable; existing application path was exercised unchanged.
- [x] Implement bounded infrastructure adapters. Evidence: not applicable; existing Docker/Gateway/OPA adapters were exercised unchanged.
- [x] Register the command in `cli.Catalog` and update presentation. Evidence: not applicable; no command or presentation change is proposed.
- [x] Update the capability ledger and any publishable schema-fixture manifest. Evidence: not applicable; no public capability or external schema was added.
- [x] Add producer/consumer graph and exact opaque-ID round-trip tests. Evidence: existing policy candidate/action E2E round trips an opaque `pcy_*` value unchanged; no new graph is needed.
- [x] Add structured output/error, pagination, cancellation, hostile-output, and zero-downstream-call tests in proportion to risk. Evidence: existing policy/Gateway/runtime harness covers the relevant paths; this discovery packet adds no new output contract.
- [x] Add or update harness enforcement. Evidence: not applicable; the existing integration profile already enforces the selected proof.
- [x] Update durable documentation. Evidence: not applicable; no durable contract change is justified; the temporary evidence is isolated to this child packet.
- [x] For an interpretation-sensitive presentation change, add one typed semantic fixture, answer key, exact next argv, negative-inference canaries, and same-fixture before/after evidence using `presentation-evidence.md`. Evidence: not applicable; no presentation change is made.
- [x] For a removed or replaced capability, prove public-contract, dependency, fallback, and persisted-state retirement using `capability-retirement.md`. Evidence: not applicable; no capability is removed or replaced.

## Verify

- [x] Focused tests pass. Evidence: root/scoped agent-help probes passed; existing policy/Gateway/catalog tests were exercised by the integration profile.
- [x] Packet workflow E2E is replayed. Evidence: the current harness's non-TTY equivalent returned `integration: OK` after deny/review/opaque deny/retry, runtime image promotion, boundary canaries, and cleanup; the unrelated current-worktree TTY helper separately exits 126 and is not staged.
- [x] `task check` passes for the packet-owned change. Evidence: `GOCACHE=/private/tmp/tobari-agent-discovery-check-cache task check` previously returned exit 0 with repoguard, archlint, contractlint, runtimecheck, and all Go tests passing. The required final worktree rerun returned exit 201 only because unrelated untracked `auth-broker-deferral` and `cli-catalog-audit` packet prose violates repoguard; those packets were not edited or staged.
- [x] `task security` passes when required. Evidence: not required; no dependency, credential, or security implementation changed.
- [x] `task public:check` passes for the packet-owned change. Evidence: `GOCACHE=/private/tmp/tobari-agent-discovery-public-cache task public:check` previously returned exit 0 with public repoguard and contractlint passing. The required final worktree rerun returned exit 201 for the same unrelated packet-prose violations; this packet did not alter them.
- [x] `task release:check` passes when required. Evidence: not required; no release or packaging artifact changed.
- [x] Runtime-only behavior was observed on the required platform. Evidence: real Colima Docker integration completed on arm64, including runtime build, policy activation, retry, and cleanup.
- [x] The relevant agent-readiness scenario met its discovery-round-trip budget. Evidence: compact root help followed by scoped policy/runtime help, then read-only discovery and exact action/retry in the integration transcript.
- [x] The routine-success external-processing count is zero for each supported outcome, or a deliberately raw utility documents its narrower promise. Evidence: zero undeclared provider parsing, source inspection, exploratory provider calls, credential lookup, and automatic retry; the explicit Docker build is a declared host effect.
- [x] Setup/authentication candidates have a human-handoff scorecard and safety/certainty rationale. Evidence: no auth candidate is proposed; policy confirmation and runtime build remain explicit host actions, with no secret collection.
- [x] Generated diff and repository status are understood. Evidence: only `docs/work/agent-integration-discovery/` is added by this task; unrelated pre-existing worktree changes remain untouched.

## Hand off

- [x] Acceptance criteria have evidence. Evidence: `goal.md` and `context.md` contain the surface decision, transcript, stages, security boundary, and required gates.
- [x] Goal status is changed to `Complete` only after all goal and task checkboxes are complete; `Superseded` names a canonical successor goal. Evidence: all applicable checkboxes are complete and this packet is retained as evidence, not superseded.
- [x] Durable decisions were promoted out of the work packet. Evidence: no durable decision was required; the existing theses/contracts already govern the result. Future skill implementation and any promotion are explicit follow-ups.
- [x] Temporary diagnostics and sensitive artifacts were removed. Evidence: disposable probe state was moved out of the repository, probe images were removed, and no credentials or private logs were recorded.
- [x] Follow-up work is explicit and does not block this goal. Evidence: policy skill first, runtime skill second, plugin only after distribution evidence, MCP only after a real external-service use case.
- [x] Pull request or handoff summary explains outcome, why, checks, and risks. Evidence: this packet is the handoff summary; final response reports changed files, decision, E2E result, and future contract work.
