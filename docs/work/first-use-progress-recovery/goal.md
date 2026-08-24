# Work Goal: Make first Workspace entry legible and recoverable

- Status: Accepted
- Decision state: Fixed by Product Owner; production implementation not started
- Retention: temporary
- Retention reason: None
- Governing contract: accepted [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md), the promoted [theses](../../00_theses.md), [product contract](../../01_product_contract.md), [architecture](../../02_architecture.md), [security model](../../03_security_model.md), and [harness](../../04_harness.md), plus the standard-authentication consequences in `docs/07_authentication.md` through `docs/09_agent_readiness_validation.md`; commits `07535a9` and `428812f` are the fixed promotion evidence
- Review/delete trigger: Delete after every fixed upstream implementation through WP06 is complete, this packet has been revalidated against the resulting integrated HEAD/worktree and final interfaces, WP10 implementation passes every required gate, and durable first-use conclusions are promoted
- Successor: None
- Owner: Product/domain/system design owner for Workspace entry and lifecycle recovery
- Target: A final integrated first-use/recovery-polish implementation after the fixed durable Manifest/copy baseline (`07535a9`, `428812f`) -> WP08 -> WP03 -> WP04 -> WP05 -> WP07 -> WP09 -> WP06 order and a mandatory integrated-HEAD re-observation gate; this packet authorizes no production implementation
- Related ADRs: ADR 0077 causal failure recovery, ADR 0078 as predecessor activation evidence superseded by ADR 0079's typed activation, and accepted ADR 0079
- Active foundation: [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md) and its promoted docs/00-04 contracts, implemented by promotion commits `07535a9` and `428812f`
- Completed design integrations: ADR 0079's durable one-time Manifest/Runtime copy contract, [ADR 0080 Runtime lifecycle](../../decisions/0080-close-the-managed-runtime-lifecycle.md), [ADR 0081 permission resume](../../decisions/0081-observe-reviewed-permission-from-an-attached-workspace.md), and [ADR 0082 build surfaces](../../decisions/0082-release-and-research-build-surfaces.md)
- Remaining fixed upstream integrations: completed WP08 Catalog/output contracts in [Architecture](../../02_architecture.md) and [Harness](../../04_harness.md), completed WP05 [ADR 0083 Host Loopback authority](../../decisions/0083-name-the-physical-host-loopback-authority.md), completed WP09 [ADR 0074 Service exposure](../../decisions/0074-expose-one-reviewed-workspace-service.md), and promoted [ADR 0085 CWD status home](../../decisions/0085-make-status-the-cwd-home.md)

The repository guard does not define a `Ready` status; `Accepted` is the
non-terminal repository status used here for a packet ready for future
implementation planning. The implementation remains unstarted.

## Outcome

From a project directory, an ordinary user runs `tobari`, reviews one safe
non-authoritative recommended draft, and reaches the first usable Workspace
without having to understand image, Gateway, OPA, Docker Compose, principal, or
journal vocabulary. `Start Workspace` first publishes one persisted Workspace
Manifest and complete immutable desired revision, then composes the separately
authoritative `manifest default set --name default` mutation, `cluster up`
application of the protection projection, and explicit Workspace entry before
attaching the child session. Routine progress may group publication and default
selection under `Save Manifest`, but their mutation-complete checkpoints and
failure recovery remain distinct.

During a long first run or re-entry, Tobari distinguishes:

- the current desired Manifest revision;
- the last successfully applied shared-cluster projection;
- the Workspace's last successfully applied entry revision;
- bounded observed runtime state; and
- the latest bounded failure or unknown attempt.

Every cancellation or Tobari failure reports confirmed retained facts and one
immediate catalog-valid `Next`. Rerun either invokes the exact idempotent
mutation boundary recorded by a journal or performs read-only observation
before another write. Status, list, show, and doctor never reconcile state.

Customize may initialize a new independent Manifest through `manifest create
--copy-from`, or a new independent Runtime source through `runtime create
--copy-source-from`, but the ordinary first-use recommendation remains a fresh
draft publication with no copy provenance. Runtime creation never implies a
build. Human build selection is `review runtimes`, direct build is `runtime
build --id`, and entry never silently builds, prunes, deletes, or retires a
Runtime.

The review screen precedes progress. After acceptance, exactly five semantic
stages run: `check_requirements`, `resolve_manifest`, `prepare_protection`,
`prepare_workspace`, and `enter_workspace`. A stage checkmark proves only its
own checkpoint; it never implies a later mutation succeeded.

## Upper decision incorporated

- The public resource is **Workspace Manifest**, routine label **Manifest**,
  CLI namespace/flag `manifest` / `--manifest`, and schema identity
  `workspace_manifest_id`. No public Context alias remains.
- The public durable-resource budget is exactly Workspace Manifest, Runtime,
  and Workspace. Runtime revision and Project root are subordinate concepts.
- A Workspace Manifest is a host-owned, CLI-managed, stable-ID desired
  declaration. It is not project YAML, generic import, or `apply -f`.
- Each accepted Manifest mutation publishes a complete immutable desired
  revision. Boundary is invariant under one WorkspaceManifestID.
- Activation is item-specific: cluster projection at explicit `cluster up`,
  entry definition at explicit Workspace entry, session defaults at a new child
  session, and creation defaults only for a new Workspace.
- A Workspace is durably bound by `(ProjectRoot, WorkspaceManifestID)` and has
  one authoritative `workspace_id`. Legacy `project_id` and `instance_id` do
  not survive the final public model.
- A Manifest binds an exact RuntimeID plus semantic Runtime revision digest.
- Standard authentication, learned permission state, and attachment authority
  remain outside Manifest desired/applied state.

## Accepted sibling decisions incorporated

- Manifest copy is `manifest create --copy-from NAME --name NAME`; Runtime
  editable-source copy is `runtime create --copy-source-from standard|NAME
  --name NAME`. `--base` has no alias, and routine wording does not say Base.
- Both copy modes create fresh independent identities and retain no
  provenance, lineage, `copied_from`, Workspace, authentication, permission,
  attachment, applied, failure, observation, or current-selection state.
- The fresh recommended Manifest draft is not a copy operation and creates no
  provenance field or hidden copy receipt.
- A created Runtime has a fresh RuntimeID and empty history. Its human build
  handoff is `review runtimes`; a direct build consumes the produced Runtime
  reference through `runtime build --id`.
- Runtime revision authority/readiness and local image availability are
  distinct. When the selected exact revision is ready but its image is missing,
  entry does not build implicitly; it directs the user to explicit `runtime
  restore`.
- Restore is exact-or-fail from a retained immutable snapshot. It may return
  unrestorable without changing revision history and must never be promised as
  guaranteed recovery.
- Root/first-use never invokes Runtime prune or delete. WP 03 owns their exact
  explicit lifecycle commands and protection graph.

## Fixed WP10 corrections

- Fresh means both an empty Manifest catalog and no installation default. The
  only review actions are `Start Workspace`, `Customize`, and `Cancel` for one
  no-authority recommended draft named `default`.
- Start publishes Manifest `default`, then separately sets the installation
  default through the canonical typed `manifest default set` mutation. A
  publication success survives default-selection failure/cancellation.
- A non-empty catalog with no default never synthesizes another recommendation
  or infers selection from a Workspace. It presents one existing-Manifest
  selection/recovery screen. `--manifest` is invocation-only and never changes
  the installation default.
- The progress stage state enum is exactly `pending`, `running`, `succeeded`,
  `skipped`, `blocked`, `failed`, or `unknown`. Review is outside progress; there is no
  `reconcile_required` stage state.
- Stable line-oriented stderr is primary. TTY redraw is presentation only:
  current-stage anti-flicker target 250 ms, elapsed after 1 s, redraw no faster
  than 4 Hz, automatic bounded substage/wait reason after 10 s, and redirected
  heartbeat at most every 30 s. V1 has no live details key.
- Before child handoff, first Ctrl-C cancels the one canonical current operation,
  waits for bounded classification, preserves mutation-complete success, prints
  retained facts/Next, and exits 130. After handoff the child owns terminal,
  streams, signals, and exact exit status.
- Direct entry `tobari [--manifest NAME] [-- <command>...]` is a first-class
  happy path. The positional-only marker is mandatory; argv is passed byte-for-
  byte without shell parsing, logging, persistence, or reconstruction.
- Standard authentication remains native inside the Workspace home. Fresh shell
  creation may show one non-blocking credential-location line; direct agent
  commands use native prompts and no Tobari banner. WP04 research auth commands
  never appear on the release first-use/recovery surface.
- Missing/mismatched/pruned selected Runtime material points first to the
  reference-producing `review runtimes`, never a bare or unchecked `runtime
  restore`. Root invokes no Runtime lifecycle mutation.
- Primary Next is a typed Catalog task with typed inputs or a typed non-command
  condition. WP10 owns a machine-checked integrated recovery graph, including
  causal classifier termination and the repository-wide
  `output_encoding_failed` handoff to read-only `version` diagnosis.

## Why now

The predecessor main baseline already closes much of the happy path through a recommended
review, legacy Context creation, shared-cluster reconciliation, Workspace
reconciliation, and attachment. It presents those operations as disconnected
messages rather than one truthful desired/applied/observed journey. Cluster
progress begins only after the legacy Context write, hides local build detail,
does not cover Workspace entry, and cannot explain what `Ctrl-C` retains.

That predecessor recovery also has two concrete defects: Customize cancellation exits
the root journey through legacy `context create`, and an interrupted-cluster
status can recommend the same failing `cluster status` instead of the journal's
recorded `up` or `down` operation.

Fresh-XDG observation exposed a separate safety case: logical state was empty
while owner-labeled Tobari Docker resources from another state tree were
running. First-use must fail closed before mutation rather than adopt Docker
resources or treat observation as a second authority.

ADR 0079 and the promoted product contract add the missing semantic requirement: progress cannot merely say that
"setup was saved." It must say which desired Manifest exists, whether the
cluster projection has been applied, and which entry revision the Workspace is
actually using.

Promotion commits `07535a9` and `428812f` changed the Manifest/schema/Catalog/
migration surfaces this packet will eventually consume. The old main revision
and predecessor observations remain historical evidence only. They are not an
implementation baseline: WP10 must re-fetch and inspect the actual integrated
HEAD after the remaining fixed upstream sequence before any production work
begins.

## Non-goals

- Do not implement production code, tests, durable contracts, CLI, migration,
  generated artifacts, or release changes in this packet-authoring change.
- Do not re-open the Workspace Manifest naming, public concept budget, absence
  of Context aliases, item-specific activation, or explicit reconciliation
  decisions accepted by ADR 0079 and promoted in docs/00-04.
- Do not add a second setup database, onboarding-complete flag, persisted
  progress record, duplicate journal, or Docker-derived logical state.
- Do not add public `init`, `setup`, `resume`, `repair`, `login`, generic
  `manifest apply`, `--file`, project Manifest discovery, or provider-specific
  Docker/Colima commands.
- Do not make Tobari install, start, stop, select, or reconfigure Docker,
  Colima, Docker Desktop, Podman, or another engine provider.
- Do not import host credentials/configuration, add authentication or learned
  permission to Manifest state, or make login a host-side setup stage.
- Do not add raw Docker logs to structured faults or routine output.
- Do not change exact direct-command argv, shell interpretation, child exit
  status, Workspace lifetime, access Boundary, or mutation authority.
- Do not promise percentages or completion-time estimates without an
  authoritative bounded total.
- Do not add a live `d` key/details toggle, second progress interface, persisted
  presentation preference, event resource, raw log view, percent, or ETA.
- Do not decide ADR 0079's remaining revision-retention, Git-slice, new-child
  session-default, Docker-migration-evidence, or migration-stop/attachment
  questions.
- Do not solve simultaneous independent XDG installations sharing the fixed
  installation owner. This slice detects and explains conflict; a broader
  installation identity requires a separate upper-level decision.
- Do not edit or duplicate the durable Manifest/copy production types, stores,
  schema, Catalog, migration, or root composition from this packet.
- Do not reintroduce `--base`, persist or display copy provenance/lineage, or
  treat the recommended fresh draft as a copy of a synthetic default.
- Do not make Runtime create build, reconcile, or select a Workspace; do not
  make first-use invoke `runtime prune`, `runtime delete`, or implicit Runtime
  retirement.
- Do not promise that `runtime restore` succeeds. Missing or changed external
  build inputs may make the retained revision unrestorable without changing its
  immutable history.

## Acceptance criteria

- [ ] A fresh interactive `tobari` presents one recommended review with exactly
      `Start Workspace`, `Customize`, and `Cancel`; the draft has no
      WorkspaceManifestID and is never rendered by `manifest list/show` as
      persisted authority.
- [ ] Fresh requires an empty Manifest catalog and no installation default.
      Start publishes fresh Manifest `default`, then separately calls canonical
      `manifest default set` with typed name `default`. The routine Save stage
      succeeds only after both mutation-complete checkpoints.
- [ ] If publication succeeds but default selection fails/cancels, retain the
      Manifest, report the default unchanged, and return typed Next
      `manifest default set` with `{name: default}`; human rendering is
      `tobari manifest default set --name default`. Never roll back or pretend
      the Manifest is selected.
- [ ] A non-empty catalog with no default shows one existing-Manifest selection/
      recovery screen and creates no recommended default. Explicit
      `--manifest` is invocation-local. A nonexistent name fails before cluster/
      Workspace mutation through typed list/show/create recovery.
- [ ] Before implementation starts, every fixed upstream through WP06 is
      complete; the implementer records the actual integrated HEAD/worktree and
      re-observes all final interfaces. Any moving/incomplete upstream blocks
      WP10 rather than becoming a local workaround.
- [ ] Routine progress uses Manifest vocabulary and separates desired Manifest,
      applied cluster projection, applied Workspace entry, bounded observation,
      and latest failure. A checkmark for one cannot imply another.
- [ ] Review precedes progress. Exactly five stages are
      `check_requirements`, `resolve_manifest`, `prepare_protection`,
      `prepare_workspace`, and `enter_workspace`, labelled `Check requirements`,
      `Save Manifest`/`Use Manifest`, `Prepare protection`, `Prepare Workspace`,
      and `Enter Workspace`. Stage state is exactly
      `pending|running|succeeded|skipped|blocked|failed|unknown`.
- [ ] Customize uses only `manifest create --copy-from` for Manifest copy and
      `runtime create --copy-source-from` for Runtime editable-source copy. It
      never says Base, never creates lineage/provenance, and keeps fresh
      recommended draft publication as a distinct non-copy path.
- [ ] Every running view shows completed stages, one current stage, monotonic
      elapsed time for a long stage, a truthful wait reason, and current
      `Ctrl-C` retention semantics without invented percentage or ETA.
- [ ] Stable line stderr lands first. Measure a 250 ms target anti-flicker
      threshold, elapsed after 1 s, maximum 4 Hz redraw, automatic bounded
      substage/wait reason after 10 s, and redirected heartbeat no more often
      than 30 s. V1 has no live details key.
- [ ] Failure output and later `status --details` show exact safe substage, WorkspaceManifestID and
      semantic revision identities, exact RuntimeID/revision digest, Docker
      context/version, desired/applied/observed distinctions, and bounded
      projected diagnostics. Presentation names, generations, ordinals, image
      tags, logs, or ordering never become authority.
- [ ] Root composition preserves separate Catalog effects and mutation-complete
      boundaries for Manifest create, Manifest default set, `cluster up`, and
      Workspace entry. A later failure cannot roll back or obscure a prior
      confirmed boundary.
- [ ] Only explicit Workspace entry reconciles Workspace runtime. Only
      `cluster up` reconciles shared resources/projection. `status`, `list`,
      `manifest show`, and `doctor` are strictly read-only and do not clear
      failures, refresh applied state, reconnect networks, or repair Docker.
- [ ] Workspace selection and progress validate `(ProjectRoot,
      WorkspaceManifestID)` and authoritative `workspace_id`; no legacy
      `project_id`, `instance_id`, display name, root string, or Docker identity
      substitutes for them.
- [ ] An attached Workspace whose pending entry adoption requires recreation is
      blocked before Docker mutation; progress shows current versus next entry,
      retained applied state, and typed non-command `end_active_session`. Once
      the external condition changes, the Next becomes `tobari`.
- [ ] Migration-required/incomplete state, unverified predecessor AppliedEntry,
      invalid compatibility projection, interrupted build, partial cluster,
      partial/unknown Workspace entry, and binary upgrade each have explicit
      desired/applied/observed recovery rows.
- [ ] Runtime creation success reports a fresh Runtime with empty history and
      hands human selection to `review runtimes`; direct action uses `runtime
      build --id`. Root does not rediscover a build target by name.
- [ ] Entry encountering a selected exact Runtime revision whose authority is
      ready but local image availability is missing performs no implicit build
      or reconciliation, retains Manifest/cluster/prior AppliedEntry facts, and
      exposes `review runtimes` (or WP03's final exact discover path) as its
      single immediate recovery path. That producer offers typed `runtime
      restore --id`; root/status never append or decode the reference.
- [ ] Restore progress says it is rebuilding and verifying exact recorded
      digest, never promises success, and preserves history/authority on
      `runtime_revision_unrestorable`, interruption, or digest mismatch.
- [ ] Root and first-use paths contain no implicit Runtime prune/delete,
      revision delete, generic GC, Docker prune, or cleanup side effect.
- [ ] Every declared failure/cancellation has one primary typed Next—Catalog
      task plus typed inputs, or typed non-command condition—and truthful
      `change_state`; no unchecked argv is appended. Unknown or confirmed-post-
      output mutation recovery is read-only.
- [ ] Rerunning the applicable exact action is idempotent only when the owning
      journal/receipt proves replay permission. No background retry, controller,
      watch, or status-triggered repair is introduced.
- [ ] Fresh XDG with pre-existing owner-labeled Docker resources fails before
      mutation with one diagnostic Next; it neither adopts nor deletes those
      resources and Docker remains observation, not logical authority.
- [ ] Docker unavailable and a stopped/unreachable Colima-backed Engine collapse
      to provider-neutral prerequisite faults. Doctor may show an observed hint
      but no provider command; typed condition says start the engine externally,
      then Next becomes `tobari`.
- [ ] Before child handoff, `Ctrl-C` reports the exact mutation boundary and
      retained desired/applied facts, waits for bounded classification, and
      exits 130. After handoff the child owns terminal, streams, signals, and
      exact exit status; attachment cleanup does not rewrite that status.
- [ ] Direct shell, `-- claude`, `-- codex exec ...`, and `-- gh auth login`
      are first-class happy paths. Missing executable after `--`, malformed
      selector placement, or invalid argv fails before all mutation; valid argv
      passes unchanged without shell parsing/logging/persistence/reconstruction.
- [ ] Fresh Workspace shell creation may show one non-blocking credential-home
      line exactly once. Ordinary re-entry/direct commands show no Tobari login
      banner; no credential inspection/dismissal state exists, and WP04 research
      auth/serve commands appear in no release first-use/help/recovery surface.
- [ ] Safe migration-required state points to typed `migrate apply`; unsafe,
      incomplete, or unknown migration points to doctor/read-only
      reconciliation. Root never migrates implicitly.
- [ ] A machine-checked integrated recovery graph rejects self-loops, unchecked
      argv, nonexistent paths, action rediscovery, closed reference cycles, and
      mutation retry after unknown. Read-only classifiers terminate causally;
      `output_encoding_failed` uses `version` and never repeats the failing task.
- [ ] Final public examples use `manifest`, `--manifest`,
      `workspace_manifest_id`, and `workspace_id` with no Context/project-ID
      alias. Schema version policy follows ADR 0079's pre-public final-V1 reset;
      unaffected shapes are pinned and deliberate renamed/recovery deltas have
      explicit predecessor/final fixtures.
- [ ] Pre-public migration retains legacy Context UUID bytes as
      WorkspaceManifestID and legacy ProjectInstance UUID bytes as WorkspaceID
      only through ADR 0079's explicit migration boundary. This packet does not
      independently choose revision retention or sufficient Docker evidence.
- [ ] A typed interpretation fixture and answer key prove task identity,
      Manifest desired revision, cluster applied revision, Workspace applied
      entry, observed state, latest failure, structured primary Next/Attention,
      and zero routine external processing independent of rendering. The same
      facts drive TTY, redirected, narrow, NO_COLOR, JSON/read, status, fault,
      and agent-readiness answers.
- [ ] Focused domain, application, infrastructure, CLI, PTY, hostile-output,
      cancellation, recovery, migration-integration, attachment, and upgrade
      tests pass, followed unconditionally by `task check`, `task security`,
      `task public:check`, and `task release:check`.

## Governing documents

- Upper decision: [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md) and the promoted [product](../../01_product_contract.md), [architecture](../../02_architecture.md), [security](../../03_security_model.md), and [harness](../../04_harness.md) contracts, especially
  the Workspace Manifest desired declaration, Workspace applied instance,
  item-specific activation, explicit reconciliation, identity rename, and
  migration direction.
- Accepted sibling decisions: ADR 0079's one-time copy initialization and the
  promoted product contract own copy vocabulary/independence; [ADR 0080 Runtime lifecycle](../../decisions/0080-close-the-managed-runtime-lifecycle.md)
  owns reference-bound build, read-only Runtime review, image availability,
  exact-or-fail restore, and explicit prune/delete.
- Thesis: Thesis 0 ordinary entry and recommended draft; synthetic/fresh-state
  zero-write reads; one installation-local cluster; logical state over Docker
  inspection; native Workspace-owned authentication; deterministic mutation
  outcome and cancellation.
- Product contract: root project entry, recommended review and Customize,
  Manifest/cluster/Workspace lifecycle, Runtime readiness, direct commands,
  causal faults, read purity, and final-V1 migration.
- Architecture/security: four-layer direction, one controlled side-effect
  boundary, Catalog authority, host-owned desired/applied receipts, exact owner
  labels, external text as untrusted data, and native authentication outside
  Manifest state.
- Existing ADRs: ADR 0077 remains causal recovery authority. ADR 0078 remains
  predecessor evidence but its aggregate activation wording is superseded by
  ADR 0079's typed activation slices after durable promotion.

## Completion definition

This packet is Accepted/Fixed for future implementation, not complete, and
production implementation has not started. WP10 begins only after the fixed
durable Manifest/copy baseline -> WP08 -> WP03 -> WP04 -> WP05 -> WP07 -> WP09 -> WP06 production
sequence completes and the actual integrated HEAD/interface re-baseline passes.
Completion requires every acceptance criterion, durable promotion, agent-
readiness evidence, all four unconditional gates, removal of temporary
diagnostics, deletion of this packet, commit, and the required
`WP10_IMPLEMENTATION_COMPLETE` or `WP10_BLOCKED` owner notification.
