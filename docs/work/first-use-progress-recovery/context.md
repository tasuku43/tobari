# Work Context: Make first Workspace entry legible and recoverable

This file separates predecessor-main facts, accepted decisions, in-progress
integration evidence, design evaluation, unconfirmed runtime evidence, and
cross-cutting questions. Legacy
Context/ProjectInstance terms appear only when describing the exact predecessor;
they are not target vocabulary.

## Baseline and repository protection

### Verified facts

- Revalidation on 2026-08-23 found the shared-checkout `HEAD` at
  `52a53bcc69a0f2bdf9bf2a6782ecd98bacd8b0e1`. The durable Manifest/copy
  promotion is present through commits `07535a9` and `428812f`.
- Accepted ADR 0079 and broad promoted changes across domain, application, CLI,
  infrastructure, tests, durable docs, and the embedded helper snapshot are
  now committed upstream evidence. The worktree still contains unrelated
  design packets; all were preserved. This revision changes only the four
  files under `docs/work/first-use-progress-recovery/`.
- `AGENTS.md`, the complete `add-capability` skill, [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md), and the promoted
  [theses](../../00_theses.md), [product contract](../../01_product_contract.md), [architecture](../../02_architecture.md), [security model](../../03_security_model.md), and [harness](../../04_harness.md) were reviewed before editing this packet.
- ADR 0079 and those durable contracts are accepted Product Owner authority and
  have higher precedence than this packet's former name-neutral assumptions.
- **Integration status supplied by the Product Owner:** the Manifest model and
  one-time Manifest/Runtime copy semantics are promoted and their temporary
  packets are deleted. WP 03 remains an accepted/fixed sibling design; that
  acceptance is not authorization to begin its production implementation.
- Therefore the predecessor observations below establish why the packet exists,
  not what the future implementer may assume. A mandatory implementation-start
  gate waits for the fixed durable Manifest/copy baseline -> WP08 -> WP03 -> WP04 -> WP05 -> WP07 ->
  WP09 -> WP06 production sequence, then fetches/rebases to that integrated
  HEAD, inspects the real clean/owned working tree, and repeats every relevant
  source, Catalog, schema, migration, test, and safe runtime observation.

## Durable upper decision: ADR 0079

### Verified decisions

- Public resource vocabulary is Workspace Manifest, Runtime, and Workspace.
  Runtime revision and Project root are subordinate concepts, not independent
  durable public resources.
- Product language is `Workspace Manifest`, routine short label `Manifest`, CLI
  namespace/flag `manifest` / `--manifest`, and schema authority field
  `workspace_manifest_id`. Public Context aliases are forbidden.
- A Workspace Manifest is host-owned and CLI-managed, with stable identity and
  complete immutable desired revisions. It is not project-owned YAML/JSON,
  generic import, repository discovery, or `apply -f`.
- Every revision under one WorkspaceManifestID preserves the same immutable
  Boundary. A Boundary change creates a different Manifest identity.
- A Manifest binds one exact RuntimeID and semantic Runtime revision digest.
  Runtime name, ordinal, image tag, image ID, and presentation cannot authorize
  the binding.
- Activation is item-specific: cluster projection at explicit `cluster up`,
  entry definition at explicit Workspace entry, session defaults for a new
  child session, and creation defaults for a newly created Workspace only.
- Workspace is the durable instance uniquely selected by `(ProjectRoot,
  WorkspaceManifestID)` and authorized by stable `workspace_id`. Legacy
  `project_id`, `instance_id`, and ProjectInstance are semantically removed.
- Desired Manifest, last successfully applied entry/cluster projection,
  observed runtime, and latest bounded failure/unknown attempt are independent
  facts. A later failure never overwrites prior successful applied authority.
- Only Workspace entry may reconcile Workspace runtime. Only `cluster up` may
  reconcile shared cluster runtime/projection. Status, list, show, and doctor
  observe only; there is no resident controller or background convergence.
- Pending entry adoption that would recreate an attached Workspace is blocked
  before Docker mutation. A desired Manifest change alone does not change a
  running Workspace.
- Standard authentication state, learned permission state, and attachment/
  Host Loopback authority are excluded from Manifest desired/applied state.
- Fresh-state recommended values are a no-authority presentation draft. An
  empty Manifest catalog remains known empty and `manifest show` cannot pretend
  a draft is persisted.
- Pre-public migration direction retains legacy Context UUID bytes as
  WorkspaceManifestID and legacy ProjectInstance UUID bytes as WorkspaceID,
  while removing the public dual vocabulary.

### Upper-decision questions that remain open

- Whether only the current Manifest revision body is retained or one previous
  body is also retained.
- Whether Git fallback bytes belong to the entry slice or a separate projection
  revision.
- Exact new-child-session default behavior when an attached Workspace has
  pending entry adoption.
- Read-only Docker evidence sufficient to synthesize AppliedEntry during
  predecessor migration.
- Required cluster-stopped and no-attachment conditions for migration.

This packet may expose dependencies and alternatives for those questions but
must not answer them independently.

## Durable one-time copy contract

- Manifest copy is exactly `manifest create --copy-from NAME --name NAME`.
  Publication revalidates the reviewed source WorkspaceManifestID, immutable
  current revision digest, and body immediately before issuing a fresh
  generation-1 Manifest identity.
- Runtime source copy is exactly `runtime create --copy-source-from
  standard|NAME --name NAME`. It copies current editable source into a fresh
  RuntimeID with empty history; it does not fork an immutable revision.
- `--base` is removed without alias. No common production derivation type is
  added. `provenance`, `lineage`, and `copied_from` are absent from persisted
  state, JSON, status, and routine display.
- Workspace, authentication, learned permission, attachment, applied/failure/
  observed facts, and current/default selection are never copied. Neither copy
  operation reconciles cluster or Workspace state.
- Packet-specific consequence: fresh first-use recommendation is ordinary
  no-authority draft publication, not `--copy-from` and not a copy of a
  synthetic saved default. Customize may expose the two target-specific copy
  flows without using the word Base.

## Accepted sibling decision: WP 03 Runtime lifecycle

- `review runtimes` is the read-only human build-selection flow. Direct build
  is reference-bound `runtime build --id <runtime-ref>`; `--name` and omitted-
  name action selection have no alias.
- A Runtime revision's immutable authority/readiness is distinct from local
  execution-image availability. Missing availability does not authorize entry
  or root to call build implicitly.
- `runtime restore --id <runtime-revision-ref>` rebuilds from the retained
  immutable snapshot and publishes availability only when the inspected digest
  exactly matches the recorded digest. Changed/missing external inputs may
  produce `runtime_revision_unrestorable`; history remains unchanged.
- Runtime retirement is explicit only: whole Runtime `runtime delete`, reviewed
  `runtime prune dry-run` followed by exact-plan `runtime prune apply`, and no
  revision delete, generic GC, global Docker prune, force, source/image public
  resource, or V1 OCI archive.
- The protection graph distinguishes current/retained Manifest revisions,
  Workspace last successful AppliedEntry, pending adoption, observed owned and
  foreign containers, and unknown evidence. Root consumes resulting
  availability/fault facts; it neither reimplements this graph nor prunes or
  deletes implicitly.
- Nested reference producer/consumer derivation is Catalog-wide and owned with
  the conformance work; WP10 must not add a Runtime-specific validator.

## Fixed Product Owner decision: WP10 journey and recovery

- Fresh requires both no persisted Manifest and no installation default.
  Publishing Manifest `default` and setting it as installation default are
  separate typed mutation authorities/checkpoints even when one routine stage
  renders them together.
- Review is outside progress. The five stages, seven execution states, routine
  labels, exact checkpoints, line-first stderr, timing thresholds, no live
  details key, and failure/status-details disclosure are fixed in `plan.md`.
- Direct argv after mandatory `--` is a first-class happy path. Before handoff
  Tobari owns progress/cancellation; after handoff the child owns exact argv,
  streams, terminal, signals, and exit status.
- Native CLIs own standard login inside the persistent Workspace home. WP04's
  four research Auth Broker commands plus `serve` are absent from the release
  Catalog and must not leak into routine first-use or recovery.
- Runtime availability recovery begins at the reference-producing `review
  runtimes`, not a bare restore command. Root never runs Runtime lifecycle
  writes.
- WP06 supplies orthogonal status facts, one structured primary Next, and
  ordered Attention. A primary Next is a typed Catalog task/input bundle or a
  typed non-command condition; status remains read-only/no-refs.
- WP07 permission wait is wait-only and non-authoritative; WP09 service
  exposure/open/cleanup keeps its independent approval and attachment
  authority. Neither is merged into first-use stages.
- WP05's `host.tobari.internal` hard cutover remains limited to its physical-
  host loopback outcome and is absent from unrelated routine stages.
- WP08 supplies Catalog-wide enum/output conformance, recursive reference
  traversal, and the `output_encoding_failed -> version` build-identity
  diagnostic. WP10 owns the final repository-wide recovery-graph audit.

## Predecessor-main behavior

### Verified facts: public journey

- `README.md` currently documents `cd /path/to/project` followed by `tobari`,
  with Linux Docker Engine or macOS Colima and an interactive terminal.
- Current main still uses the legacy public Context namespace. With no
  persisted legacy Context, root shows a recommended review; Start revalidates
  the empty catalog, composes legacy Context create and cluster up, then enters.
  Customize opens the six-stage legacy Context wizard.
- The current recommended draft is a typed synthetic value with display name
  `default`, guided selection, human selector `standard@1`, read-write project
  access, native readiness enabled, no host configuration import, and shell or
  exact direct-command intent. It has no durable legacy Context UUID.
- The recommended screen presents project path, access, network posture,
  `standard@1`, host-config exclusion, session intent, and exactly Start,
  Customize, and Cancel.
- The current root performs recommended review, generic Docker readiness,
  durable legacy Context creation, cluster up, Workspace selection/runtime
  reconciliation, and attachment in that order.
- Current root prints `Context created` before cluster progress and `Shared
  services ready` after cluster up. Progress does not cover prior readiness/
  creation or later Workspace reconciliation/attachment.
- Direct argv after `--` is executed without shell interpretation and returns
  the child's exact exit status. Workspace persists after child exit.

### Verified facts: progress and recovery

- Current cluster progress has nine fixed internal signals grouped into three
  human phases: prepare environment, start services, and verify readiness.
- Standard Runtime/Gateway preparation may perform a long local Docker/BuildKit
  build. It is not the separate custom Runtime build capability.
- Runtime build already demonstrates the correct diagnostic boundary: fixed
  typed progress metadata is separate from bounded visibly projected
  Docker/BuildKit text. External prose does not determine state.
- Current state has no persisted setup-progress resource. Durable facts live in
  the legacy Context store, cluster state/journal, root/instance records,
  Workspace home, and purpose-specific journals.
- A legacy Context commit completes before cluster reconciliation and is
  retained if the later cluster action fails.
- `cluster up` writes a schema-1 journal containing operation `up` and bounded
  phase, and explicit rerun can resume an interrupted up.
- Cluster status fails closed while a reconcile journal exists. Infrastructure
  currently advertises both up/down recovery, while Catalog maps the public
  interrupted fault back to `cluster status`; that can create a status-to-status
  recovery self-loop.
- Workspace logical records, not Docker presence, define existence. Current
  root/instance journals protect interrupted lifecycle work.
- Current fault schema 2 carries kind/code, phase, change state, retry metadata,
  and validated next actions.

### Verified PTY observations

- The observed platform was macOS arm64 with running Colima, Docker client
  28.1.1, Engine 27.4.0, context `colima`, and Compose 2.24.6.
- The available standard-profile binary identified development commit
  `bb6413870fbb037d32179e95ce3d69b57f29c1eb`, older than current source. It was
  used only for bounded terminal/precondition evidence.
- With fresh XDG, the exact predecessor command `context list --format json`
  returned legacy schema 1 with `context_state=synthetic_default`, display
  `default`, and no items. `cluster status --format json` returned unconfigured
  state. Those reads created no XDG file.
- Recommended-screen `q` returned `operation_canceled`, precondition,
  `change_state=none`, and Next `tobari`, with no state.
- Customize then Escape returned `operation_canceled` and no state but changed
  Next to the legacy `tobari context create`.
- Removing Docker from PATH produced `docker_cli_unavailable`; pointing
  `DOCKER_HOST` at a nonexistent socket produced
  `docker_engine_unavailable`. Both failed before writes with only doctor Next.
- Doctor also observed running `io.tobari.owner=default` containers belonging
  to another XDG state tree while fresh logical cluster state was empty.
- Because Docker names/labels are not isolated by XDG and user resources were
  active, no full Start, build interruption, cluster mutation, Workspace
  mutation, or cleanup experiment was performed.

## Evaluation against ADR 0079

### Contradictions removed by this revision

- The former packet treated the final Context name as unresolved. ADR 0079 fixes
  Workspace Manifest and forbids Context aliases.
- The former packet counted Project/Access/Workspace as routine concepts but
  did not state the three-resource budget. Project root is now a subordinate
  value; Access is a Manifest Boundary presentation, not another resource.
- The former packet described a confirmed "work mode save" without distinguishing
  desired from applied. A Manifest publication confirms desired state only;
  cluster and Workspace applied receipts remain separate.
- The former packet proposed preserving current public command paths and most
  schema semantics. ADR 0079 requires a pre-public `manifest` and identity reset
  with explicit predecessor migration and no public aliases.
- The former packet used `standard@1` as if it were sufficient persisted
  identity. Target authority is exact RuntimeID plus semantic revision digest;
  `standard@1` may remain human draft/selection text only.
- The former packet referred to root/instance identity without adopting the
  final WorkspaceID/WorkspaceManifestID model.

### Product diagnosis

- The missing product object is not a persistent SetupRun. It is an
  invocation-scoped interpretation of three existing/final authorities:
  Workspace Manifest desired revision, cluster applied projection, and
  Workspace applied entry.
- Progress cannot infer authority from checkmarks, order, elapsed time, Docker
  resources, image tags, or diagnostic prose. Each stage must carry typed task
  identity and the exact desired/applied/observed dimension it reports.
- First-use root is a composition, not one atomic mutation. Manifest create,
  cluster up, and Workspace entry keep their separate effects, output-complete
  boundaries, journals, and recovery. Progress is the joining presentation.
- Re-entry uses the same journey but may show Manifest desired newer than both
  cluster and Workspace applied state. That difference is normal pending
  activation, not drift or failure by itself.
- A new child session is not equivalent to entry adoption. Session-default
  behavior remains dependent on ADR 0079's unresolved attached-session decision.
- Native login is not a prerequisite or Manifest field. A newly created
  Workspace may receive invocation-scoped guidance before handoff; no banner
  state is persisted.
- One immediate Next can be read-only even when eventual recovery requires an
  external condition. For attached adoption, status is the safe immediate
  observation while the user ends attachments before a later entry.

## Relevant target structure

- Entry point: `cmd/tobari` through `internal/cli` composition.
- Domain: final WorkspaceManifestID/revision/activation values,
  RuntimeID/RuntimeRevision, ProjectRoot, WorkspaceID, AppliedEntry,
  reconciliation attempt, observed runtime, operation, and fault types.
- Application: Manifest desired mutation, cluster up, Workspace entry, and
  read-only status/doctor/list/show remain separate use cases. The entry journey
  owns only task ordering and interpretation through narrow ports.
- Infrastructure: trusted Manifest/Workspace stores, exact migration decoder,
  Docker observation/reconciliation, journals, owner labels, receipts, and
  bounded diagnostics.
- CLI/Catalog: final `manifest` namespace, `--manifest`, renamed JSON identities,
  root journey presentation, exact recovery, and child handoff.
- Harness: desired/applied/observed fixtures, migration predecessor/final
  fixtures, zero-write reads, cancellation, attachment block, hostile output,
  PTY evidence, and agent journey.

## Constraints

### Product and compatibility

- `tobari` must close ordinary entry without requiring users to call Manifest
  create, cluster up, and entry separately on routine first use.
- Recommended draft remains no-authority. Start must revalidate both empty
  catalog and absent default, publish Manifest `default`, then cross the
  separate default-selection mutation. Non-empty/no-default uses selection, not
  another synthetic recommendation.
- Public final V1 has no `context`, `--context`, `context_id`, `project_id`, or
  `instance_id` compatibility surface.
- Manifest means one host-owned CLI-managed resource. No project file or
  generic deserialization path can select authority.
- Root progress is human/TTY output before child attachment. Machine consumers
  use explicit final-V1 Manifest/status/cluster JSON contracts.
- No resident reconciliation, hidden automatic retry, or read-triggered repair.
- Customize copy wording follows ADR 0079's one-time copy contract exactly. Fresh publication and copy
  initialization are distinct outcomes, and no provenance is persisted or
  inferred from presentation.
- Runtime create and Runtime build are distinct. Root cannot treat fresh
  Runtime publication as build completion or rediscover a build target by name.
- Runtime `ready` does not imply image availability. Missing image material for
  the exact selected revision is a typed recovery condition, not permission for
  an implicit build.
- `--manifest` is invocation-only. Direct argv requires `--`, remains exact, and
  invalid grammar fails before every side effect.
- Primary Next is structured semantics, not necessarily a command string. Typed
  non-command conditions cover external engine startup and ending active
  sessions without inventing provider or detach commands.

### Architecture and security

- Domain remains pure; application owns task semantics and narrow ports;
  infrastructure owns Docker/filesystem/process I/O; CLI composes and renders.
- Desired publication, cluster application, and Workspace entry remain three
  distinct mutation-complete boundaries. Later failure cannot reclassify an
  earlier confirmed success.
- Unknown post-action state blocks replay until read-only observation provides
  a safe next action. Read-only commands never write attempt/applied receipts.
- Default selection is its own mutation-complete boundary. Manifest publication
  success cannot be rolled back or relabelled when default setting fails.
- Docker evidence may veto or classify bounded observation but cannot create a
  Manifest/Workspace identity, AppliedEntry, or deletion authority.
- An attached-adoption guard executes before Docker mutation.
- External diagnostics remain untrusted and visibly projected; no credential,
  private store path, environment value, or external prose enters semantic
  failure state.
- Standard authentication and attachment authority remain Workspace/session
  owned, outside Manifest revisions and migration-readable state.
- Release first-use reads no credentials and exposes none of WP04's research-
  only auth/serve surface. Native login occurs through the exact child path.

### Harness and repository

- Interpretation-sensitive output needs one typed fixture and answer key with
  every requested identity/scope and negative-inference canaries.
- Migration evidence uses synthetic IDs/roots/digests and never reads native
  credentials.
- Runtime-only behavior must be observed in a disposable Docker/XDG boundary;
  active developer resources are never experimental targets.
- Future implementation requires `task check`, `task security`, and
  `task public:check` because public vocabulary, principal identity, migration,
  and mutation/read boundaries change.
- The first WP10 implementation action is a read-only post-WP06 integrated-HEAD
  re-observation gate. It must not edit any upstream production schema/Catalog/
  file while the fixed implementation chain is incomplete.

## External comparison evidence

Sources were checked on 2026-08-23 and remain analogy inputs only.

- [Docker Sandboxes](https://docs.docker.com/ai/sandboxes/) and
  [Usage](https://docs.docker.com/ai/sandboxes/usage/) support one entry command,
  path-based re-entry, and a separate diagnostic action.
- [NVIDIA OpenShell Manage Sandboxes](https://docs.nvidia.com/openshell/sandboxes/manage-sandboxes)
  demonstrates explicit lifecycle phases and distinct foreground Ctrl-C versus
  detach semantics, but its gateway/driver/provider nouns are too broad for
  Tobari's routine surface.
- [nono](https://github.com/nolabs-ai/nono) demonstrates the value of a
  prepackaged safe default and minimal start latency; Tobari instead must make
  local build waiting truthful.
- [Clawker](https://github.com/schmitthub/clawker) demonstrates preset versus
  Customize, content-addressed builds, staged readiness, and persistent
  in-container login. Tobari intentionally avoids separate init/build/run and
  host-config copy.
- [agentbox](https://github.com/mattolson/agent-sandbox) demonstrates the setup
  tax of separate engine bootstrap, generated project configuration, init, and
  exec. Tobari keeps provider setup external but closes its own entry task.

## Packet integration and conflicts

- [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md) and promoted docs/00-04 are the durable production foundation and supersede this
  packet's old Context/name-neutral and compatibility conclusions. Commits
  `07535a9` and `428812f` are frozen promotion evidence; WP10 still re-baselines
  the later fully integrated HEAD before implementation.
- ADR 0079's one-time copy section and the promoted product/Catalog contracts
  fix Customize copy vocabulary, independence, no provenance, and
  Runtime-create meaning.
- WP 03 Runtime Retirement fixes `review runtimes`, reference-bound build,
  availability/restore, and explicit cleanup boundaries. It is accepted but
  not reported as implementing.
- WP 08 owns recursive Catalog reference traversal, shared output enum
  conformance, and its focused `output_encoding_failed -> version` handoff.
- WP 04 owns release/research executable vocabulary; release first-use excludes
  research `auth` and `serve` paths.
- WP 05 owns the `host.tobari.internal` hard cutover; WP10 does not expose that
  authority name in unrelated stages.
- WP 07 owns wait-only permission recovery without policy authority. WP10 may
  surface Attention/handoff but does not approve or retry denied effects.
- WP 09 owns service exposure review/open/stop/attachment cleanup; WP10 does not
  merge service approval into first-use authority.
- WP 06 owns orthogonal status axes, structured primary Next, ordered Attention,
  typed non-command guidance, and read-only no-reference status semantics.
- `context-capability-envelope` and `context-source-access` remain security
  evidence for the immutable Manifest Boundary but their public Context and flat
  lifecycle vocabulary is superseded by ADR 0079.
- `first-public-release-core` and `first-public-release-artifacts` must consume
  final Manifest/Workspace schemas and must not publish predecessor names.
- Runtime retirement/status/copy work must preserve exact RuntimeID + semantic
  revision references needed by desired and applied receipts.
- Status work must expose desired/applied/observed/failure without mutation.
- Authentication narrowing, permission resume, Host Loopback, and service
  exposure consume final WorkspaceManifestID/WorkspaceID principal identity but
  remain outside Manifest desired/applied state.
- ADR 0070 migration exclusivity and ADR 0078 aggregate activation wording are
  superseded where [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md) and its promoted contracts conflict.

## Unknowns and dependencies

- [ ] After every fixed upstream implementation through WP06 completes, record
      the integrated HEAD/worktree ownership; re-read promoted contracts and
      inspect every final domain/application/infrastructure/CLI/Catalog/schema/
      migration/test interface WP10 consumes. Moving/inconsistent upstream work
      yields `WP10_BLOCKED` before production edits.
- [ ] Consume the owner decision on current-only versus one-previous Manifest
      revision body; progress needs semantic identities but does not require
      public unbounded history.
- [ ] Consume the Git fallback slice decision before defining `entry_revision`
      and exact advanced details.
- [ ] Consume the attached new-child-session-default decision. This packet must
      not imply that starting another child adopts a pending entry revision.
- [ ] Consume the reviewed Docker evidence set for migration before classifying
      legacy runtime as AppliedEntry versus unverified/pending.
- [ ] Consume migration cluster-stop/no-attachment preconditions and represent
      their exact read-only recovery without inventing automatic cleanup.
- [ ] Decide whether simultaneous independent XDG installations on one Engine
      are unsupported or receive a separate installation identity design.
- [ ] Measure cold/warm standard Runtime/Gateway preparation at the final
      implementation HEAD across supported Docker Engine, Docker Desktop, and
      Colima setups; record versions/medians/ranges, not ETA promises.
- [ ] Verify stopped-Colima behavior without disrupting unrelated containers.
- [ ] Remeasure the target 250 ms anti-flicker threshold on the integrated
      implementation and supported terminals. Any adjustment requires owner
      review; the fixed 1 s elapsed threshold, 4 Hz redraw cap, 10 s automatic
      detail reveal, and 30 s redirected-heartbeat cap do not change through
      implementation measurement. No evidence may add a live details key or a
      second progress interface.
- [ ] Validate the exact one-line fresh-shell native-login wording; direct
      commands and ordinary re-entry remain banner-free.
- [ ] Define exact final-V1 fault codes/typed Next for migration required/
      incomplete, compatibility projection invalid, attached adoption blocked,
      and unbound Docker ownership.
- [ ] Consume WP 03's final availability fields and exact reference kinds so
      entry can distinguish ready authority from missing image material without
      inspecting Docker or decoding a Runtime reference.
- [ ] Freeze the exact entry fault and final discover-first restore/
      unrestorable handoff from WP03/WP06 Catalog;
      the accepted primary Next is `review runtimes`, not bare restore.
- [ ] Audit every final Catalog fault and every classifier result after WP08/
      WP06 integration to construct the machine-checked causal recovery graph,
      including repository-wide output-encoding chains.

## Thesis evidence

- Repeated confusion: a root first-use composition crosses desired publication,
  shared application, Workspace application, and attachment but current output
  presents only disconnected lifecycle messages.
- Repeated unsafe workaround pressure: a SetupRun record, raw log parser,
  root-handler policy copy, synthetic saved default, or Docker adoption would
  duplicate authority.
- Upper thesis direction: Workspace Manifest desired state is followed only at
  explicit item-specific boundaries; Workspace and cluster applied receipts are
  independent; observations do not reconcile.
- Downstream impact: final Manifest vocabulary/schema, entry/cluster/status
  ports, migration, Catalog recoveries, presentation fixtures, README, and
  harness must agree before completion.

## Reproduction or observation

These commands record the exact predecessor only; final-V1 examples use the
`manifest` namespace.

```sh
# Legacy current-main observation with fresh XDG:
bin/tobari context list --format json
bin/tobari cluster status --format json
bin/tobari doctor --format json

# Real PTY:
bin/tobari -- /bin/true
# q -> operation_canceled, change_state none, Next tobari
# Customize then Esc -> legacy Next context create

# Bounded generic readiness projections:
PATH=/usr/bin:/bin bin/tobari -- /bin/true
DOCKER_HOST=unix:///tmp/nonexistent.sock bin/tobari -- /bin/true
```

Expected and observed: reads, review cancellation, and failed prerequisites
created no XDG or Docker state. Full mutation was intentionally not attempted
against active owner-labeled resources.

## Security and public-boundary notes

- Assets: trusted Manifest desired store/revisions, Runtime revisions, cluster
  applied projection, Workspace logical/applied/failure records and home,
  observed Docker resources, project root, and transient child attachment.
- Credentials: native client state remains only in Workspace home and is absent
  from Manifest, progress, migration evidence, fixtures, and diagnostics.
- New boundary: none. The future slice adds typed interpretation and a bounded
  read-only ownership-conflict observation within existing adapters.
- Output: human progress is invocation-scoped; exact status/Manifest JSON
  remains complete for its declared task. No pagination or setup event stream.
- Retry: explicit only. Journal/receipt evidence determines replay permission;
  Docker cache/rate evidence is separate.
- Publication: external comparison material is linked/paraphrased only; no
  code, schema, or asset is imported.

## Glossary

- **Workspace Manifest / Manifest:** host-owned, CLI-managed stable desired
  declaration with immutable complete revisions and invariant Boundary.
- **Recommended draft:** fresh-state presentation with no
  WorkspaceManifestID; it is never a persisted Manifest item.
- **Desired:** current Manifest revision selected for future explicit
  activation.
- **Applied:** last successfully completed cluster projection or Workspace entry
  receipt. Desired publication alone is not applied.
- **Observed:** bounded read-only external/runtime evidence; it may differ from
  desired and applied.
- **Latest failure:** bounded classified or unknown attempt for an exact desired
  revision; it does not replace last successful applied authority.
- **ProjectRoot:** canonical host source-directory value, not a public resource
  or ID.
- **Workspace:** durable instance bound to `(ProjectRoot,
  WorkspaceManifestID)` and authorized by `workspace_id`.
- **Journey projection:** invocation-scoped typed presentation joining separate
  canonical results; not a resource, journal, or mutation authority.
- **Immediate Next:** one primary typed Catalog task with typed inputs, or one
  typed non-command condition, safe at the current fault boundary. A read-only
  classifier may yield a later action only through a causally terminating
  declared outcome.
- **Runtime availability:** replaceable local execution-image state for one
  immutable Runtime revision. It is not revision authority or history.
- **Restore:** explicit exact-or-fail attempt to recreate missing availability
  from a retained immutable snapshot without rewriting revision history.
