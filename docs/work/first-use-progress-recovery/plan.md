# Work Plan: Make first Workspace entry legible and recoverable

- Status: Planned
- Decision state: Accepted/Fixed by Product Owner; production implementation not started
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)
- Active foundation: accepted [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md) and promoted docs/00-04 contracts, with commits `07535a9` and `428812f` as implementation evidence
- Accepted design integrations: ADR 0079's durable Manifest/Runtime one-time copy contract and [WP 03 Runtime Retirement](../runtime-retirement/plan.md); the latter's acceptance does not authorize implementation
- Fixed implementation prerequisite: durable Manifest/copy baseline -> WP08 -> WP03 -> WP04 -> WP05 -> WP07 -> WP09 -> WP06, followed by one integrated-HEAD re-baseline gate

The design is Accepted/Fixed. `Planned` records implementation state only; no
production implementation is authorized or included here.

## Chosen approach

Refine the existing root `tobari` outcome with one typed, invocation-scoped
Workspace-entry journey. The journey composes, but never merges the authority
of, these canonical operations:

```text
recommended draft/existing selection      review, outside progress
  -> Manifest create (fresh only)         desired Manifest authority
  -> manifest default set (fresh only)    installation default authority
  -> persisted Manifest resolve           invocation selection
  -> cluster up                           applied protection authority
  -> Workspace entry                      applied Workspace-entry authority
  -> child attachment                     transient session authority
```

The journey adds no durable SetupRun, no new controller, no second journal, and
no mutation authority. It renders the current desired Manifest, last successful
cluster application, last successful Workspace entry, bounded observation, and
latest bounded failure as separate typed facts.

Use two presentation levels:

1. Routine stages use only the fixed labels: Check requirements, Save/Use
   Manifest, Prepare protection, Prepare Workspace, and Enter Workspace.
2. Failure output and later `status --details` expose validated stable IDs,
   semantic revisions, exact substage, and bounded visibly projected
   diagnostics. Running progress has no second details interface.

Before any fresh-state write, a read-only fail-closed ownership observation
rejects fixed owner-labeled Docker resources that cannot be bound to the current
trusted state tree. Observation may veto; it may not adopt, delete, synthesize
Manifest/Workspace state, or publish applied receipts.

Before future implementation, run a mandatory re-observation gate only after
the complete fixed upstream sequence through WP06. Fetch integrated main,
record actual HEAD/worktree ownership, inspect all landed domain/application/
infrastructure/CLI/Catalog/JSON/schema/migration/tests and safe current binary,
and revise or block this packet before editing production. WP10 never
pre-implements an upstream authority.

## Concept budget and independence

| Level | Concepts | Budget/change |
| --- | --- | --- |
| Durable public resources | Workspace Manifest, Runtime, Workspace | Exactly three, fixed by ADR 0079 |
| Public subordinate values | Runtime revision, Project root | Runtime revision has availability and explicit restore but no independent delete; Project root remains a value |
| Public ephemeral workflow values | Runtime prune plan | Owned by WP 03; absent from routine first-use and never persisted as journey authority |
| Manifest-owned value groups | Boundary, cluster projection input, entry definition, session defaults, creation defaults | Item-specific activation, not public resources |
| Journey semantics | Stage, wait reason, retained desired/applied facts, cancellation meaning | One cohesive internal value family; invocation-scoped and non-persistent |
| Infrastructure internals | Docker images/cache, services, networks, journals, owner labels, observed IDs | Hidden routinely; bounded exact diagnostics only |

Access is a routine summary of the Manifest Boundary, not a fourth resource.
Project is ordinary user language for the source directory; ProjectRoot is a
subordinate trusted value, not a Project resource or ID.

## Ownership, scope, lifetime, mutability, and authority

| Item | Owner and scope | Lifetime | Mutability and authority | Routine presentation |
| --- | --- | --- | --- | --- |
| Recommended draft | Domain value for one fresh-state review | Until choice/cancel | No ID, no revision, not persisted, never accepted as WorkspaceManifestID | Recommended Manifest |
| Workspace Manifest | Host-owned CLI-managed installation resource | Until explicit delete/migration | Stable WorkspaceManifestID; accepted mutation publishes a complete immutable desired revision; Boundary invariant under the ID | Manifest |
| Installation default selector | Installation-owned selector | Until explicit set/replacement | Separate typed authority mutated only by `manifest default set`; never inferred from catalog size, Workspace, CWD, or `--manifest` | Default Manifest |
| Runtime | Installation resource | Until explicit lifecycle removal | Stable RuntimeID, mutable bounded source, immutable successful revisions; exact ID+digest authorize binding; image availability is replaceable evidence | Standard/custom tools; availability failure points to explicit restore |
| Shared cluster projection | Cluster application and trusted applied receipt | Until explicit cluster lifecycle | Desired aggregate derives from current Manifests/system input; only `cluster up` mutates/applies | Shared protection |
| Workspace | Host logical resource under `(ProjectRoot, WorkspaceManifestID)` | Until explicit delete | Stable `workspace_id`; only entry reconciles runtime and records AppliedEntry; prior success survives later failure | Workspace |
| Workspace home/native client state | Workspace and native client | Workspace lifetime | Creation defaults apply once; client owns login/logout; credentials are not Manifest state | Sign in inside Workspace |
| Child attachment | One host invocation/Attachment Epoch | Child session | Session-scoped; never desired/applied Manifest authority | Enter Workspace |
| Journey projection | Workspace-entry application task | One invocation | In-memory typed events only; not listable, resumable, or mutable as a resource | Progress view |
| Docker conflict observation | Narrow infrastructure read | One preflight | Can reject only; Docker names/labels cannot construct logical authority | Doctor diagnosis on failure |

## Stage vocabulary and semantic checkpoints

Review/selection precedes progress and is not a stage. Candidate enum names are
internal, not public JSON fields or inputs.

| Order | Candidate stage | First-use label | Re-entry label | Canonical work | Confirmed checkpoint |
| --- | --- | --- | --- | --- | --- |
| 0 | `check_requirements` | Check requirements | Check requirements | Read-only terminal/root/state/Docker/Engine/migration/ownership preflight; invalid direct argv fails here before I/O | Read-only preflight confirmed only |
| 1 | `resolve_manifest` | Save Manifest | Use Manifest | Fresh: canonical Manifest create then separate canonical default set; existing: validate explicit/default persisted Manifest without changing default | Fresh requires both desired publication and default selection; existing confirms invocation selection only |
| 2 | `prepare_protection` | Prepare protection | Prepare protection | Canonical `cluster up`, including finally integrated built-in standard/Gateway preparation, projection, health, verification | Cluster applied receipt only |
| 3 | `prepare_workspace` | Prepare Workspace | Prepare Workspace | Select/create by `(ProjectRoot, WorkspaceManifestID)`, validate exact Runtime revision availability, attachment guard, creation defaults, explicit entry, AppliedEntry | Workspace AppliedEntry only |
| 4 | `enter_workspace` | Enter Workspace | Enter Workspace | Resolve allowed new-session defaults, optional fresh-shell login line, attachment, exact child handoff | Child handoff only |

Each event has exactly one project-entry task identity, stage, status
(`pending`, `running`, `succeeded`, `skipped`, `blocked`, `failed`, or
`unknown`), fixed wait-reason kind, and typed retained facts.
Application code emits boundaries; the CLI owns monotonic time and redraw.

The same Manifest mutation may change several slice revisions, but progress
never calls it applied merely because desired publication succeeded:

```text
Manifest desired revision       published by Manifest mutation
installation default            selected only by manifest default set
cluster projection revision     applied only by cluster up
Workspace entry revision        applied only by Workspace entry
session defaults revision       resolved for a new child, subject to ADR 0079 follow-up decision
creation defaults revision      applied once only for a new Workspace
```

## Routine UX wireframes

Wireframes specify semantic hierarchy, not final glyph/color/spacing. Redirected
and narrow terminals receive a stable line-oriented equivalent.

### Recommended fresh-state review

```text
Start a protected Workspace for:
  /work/example

Manifest
  Recommended · not saved until Start Workspace

Project files
  Read-write · changes appear directly in this project

Access
  Claude Code and Codex routine traffic   allowed
  Other requests                          exact review
  Private and unsafe destinations         denied

Runtime
  Standard tools · exact revision pinned by Tobari

Sign-in
  Done inside this Workspace · host configuration is not imported

Session
  Open a shell

Action
❯ Start Workspace
  Customize
  Cancel
```

This screen appears only when the Manifest catalog is empty and no installation
default exists. Start publishes Manifest `default`; it does not silently select
it inside create.

### Existing catalog with no installation default

```text
Choose a saved Manifest for this installation

  review       Standard tools · read-write project
  restricted   Standard tools · read-only project

Selecting one can set the installation default explicitly.
`--manifest NAME` uses one saved Manifest for this invocation only.

Action
❯ Use selected Manifest
  Cancel
```

No recommended draft is synthesized, even when one Manifest or one same-root
Workspace exists. An unknown explicit name fails before protection/Workspace
mutation and routes through typed Manifest discovery/create recovery.

Customize edits a typed draft. It does not publish a Manifest revision until
the user accepts the create mutation. Final UI and recovery use `manifest`, not
the predecessor namespace.

Customize keeps three semantically distinct creation paths:

```text
Fresh Manifest draft (recommended)
  -> ordinary Manifest publication; no copy source or provenance

Copy an existing Manifest
  -> manifest create --copy-from NAME --name NAME
  -> exact current immutable revision Review and publication revalidation

Create a Runtime source
  -> runtime create --copy-source-from standard|NAME --name NAME
  -> fresh RuntimeID, empty history, no build or reconciliation
  -> review runtimes
  -> runtime build --id <runtime-ref>
```

Routine labels use `Copy from Manifest` and `Copy source from`; they never say
Base, clone, parent, fork, lineage, or inherit. Successful copy does not display
or persist source provenance, and neither path triggers cluster up or Workspace
entry.

### First run with a long local build

```text
Starting Workspace

✓ Check requirements
✓ Save Manifest                   desired saved · default selected
⠹ Prepare protection             applying protection · 02:14
  Substage: build pinned local components
  Wait: first preparation or changed trusted build input can take several minutes
○ Prepare Workspace              no applied entry yet
○ Enter Workspace

Ctrl-C stops this cluster-up attempt.
The Manifest remains saved; no Workspace entry is confirmed.
```

No percentage or ETA appears without an authoritative total. `desired revision
saved` cannot be shortened to `setup applied`. The substage/wait reason appears
automatically only after 10 seconds; it is fixed semantic metadata, not raw
BuildKit prose.

### Manifest saved but default selection failed

```text
Could not select the saved Manifest as installation default

✓ Check requirements
✗ Save Manifest
○ Prepare protection
○ Prepare Workspace
○ Enter Workspace

Retained
  Manifest default was published successfully.
  Installation default is unchanged.

Change state   none for default selection
Next           tobari manifest default set --name default
```

The structured Next is Catalog path `manifest default set` plus typed input
`name=default`; the renderer, not fault prose, constructs the shown argv.

### Re-entry with pending Manifest adoption

```text
Opening Workspace

✓ Check requirements
✓ Use Manifest                    desired review@3
✓ Prepare protection              current for desired projection
⠹ Prepare Workspace              applying next entry review@3 · 00:07
○ Enter Workspace

Current entry   review@2
Next entry      review@3
```

Names/generations are human correlation. Exact IDs/digests remain detail-only
authority.

### Attached adoption block

```text
Workspace entry is blocked

Manifest       desired review@3
Current entry  review@2
Observed       attached

The running Workspace is unchanged. End its attached sessions before entry can recreate it.

Change state   none
Next condition end_active_session
```

No status polling loop is created. After the external condition changes, the
primary Next becomes `tobari` (with typed `--manifest` input only when needed).

### Failure after Manifest publication

```text
Could not prepare protection

✓ Check requirements
✓ Save Manifest
✗ Prepare protection
○ Prepare Workspace
○ Enter Workspace

Retained
  Manifest desired revision is saved.
  Cluster applied revision is unchanged.
  No Workspace applied entry was recorded by this attempt.

Change state   partial
Next           tobari cluster up
```

### Selected Runtime image is missing

```text
Workspace cannot use the selected Runtime revision

Manifest       desired review@3
Runtime        frontend@4 · revision ready
Availability   missing
Current entry  review@2

Tobari did not build or change the Workspace.
Select the exact revision recovery through Runtime Review.

Change state   none
Next           tobari review runtimes
```

Runtime Review supplies the opaque `runtime-revision` reference and offers
typed `runtime restore --id <runtime-revision-ref>`. Root/status never append,
decode, or reconstruct that reference. Restore must not say “will recover”; it
may say “rebuilding and verifying exact recorded digest.”

The separate restore action may render bounded progress without joining the
root journey or promising completion:

```text
Restoring Runtime frontend@4
✓ Validate retained snapshot
⠹ Rebuild candidate image          external inputs may have changed · 01:42
○ Verify recorded digest
○ Publish availability

Ctrl-C may leave an exact journaled attempt; follow the reported read-only Next.
```

Only an exact digest match completes `Publish availability`. Mismatch renders
`unrestorable`, leaves immutable history unchanged, and does not resume root
automatically. Success/already-available returns typed Next `tobari` with
`--manifest` only when selected invocation scope requires it; unrestorable
returns to `review runtimes` to select or build a different Runtime.

### Unknown Workspace entry result

```text
Workspace entry was interrupted

Manifest desired and the previous applied entry are still known.
The attempted runtime result is not classified, so Tobari will not repeat the write.

Change state   unknown
Next           tobari status
```

This status invocation is a one-pass classifier. None of its outcomes may
return `status`; each terminates at an explicit write, typed external condition,
causal doctor result, or terminal no-action state.

### Exact details

```text
Exact details
  Workspace Manifest ID   018...
  Manifest revision       sha256:...
  Cluster desired         sha256:...
  Cluster applied         sha256:...
  Workspace ID            018...
  Entry desired           sha256:...
  Entry applied           sha256:...
  Runtime binding         runtime_id 018... · revision sha256:...
  Runtime availability    available|missing|mismatched|unknown
  Observed runtime        healthy · detached
  Current substage        prepare_images
  Last diagnostic         [bounded, visibly projected BuildKit line]
```

The diagnostic tail never determines identity, completion, retryability, or
change state. This block appears on failure and later through `status
--details`; it is not a live key or second progress interface. After 10 seconds,
running progress reveals only one bounded exact substage and sanitized wait
reason, not IDs/digests or a diagnostic tail.

## Fault and recovery matrix

`Retained` describes aggregate journey facts; `change_state` describes the
causal canonical operation. `Next` is structured as one Catalog task plus typed
inputs or one typed non-command condition; shown argv is derived by Catalog.

| Condition | Retained facts | `change_state` | Primary typed Next | Causal rule |
| --- | --- | --- | --- | --- |
| Cancel recommended review/Customize | No new authority | `none` | task `tobari` | Review is outside progress; exit 130 after rendering |
| Invalid `--manifest`, missing executable after `--`, or malformed argv | No state/I/O | `none` | task `help tobari` | Parse and validate before setup/Docker |
| Docker/Engine/context/Compose unavailable | No new authority | `none` | task `doctor` | Doctor terminates at condition `start_engine_externally`; after change, task `tobari`; no provider command |
| Fresh logical state sees unbound owner resources | No logical adoption/deletion | `none` | task `doctor` | Observation cannot create authority |
| Non-empty catalog with no default | Catalog retained; default absent | `none` | task `manifest list` | Existing selection UI; never synthesize recommended default |
| Explicit `--manifest` not persisted | Catalog/default unchanged | `none` | final typed `manifest list`, `manifest show`, or `manifest create` outcome | Exact fault selects from this closed set; no cluster/Workspace mutation or synthetic Manifest |
| Fresh recommended draft invalid | Nothing published/selected | `none` | task `tobari` | Return to review |
| Manifest create fails before action | No new desired revision | `none` | task `tobari` only when owning fault proves retry | No default mutation |
| Manifest create partial/unknown or output fails after possible commit | Existing plus possible new revision; never infer | `partial|unknown|confirmed` | task `manifest list` or `manifest show` per final owner result | Reconcile persisted Manifest before any default set/root replay |
| Manifest `default` published; default set fails/cancels before action | Manifest retained; default unchanged | `none` for selector | task `manifest default set`, typed `{name: default}` | Separate mutation checkpoint; never rollback create |
| Default set partial/unknown/output failure | Manifest and prior default retained; selector outcome uncertain/confirmed | `partial|unknown|confirmed` | task `manifest list`/final default-state read | Read-only reconciliation; never blind root replay |
| Manifest copy source changes | Existing Manifests unchanged | `none` | task `manifest show` | Fresh review; no provenance |
| Runtime source copy fails/drifts | Existing state unchanged | `none` | task `runtime list` | No build/reconcile/lineage |
| Runtime create succeeds with empty history | Fresh Runtime retained; other authorities unchanged | `confirmed` | task `review runtimes` | Discover exact Runtime ref before `runtime build --id` |
| Safe predecessor migration required | Predecessor unchanged until explicit action | `none` | task `migrate apply` with final typed inputs | Root never migrates implicitly |
| Migration unsafe/incomplete/unknown | Backup/predecessor/final evidence retained | `partial|unknown` | task `doctor`/final read-only migration classifier | No write until classifier terminates safely |
| Protection preparation canceled before journal | Manifest/default retained; prior cluster applied | `none` | task `cluster up` | Built-in standard preparation remains cluster-owned |
| Known interrupted cluster `up` | Prior applied retained; up journal retained | `partial` | task `cluster up` | Resume same causal operation only |
| Known interrupted cluster `down` | Prior facts retained; down journal retained | `partial` | task `cluster down` | Never reverse operation |
| Cluster journal unknown/contradictory | Desired/prior applied retained | `unknown` | task `doctor`/final read-only classifier | Classifier cannot point back to itself or an opposite write |
| Binary/compatibility projection changed | Manifests/homes/applied receipts retained | `none|unknown` | final cluster/status classifier outcome | Explicit cluster up only when typed evidence authorizes |
| Selected Runtime revision ready; material missing/mismatched/pruned | Desired, cluster, prior AppliedEntry/history retained | `none` | task `review runtimes` | Producer offers reference-bound restore; root/status append no ref |
| Restore restored/already available | Availability only changed | `confirmed` | task `tobari`, typed Manifest input if needed | Explicit re-entry; no auto-continue |
| Restore unrestorable/digest mismatch | Revision/history and all desired/applied facts retained | `none` | task `review runtimes` | Choose/build another Runtime; no promise another restore works |
| Restore partial/unknown | Authority/history retained; availability uncertain | `partial|unknown` | WP03 read-only Runtime classifier | No replay until causal result |
| Workspace validation fails before entry | Desired/cluster/prior AppliedEntry retained | `none` | task `status` only when it can classify | Status outcome must terminate at write, condition, or doctor—not status |
| Attached recreation block | Desired/prior AppliedEntry/attachment retained | `none` | condition `end_active_session` | After external change, task `tobari`; no polling/self-loop |
| Workspace entry partial/unknown | Prior AppliedEntry retained; attempt recorded | `partial|unknown` | task `status` | One classifier pass; its outcomes cannot return status again |
| Status proves safe pending entry | Desired/prior applied/observed explicit | `not_applicable` | task `tobari`, typed Manifest input if needed | Entry is sole reconciler |
| Attachment fails after AppliedEntry | Desired/cluster/AppliedEntry/home retained | `none` for attach | task `tobari` | Retry handoff without deleting state |
| `output_encoding_failed` | Underlying typed result/state unchanged | `not_applicable` or owning confirmed state | task `version` | Never retry the same encoding-failing command; audit continuation from version |
| Native client login prompt/failure | All Tobari authority retained | child-owned | native client guidance | No Tobari auth stage or research command |
| Child exits nonzero/receives signal | All durable state retained | child-owned | none injected | Exact child status/streams; bounded attachment cleanup only |

The final integrated Catalog recovery graph is generated from all declared
fault outcomes, structured Next variants, typed inputs, and reference producers.
It rejects self-loops, unchecked/free-form argv, nonexistent paths, action-local
rediscovery, closed required-reference cycles, opposite-operation recovery, and
mutation retry after unknown. A read-only classifier is allowed only when every
outcome reaches a write, a typed external condition, a terminal result, or a
different causal diagnostic in finite steps.

## Ctrl-C contract by boundary

Before handoff, the first Ctrl-C cancels only the one current canonical
operation through its context, waits for its bounded structured/journal outcome,
preserves any mutation-complete result, renders retained facts and typed Next,
and exits 130. A second journey mutation is never started while classification
is pending.

| Interruption point | Terminal/action owner | Required semantic result |
| --- | --- | --- |
| Draft review/Customize | CLI | `operation_canceled`, no new authority, `change_state=none`, typed Next `tobari`, exit 130 |
| Requirements | Entry application | No desired/applied mutation; bounded cancellation result, exit 130 |
| Manifest create before confirmed output | Manifest mutation | Preserve valid structured outcome; partial/unknown reconciles with `manifest list` |
| Manifest default set | Default-selector mutation | Published Manifest remains; prior/no default remains unless confirmed; partial/unknown uses default-state read, exit 130 |
| After Manifest/default confirmation, before cluster journal | Journey between boundaries | Desired Manifest/default retained; cluster applied unchanged; cluster action `none` |
| During cluster up | Cluster mutation | Prior cluster applied retained; bounded attempt/journal determines partial/unknown and exact recovery |
| After cluster confirmation, before Workspace entry | Journey between boundaries | Desired Manifest and cluster applied retained; Workspace prior applied unchanged |
| During Workspace entry | Workspace mutation | Prior AppliedEntry retained; attempt partial/unknown recorded and status first |
| After AppliedEntry confirmation, before attachment | Journey/transient attach | Workspace applied success retained; `tobari` may attach again |
| After child handoff | Child/PTY | Exact argv/TTY/stdin/stdout/stderr/signals/exit; bounded attachment cleanup runs but Tobari does not rewrite Ctrl-C or nonzero status |

## Public contract

### Capability ledger direction

| Surface | Status/effect | First-use integration |
| --- | --- | --- |
| `tobari` | Existing fixed-CWD RoleAct/EffectCreate outcome | Fresh composes Manifest create, separate default set, cluster up, Workspace entry, then child; existing selection omits create/default mutation unless user explicitly chooses to set default |
| `manifest list/show/create/delete`, `manifest default set` | Durable host Manifest lifecycle/read/default-selection surface from ADR 0079 and the product contract | Root uses canonical create/resolve; fresh list is empty, show cannot fabricate draft, and default selection does not retarget a Workspace |
| `manifest create --copy-from NAME --name NAME` | ADR 0079 fixed-target one-time copy mode | Customize-only one-time initializer from exact current immutable revision; fresh first-use recommendation does not use it and stores no provenance |
| `manifest runtime set` | Desired-state Manifest mutation | Publishes a new desired revision only; never reconciles cluster/Workspace |
| `runtime create --copy-source-from standard\|NAME --name NAME` | ADR 0079 fixed-target Runtime source copy | Customize creates independent editable source with fresh ID/empty history; no build or reconcile |
| `review runtimes` | WP 03 RoleDiscover/EffectRead | Human build selection and reference producer; no action on cancel |
| `runtime build --id <runtime-ref>` | WP 03 reference-bound Runtime write | Direct build consumes exact produced Runtime ref; no `--name` or omitted-target rediscovery |
| `runtime restore --id <runtime-revision-ref>` | WP 03 reference-bound Runtime-revision write | Offered only by reference-producing Runtime Review; exact digest match or unrestorable, never implicit entry behavior |
| `runtime prune dry-run/apply`, `runtime delete` | WP 03 explicit lifecycle only | Never invoked or suggested as routine first-use cleanup; root performs no implicit retirement |
| `cluster up` | Fixed-cluster create/reconcile | Sole shared projection/runtime reconciler |
| `cluster status` | Read-only complete observation | Never clears journal/failure or repairs; actual interrupted fault selects recorded operation |
| `status` | Fixed-CWD read | Desired/applied/observed/failure and exact next; never reconciles Workspace |
| `doctor` | Installation/project diagnostics read | Docker/migration/unbound-resource diagnosis; never converges |
| Progress/details | Internal presentation | No public command, input, resource, reference, or event schema |

The final command inventory and mutation bindings are owned by the durable
product/Catalog contracts promoted with ADR 0079.
This packet consumes them and adds no alias or competing registry.

### Inputs and outputs

- Root input remains canonical CWD plus optional exact argv after `--`. Final
  selection uses `--manifest` where the upper Catalog permits it.
- Public grammar remains `tobari [--manifest NAME] [-- <command>...]`.
  `--manifest` selects one persisted Manifest for this invocation only and does
  not update the installation default.
- The positional-only marker is mandatory for a direct command. The executable
  and every later value pass unchanged with no shell parsing, normalization,
  logging, persistence, or reconstruction. Bare `--`, wrong selector placement,
  or invalid argv fails before all setup/state/Docker effects.
- No `--context`, public alias, progress flag, provider selector, stage input,
  raw ID input, or generic Manifest file input is added.
- Customize adds no generic copy flag. `--base` is absent; only
  `--copy-from` for Manifest and `--copy-source-from` for Runtime are used.
- Root progress goes to Tobari-owned stderr before handoff. Successful child
  stdout/stderr/TTY/exit remain exact.
- First fresh use remains interactive even for a direct child command. Once
  Manifest/default/Workspace authority exists, noninteractive direct entry
  follows the final terminal contract.
- Root success is child attachment/execution, not a JSON progress envelope.
  Machine consumers use final `manifest`, `cluster status`, and `status` JSON.
- Human stage labels are not parser fields. Stable identities, desired/applied/
  observed objects, failure state, next argv, and output schemas are Catalog
  contracts.
- Delivery is complete for one invocation; collection coverage is not
  applicable. No cursor/pagination or event stream.
- Routine success external-processing count is zero.

### Progress output contract

- Stable line-oriented stderr is normative; TTY redraw must carry identical
  facts and is capped at 4 Hz.
- Hide a fast current stage below a target 250 ms anti-flicker threshold; show
  monotonic elapsed only after 1 s.
- At 10 s in one stage, reveal one bounded exact substage plus sanitized fixed
  wait reason automatically. Do not reveal raw provider output.
- Redirected stderr emits an initial/current transition and then a bounded
  heartbeat no more often than every 30 s while work remains running.
- Failure/cancellation always renders confirmed retained facts, causal
  `change_state`, and one structured primary Next. Exact IDs/digests and a
  bounded visibly projected diagnostic tail appear only on failure and later
  `status --details`.
- No public progress flag, persisted preference, event resource, live key,
  percent, ETA, or raw logs exist.

### Public vocabulary/schema compatibility

- This is a pre-public contract reset, not backward-compatible additive naming.
  Final examples use `manifest`, `--manifest`, `workspace_manifest_id`,
  `workspace_id`, and `project_root`; predecessor Context/project/instance
  spellings have no alias or ordinary dual reader.
- Existing final-unaffected success/fault shapes remain pinned. Deliberate
  changes include renamed Manifest/Workspace identities, desired/applied/
  observed status structure, new migration/attachment/ownership faults, and
  operation-matching interrupted-cluster Next actions. Each gets exact
  predecessor/final fixtures rather than a false semantic-compatibility claim.
- Copy state/JSON/status contains no `base`, `copied_from`, source identity,
  provenance, lineage, or parent field. The recommended draft has no synthetic
  saved source or hidden copy receipt.
- Runtime schema keeps revision authority/readiness distinct from typed image
  availability. Missing/unrestorable availability never rewrites the semantic
  revision, appends history, or changes Manifest/Workspace applied state.
- Manifest create output proves desired publication only. Default-set output
  proves installation selection separately; root progress joins the two typed
  results but adds no setup JSON/event schema. WP06 status owns the orthogonal
  persisted/default/invocation-selection facts and structured Next/Attention.
- Schema version policy follows ADR 0079: the exact unpublished predecessor is
  decoded only by explicit migration; final public/persisted contracts use the
  selected final V1 shape rather than public schema-2 aliases.
- Human first-use output intentionally changes. Child output/argv semantics do
  not.

### Direct entry and standard authentication

```sh
cd project && tobari
cd project && tobari -- claude
cd project && tobari -- codex exec --full-auto
cd project && tobari -- gh auth login
```

The first enters the default shell. The others execute exact argv inside the
selected Workspace. Native CLIs own login and persist credentials only in that
Workspace home after policy applies. On creation of a fresh Workspace, shell
entry may print one non-blocking line: credentials stay in this Workspace; sign
in with the tool when needed. It is not persisted or repeated on re-entry.
Direct commands rely on native prompts and receive no Tobari banner. Release
first-use/help/recovery contains none of WP04's research-only `auth` or `serve`
paths.

## Authority and presentation identity

```text
recommended draft                    no ID / no authority
  -> Workspace Manifest revision     desired authority
  -> installation default selector   separate selection authority
  -> cluster applied receipt         shared applied authority
  -> Workspace AppliedEntry          instance applied authority
  -> observed Docker facts           bounded evidence, not authority alone
  -> child Attachment Epoch          transient session authority
```

Presentation consumes the complete typed result:

```text
desired + default + applied + observed + latest failure + Next + Attention
                    -> validated journey projection -> human renderer
bounded external diagnostics -------------------------------^
```

The renderer cannot create equality, identity, completeness, confidence, or
replay permission from names, generations, timestamps, row order, indentation,
checkmarks, elapsed time, image tags, Docker labels, or log prose.

## Layer changes

- Domain: consume final WorkspaceManifestID/revision/slice types, Runtime exact
  binding, ProjectRoot, WorkspaceID, AppliedEntry, reconciliation attempt, and
  observed-state variants. Add a closed project-entry journey event vocabulary
  whose request dimensions include ProjectRoot and WorkspaceManifestID where
  persisted state exists.
- Application: own journey ordering and retained-fact interpretation while
  composing canonical Manifest create/default set, cluster, Workspace-entry,
  and attachment ports plus WP06 structured Next/Attention.
  Do not copy their selection, mutation, receipt, or recovery policy. Preserve
  each mutation-complete result before later cancellation.
- Infrastructure: reuse final Manifest/Workspace stores and Docker adapters.
  Add only bounded ownership-conflict observation and fixed progress metadata
  not already emitted. Diagnostics remain on a separate purpose-bound writer.
- CLI/Catalog: wire the final canonical services, render progress on stderr,
  keep final Manifest vocabulary and child streams exact, normalize Customize
  cancellation to the root journey, and consume final fault/Next declarations.
  Add only WP10's generic recovery-graph validation over WP08 traversal; no
  second Catalog or owner-specific reference walker.

Application packages do not import infrastructure or presentation. If the
journey needs sibling use cases, its owner defines domain-only narrow ports and
CLI composition adapters invoke the canonical services; root does not recreate
Manifest/cluster/Workspace policy.

## Error and cancellation behavior

- Validate task identity, ProjectRoot, WorkspaceManifestID, WorkspaceID, desired
  revision, applied scope, and observation bounds before presentation.
- Precondition failures occur before writes and report `change_state=none`.
- Confirmed desired/applied boundaries remain confirmed after any later fault.
- Manifest publication and installation-default selection are distinct
  mutation-complete boundaries. Default failure never rolls back or selects the
  published Manifest by implication.
- Manifest publication never claims cluster/Workspace adoption.
- `cluster status`, `status`, `manifest list/show`, and doctor never mutate,
  converge, clear failure, refresh applied state, or reconnect resources.
- Explicit replay is allowed only when the owning journal/receipt/fault proves
  it. Rate/cache evidence is independent.
- A validated interrupted up journal yields only cluster up; down yields only
  cluster down. Corrupt/ambiguous state chooses read-only diagnosis, not a write.
- Attached required recreation fails before Docker mutation. Desired/applied
  remain different and returns typed condition `end_active_session`.
- Partial/unknown entry keeps prior AppliedEntry and uses status only as a
  one-pass classifier whose outcomes cannot point back to status.
- Ready Runtime authority with missing image availability fails before entry
  mutation and points to Catalog-owned `review runtimes`. Root does not build,
  restore, prune, delete, or decode a revision reference itself.
- `runtime_revision_unrestorable` preserves the immutable revision/history and
  all Manifest/Workspace facts. It is a terminal result for that restore
  attempt, not evidence that another retry will succeed.
- Confirmed mutation plus output failure remains confirmed; recovery is
  read-only.
- `output_encoding_failed` never recommends the same task. `version` captures
  build identity, and the integrated graph verifies its causal continuation.
- Before handoff, cancellation exits 130 only after bounded classification.
- Child nonzero/foreground Ctrl-C after handoff is not a first-use fault.

## Security and trust boundary

- No boundary widens. Host-owned Manifest desired state and Workspace/cluster
  applied receipts are trusted inputs supplied by ADR 0079's durable model. Docker/filesystem/
  process/PTY/network effects stay in infrastructure.
- Owner-resource preflight is a read-only precondition within root EffectCreate.
  It can reject only; it cannot discover a public resource, adopt, delete, or
  synthesize desired/applied state.
- Routine output excludes Docker IDs/names, host private state paths,
  credentials, environment values, private destinations, and raw external text.
- Advanced exact IDs/digests are validated host facts. Opaque identities remain
  exact; printable diagnostics are visibly projected and bounded.
- Standard authentication/native client state, learned permissions, and
  attachment authority remain outside Manifest desired/applied/migration state.
- First-use performs no host login/setup or credential/provider inspection; the
  fresh-shell line is static guidance after policy and Workspace creation.
- Copy provenance is not security authority or diagnostic state and is neither
  stored nor shown. Runtime availability is validated typed evidence from the
  WP 03 boundary; WP10 never authorizes from Docker tag/image presentation.
- Runtime prune/delete remain separately confirmed destructive actions. Root
  does not invoke them, even to repair first-use disk pressure or stale state.
- Doctor may project a selected Docker context/provider hint as untrusted
  observation only. It grants no provider authority and emits no Colima/Docker
  Desktop/Podman start command; external startup is a typed condition.
- WP07 permission wait and WP09 service approval/open/cleanup remain independent
  authorities/Attention. WP05's host-loopback authority and WP04 research
  commands stay out of unrelated first-use stages.
- No new dependency, executable, destination, provider integration, or external
  schema is introduced by the journey.

## Alternatives considered

### A. Add `tobari init`, `build`, and `enter`

This makes mechanics explicit but splits one routine outcome, makes users
coordinate desired/applied order, and pressures the product toward a fourth
initialized/setup resource. Keep explicit canonical actions for advanced and
recovery use while root composes them.

### B. Automatically start without recommended review

This removes the only routine confirmation of direct project writes, Boundary
summary, host-config exclusion, Runtime selection, and session intent. Reject.

### C. Expose internal services/stages and raw Docker logs

This turns topology and untrusted provider prose into routine vocabulary and
encourages state inference from output. Use five semantic stages plus exact
bounded details.

### D. Persist `SetupRun` or onboarding-complete state

A reconnectable dashboard could be attractive for long builds, but it would
duplicate Manifest desired, cluster/Workspace applied receipts, and existing
journals. It has no independent user owner or lifecycle. Reject for authority,
not implementation size.

### E. Treat Manifest mutation as live rollout

This would collapse desired and applied, mutate running/attached Workspaces,
and introduce controller-like behavior explicitly rejected by ADR 0079. Reject.

### F. Treat the whole Manifest revision as one activation snapshot

This misrepresents cluster, entry, session, and creation lifetimes. Use typed
item-specific activation fixed by ADR 0079.

### G. Pull all prebuilt Runtime/Gateway images

This changes supply-chain, release-lock, licensing, offline, and architecture
contracts. It may be evaluated separately but does not solve truthful progress.

### H. Start/manage Colima automatically

This adds provider selection and process mutation beyond the generic Docker
boundary. Keep doctor as provider-neutral handoff.

### I. Adopt Docker resources found from fresh state

Labels do not prove Manifest/Workspace/applied identity. Adoption would infer
authority from observation and may mutate another state tree. Fail closed.

### J. Build or clean Runtime material implicitly during entry

This would let entry rediscover a Runtime target, conflate immutable revision
authority with image availability, and cross WP 03's reference/confirmation/
protection boundaries. Missing availability gets explicit exact-or-fail
`runtime restore`; storage cleanup remains explicit prune/delete. Reject.

## Human-handoff scorecard

| Candidate | One default decision | Desired/applied clarity | Wait clarity | One-Next recovery | Authority count | Decision |
| --- | --- | --- | --- | --- | --- | --- |
| Current disconnected predecessor | Yes | Weak | Cluster middle only | Inconsistent/self-loop | Existing authorities but unclear | Replace presentation |
| Chosen ephemeral Manifest journey | Yes | Explicit per boundary | Strong without fake ETA | Catalog-bound | No new authority | Select |
| Separate init/build/enter | No | Explicit but user-coordinated | Strong per command | Fragmented | Risks setup state | Reject |
| Persisted operation dashboard | Yes | Potentially strong | Strongest reconnect UI | Potentially strong | Duplicates receipts/journals | Requires separate thesis |
| Live-follow Manifest controller | Yes | Collapses desired/applied | Background | Automatic | Adds controller authority | Rejected by ADR 0079 |

## Implementation dependency order

1. Start from the promoted durable Manifest/copy baseline, then wait for production completion in this exact order: WP08, WP03,
   WP04, WP05, WP07, WP09, then WP06. WP10 edits none of their production
   surfaces ahead of ownership.
2. Run one final integrated-HEAD gate: fetch main, record HEAD/status/ownership,
   re-read every promoted contract and inspect/replay final domain/application/
   infrastructure/CLI/Catalog/schema/migration/tests/current binaries. Report
   `WP10_BLOCKED` on contradiction or moving upstream.
3. Freeze the final journey/recovery fixtures against WP04 release-only surface,
   WP05 name cutover, WP07 wait-only handoff, WP09 exposure lifecycle, WP06
   orthogonal Next/Attention, and WP08 enums/reference/recovery mechanics.
4. Freeze the five-stage/fault matrix and add presentation-independent
   desired/applied/observed/failure fixtures plus exact predecessor/final schema
   deltas, Runtime availability/restore variants, and zero-write canaries.
5. Add the domain journey types/invariants and application orchestration through
   final canonical ports.
6. Add the repository-wide machine-checked Catalog recovery graph and only
   missing bounded ownership/progress signals; reuse WP08 traversal.
7. Implement stable line stderr first, then bounded TTY redraw/timing,
   Customize/default recovery, direct entry, native-login line, and terminal
   handoff. Add no live details interface.
8. Exercise migration-required/incomplete, cache warm/cold, cancellation,
   interrupted/partial cluster, attached adoption, partial/unknown entry,
   Runtime missing/unrestorable/unknown availability, compatibility invalid,
   engine stopped, output failure, binary upgrade, direct children, and XDG
   collision in isolated PTY/Docker environments.
9. Promote durable first-use consequences and README/site/readiness, run all
   four gates unconditionally, remove this packet, commit, and send the required
   completion/block notification with final interfaces and evidence.

Do not implement a compatibility shim or a temporary WP10 copy/Runtime lifecycle
surface outside the promoted ADR 0079 stores/copy contracts or ahead of the
accepted WP 03 contract.

## Verification

### Domain and application

- Record the post-WP06 integrated implementation HEAD and prove fixtures reflect
  every final upstream contract rather than predecessor/intermediate shapes.
- Reject wrong task identity, ProjectRoot/WorkspaceManifestID/WorkspaceID scope,
  multiple running stages, non-seven-value status, illegal transition, applied
  checkmark without exact receipt, and wait reason on a non-running stage.
- Cover empty draft, Manifest desired-only, cluster lag, Workspace pending,
  current, attached-blocked, failed-none/partial/unknown, drifted, unavailable,
  incomplete migration, and binary-upgrade states.
- Prove root composes canonical mutation boundaries and preserves each confirmed
  output across later fault/cancellation.
- Prove fresh definition, separate default set, publication-success/default-
  failure, default partial/unknown, nonempty/no-default, invocation-only
  `--manifest`, and nonexistent explicit Manifest semantics.
- Prove desired Manifest mutation never touches cluster/Workspace runtime and
  status/read commands perform zero mutation.
- Prove fresh recommended publication uses no copy initializer/provenance;
  Manifest and Runtime Customize copy consume ADR 0079's target-specific behavior
  and activate nothing.
- Prove Runtime create success has empty history, Review supplies an opaque
  Runtime ref, build consumes it unchanged, and root never selects build by
  name or omission.
- Prove ready authority plus missing availability rejects entry before mutation,
  points to `review runtimes`, and preserves all desired/applied/history facts
  for already-available, restored, unrestorable, and unknown restore results.

### Infrastructure and security

- Review/Cancel/readiness/migration refusal/unbound-owner conflict perform zero
  Manifest/XDG/Docker writes.
- Docker conflict cannot synthesize IDs/receipts or authorize cleanup.
- Attachment guard precedes every recreating Docker call.
- Interrupted up/down journals yield one matching action without status self-loop
  or operation reversal.
- Build diagnostics are bounded/visibly projected; hostile controls/prose never
  change semantic answers.
- Standard auth, learned permission, attachment, and native credentials remain
  outside Manifest desired/applied/migration state.
- Provider hint remains observation; no provider start command or authority is
  created. Stopped Colima/Desktop/Engine terminates at typed external condition.
- Direct argv and child streams are neither logged nor persisted; release
  first-use contains no WP04 research auth/serve path.
- Root never invokes Runtime prune/delete, generic Docker prune, revision
  delete, or force under any first-use/recovery condition.

### CLI, Catalog, and schemas

- Whole-Catalog tests contain final `manifest` / `--manifest` and no public
  Context alias, generic import, `apply -f`, progress mode, or provider command.
- Catalog tests contain only ADR 0079's `--copy-from`/`--copy-source-from`, no
  `--base` alias/provenance fields, and only WP 03 `review runtimes`,
  reference-bound `runtime build --id`, and `runtime restore --id` handoffs.
- Root remains fixed-CWD RoleAct/EffectCreate; all composed final faults and one
  structured primary Next are declared without free-form/unchecked argv.
- Catalog-wide recovery graph tests reject self-loops, nonexistent paths,
  unchecked inputs, action rediscovery, reference cycles, opposite operation,
  unknown replay, and nonterminating read-only classifier chains; output
  encoding routes to `version`, never the failing command.
- Final JSON fixtures expose desired/applied/observed/failure with
  `workspace_manifest_id`, `workspace_id`, and `project_root`; no consumer must
  infer from names/tags/order.
- Unaffected final contracts remain pinned. Every intentional predecessor/final
  rename or recovery delta has exact positive/negative fixtures.
- Same semantic fixture drives recommended, Customize, first run, pending
  re-entry, attached block, failure, recovery, narrow/redirected/NO_COLOR, and
  direct-child presentation plus JSON/read surfaces, WP06 status, fault, and
  agent-readiness answers.
- Direct argv tests cover shell, Claude, Codex, `gh auth login`, bare `--`,
  selector misplacement, dash-prefixed later values, exact byte/order/duplicate
  preservation, zero pre-parse effect, and exact child status/signal.

### Runtime/PTy evidence

- Fixed-dimension parent-owned PTY evidence covers first Ctrl-C/exit 130 at every
  pre-handoff boundary, resize, 250 ms/1 s/4 Hz/10 s thresholds, no live details
  key, long wait, late cancellation, and child handoff.
- Redirected evidence proves bounded heartbeat at most every 30 s and identical
  stage/retained/Next semantics without TTY redraw.
- Isolated Engine evidence covers warm/cold build, Engine loss, interrupted
  Compose/health, partial Workspace, attached block, migration, binary upgrade,
  and owner-resource collision.
- Supported Docker Engine, Docker Desktop, and Colima observations record
  versions, sample counts, medians, and ranges for cold/warm standard Runtime
  and Gateway preparation as evidence only; no public ETA derives from them.
- Isolated evidence removes one selected revision image, proves entry performs
  zero implicit build, exercises exact restore success and unrestorable digest
  mismatch, and byte-compares Runtime history plus Manifest/Workspace state.
- Migration observation follows ADR 0079's reviewed cluster-stop/no-attachment and
  Docker-evidence rules; this packet does not choose them.
- Native login covers one fresh-shell line, no re-entry/direct banner, and
  native Claude/Codex/gh prompts with synthetic/optional secret-free evidence.

### Required gates

- Focused tests and `task check:fast` during slices.
- `task check`, `task security`, `task public:check`, and `task release:check`
  all pass unconditionally for this final integrated change.
- Agent journey completes routine success with zero custom parser/source
  inspection and recovers attached/unknown/reference-required failures through
  typed Next/Attention without reconstructing argv.

## State and schema migration

- Journey state, elapsed time, details preference, login-banner dismissal, and
  stage history are never persisted.
- Copy provenance/lineage/source identity is never added by fresh publication or
  Customize. WP10 adds no migration/backfill for predecessor Base-created
  records.
- Runtime image availability and WP 03 journals/receipts stay outside Manifest,
  Workspace AppliedEntry authority, and journey persistence. WP10 creates no
  lifecycle sidecar.
- Final public/persisted state follows ADR 0079's pre-public reset: legacy Context
  UUID -> WorkspaceManifestID, legacy ProjectInstance UUID -> WorkspaceID,
  canonical root -> ProjectRoot, with no public aliases or ordinary dual reader.
- Existing Workspace homes, Runtime IDs/history, learned rules, and eligible
  non-secret state are retained only through ADR 0079's exact migration.
- AppliedEntry is synthesized only when the reviewed read-only Docker evidence
  proves exact desired equivalence; otherwise progress/status show unverified or
  pending and explicit entry performs reconciliation.
- Migration requires the upper decision on cluster-stop/no-attachment and
  revision-body retention. Root never migrates implicitly.
- Explicit `cluster up` follows successful migration before Workspace entry.
- A compatible rollback uses ADR 0079's owner-only backup procedure and never lets
  old/new binaries share one migrated state tree.
- Multi-XDG installation identity remains separate; unbound resources are a
  pre-write refusal, not migration/adoption.

## Rollout and rollback

- The complete fixed upstream sequence through WP06 lands first. WP10 then
  passes one integrated re-observation gate; it does not freeze any intermediate
  worktree or implement an upstream authority locally.
- Land semantic fixtures/events before renderer replacement. Preserve canonical
  actions throughout.
- No feature flag: it would create competing first-use contracts and stale
  recovery. Use reviewable slices and final-V1 migration gating.
- Renderer changes can roll back without state migration because they persist
  nothing. Domain/persistence rollback follows ADR 0079's exact backup boundary.
- Fast ready paths render stable completed lines without animation; human output
  may change, machine schemas/child streams follow their explicit contracts.

## Documentation promotion

Before completion promote:

- Product: fresh/default selection semantics, five routine stages, direct entry,
  desired/applied/observed/failure display, retained-state/Ctrl-C/exit 130,
  structured Next/Attention, native-login location, and root stderr timing.
- Architecture: ephemeral journey over separate canonical Manifest/cluster/
  Workspace/attachment boundaries and conflict veto without Docker authority.
- Security: zero-write conflict/migration/attachment preflight, bounded details,
  and proof that auth/permission/attachment remain outside Manifest state.
- Harness: typed first-use/recovery graph, exact predecessor/final fixtures, PTY
  timing/cancellation, redirected heartbeat, isolated Docker/migration/
  collision, output-encoding chains, and zero-processing score.
- README/help/site/readiness: final Manifest/default vocabulary, long-build
  expectation, Current/Next entry, provider-neutral prerequisites, reference-
  safe recovery, shell/Claude/Codex/gh direct entry, and release-only auth UX.

WP10 promotes only final first-use and repository-wide recovery-graph polish.
After all gates it removes this temporary packet, commits, and notifies the root
owner with `WP10_IMPLEMENTATION_COMPLETE` plus final interfaces, gate evidence,
HEAD/status, retention, and cross-audit readiness; an unresolved integration
contradiction sends `WP10_BLOCKED` instead.
