# Work Tasks: Make first Workspace entry legible and recoverable

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)
- Active foundation: accepted [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md) and promoted docs/00-04 contracts, with commits `07535a9` and `428812f` as implementation evidence
- Accepted design integrations: ADR 0079's durable Manifest/Runtime one-time copy contract and [WP 03 Runtime Retirement](../runtime-retirement/tasks.md); the latter is not an implementation-start notice
- Decision state: Accepted/Fixed by Product Owner; WP10 production implementation not started
- Fixed prerequisite order: durable Manifest/copy baseline -> WP08 -> WP03 -> WP04 -> WP05 -> WP07 -> WP09 -> WP06 -> WP10 integrated-HEAD gate

This checklist is dependency ordered. Checked items are design/research evidence
only. All production, test, migration, durable-document, and gate work remains
future work and is not authorized by this packet edit.

## Understand and realign

- [x] Confirm integrated shared-checkout identity and protect parallel untracked
      packet work. Evidence: `context.md` records `52a53bc`, promotion commits
      `07535a9`/`428812f`, and four-file scope.
- [x] Re-read `AGENTS.md`, the complete `add-capability` skill, ADR 0079, and
      promoted docs/00-04 contracts at the integrated checkout.
- [x] Classify ADR 0079 and promoted contracts as the accepted upper decision and enumerate all former
      packet contradictions. Evidence: `context.md` evaluation.
- [x] Replace target public Context/work-mode vocabulary with Workspace Manifest,
      routine Manifest, `manifest`, `--manifest`, and
      `workspace_manifest_id`; retain legacy terms only as predecessor facts.
- [x] Adopt the exact three-resource budget and subordinate Runtime revision/
      ProjectRoot model.
- [x] Realign first-use authority to separate Manifest desired, cluster applied,
      Workspace AppliedEntry, observed runtime, and latest bounded failure.
- [x] Realign Workspace selection/authority to `(ProjectRoot,
      WorkspaceManifestID)` and `workspace_id` and remove target
      project/instance identity aliases.
- [x] Record ADR 0079's unresolved revision-retention, Git slice, attached child
      defaults, Docker migration evidence, and migration-stop/attachment
      questions without deciding them.
- [x] Preserve prior fresh-XDG/PTy/Docker observations as exact predecessor
      evidence and avoid unsafe full-start mutation of active resources.
- [x] Re-read ADR 0079's durable one-time copy contract and accepted WP 03
      evidence; retain target-specific copy, no provenance/lineage,
      reference-bound build, Runtime Review, availability/restore, and
      explicit-only retirement decisions without starting WP10 implementation.
- [x] Record that commits `07535a9` and `428812f` promoted the durable
      Manifest/copy domain/schema/Catalog/migration surfaces, and preserve the
      shared checkout by editing only this packet's four files.
- [x] Read fixed WP04/WP05/WP07/WP08/WP09/WP06 interfaces and record their
      release/research, host-name, wait-only, Catalog traversal/output,
      service-lifecycle, and status Next/Attention ownership without
      pre-implementing them.
- [ ] Mandatory implementation-start gate after the complete upstream sequence:
      fetch integrated main, record actual HEAD/worktree ownership, re-read all
      promoted contracts, inspect every final interface and safely observed
      binary, then update or report `WP10_BLOCKED` before production edits.
- [ ] Measure cache-warm/cold final standard Runtime/Gateway preparation in an
      isolated Engine at implementation HEAD.
- [ ] Observe real stopped-Colima behavior only in a disposable environment.

## Blocking upper dependencies

- [x] Durable Manifest/copy production and contracts are promoted. Evidence:
      ADR 0079, docs/00-04, commits `07535a9` and `428812f`.
- [ ] WP08, WP03, WP04, WP05, WP07, WP09, and WP06 production and durable
      promotion complete in the fixed order. WP10 preempts none of their
      production/schema/Catalog/migration/generated surfaces.
- [ ] The final integrated-HEAD gate records real HEAD/status/ownership and
      verifies that no intermediate interface is treated as final.
- [x] Durable WorkspaceManifestID/revision/activation, Runtime exact binding,
      ProjectRoot, WorkspaceID, AppliedEntry, failure-attempt, and observed-state
      domain contracts are available through ADR 0079 and promoted docs/00-04.
- [x] Durable Manifest/Workspace stores and exact unpublished-predecessor migration
      direction are accepted; no mixed Context/Manifest reader is introduced.
- [ ] Owner decides current-only versus one-previous Manifest revision body.
      Progress consumes semantic identities but does not add public history.
- [ ] Owner decides Git fallback slice membership before exact entry revision
      and advanced detail fixtures are frozen.
- [ ] Owner decides new-child session defaults while entry adoption is pending
      on an attached Workspace; this journey does not infer live adoption.
- [ ] Security/runtime owners approve sufficient read-only Docker evidence for
      migration AppliedEntry synthesis.
- [ ] Security/runtime owners approve cluster-stop/no-attachment migration
      preconditions and rollback.
- [ ] Final Catalog supplies exact faults/next argv for migration required/
      incomplete, attachment block, compatibility projection invalid, and
      predecessor rejection.
- [x] Promoted copy surfaces supply exactly `manifest create --copy-from`
      and `runtime create --copy-source-from`, no `--base` alias or provenance,
      fresh independent identities, and zero reconciliation. Evidence: ADR 0079,
      the product contract, and commit `07535a9`.
- [ ] Implemented WP 03 surfaces supply `review runtimes`, reference-producing
      Runtime reads/create, `runtime build --id`, typed revision availability,
      `runtime restore --id`, and exact restore faults/recovery. If they are not
      yet implemented, coordinate their owner-approved order rather than adding
      WP10-local substitutes.
- [ ] WP08 supplies one recursive Catalog output/reference traversal, final
      enums, structured recovery representation, and
      `output_encoding_failed -> version`; WP10 adds no parallel walker.
- [ ] WP04 supplies release/research surface separation; release first-use has
      no research auth/serve path.
- [ ] WP05 supplies `host.tobari.internal` only to its owned outcome; WP10 adds
      no unrelated stage vocabulary.
- [ ] WP07 supplies wait-only permission recovery without policy authority;
      WP10 treats it only as final child/status attention.
- [ ] WP09 supplies service exposure/open/cleanup authority; WP10 does not merge
      approval or attachment cleanup semantics into setup stages.
- [ ] WP06 supplies orthogonal status facts, one typed primary Next, ordered
      Attention, no-reference status, and terminating non-command guidance.
- [ ] Cross-packet owners resolve Manifest/status/Runtime/release/auth/
      permission/Host Loopback/service-exposure interfaces before production
      merge.

## Decide first-use contract

- [x] Keep one root `tobari` routine outcome and reject separate required
      init/build/enter commands.
- [x] Choose an invocation-scoped typed journey, not a SetupRun resource,
      onboarding flag, duplicate journal, or background controller.
- [x] Define the five semantic stages and desired/applied/observed checkpoints.
- [x] Define owner/scope/lifetime/mutability/authority for draft, Manifest,
      Runtime, cluster projection, Workspace, home, attachment, journey, and
      Docker conflict observation.
- [x] Update wireframes, fault matrix, Ctrl-C contract, schema/migration
      direction, alternatives, and human-handoff scorecard.
- [x] Update Customize and recovery design to ADR 0079/WP 03: fresh draft is not a
      copy, Runtime create does not build, missing image availability points to
      reference-producing `review runtimes`, restore remains exact-or-fail, and
      root never invokes a Runtime lifecycle mutation.
- [x] Fix fresh/default semantics: publish Manifest `default`, cross a separate
      default-set mutation checkpoint, retain publication across selector
      failure, and use existing selection for nonempty/no-default state.
- [x] Fix review outside exactly five stages, the exact five labels, exact seven
      stage states, and checkpoint-local checkmarks.
- [x] Fix line-first progress timing, automatic 10-second disclosure, bounded
      redirected heartbeat, and no live details key/second interface.
- [x] Fix pre-handoff Ctrl-C/exit 130 and post-handoff exact child ownership.
- [x] Fix direct shell/Claude/Codex/gh login paths and native authentication
      wording with no release research-auth leakage.
- [x] Fix structured Next, attached `end_active_session`, causal migration/
      cluster/Workspace/provider recovery, and repository-wide graph audit.
- [x] Correct the former false semantic-compatibility claim: unaffected final
      contracts are pinned, while intentional predecessor/final identity,
      status, fault, and recovery changes receive explicit fixtures.
- [x] Product Owner approved the exact stage labels and Current/Next semantics.
- [x] Product Owner rejected a V1 live details key and fixed automatic bounded
      disclosure/failure plus later `status --details`.
- [x] Product Owner fixed the one-line fresh-shell native-login guidance and no
      ordinary re-entry/direct-command banner.
- [ ] Decide whether simultaneous XDG installations are unsupported or require
      a separate installation-identity packet. Do not widen this packet.
- [x] Product Owner accepted the recovery constraints and one structured primary
      Next model; final implemented codes/typed inputs are re-baselined later.
- [ ] Validate routine labels `Copy from Manifest` and `Copy source from`; reject
      Base/clone/fork/lineage wording and any saved-source presentation.

## Contract and semantic fixtures

- [ ] Promote approved first-use/progress consequences only after the complete
      fixed upstream sequence and integrated-HEAD gate.
- [ ] Add a presentation-evidence file from the current template for the future
      implementation change.
- [ ] Add one typed project-entry fixture carrying task identity, ProjectRoot,
      WorkspaceManifestID, desired Manifest/slice identities, cluster desired/
      applied, WorkspaceID/AppliedEntry, bounded observation, attachment,
      latest failure, stage, wait reason, retained facts, structured Next, and
      ordered Attention.
- [ ] Add answer-key variants for fresh draft, desired-only Manifest, cluster
      lag, first Workspace, current, pending entry, attached-blocked,
      failed-none/partial/unknown, drifted, unavailable, migration incomplete,
      unverified AppliedEntry, compatibility invalid, and upgrade.
- [ ] Add fresh empty-catalog/no-default, publication then default selection,
      publication-success/default-failure, default partial/unknown, nonempty/no-
      default selection, invocation-only explicit Manifest, and nonexistent
      explicit Manifest variants.
- [ ] Add ADR 0079 copy-contract answer-key variants for fresh non-copy publication, Manifest
      current-revision copy revalidation, Runtime editable-source copy with
      fresh ID/empty history, and copy failure with zero reconcile/provenance.
- [ ] Add WP 03 answer-key variants for ready+available, ready+missing,
      restored, already available, unrestorable digest mismatch, and
      interrupted/unknown restore while preserving immutable history and prior
      AppliedEntry.
- [ ] Add negative-inference canaries proving names, generations, ordinals,
      image tags, timestamps, labels, row order, checkmarks, elapsed time,
      diagnostics, and Docker presence cannot create identity/equality/
      completeness/replay permission.
- [ ] Pin final unaffected Catalog/schema/exit/child contracts and add exact
      predecessor/final fixtures for every intentional rename or recovery delta.
- [ ] Prove routine-success external-processing count is zero.

## Domain and application

- [ ] Add a closed project-entry journey task identity, stage/status/wait-kind,
      and retained desired/applied/observed/failure types that consume ADR 0079
      identities rather than recreating them.
- [ ] Reject illegal stage transitions, wrong scope/identity, multiple running
      stages, any value outside the fixed seven-state enum, applied checkmark
      without its exact receipt, and wait reason on a non-running stage.
- [ ] Extend the Workspace-entry application owner with the smallest canonical
      Manifest/cluster/Workspace/attachment ports required by the journey.
- [ ] Prove application imports no infrastructure/CLI and parses no Docker
      diagnostics or presentation.
- [ ] Keep review outside progress. Compose requirements, Manifest create,
      separate default set, persisted selection, cluster up, Workspace entry,
      and attachment without duplicating mutation/receipt/recovery policy.
- [ ] Prove `resolve_manifest` succeeds on fresh state only after confirmed
      create and confirmed default set; retain create across selector failure.
      Existing selection performs neither mutation unless explicitly setting
      the missing installation default.
- [ ] Keep recommended fresh Manifest publication separate from ADR 0079 copy
      initialization. Consume target-specific copy ports only for Customize and
      retain no source/provenance/lineage in journey results.
- [ ] Consume Runtime references and availability from canonical WP 03 results;
      never decode references, infer availability from `ready`, or inspect
      Docker in application code.
- [ ] Preserve each confirmed mutation-complete result before invoking the next
      boundary; later cancellation cannot turn success into replay permission.
- [ ] Emit desired Manifest, cluster applied, Workspace AppliedEntry, observation,
      and failure independently; no progress checkmark implies another boundary.
- [ ] Enforce that only entry reconciles Workspace runtime and only cluster up
      reconciles shared state. Read ports cannot expose repair methods.
- [ ] Preserve previous AppliedEntry and record exact attempted revision across
      entry none/partial/unknown outcomes.
- [ ] Reject recreating pending adoption while attached before any Docker call.
- [ ] Return typed `end_active_session` for attached recreation; after external
      detach return typed `tobari`, never `status` polling.
- [ ] Reject Workspace entry before mutation when the selected exact Runtime
      revision is ready but unavailable; preserve prior AppliedEntry and return
      the Catalog-owned `review runtimes` handoff.
- [ ] Preserve exact-or-fail restore results: success changes availability only;
      unrestorable/mismatch/unknown never appends or rewrites Runtime history or
      mutates Manifest/Workspace state.
- [ ] Normalize root Customize cancellation to Next `tobari` without retaining
      a public predecessor namespace.
- [ ] Compose WP06 structured primary Next/Attention and WP08 Catalog traversal;
      no free-form command string or WP10-local reference walker exists.
- [ ] Add table-driven application tests for every fault and Ctrl-C row.

## Infrastructure

- [ ] Add a narrow read-only observation for fixed owner-labeled Docker
      resources unbound from trusted final state.
- [ ] Prove conflict observation writes nothing, synthesizes no IDs/receipts,
      adopts nothing, and authorizes no cleanup.
- [ ] Add only missing fixed, secret-free signals for local component build,
      cluster projection application/health, Workspace entry reconciliation,
      and attachment handoff.
- [ ] Add monotonic timing/heartbeat support for measured 250 ms anti-flicker,
      1 s elapsed, maximum 4 Hz redraw, 10 s automatic substage/wait disclosure,
      and redirected heartbeat no more often than 30 s.
- [ ] Keep Docker/BuildKit output in a separate bounded visibly projected
      writer; add hostile control/truncation tests.
- [ ] Make canonical interrupted-cluster faults retain validated journal
      operation and publish one matching Next: up never offers down, down never
      offers up, neither self-loops through status.
- [ ] Prove corrupt/ambiguous journal/receipt state fails closed to read-only
      doctor/classification and never selects an opposite write.
- [ ] Prove attached block precedes every recreating Docker action.
- [ ] Prove final exact RuntimeID/revision digest is resolved before mutation;
      tag/name/ordinal/image ID cannot substitute.
- [ ] Prove availability observation is bounded and typed by the WP 03 adapter;
      root never starts `runtime build`, restore, prune, or delete as an
      infrastructure shortcut.
- [ ] Prove compatible binary upgrade preserves Manifest/Workspace IDs, homes,
      prior applied receipts, and bounded failure while explicit actions apply
      new trusted revisions.
- [ ] Prove standard authentication, learned permission, attachment authority,
      native credentials, provider helpers, and Colima management remain outside
      Manifest desired/applied and journey state.
- [ ] Observe stopped Colima/Desktop/Engine in disposable environments; project
      provider hint as bounded evidence only, return typed external-start
      condition, and emit no provider command.

## CLI and Catalog

- [ ] Implement stable line-oriented semantic output first; add TTY redraw only
      as a presentation optimization with identical answers.
- [ ] Render one running stage using the fixed timing thresholds, truthful wait
      reason, desired/applied/observed/failure state, retained facts, Ctrl-C
      meaning, and no percent/ETA/raw logs.
- [ ] Put validated IDs/digests and projected diagnostic tail only on failure
      and later `status --details`; add no live key, public progress input,
      persisted preference, event resource, or second registry.
- [ ] Stop rendering before child handoff and preserve exact raw streams,
      signals, resize, argv, and exit status.
- [ ] Consume final `manifest` / `--manifest` Catalog surface and remove all
      public Context aliases, synthetic persisted defaults, generic import,
      project Manifest discovery, and `apply -f` paths.
- [ ] Customize consumes only `manifest create --copy-from` and `runtime create
      --copy-source-from`; reject `--base`, generic copy/derive flags, and all
      provenance/lineage fields in human/JSON/status output.
- [ ] Runtime-create success hands human users to `review runtimes`; direct
      build uses the produced opaque ref with `runtime build --id`. Reject
      `--name` and omitted-target action selection.
- [ ] Render ready authority and image availability independently. Missing
      availability has exactly one `review runtimes` Next; its produced ref
      enables restore. Restore progress says rebuild/verify and never promises
      success.
- [ ] Prove no first-use/root Catalog path or internal composition invokes
      `runtime prune dry-run/apply`, `runtime delete`, revision delete, generic
      GC, Docker prune, or force.
- [ ] Keep root fixed-CWD RoleAct/EffectCreate and expose every composed final
      fault/structured Next through Catalog without unchecked/free-form argv.
- [ ] Preserve `tobari [--manifest NAME] [-- <command>...]`; validate bare `--`,
      selector placement, and argv before any effect, then pass executable and
      every later value unchanged without shell parsing/logging/persistence.
- [ ] Before handoff, first Ctrl-C cancels one operation, waits for bounded
      classification, renders retained facts/Next, and exits 130. After handoff,
      preserve exact child terminal/streams/signals/exit and bounded cleanup.
- [ ] Render one non-blocking native credential-location line only for a fresh
      Workspace shell; no re-entry/direct banner and no WP04 research auth/
      serve surface in release output/help/Next.
- [ ] Keep `status`, `cluster status`, `manifest list/show`, and doctor strictly
      read-only with zero convergence side effects.
- [ ] Add Current/Next entry, attached-blocked, migration, invalid projection,
      pending/failed/unknown, and direct-child CLI tests from the same fixture.
- [ ] Generate and validate the complete integrated recovery graph: reject
      self-loop, nonexistent path, unchecked inputs, action rediscovery, closed
      reference cycle, opposite write, unknown replay, and classifier chains
      that do not terminate causally. Audit `output_encoding_failed -> version`
      and every continuation.
- [ ] Verify root agent index remains bounded and scoped help exposes complete
      final Manifest/Workspace contracts.
- [ ] Add public-vocabulary tests rejecting `context`, `--context`,
      `context_id`, `project_id`, `instance_id`, and ProjectInstance in final
      public/schema output, while exact predecessor migration fixtures retain
      byte identity privately.

## Recovery and migration scenarios

- [ ] Run this section only after the mandatory post-WP06 integrated gate;
      record actual HEAD/status and every final upstream surface used.
- [ ] Fresh state/no Docker resources: one review, no-authority draft, Manifest
      `default` publish, separate default set, protection apply, Workspace
      create/entry, native-login line, and attachment.
- [ ] Publication success plus default-set fail/cancel/partial/unknown: retain
      Manifest, preserve prior/no default, reconcile through typed Manifest
      reads/default set, and never blind-replay root.
- [ ] Nonempty/no-default, explicit existing Manifest, and nonexistent explicit
      Manifest: no synthetic recommendation, invocation-only selection, and
      zero cluster/Workspace effect on invalid selection.
- [ ] Fresh state plus unbound owner resources: zero mutation,
      `change_state=none`, one doctor Next, no adopt/delete.
- [ ] Docker CLI missing, incompatible/unreachable Engine, invalid context, and
      Compose missing: zero state, doctor classification, typed external-start
      condition, no provider command, then `tobari` after availability.
- [ ] Cancel review, Customize, Manifest create, default set, pre-journal build,
      cluster build/start/health, Workspace entry, and pre-attach; assert one
      operation canceled, bounded classification, retained facts, exit 130.
- [ ] After handoff, assert exact child Ctrl-C/nonzero status and streams while
      bounded attachment cleanup completes without status rewriting.
- [ ] Customize fresh publication versus Manifest copy versus Runtime source
      copy: prove exact flags, fresh identities, no Base/provenance, and no
      cluster/Workspace reconciliation.
- [ ] Create a Runtime source, prove empty history, select via `review runtimes`,
      and hand the produced reference unchanged to `runtime build --id`.
- [ ] Interrupt cold standard build and resume only through the exact owning
      boundary/journal while preserving safe cache and prior receipts.
- [ ] Recover schema-1 cluster up/down journals without competing/self-looping
      action; status observes only.
- [ ] Publish a new desired Manifest while Workspace remains on prior AppliedEntry;
      prove desired change alone mutates no runtime.
- [ ] Block attached required recreation with zero Docker mutation, show
      Current/Next, return `end_active_session`, end externally, then return
      `tobari`; never poll status.
- [ ] Reconcile entry none/partial/unknown through prior AppliedEntry and
      read-only status barrier.
- [ ] Remove the selected revision's image material in an isolated Engine;
      prove entry performs zero implicit build, returns `review runtimes`, and
      preserves desired/cluster/prior AppliedEntry.
- [ ] Exercise restore already-available, exact restored, digest mismatch/
      unrestorable, cancellation, and partial/unknown journal results; assert
      exact Next, no history rewrite, and no automatic root continuation.
- [ ] Exhaust fault paths and prove root never invokes Runtime prune/delete,
      revision deletion, generic/global Docker cleanup, or force.
- [ ] Exercise invalid/missing cluster compatibility projection and binary
      upgrade through explicit cluster up before entry.
- [ ] Exercise known up/down journals and contradictory journal; assert same
      operation or read-only doctor, never opposite write/self-loop.
- [ ] Exercise exact predecessor migration required/rejected/incomplete,
      unverified AppliedEntry, post-migration cluster up, and later explicit
      entry using ADR 0079's selected evidence/preconditions.
- [ ] Safe migration-required returns typed `migrate apply`; unsafe/incomplete/
      unknown returns read-only diagnosis. Root never migrates implicitly.
- [ ] Force representative `output_encoding_failed` results; assert empty
      success output, Next `version`, no retry of the failing command, and a
      finite audited continuation.
- [ ] Run shell, Claude, Codex, and `gh auth login` direct paths plus bare `--`/
      malformed argv; assert exact argv/streams/status and zero pre-parse effect.
- [ ] Retain legacy UUID bytes as final WorkspaceManifestID/WorkspaceID and
      reject public dual vocabulary/ordinary dual readers.
- [ ] Roll back via ADR 0079's owner-only backup procedure; prove journey persists
      no state that blocks the prior compatible renderer.

## Documentation and harness

- [ ] Update product/architecture/security/harness with only the additional
      first-use/repository-recovery consequences after all upstream promotion.
- [ ] Update README quick start and Catalog-derived help with Manifest wording,
      first-build wait, desired/applied Current/Next, Ctrl-C retention,
      provider-neutral readiness, native login, and direct command.
- [ ] Update site and agent readiness with shell/Claude/Codex/gh forms,
      structured Next/Attention, release-only auth vocabulary, and zero external
      parser/source inspection.
- [ ] Document Customize with ADR 0079's exact copy flags and Runtime create/build
      handoff, and document missing availability with WP 03 exact-or-fail
      restore without promising reproducibility.
- [ ] Add parent-owned PTY evidence outside the repository with fixed dimensions,
      scheduled inputs, redaction, and raw digest.
- [ ] Add isolated Docker evidence for warm/cold build, Engine loss,
      interrupted cluster, attached adoption, partial/unknown entry, migration,
      upgrade, and XDG/resource collision.
- [ ] Measure cold/warm standard Runtime and Gateway preparation on supported
      Docker Engine, Docker Desktop, and Colima setups; record versions,
      medians, ranges, and sample counts as evidence, never public ETA.
- [ ] Add the human-handoff scorecard and safety/certainty rationale to future
      presentation evidence.
- [ ] Replay the final agent journey within discovery budget and zero routine
      external processing.
- [ ] Recheck external comparison sources for drift; embed no external assets.

## Verify

- [ ] Focused domain/application/infrastructure/CLI/Catalog tests pass. Evidence:
- [ ] Desired/applied/observed/failure fixtures and negative-inference canaries
      pass. Evidence:
- [ ] ADR 0079 copy/no-provenance/no-reconcile and WP 03 reference/build/
      availability/restore/no-cleanup integration fixtures pass. Evidence:
- [ ] Fresh-state/read-only zero-write and hostile-output suites pass. Evidence:
- [ ] PTY routine/details/failure/recovery/cancel/handoff evidence passes. Evidence:
- [ ] Line/TTY timing, maximum 4 Hz redraw, 10 s disclosure, 30 s redirected
      heartbeat, no live details key, and exact exit-130/child-status tests pass.
      Evidence:
- [ ] Machine-checked complete Catalog recovery graph and
      `output_encoding_failed` cross-audit pass. Evidence:
- [ ] Direct shell/Claude/Codex/gh login and native-auth banner fixtures pass.
      Evidence:
- [ ] Isolated Docker/Colima/migration matrix passes without developer-resource
      mutation. Evidence:
- [ ] `task check:fast` passes during implementation. Evidence:
- [ ] `task check` passes as the completion gate. Evidence:
- [ ] `task security` passes. Evidence:
- [ ] `task public:check` passes. Evidence:
- [ ] `task release:check` passes unconditionally. Evidence:
- [ ] Catalog/schema/site/generated diffs are understood and regenerate cleanly.
      Evidence:
- [ ] Repository status preserves all unrelated parallel changes. Evidence:
- [ ] Gate evidence records the post-WP06 integrated implementation HEAD, not predecessor
      `6a26a3c` or an intermediate dirty-worktree snapshot. Evidence:

## Hand off

- [ ] Every acceptance criterion has evidence.
- [ ] Every implementation-time upstream question used by this slice has a
      landed owner decision/interface; WP10 records dependencies rather than
      reopening upstream authority.
- [ ] Conflicting predecessor Context/first-release packet conclusions are
      superseded or integrated; no mixed first-use contract remains.
- [ ] Durable first-use conclusions are promoted after every upstream contract;
      no policy remains only in this packet.
- [ ] Temporary PTY/Docker/timing/diagnostic artifacts are absent from the repo.
- [ ] Goal is never marked Complete while temporary or before all gates pass.
- [ ] This temporary packet is deleted in the same handoff that completes the
      implementation.
- [ ] Completion is committed, then the root owner receives
      `WP10_IMPLEMENTATION_COMPLETE` with final interfaces, all gate evidence,
      HEAD/status, retention, and cross-audit readiness; unresolved integration
      sends `WP10_BLOCKED` with exact evidence instead.
- [ ] Handoff reports user outcome, concept budget, Manifest desired versus
      cluster/Workspace applied state, identities, schema/migration, trust
      boundary, dependencies, checks, and remaining cross-cutting risks.
- [ ] Handoff also reports ADR 0079 exact copy flags/no provenance, WP 03
      reference-bound build and exact-or-fail restore, and proof that root owns
      no implicit Runtime prune/delete path.
