# Work Tasks: Make `status` the CWD-centric Workspace home

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)
- Decision state: Accepted/Fixed by Product Owner
- Higher/upstream decisions, in fixed order: promoted WP01+WP02
  [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md)
  and durable [docs/00](../../00_theses.md) through
  [docs/04](../../04_harness.md), the completed WP08 Catalog/output contracts
  in [Architecture](../../02_architecture.md) and [Harness](../../04_harness.md), completed
  [ADR 0080 Runtime lifecycle](../../decisions/0080-close-the-managed-runtime-lifecycle.md), WP04
  [build profiles](../build-profile-contract/goal.md), WP05
  [host-loopback name](../host-loopback-name/goal.md), WP07
  [first-use recovery](../first-use-progress-recovery/goal.md), and WP09
  [service exposure](../service-exposure-ux/goal.md)
- Implementation state: Planned after all upstream implementations through
  WP09; only packet research and fixed design are complete

This checklist is ordered by dependency. Checked items are design evidence, not
production implementation evidence.

## Packet preparation and WP 01 alignment

- [x] Fetch and identify the original current-main baseline at
      `6a26a3c274d2c2ce8dc8c59321ffb7ba67594b42`; protect unrelated shared
      worktree additions.
- [x] Read `AGENTS.md`, `docs/00_theses.md`, governing `docs/01` through `04`,
      relevant `docs/07` through `09`, and the `add-capability` skill.
- [x] Inspect current status/list/legacy-Context/Runtime/cluster/policy/review/
      service human, JSON, Catalog, domain, application, infrastructure, tests,
      README, ADRs, and related history.
- [x] Observe fresh read-only behavior in private temporary XDG roots and record
      zero created files.
- [x] Read the former WP01 packet before promotion and now bind its accepted
      decisions to ADR 0079 and promoted docs/00 through docs/04.
- [x] Replace the packet's future public vocabulary with Workspace Manifest,
      routine Manifest, `manifest`, `--manifest`,
      `workspace_manifest_id`, WorkspaceID, and ProjectRoot; retain old terms
      only as explicit migration/current-main evidence.
- [x] Replace the earlier adoption-only proposal with the WP 01 desired /
      last-successfully-applied / observed / latest-bounded-failure read model.
- [x] Separate cluster, entry, session, and creation activation in schema,
      wireframes, empty states, Next, and verification.
- [x] Correct broken-cluster/service availability coupling and define
      `continue_attached` as typed non-command action handling.
- [x] Verify WP01+WP02 promotion in commits `07535a9` and `428812f`, bind this
      packet to ADR 0079 and promoted docs/00 through docs/04, and confirm both
      temporary packets were deleted.
- [x] Consume the promoted WP02 copy contract and product-owner-fixed WP03
      Runtime lifecycle decision as design constraints without treating WP03
      as an implementation-start notice.
- [x] Remove copy lineage/provenance from the status design; separate Runtime
      ready/availability/observed/last-use facts; exclude full prune protection
      scanning and all lifecycle mutations from routine status.
- [x] Replace project-local current Manifest selection with explicit
      `--manifest` or WP01 installation default only; preserve siblings when no
      default exists.
- [x] Replace the overloaded adoption/overall enum with fixed orthogonal axes,
      structured primary Next, and separately ordered Attention.
- [x] Extend the implementation gate through WP09 and record WP04/WP05/WP07/
      WP08/WP09 negative and consumer boundaries.

## All-upstream mandatory pre-implementation re-baseline

- [x] Verify the first dependency stage, WP01+WP02, is promoted in ADR 0079,
      docs/00 through docs/04, and commits `07535a9`/`428812f`.
- [ ] Receive remaining completion/integration handoffs in fixed order for
      WP08, WP03, WP04, WP05, WP07, and WP09. Record exact integrated HEAD,
      branch, `origin/main` relationship, and working-tree ownership; block
      WP06 while any upstream files move or ownership is ambiguous.
- [x] Confirm ADR 0079 and promoted thesis/product/architecture/security/
      harness consequences replace incompatible legacy public contracts.
- [ ] Final WP 01 domain types exist for WorkspaceManifestID, immutable Manifest
      revision/slices, Runtime binding, WorkspaceID/Workspace, AppliedEntry,
      reconciliation attempt, observed runtime, ProjectRoot, and cluster
      projection.
- [ ] Final public command/field inventory contains only `manifest`,
      `--manifest`, `workspace_manifest_id`, `workspace_id`, and `project_root`
      for these meanings, with no alias.
- [ ] Exact predecessor migration and final-V1 store readers expose sufficient
      read-only desired/applied/failure facts for status.
- [ ] First-public-release owners confirm the old status schema/vocabulary is
      not frozen or shipped before the final schema-1 reset.
- [ ] Re-read the actual post-WP09 integrated Catalog/generated schemas, domain,
      application, infrastructure, migration, and tests; diff their exact
      fields/enums/ports against this packet before writing status code.
- [ ] Build/run the integrated binary only through isolated read-only probes;
      verify fresh zero writes and observe Manifest selection, desired/applied/
      failure, Runtime readiness/availability, and cluster behavior.
- [ ] Re-measure Docker/live-owner calls on final adapters, including WP09, and
      freeze a justified numeric `B_status` ceiling.
- [ ] Publish a production-file ownership/non-overlap map: all upstream types,
      schemas, Catalog conformance, migration, Runtime, wait, research/release,
      host naming, and exposure owner protocols remain upstream; status begins
      only in settled consumer-specific surfaces.

## Upstream facts to consume without WP06 invention

- [ ] Consume WP01's implemented retention, Git slice, attached-child,
      migration-evidence, and migration-precondition contracts exactly; do not
      settle or emulate them in WP06.
- [ ] Consume WP03 Runtime axes, WP07 wait state, WP08 recursive Catalog
      conformance, and WP09 bounded authenticated owner summary without adding
      competing types or protocol extensions.
- [ ] Prove WP04 research-only auth/serve and the WP05-retired host name are
      absent from release status, fixtures, primary Next, and Attention.

## Fixed status design and implementation-time bindings

- [x] Fix separate axes: Workspace presence, entry state, observed Runtime,
      Runtime revision authority, execution-material availability, optional
      upstream integrity/migration evidence, and separate latest attempt.
- [x] Fix exact entry precedence and prove older failure cannot override a newer
      desired identity.
- [x] Fix cluster desired/applied/observed/failure as independent axes; only
      `cluster up` reconciles.
- [x] Fix primary Next safety order and separate ordered Attention; fix exact
      path+typed-input or non-command representation with no stored argv.
- [ ] Bind fixed axis meanings to final upstream type/field spellings without
      introducing `overall_status`, `adoption_state`, or equivalent.
- [ ] Bind owner churn, owner-record bound, and global deadline to WP09's final
      contract; unavailable facts are `not_observed`, never false zero.
- [ ] Freeze `B_status` from the post-WP09 integrated implementation and validate it on
      every supported Engine and local virtualization environment; retain fresh
      zero calls, no sibling/installation scaling, and details/JSON parity.
- [ ] Bind Runtime authority/availability/usage enums to actual WP03 types;
      never infer last use or collapse ready/available.
- [x] Fix status as `RoleUtility`/`EffectRead`/no-refs; reference-bound Runtime
      actions route through an owning discover/review task such as
      `review runtimes`, using WP08 conformance.
- [ ] Obtain joint lifecycle/Manifest, Runtime, migration, cluster, policy,
      service-exposure, security, harness, and first-release approval.

## Semantic contract and fixtures first

- [ ] Freeze one task-owned typed status corpus and presentation-independent
      answer key over exact task, ProjectRoot, WorkspaceManifestID, WorkspaceID,
      desired slice revisions, AppliedEntry, observation, failure, cluster
      projection axes, Runtime authority/material, migration evidence, Attention,
      and primary Next.
- [ ] Cover recommended draft, persisted Manifest without Workspace,
      current/detached, current/attached, same-root multiple Manifest bindings,
      entry pending, attached block, matching/older classified failure, unknown,
      every observed Runtime and material combination, and migrated-unverified
      evidence.
- [ ] Include absent/null, known empty arrays, explicit zero/false, bounded
      window/unparsed, unavailable owner, changed anchor, and invalid-result
      cases.
- [ ] Add negative-inference canaries for Manifest name, generation, Runtime
      ordinal/name, image tag, Docker ID, sibling order, attachment proximity,
      timestamp, heading, color, and indentation.
- [ ] Add WP 02 negative canaries proving no `base`, `copied_from`, source,
      parent, provenance, ancestry, lineage, copied Workspace state, or
      copy-triggered reconciliation appears in status, JSON, help, or fixtures.
- [ ] Add WP 03 fixtures that vary Runtime revision authority, local execution
      availability, selected observed container, desired/applied relation, and
      exact/unknown last-use evidence independently.
- [ ] Add cross-product canaries for `current+stopped`, `current+missing`,
      `pending+running`, and `pending+available`; reject any serialized overall/
      adoption status.
- [ ] Add canaries proving cluster/session/creation-only revisions do not make
      the Workspace entry state pending.
- [ ] Add failing exact Catalog tests for
      `status [--manifest NAME] [--details] [--format text|json]`,
      `RoleUtility`/`EffectRead`,
      no refs, complete delivery, scalar coverage, recursive fields, failures,
      and root help budget.
- [ ] Add failing negative Catalog/argv/schema tests proving no retired command,
      flag, JSON key, or legacy instance ID survives final V1.
- [ ] Add failing exact schema-1 aggregate tests for desired/applied/observed/
      failure, orthogonal status axes, cluster axes, primary Next, and ordered
      Attention objects.
- [ ] Add failing schema/presentation tests excluding raw Docker tag/image/
      container IDs, private snapshot paths, inferred `last_used`, and any claim
      of installation-exhaustive Runtime prune/delete protection.

## Domain consumer

- [ ] Reuse WP01 authority and upstream fact types; derive only the fixed
      orthogonal axes and do not create competing Manifest, Workspace, revision,
      AppliedEntry, attempt, Runtime, wait, or owner-summary types.
- [ ] Reuse WP 03 Runtime readiness/availability/usage-certainty and protection-
      reason projections when they exist; do not create parallel Runtime
      lifecycle, derivation, journal, receipt, reference, or prune-planner types.
- [ ] Add only the task-owned StatusHomeSnapshot with exact task/requested
      scope, selected Manifest/Workspace, exhaustive same-root logical scope,
      cluster projection, orthogonal facts, Attention, and primary Next.
- [ ] Validate a recommended draft has no WorkspaceManifestID, published
      revision, generation, Workspace, or applied authority.
- [ ] Validate every persisted selected Workspace matches exact ProjectRoot,
      WorkspaceManifestID, and WorkspaceID and that siblings are unique.
- [ ] Validate desired entry, AppliedEntry, observed runtime, and latest attempt
      independently; reject invalid/mismatched facts rather than degrading them.
- [ ] Derive entry `failed` only for the latest final attempt matching current
      desired identity; prove older failure remains detail and cannot win.
- [ ] Validate desired/applied Runtime bindings, immutable revision readiness,
      local image availability, and selected observed container independently;
      keep `last_used=unknown` without an exact approved receipt.
- [ ] Limit any protection reasons to current/retained Manifest revision,
      pending adoption, last applied, and selected observed container facts
      already in the snapshot; mark coverage selected/non-exhaustive and never
      derive prune/delete eligibility.
- [ ] Validate cluster desired/applied/observed/failure independently from the
      selected Manifest and Workspace entry state.
- [ ] Validate policy/login/service owners and coverage without including them
      in Manifest desired/applied state.
- [ ] Implement/consume the closed Next table, including typed non-command
      guidance and exact Catalog path plus typed inputs for executable actions;
      return ordered Attention independently and store no argv/free-form command.

## Application

- [ ] Add one StatusHome request/use case only after all upstream implementations
      through WP09 and re-baseline; do not call sibling public handlers or parse
      rendered documents.
- [ ] Define the smallest read-only local desired/applied/failure, selected
      runtime, shared projection, bounded policy, and live-owner ports owned by
      the use case.
- [ ] Consume ordinary Manifest/Runtime state only; do not branch on or recover
      copy provenance, and do not call WP 03 delete/prune/restore/build use
      cases from StatusHome.
- [ ] Resolve ProjectRoot before Manifest. Use exact explicit `--manifest` or
      WP01 installation default only; preserve a typed no-authority recommended
      draft plus exhaustive same-root siblings when no default exists.
- [ ] Prove an existing/only/attached Workspace never becomes selection and
      status never mutates installation default or binding.
- [ ] Read local authority once, select exact
      `(ProjectRoot, WorkspaceManifestID)`, and short-circuit external I/O for
      empty/unconfigured state.
- [ ] Limit live Workspace/policy/service observation to exact selected
      WorkspaceID/WorkspaceManifestID/ProjectRoot; same-root siblings remain
      logical/receipt summaries.
- [ ] Read entry, cluster, session, and creation slice state independently;
      never derive entry pending from the whole Manifest generation.
- [ ] Capture and revalidate Manifest pointer/digest, AppliedEntry/attempt,
      Runtime binding, cluster projection, and owner anchors; return
      `status_snapshot_changed` after one bounded retry instead of mixed state.
- [ ] Map only authoritative expected absence/unavailability to typed degraded
      sections; preserve cancellation and invalid-result faults.
- [ ] Prove status never writes AppliedEntry/failure, clears unknown evidence,
      publishes desired state, or calls Workspace/cluster reconciliation.

## Infrastructure

- [ ] Reuse final-V1 migrated stores through pure observe methods only; never
      initialize, migrate, repair, publish, acquire a mutation lock, recover a
      journal, or clean records.
- [ ] Add one strict multi-container inspect for Gateway, OPA, and selected
      Workspace with exact ownership/role/revision validation.
- [ ] Add one strict multi-network inspect for shared control/egress and the
      selected Project network.
- [ ] Reuse the container batch for observed spec/principal/attachment evidence;
      add no second status-only Workspace inspect.
- [ ] After re-baseline, implement the measured `B_status` observation plan;
      treat one Engine probe, batched container/network inspection, and bounded
      denial observation as provisional until the final WP 01 adapters prove
      their shape.
- [ ] Prove empty Manifest/unconfigured state uses zero Docker and owner calls;
      same-root siblings and `--details` add none.
- [ ] Consume WP09's pure authenticated owner summary carrying only its approved
      semantic scope/counts; add no protocol field, cleanup, action refs, URLs,
      ports, payloads, or credentials in WP06.
- [ ] Preserve same-UID/peer-PID/nonce, strict schema/size, finite owner count,
      per-call/global deadline, deterministic ordering, and cancellation.
- [ ] Add forged/replayed/cross-Manifest/cross-Workspace/cross-root/stale/
      disappearing/symlink/oversized/duplicate/malformed owner tests.
- [ ] Prove status owner discovery never removes a record/socket; cleanup has a
      separately governed owner.
- [ ] Add Engine/Desktop/Colima integration for batched missing objects, broken
      engine, cluster projection mismatch, attachment churn, migration
      unverified state, and zero mutation.
- [ ] Prove status performs no installation-wide Runtime protection inventory,
      no Runtime journal/receipt creation or recovery, and no build/restore/
      prune/delete Docker or filesystem mutation.

## CLI, Catalog, and presentation

- [ ] Replace the status Catalog shape atomically with Manifest-only inputs,
      fields, failures/recoveries, and generated projections.
- [ ] Keep capability `tobari.lifecycle` unless the integrated ledger requires
      an explicit reviewed replacement; keep `RoleUtility`, `EffectRead`,
      complete scalar delivery, no pagination, and no refs.
- [ ] Render routine `Manifest`, `Current`, `Next entry`, observed runtime,
      bounded failure, cluster projection, independent attention, and Next from
      typed facts only.
- [ ] Render any headline from typed axes as non-authoritative presentation;
      emit no `overall_status`/`adoption_state` in JSON or domain persistence.
- [ ] Render Runtime `ready`, semantic `availability`, observed container state,
      and `last_used` certainty as distinct facts; hide raw Docker identities and
      private snapshot paths from routine and public JSON.
- [ ] Never call desired state current/active/applied without matching
      AppliedEntry and validated observation.
- [ ] Render `--details` from the same result and prove zero extra port calls and
      byte-identical JSON.
- [ ] Keep IDs, complete digests, home, attempt phases, bounds, and sibling
      receipt rows in details/JSON while hiding healthy diagnostics in routine.
- [ ] Show no permission/rule/request/exposure refs; direct discovery to exact
      specialist commands.
- [ ] Show no copy/base/parent/provenance/lineage. Reject `--base`,
      `runtime build --name`, and omitted-name action selection with no alias or
      recovery suggestion that revives them.
- [ ] Reject `--context`, `manifest use`, any project-local current selector,
      raw Runtime action selection, WP04 research-only recovery, and the
      WP05-retired host name.
- [ ] Route reference-bound Runtime recovery through `review runtimes`/owning
      discover with no direct ref output; consume WP08 recursive conformance.
- [ ] Ensure Runtime prune/delete/restore/build appear, if relevant, only as
      typed Next commands; status never invokes them or claims their mutation
      preconditions have been completely checked.
- [ ] Represent primary Next as exact Catalog path plus typed inputs or typed
      non-command guidance; render argv only through common validation/quoting.
      Preserve all ordered Attention items independently.
- [ ] Add routine/details goldens for every required wireframe from the same
      typed corpus and answer key.
- [ ] Add styled-TTY, non-empty-NO_COLOR-TTY, redirected-text, and JSON parity;
      preserve external visible projection and exact opaque values.
- [ ] Prove `wait_for_detach` and `continue_attached` are handled as typed
      non-command guidance, not command paths or empty argv.
- [ ] Update scoped agent help and keep the root index bounded without old
      vocabulary or implementation detail.

## Empty, failure, and recovery tests

- [ ] No installation default: recommended draft, no authority ID/revision,
      null selected receipts, exhaustive same-root siblings including non-empty,
      no inferred selector, and zero state/Docker/owner side effects.
- [ ] Persisted Manifest/no Workspace: desired authority present, applied and
      observed inapplicable, exact entry target preserved.
- [ ] Current axis: desired entry equals AppliedEntry with required consistency;
      separately cover observed stopped and material missing.
- [ ] Pending axis: desired entry differs, prior AppliedEntry remains Current;
      separately cover observed running and material available.
- [ ] Blocked attached: recreation-required pending state, zero Docker mutation,
      no failure receipt, Current remains prior success.
- [ ] Failed none/partial: exact attempted revision/change-state preserved;
      `entry_state=failed` only when latest final attempt matches current desired;
      older failure does not override newer desired.
- [ ] Unknown: no replay; read-only recovery only; observation may classify but
      status never clears the record.
- [ ] Observed drifted/not_observed/unknown and expected unavailable sections:
      no desired/current inference; invalid/incomplete authority is a command
      fault; no cleanup or migration inside status.
- [ ] Runtime ready/image missing: ready remains true, availability is missing,
      observed is separate, `last_used` remains unknown, protection coverage is
      selected-only, and Next uses a Catalog-valid discover/review path.
- [ ] Runtime availability matrix: available/missing/mismatched/unknown/pruned
      does not rewrite desired, AppliedEntry, revision history, or last use.
- [ ] Copy results: fresh Manifest generation 1 and fresh zero-history Runtime
      have no lineage; copy causes no Workspace/adoption/selection state and no
      reconciliation.
- [ ] Cluster projection: desired/applied/observed/failure matrix independent
      from Workspace entry and service availability.
- [ ] Primary Next/Attention: a migration or cluster prerequisite wins primary
      precedence while all pending permission/service Attention remains ordered.
- [ ] Owner summary unavailable: permission/service/attachment facts are
      `not_observed`, never zero.
- [ ] Anchor race/invalid adapter: no partial JSON or text result.

## Migration and compatibility consumer

- [ ] Wait for WP 01's exact predecessor decoder and migration contract; add no
      status-local compatibility reader.
- [ ] Verify retained UUID bytes appear only as final
      WorkspaceManifestID/WorkspaceID identities after migration.
- [ ] Verify migrated ProjectRoot, Workspace home, Runtime IDs/history, learned
      rules, and creation receipt remain correlated without credential reads.
- [ ] Show verified AppliedEntry only when migration evidence proves exact
      desired spec; otherwise preserve explicit bounded migrated-unverified
      evidence without an inferred overall state.
- [ ] Keep cluster desired/applied state separate and require the explicit
      post-migration `cluster up` selected by WP 01.
- [ ] Reject old public fields/flags/commands and all implicit fills, dual reads,
      dual writes, or aliases.
- [ ] Follow WP 01 backup/precondition/rollback decisions; status performs no
      migration or rollback.

## Durable documentation and generated surfaces — implementation phase only

- [ ] After every upstream implementation through WP09 and re-baseline, update
      product status outcome, explicit/installation-default Manifest selector,
      Current/Next entry, exact JSON, empty draft, sibling scope, and Next.
- [ ] Update architecture with the consumer read model, slice activation,
      orthogonal status axes, structured Next/Attention, Runtime ready/
      availability split, consistency anchors, selected observation, and
      measured Docker/owner budgets.
- [ ] Update security with AppliedEntry/attempt observation, no credential
      inference, owner summary, no cleanup/reconciliation, and
      invalid/unavailable distinction.
- [ ] Update harness with desired/applied/observed/failure corpus, migration
      consumer, no-default purity, `0/B_status` budget, no-copy-lineage,
      Runtime availability/unknown-use, no lifecycle mutation, negative
      inference, terminal parity, and agent action handling.
- [ ] Update Catalog, capability/schema ledgers, README, English/Japanese site,
      completion, generated help/reference, and public-vocabulary negative tests
      without hand-editing generated/release files.

## Verification

- [ ] Focused domain/application/CLI/infrastructure tests pass. Evidence:
- [ ] Exact no-default recommended-draft zero-write/zero-call canary passes.
      Evidence:
- [ ] Configured post-WP09 integrated `B_status` Docker call and finite
      owner-call canaries pass.
      Evidence:
- [ ] Desired/applied/observed/failure and slice-activation corpus passes.
      Evidence:
- [ ] WP 02 lineage/base/copy-state negatives and WP 03 readiness/availability/
      unknown-use/no-lifecycle-mutation corpus pass. Evidence:
- [ ] Migration consumer retains IDs and distinguishes verified/unverified
      AppliedEntry without old public vocabulary. Evidence:
- [ ] Owner security/churn/no-cleanup tests pass. Evidence:
- [ ] TTY/NO_COLOR/redirected/details/JSON/agent-help evidence passes.
      Evidence:
- [ ] WP04 research paths, WP05 retired host name, project-current selectors,
      aggregate status enums, and raw/ref-bearing actions are absent. Evidence:
- [ ] One scoped help plus one status invocation closes routine interpretation
      with zero custom parsing and exact command/non-command action handling.
      Evidence:
- [ ] `task check` passes. Evidence:
- [ ] `task security` passes. Evidence:
- [ ] `task public:check` passes. Evidence:
- [ ] `task release:check` passes. Evidence:
- [ ] Relevant runtime/integration profile passes on supported platforms.
      Evidence:
- [ ] Generated diff and final repository status are understood. Evidence:

## Hand off

- [ ] All upstream implementations through WP09, the integrated re-baseline
      gate, and all fixed WP06 acceptance criteria have evidence.
- [ ] No upstream semantic/type/protocol question was silently reimplemented in
      WP06.
- [ ] Durable decisions are promoted and this temporary packet is removed in
      the implementation completion change.
- [ ] Goal becomes `Complete` only after implementation and gates, never after
      this design revision.
- [ ] Handoff explains Current/Next truth, migration, independent activation,
      Runtime readiness/availability, absence of lineage, selected-only
      protection, measured call budgets, trust boundary, checks, and remaining
      risks.
- [ ] Commit and notify control with `WP06_IMPLEMENTATION_COMPLETE` or
      `WP06_IMPLEMENTATION_BLOCKED`, final interfaces, all gate results, exact
      HEAD/status, packet retention/removal, and WP10 readiness.
