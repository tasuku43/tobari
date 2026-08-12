# Work Tasks: Publish the smallest defensible Tobari V1

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)
- Retirement record: [capability-retirement.md](capability-retirement.md)
- Context decision: [context-capability-envelope](../context-capability-envelope/tasks.md)
- Source access: [context-source-access](../context-source-access/tasks.md)
- Policy compaction retirement: [policy-compaction-retirement](../policy-compaction-retirement/tasks.md)
- Policy presets: [policy-presets](../policy-presets/tasks.md)
- Authentication narrowing: [v1-auth-narrowing](../v1-auth-narrowing/tasks.md)
- Release artifacts: [first-public-release-artifacts](../first-public-release-artifacts/tasks.md)

Use checkboxes for atomic work and add evidence after completion. This file
tracks execution; it does not override the goal, context, plan, theses, or
security invariants.

## Understand

- [x] Read governing theses, product, architecture, security, harness, public,
      release, authentication, external-contract, and readiness documents.
      Evidence: `docs/00_theses.md` through
      `docs/09_agent_readiness_validation.md` and `docs/THREAT_MODEL.md`
      inspected on 2026-08-12.
- [x] Observe current behavior. Evidence: V1/unpublished component selection,
      fixed resource constants, direct bind mount, empty GitHub Release list,
      catalog commands, provider plans, and companion state recorded in
      `context.md`.
- [x] Record verified facts and unknowns in `context.md`.
- [x] Record repeated decisions, friction, and potential thesis workarounds as
      thesis evidence.
- [x] Confirm the proposed public outcome and non-goals in `goal.md`.

## Decide

- [x] Compare full-current-scope, clone-first, microVM-first, compaction-first,
      and narrow-static-broker approaches in `plan.md`.
- [x] Identify public-contract and compatibility impact.
- [x] Classify retained utility/discover/act roles and opaque reference flow.
- [x] Classify `policy.learning` and `authentication.broker` as narrowed public
      capabilities and all removed variants as excluded from V1.
- [x] Identify effects, targets, assets, and trust-boundary changes.
- [x] Decide authentication, output delivery, collection coverage, timeout,
      retry, idempotency, and schema-drift contracts for the retained external
      boundary.
- [x] Obtain maintainer approval for the exact retained/retired matrix in
      `plan.md` before code deletion. Evidence: product-owner direction and
      accepted ADR 0030 on 2026-08-12.
- [x] Obtain maintainer approval for Concept A: immutable Context source access
      plus snapshotted built-in/custom policy presets. Evidence: product-owner
      direction on 2026-08-12.
- [x] Complete the Context capability-envelope ADR/design gate and keep both
      implementation packets aligned with it. Evidence: ADR 0029 accepted and
      source-access, compaction, auth, and preset packet dependencies connected
      on 2026-08-12.
- [x] Add a superseding ADR for the durable V1 scope and retirement decision.
      Evidence: `docs/decisions/0030-narrow-first-public-v1.md`.
- [x] Revise the product thesis first, then list and review every required
      downstream propagation diff. Evidence: Theses 3, 4, 8, and 9 establish
      static broker, exact learning, Context source access, and preset
      guardrail decisions before mechanism deletion; downstream alignment is
      tracked by the execution packets and integration lane.

## Contract and catalog

- [x] Add negative catalog tests for retired `policy compactions` and
      `policy compact` paths and the absent `policy-compaction` reference kind.
- [x] Remove compaction command specs, routing, handlers, faults, presentation,
      generated entries, and reference-flow edges.
- [x] Make `auth login --provider github` the sole helper-backed login contract;
      remove provider omission/selector behavior and `--method`.
- [x] Remove AWS, Datadog, OpenAI, Anthropic, and Chatwork provider choices,
      provider-specific output fields, faults, recovery actions, and help.
- [x] Clarify in root/scoped help, `auth status`, Context/status output, and
      documentation that tool-native credentials are Workspace-owned and
      `auth` commands report brokered credentials only.
- [x] Update `.harness/capabilities.json`, schema/claim ledgers, catalog-derived
      architecture data, and negative fallback checks.
- [x] Complete `context-source-access` catalog/report work and its integration
      evidence. Evidence: the integration journey checks one selected live
      bind for each mode on the same root, mutation failure in read-only mode,
      writable home/tmpfs, no writable source alias, and host/read-write
      observation. The local Docker run remains environment-blocked before
      execution and is not counted as supported-platform observation.
- [x] Complete `policy-presets` commands, Context selection/report work,
      `policy.presets` capability ledger entry, and guardrail evidence.
      Evidence: the integration journey validates the exact three built-ins,
      zero immediate grants, explicit Context selection, and custom source
      revision versus immutable Context snapshot; focused domain/OPA tests own
      the terminal-denial matrices.

## Static broker core

- [x] Add contract tests that accept only static primary-secret provider plans,
      one GitHub helper ID, and strict non-executable owner schema V1.
- [x] Preserve and re-prove project-specific handle issuance, exact Context,
      project, provider, revision, target, source-binding and header-binding
      validation, rotation, revocation, and re-entry behavior.
- [x] Preserve and re-prove handle removal before OPA, zero secret resolution
      on deny, one resolution after allow, exact replacement, and no malformed-
      handle forwarding or fallback.
- [x] Preserve and re-prove root-key/vault integrity, locked startup, encrypted
      static credential records, secret-free logs/output, and synthetic-only
      fixtures.
- [x] Retain the GitHub fixed-argv, private-home, digest-stability, bounded-
      output, browser-handoff, cancellation, and cleanup contract without an
      exact version string.
- [x] Retain protected non-terminal stdin import and reject terminal input
      before reading a byte.

## Retirement implementation

- [x] Remove the static managed Gateway adapter, its selection, Context paths,
      policy fields, secret mounts, profile injection, documentation, and tests.
- [x] Remove AWS companion execution, credential export, SigV4 signing,
      identity-center/console login drivers, and provider state.
- [x] Remove Datadog OAuth acquisition/refresh and its token endpoint client.
- [x] Remove OpenAI Codex OAuth, Anthropic Claude setup-token acquisition,
      exact-version drivers, PTY/shim contracts, and provider state.
- [x] Remove built-in Chatwork while preserving the generic owner static
      manifest boundary.
- [x] Remove the companion bridge, encrypted session protocol, resident host
      process, cluster/doctor state, runtime assets, and environment values.
- [x] Remove policy compaction domain/application/infrastructure state and
      source forms while preserving exact learned allow/deny and batch review.
- [x] Remove every dependency, fixture, generated file, image package, build
      step, and manual release check that no retained capability owns.
- [x] Complete every public-contract, dependency, fallback, and persisted-state
      item in `capability-retirement.md`.

## Runtime and security contracts

- [x] Keep direct binding while adding immutable Context-selected read-only or
      read-write access; add negative claims for clone/overlay/snapshot and
      whole-Workspace read-only semantics.
- [x] Keep Docker-only runtime, fixed CPU/memory/PID limits, and shared
      Gateway/OPA/Auth Broker topology; remove status fields owned only by the
      companion. Evidence: the integration journey retains resource/topology
      assertions, requires Auth Broker control-only attachment, forbids its
      egress attachment, and rejects the retired companion status field.
- [x] Keep transparent HTTP/HTTPS, synthetic non-recursive DNS, lazy upstream,
      public-address pinning, no raw-protocol fallback, and guarded project
      principal derivation unchanged.
- [x] Enforce preset guardrails before baseline grants, learned rules, and
      Advanced Rego; prove `offline` and non-GET denial are terminal and perform
      no DNS, credential resolution, upstream, or candidate creation.
- [x] Update exact-effect wording and add negative tests proving ordinary body,
      query/header values, GraphQL arguments/variables, and provider business
      semantics are not claimed as permission identity.
- [x] Re-run threat analysis for the smaller shared Broker/Gateway blast radius
      and confirm no removed code leaves a callable secret or network path.

## Release

- [ ] Build, inspect, publish, and digest-pin immutable Linux amd64/arm64 Gateway
      and Auth Broker V1 indexes from canonical source snapshots.
- [ ] Build supported CLI archives and SHA-256 checksums from the reviewed tag.
- [ ] Generate and publish reviewed SBOMs for CLI and OCI artifacts.
- [ ] Generate CI-identity provenance/attestations and verify them from a clean
      environment without a long-lived repository signing secret.
- [ ] Publish one GitHub Release whose tag, source, archives, component digests,
      checksums, SBOMs, provenance, and release metadata agree.
- [ ] Publish and verify the documented Homebrew formula from a clean host;
      record install, version, doctor, and uninstall observations.
- [x] Update README, security, public-repository, release, and installation
      claims from executable/machine-readable sources where available.

## Verify

- [x] Focused tests pass. Evidence: packet-local Go, OPA, Gateway, Auth Broker,
      release-artifact, catalog, source-equality, and site checks are recorded
      in the child packets.
- [x] `task check` passes. Evidence: `mise exec -- task check` passed on
      integrated V1 HEAD on 2026-08-12, including race and Playwright 40/40.
- [x] `task security` passes. Evidence: `mise exec -- task security` passed on
      the same HEAD.
- [ ] `task public:check` passes. Evidence:
- [ ] `task release:check` passes. Evidence:
- [ ] Runtime-only behavior is observed on every required platform. Evidence:
- [x] The relevant agent-readiness scenario meets the scoped-help discovery
      budget and zero external-processing contract. Evidence: catalog-derived
      root/scoped help exposes exact selectors, defaults, fields, faults, and
      recoveries; the retained readiness journey requires no source inspection
      or undeclared join.
- [ ] GitHub login and static import have a human-handoff scorecard with
      safety/certainty rationale. Evidence:
- [ ] Deny/review/allow/manual-retry, static broker, rotation/logout/re-entry,
      clean install, and Homebrew observations are recorded without secrets.
      Evidence:
- [x] Generated diff, dependency diff, release artifact set, and repository
      status are understood. Evidence: source-fixed site data, 34-command
      catalog, schema/capability ledgers, no new dependency, and exact synthetic
      release inventory were reviewed on the integration branch.

## Hand off

- [ ] Acceptance criteria have evidence.
- [ ] Goal status is changed to `Complete` only after every goal and task
      checkbox is complete.
- [x] Durable decisions are promoted out of this work packet.
- [ ] Temporary diagnostics, old development state, and sensitive artifacts
      are removed.
- [x] Clone/overlay, configurable resources, permission leases, MCP,
      microVM/remote backends, and service fairness remain explicit non-goals;
      none silently blocks this release.
- [ ] Handoff explains the retained product, removed surface, release evidence,
      checks, accepted risks, and triggers for future work.
- [ ] This temporary packet is removed in the completion handoff.
