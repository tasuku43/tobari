# Work Plan: Close the managed Runtime lifecycle

- Status: Accepted
- Decision state: Fixed by Product Owner; the WP03 design is not reopened
- Implementation state: Planned; implementation not started
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)
- Implementation prerequisites: WP01 + WP02 completion audit, then completed
  WP08 Catalog/domain output conformance, then WP03 implementation
- Upstream audit evidence: integrated ADR 0079 and `docs/00` through `docs/04`
  at HEAD `52a53bc`, with model/copy commit `07535a9` and migration/security
  commit `428812f`

## Chosen approach

Close the lifecycle with three task-owned outcomes while keeping one public
Runtime aggregate:

1. **Retire one complete managed Runtime** through exact reference-bound
   `runtime delete` after a complete Workspace Manifest retention, Workspace
   desired/applied-state, and Docker-use barrier.
2. **Review then reclaim unused Docker execution material** through distinct
   `runtime prune dry-run` and `runtime prune apply` commands. The read produces
   a deterministic opaque plan; the write consumes it unchanged and revalidates
   every protection under the installation lifecycle lock.
3. **Restore exact revision availability** through reference-bound `runtime
   restore`. Rebuild from the retained immutable source snapshot and publish
   only when compatibility and recorded image-content digest match exactly.

Keep immutable revision history and snapshots until whole-Runtime deletion.
Prune image material, not revision authority. Keep editable source, immutable
snapshots, Docker material, and interruption journals as separate internal
lifecycle facets, but do not expose them as independently mutable resources.
Every destructive operation uses exact Tobari ownership, never a Docker name
prefix, global prune, or force.

Align the existing creation/build half of the lifecycle at the same time:
`runtime create` remains a fixed-target create against the installation Runtime
catalog and produces the new child Runtime reference; `runtime build` changes
from a catalog fixed-target write to a reference-bound write of one exact
Runtime. A dedicated read-only Runtime Review preserves human target selection
without making the direct action rediscover by name.

## Implementation entry and fixed order

The Product Owner-fixed physical order is:

```text
WP01 + WP02 completion audit -> WP08 -> WP03
```

The WP01+WP02 completion audit is satisfied by ADR 0079, integrated `docs/00`
through `docs/04`, commits `07535a9`/`428812f`, and the observed integration
HEAD `52a53bc`. WP03 production work still waits until WP08 reports
`WP08_IMPLEMENTATION_COMPLETE`. Re-read the integrated governing documents,
Catalog/domain outputs, migration state, and opaque-reference traversal before
the first WP03 edit. If any upstream interface differs from this packet, report
the mismatch as an implementation-entry blocker; do not reopen or silently
rewrite the Product Owner-fixed WP03 contract.

## Alternatives considered

### Comparison matrix

Scores are qualitative; 5 is best. Concept count covers durable public semantic
units, not commands, fields, or internal structs.

| Option | User outcome | Referential safety | Recovery | Docker abstraction | Public concept cost | Decision |
| --- | ---: | ---: | ---: | ---: | --- | --- |
| A. `runtime delete` only | 3 | 5 | 3 | 5 | 0 new resources | Incomplete: no safe unused-image reclaim or image-loss recovery |
| B. `runtime revision delete` | 2 | 2 | 2 | 4 | revision becomes independently mutable | Reject |
| C. `runtime prune --dry-run` then same path mutates | 4 | 4 | 3 | 4 | ephemeral plan implicit | Reject: argv changes effect/role/contract |
| D. `runtime prune dry-run` + `runtime prune apply` | 5 | 5 | 5 | 5 | one ephemeral plan reference | Choose |
| E. public `gc` | 2 | 2 | 2 | 1 | generic collector concept | Reject |
| F. public source/image delete resources | 3 | 3 | 3 | 2 | two new resources and half-states | Reject |
| G. delete + reviewed prune + exact restore | 5 | 5 | 5 | 5 | existing Runtime/revision plus ephemeral plan | Choose combined lifecycle |

### Whole-Runtime delete only

Whole deletion is necessary for abandoned zero-revision source and intentional
retirement. It is insufficient for a long-lived Runtime whose old or temporarily
unused image material consumes Docker storage while source and revision history
remain useful. It also does not handle external image disappearance.

### Individual `runtime revision delete`

Deleting one revision would make an existing subordinate immutable value
independently mutable, break contiguous ordinals or require tombstones, change
`name@ordinal` interpretation, complicate every Manifest/Workspace reference,
and destroy the snapshot needed for exact recovery. The storage outcome does
not require it: Docker material can be pruned while authority/history remains.
Do not add the command, reference consumer, hidden parser alias, or state
tombstone.

### One `runtime prune --dry-run` flag

A boolean that switches between a read and destructive write makes Catalog
effect, role, mutation facts, errors, and result schema input-dependent. It also
permits the candidate set to change between an informal dry-run and execution.
Use two complete catalog leaves. The chosen spelling intentionally replaces the
hypothetical flag with `runtime prune dry-run` and `runtime prune apply`.

### Generic `gc`

`gc` describes an implementation mechanism, not a user-owned target. It invites
global Docker image/cache/container cleanup and makes protection reasons hard to
review. Do not add root `gc`, `runtime gc`, an automatic timer, or a hidden
daemon sweep.

### Public source/image separation

Separate internal lifetimes are correct: source is mutable authority input,
snapshots are immutable recovery evidence, and Docker image material is
replaceable availability. Separate public delete resources are not. A user
should not be able to delete source while keeping a misleading active Runtime,
delete snapshots while retaining selectable history, or target an image by
Docker identity. Whole Runtime retirement and task-owned prune/restore expose
the safe outcomes without public half-states.

### Export an OCI archive per revision

An archive would make restore independent of external Dockerfile inputs but
would add roughly image-scale installation storage, archive integrity and
format contracts, licensing/export questions, and its own prune policy. Defer
until exact rebuild failures are observed. V1 restore is exact-or-fail and says
when external inputs make a historical image unreconstructible.

## Design

### Public and internal concept count

| Surface | Concept | Disposition |
| --- | --- | --- |
| Public durable resource | Managed Runtime | Existing; gains complete retirement |
| Public subordinate value | Immutable Runtime revision | Existing; gains opaque reference and availability, never delete |
| Public ephemeral workflow value | Runtime prune plan | New opaque reference; no persisted user-managed resource |
| Public action | Restore revision availability | New command; does not mutate revision identity |
| Public source/image resources | Source, snapshot, Docker image | None; remain Runtime facets |
| Internal authority | Runtime manifest and Workspace Manifest bindings | Exact Runtime ID + semantic revision digest; Manifest identity is `workspace_manifest_id` |
| Internal evidence | Workspace `last_successful_entry`, pending desired adoption, image observation, usage/storage observation | Consumes ADR 0079 and the durable architecture protection seam; no new public resource |
| Internal transaction state | Build journal, retirement journal, terminal receipt | New owner-only schema-1 records |

This packet adds zero durable public resources and preserves ADR 0079's total budget
of Workspace Manifest, Runtime, and Workspace. One existing subordinate value
becomes reference-addressable, and one ephemeral plan reference is added. Docker image
ID, tag, container ID, snapshot path, journal, and receipt are not public
resources or action targets.

### Lifecycle model

| Facet | Owner/scope | Lifetime | Mutability | Delete/prune rule |
| --- | --- | --- | --- | --- |
| Managed Runtime identity | Tobari installation | Create to confirmed Runtime delete receipt | Immutable ID; name not authority | Delete only as whole managed Runtime |
| Editable source | Managed Runtime | Runtime lifetime | User-editable within bounded owner-only contract | Whole Runtime delete only |
| Successful revision | Managed Runtime subordinate | Runtime lifetime | Immutable digest, ordinal, evidence | Never individually deleted |
| Source snapshot | Successful revision | Runtime lifetime | Immutable owner-only bytes/modes | Whole Runtime delete only |
| Revision image material | Docker content store, associated by exact Tobari evidence | May be locally present or absent within revision lifetime | Availability changes; authority does not | Prune only when fully unprotected; restore exact-or-fail |
| Failed build candidate | Exact journaled Runtime build attempt | Until resumed build or explicit prune | Disposition only | Prune after ownership/use revalidation |
| BuildKit cache | Docker daemon | Docker-owned | External/global | Never targeted by V1 Runtime lifecycle |
| Usage/storage observation | Workspace applied evidence + bounded local/Docker inspection | Observation/applied-record lifetime | Diagnostic | Never target authority |
| Retirement journal/receipt | Tobari installation | Started action through bounded recovery evidence | Monotonic phases | Exact resume/idempotency only |

### Authority and presentation identity

- Define opaque reference kinds `runtime`, `runtime-revision`, and
  `runtime-prune-plan` in domain vocabulary with kind-specific constructors and
  exact validators.
- Extend Catalog output reference derivation to walk validated object and array
  schemas and publish canonical field paths such as `items[].runtime_ref` and
  `runtime.revisions[].revision_ref`. Reachability, scoped help, fixtures, and
  runtime output validation must use the same recursive paths. This is a
  Catalog-wide mechanical contract, not a Runtime handler exception.
- A `runtime` reference is deterministically derived from stable Runtime ID.
  A `runtime-revision` reference is derived from stable Runtime ID plus semantic
  source digest. Infrastructure resolves a supplied reference by deriving and
  comparing candidates exactly; the action never parses a name, ordinal,
  Docker tag, or encoded internal field from user input.
- A prune-plan reference is a versioned digest of the canonical ordered
  candidate identities and protection observation token. `observed_at` is
  output metadata and is excluded from the digest. The action accepts the
  reference unchanged, recomputes the candidate set under lock, and either
  matches exactly or fails before mutation as stale.
- Human display remains `<name>@<ordinal>`. `head` means latest successful
  ordinal only. Neither head nor name protects material; exact authority is
  Runtime ID plus semantic revision digest, and protection comes from Manifest
  retention, Workspace desired/applied state, or exact Docker use.
- Semantic source digest is immutable revision authority. Recorded image
  content digest is internal verification evidence. Docker tag and `.Id` never
  appear as a selector, reference kind, confirmation target, or routine human
  concept.
- Runtime name may be reused after confirmed deletion. A fresh Runtime receives
  a fresh ID/reference. An old deletion receipt cannot resolve to or affect the
  new same-name Runtime.

### Public contract

#### Existing create and build target correction

```text
runtime create [--copy-source-from <standard|runtime-name>] --name <name> \
  [--format text|json]
review runtimes
runtime build --id <runtime-ref> [--format text|json]
```

- Consume ADR 0079 and `docs/00` through `docs/04` for the fixed
  `--copy-source-from` input; this packet does not own copy semantics. Create
  remains `RoleAct`, `EffectCreate`,
  with the command-owned `tool_local` fixed target `runtimes`, explicit empty
  `target_inputs`, impact one/no/no/no, and no consumed reference. Its confirmed
  child output produces one `runtime` reference whose kind differs from the
  fixed creation scope.
- Build becomes `RoleAct`, `EffectWrite`, with required single opaque `--id`
  kind `runtime`, `TargetKind=runtime`, `TargetInputs=[--id]`,
  `TargetIDInput=--id`, no parent/fixed target, and impact one/no/yes/no. It no
  longer accepts `--name` or omission-based action selection.
- `review runtimes` is `RoleDiscover`, `EffectRead`, produces `runtime`
  references, offers `runtime build` as its exact action, requires explicit yes
  before crossing into the action, and remains read-only on cancellation,
  unavailable terminal streams, or redirected use. Delete and prune retain
  their own stronger typed confirmation contracts; Review does not weaken them.
- This corrects target identity instead of treating a name-selected existing
  Runtime mutation as a write to the catalog singleton. It is a deliberate
  pre-public argv change with no `--name` build alias or hidden rediscovery.

#### Discovery commands

Revise existing reads to `RoleDiscover` because they produce references:

```text
runtime list [--format text|json]
runtime show --name <name> [--format text|json]
runtime history --name <name> [--format text|json]
```

- `runtime list` produces one `runtime_ref` for every managed Runtime and the
  built-in standard Runtime, plus a `revision_ref` for every existing head
  regardless of local image availability. Delivery is complete and coverage
  exhaustive.
- `runtime show` and `runtime history` produce the selected `runtime_ref` and a
  `revision_ref` for every successful revision. Delivery is complete and
  coverage is `not_applicable` because the selected Runtime is scalar and its
  complete revision inventory is nested task-owned state.
- Name remains a human discovery input with bounded local completion. It is not
  consumed by a mutation. If post-deletion reference lookup is required after
  implementation experiments, add a mutually exclusive `--id` read input only
  after Catalog grammar can declare exactly-one-of; do not weaken action target
  binding.

Add the read-only planning leaf:

```text
runtime prune dry-run [--format text|json]
```

- Role/effect: `RoleDiscover`, `EffectRead`.
- Inputs: format only; no age, name, Docker selector, force, or automatic
  confirmation input in V1.
- Output: one required `plan_ref` of kind `runtime-prune-plan`, `observed_at`,
  complete ordered `candidates`, complete `protected` items and reasons, exact
  local logical byte evidence, bounded image virtual-byte evidence, and explicit
  unknown reclaimability. Empty candidate sets still produce a typed plan and
  `empty=true`.
- Delivery is complete; collection coverage is exhaustive for Runtime-owned
  revision/candidate image material visible in the bounded observation.
- It performs no directory creation, journal write, timestamp update, Docker
  removal, tag repair, or manifest cleanup.

#### `runtime delete`

```text
runtime delete --id <runtime-ref> --confirm=delete [--format text|json]
```

- Capability: use one `runtime.lifecycle` capability containing existing
  create/build/read and new retirement outcomes rather than parallel partial
  capabilities. This Product Owner-fixed pre-public replacement is implemented
  atomically in the capability ledger; no alias remains.
- Role/effect: `RoleAct`, `EffectWrite`.
- Inputs: required single text `--id`, reference kind `runtime`; required single
  enum `--confirm` whose only value is `delete`; optional format. No name,
  ordinal, tag, path, force, recursive, age, or cascade input.
- Binding: `TargetKind=runtime`, `TargetInputs=[--id]`,
  `TargetIDInput=--id`, no parent input, no fixed target.
- Impact: cardinality one, notification no, access change yes, destructive yes.
- Preconditions: exact managed target exists or has a terminal receipt;
  standard is protected; the complete authoritative current/retained Workspace
  Manifest revision inventory and all Workspace desired/applied collections
  validate; no protected revision or owned/foreign container use is observed;
  migration evidence is verified; exact source/snapshot/journal paths and owned
  image evidence validate; confirmation matches.
- Complete result: task, exact target reference/name, state
  `deleted|already_deleted`, source/snapshot logical-byte dispositions,
  revision and build-candidate image dispositions
  `removed|already_absent|preserved_shared`, removed exact tag count, unknown or
  authoritative reclaimed bytes, receipt revision, and invariants stating zero
  Workspace Manifest, Workspace, `workspace_id`, home, applied receipt, Project
  root, credential, and shared resource deletions.
- Human output first shows `Delete Runtime <name>`, counts and protection
  review, irreversible source/snapshot/history loss, separately uncertain Docker
  reclaim, and the exact confirmation token. It never asks for or prints a
  Docker ID.

#### `runtime prune apply`

```text
runtime prune apply --plan <runtime-prune-plan-ref> --confirm=prune \
  [--format text|json]
```

- Role/effect: `RoleAct`, `EffectWrite`.
- Inputs: required single opaque `--plan`; required single enum `--confirm` with
  only `prune`; optional format. No re-selection filters.
- Binding: `TargetKind=runtime-prune-plan`, `TargetInputs=[--plan]`,
  `TargetIDInput=--plan`, no parent, no fixed target.
- Impact: cardinality many, notification no, access change yes, destructive yes.
- Preconditions: exact plan recomputes unchanged under lifecycle lock; all
  candidates remain present or are already completed by the same journal; no
  new binding/applied/container protection exists; every image has exact
  Runtime ownership evidence.
- Complete result: task, exact plan reference, state
  `applied|already_applied|empty`, per-item Runtime/revision display identity and
  disposition, exact tag removal count, preserved-shared state, bounded byte
  evidence, journal receipt revision, and zero source/snapshot/history deletion.
- A plan that changed before its journal starts returns
  `runtime_prune_plan_stale`, `change_state=none`, and `runtime prune dry-run` as
  read-only recovery. It never silently applies a subset or a refreshed plan.

#### `runtime restore`

```text
runtime restore --id <runtime-revision-ref> [--format text|json]
```

- Role/effect: `RoleAct`, `EffectWrite`. It changes the availability facet of an
  existing immutable revision; it does not create or replace revision identity.
- Binding: required single opaque `--id` kind `runtime-revision`;
  `TargetKind=runtime-revision`, `TargetInputs=[--id]`,
  `TargetIDInput=--id`, no parent, no fixed target.
- Impact: cardinality one, notification no, access change yes, destructive no.
- Preconditions: managed Runtime and retained immutable snapshot exist and
  validate; no retirement is active; Docker/build boundary is available.
- Behavior: if exact compatible image material already exists, return
  `already_available`. Otherwise build only from the retained snapshot to a
  journaled staging tag, run compatibility validation, inspect content digest,
  compare to the recorded digest, and publish the exact normal tag only on
  equality. On mismatch, remove or retain the exact candidate according to its
  journal and return `runtime_revision_unrestorable`; do not update manifest,
  semantic digest, ordinal, Workspace Manifest, or Workspace.
- Complete result: task, revision reference and human display, availability
  `restored|already_available`, digest match boolean, build artifact disposition,
  and no revision append.

#### Existing build contract refinement

- Before Docker build, persist an owner-only build journal containing Runtime
  reference, semantic source digest, exact immutable temporary snapshot, exact
  staging tag, and monotonic phase.
- Build supplies exact owner/Runtime/revision labels through trusted CLI
  arguments and verifies them after load; Dockerfile content cannot broaden or
  replace them.
- Only compatibility-validated, digest-inspected material may be promoted to the
  final Runtime tag and manifest. Every process-death boundary is classifiable
  as no change, resumable candidate, committed revision, or terminal receipt.
- BuildKit cache remains explicitly outside the journal and prune outcome.
- Duplicate semantic source returns `no_change` only if exact image availability,
  compatibility, and recorded content digest validate. Otherwise return
  `runtime_revision_unavailable` with `runtime restore` recovery.
- Direct build consumes its Runtime reference unchanged through the application
  port. Only the separate Review may discover/select a target; the action does
  not normalize a name or decode the reference.

### Human presentation

Routine text uses domain language:

```text
Runtime frontend
Head: frontend@4
Source: editable · 12.4 MiB
Revisions: 4

frontend@3
Availability: unused local image · prune candidate
Last successfully applied: 2026-08-20T04:05:06Z
Last used: unknown
Snapshot: retained · 11.9 MiB
Image size: 1.8 GiB virtual (shared/reclaimable bytes unknown)
```

- `name@ordinal` leads; short semantic digest may appear in details.
- `available`, `missing`, `mismatched`, `unknown`, and `pruned` are explicit
  availability states. `ready` continues to mean a valid successful revision
  exists and must not imply local image availability.
- Protection reasons are closed values: `standard`,
  `manifest_current_binding`, `manifest_retained_revision`,
  `workspace_pending_adoption`, `workspace_last_successful_applied`,
  `workspace_observed_runtime`, `workspace_container`, `external_container`,
  `active_build`, `active_retirement`, `migration_unverified`, or
  `observation_unknown`. A candidate may carry multiple reasons; preview never
  collapses desired, applied, retained-history, and observed edges into one.
- Last-successfully-applied may render exact
  `last_successful_entry.reconciled_at`. `last_used` renders an exact observed
  time or `unknown`; absence is never interpreted as never-used and migration
  never synthesizes use.
- Source/snapshot bytes are logical. Image bytes are virtual/non-additive and do
  not become a reclaim promise.
- Destructive Review names source, snapshots, history, images, and preserved
  non-targets separately. There is no generic “cleaned” success.

### JSON and state schema

#### Public JSON

Keep public result schemas at version 1 because this is a pre-public ideal
contract reset, but replace provisional infrastructure-shaped Runtime fields in
the same implementation:

- Add `runtime_ref` and `revision_ref` opaque fields.
- Retain semantic `source_digest`, ordinal, created time, name, kind, and
  editable `source_path` needed by the user.
- Replace public Docker `image` selector, `image_digest`, and immutable
  `snapshot_path` with typed nested `availability`, `storage`, `last_used`, and
  `snapshot` state. Exact Docker identities and private snapshot paths remain
  infrastructure-only.
- Use required state enums plus nullable values for real unknowns. Preserve
  empty revision arrays and exact task/scope identity.
- Add task-owned schema-1 envelopes for delete, prune dry-run, prune apply, and
  restore. Do not reuse the current create/build report with optional booleans
  for destructive results.
- Protection objects use `workspace_manifest_id`, optional immutable
  `manifest_revision`, and `workspace_id` when those identities are relevant.
  They never expose `context`, `context_id`, `project_id`, or `instance_id`
  compatibility fields.
- Update catalog schema fixtures, capability ledger, generated architecture
  data, human fixtures, and negative canaries atomically. No alias fields or
  compatibility projection remain before first public V1.

#### Persisted state

- Keep `RuntimeManifest` schema 1 and its immutable revision meaning. No
  revision tombstone, image-availability bit, last-used timestamp, prune bit,
  copy lineage, or deletion marker belongs in the authoritative manifest.
- Add separate owner-only schema-1 build journal and retirement journal/receipt
  stores with strict versions, exact stable references, monotonic phases,
  bounded candidate counts, canonical private paths, and atomic writes.
- Treat `last_successful_entry.reconciled_at` only as
  last-successfully-applied evidence. Do not add or infer persisted
  `last_used_at` in WP03. The Product Owner fixed no V1 usage receipt; migrated
  and receipt-free history remains `unknown`. Any future usage capability is
  outside this packet.
- A completed Runtime retirement receipt retains old stable Runtime reference,
  former human name, completion time, and non-sensitive disposition summary.
  It contains no source path/content, Docker output, Project root, Manifest
  display name, Workspace ID, or credential fact.
- Existing current Runtime manifests need no rewrite. New sidecar stores are
  created only by future write commands, never reads. Exact supported
  predecessor migration must initialize ADR 0079 Workspace applied-state certainty
  before prune is enabled; unknown legacy evidence fails closed. The migration
  preserves predecessor Context UUID as WorkspaceManifestID and predecessor
  ProjectInstance UUID as WorkspaceID, without dual vocabulary or aliases.

#### Migration ordering and gate

1. Consume the integrated ADR 0079 Workspace Manifest and Workspace stores,
   immutable desired revision model, and authoritative retention-inventory
   query first; commits `07535a9` and `428812f` are upstream evidence, not WP03
   implementation work.
2. Run a bounded migration preflight and backup. Whether it requires the cluster
   stopped and no attachments, as fixed by ADR 0079 and the durable security/
   harness contracts; WP03 does not weaken that precondition.
3. Preserve IDs exactly, then synthesize `last_successful_entry` only when
   predecessor state plus sufficient read-only Docker evidence proves the exact
   desired spec. Otherwise record migration-unverified/pending state.
4. Re-enter shared state only through explicit `cluster up`; Workspace runtime
   adoption remains exclusive to explicit entry. Migration and reads perform no
   Runtime cleanup.
5. Enable `runtime prune dry-run`, apply, and delete only after every migrated
   Workspace contributes exact current/applied/pending evidence or a
   fail-closed barrier. Do not ship a dual Context/Manifest reader as fallback.

### Protection and candidate algorithm

Under the installation lifecycle lock, application obtains one complete typed
protection snapshot through task-owned ports:

1. Validate the complete Runtime catalog, active build/retirement journals, and
   exact target references.
2. Load the integrated complete authoritative Workspace Manifest retention
   inventory: every current immutable desired revision plus every retained
   revision body exposed by the durable store. Index each
   `(runtime_id, semantic_revision)` binding and label current versus retained;
   this packet never shortens that inventory.
3. Validate every durable Workspace's `workspace_manifest_id`, current desired
   comparison, pending desired adoption, and `last_successful_entry`. Index the
   exact Runtime revision for applied and pending edges separately. Unknown,
   predecessor-only, or migration-unverified state creates a fail-closed
   barrier rather than an inferred edge.
4. Keep `last_reconciliation_failure` diagnostic: it is not a deletion
   authority by itself. If its attempted Manifest revision body is retained and
   exactly resolves a Runtime, that Runtime is already protected as
   `manifest_retained_revision`; otherwise do not reconstruct a binding from
   labels or timestamps.
5. Ask infrastructure for bounded exact Tobari-owned Workspace container
   observations, including stopped containers, and compare inspected image
   content IDs internally to recorded revision evidence. Any use protects.
6. Inspect foreign container use of exact candidate content. It is not Tobari
   authority, but it prevents forceful content removal and either blocks or
   results in exact untag-only preservation.
7. For prune, select present managed revision images and confirmed abandoned
   build candidates with no protection. Standard and BuildKit cache are always
   excluded. Head has no special authority protection.
8. For Runtime delete, any current/retained Manifest, pending adoption,
   last-successful applied, or owned-container protection blocks the
   whole action. A foreign container blocks before mutation. Foreign tags
   without container use cause exact Tobari untag and `preserved_shared`.
9. Revalidate exact snapshot safety, source safety, image ownership, and the
   canonical candidate-set token immediately before the first destructive step.

The dry-run uses the same pure planning function and typed observation but no
lock-held effect or state write. Apply repeats planning under the lock; it does
not trust stale presentation.

### Edge-case decisions

- **Source deletion:** only whole-Runtime delete removes editable source. It is
  quarantined atomically after the journal starts and removed only as part of
  the confirmed transaction.
- **Revision deletion:** unsupported. History and snapshots remain until whole
  Runtime deletion.
- **Zero-revision Runtime:** list/show reports `ready=false`, empty history,
  source bytes, and no image availability. It can be deleted normally; prune has
  no revision candidate.
- **Head revision:** latest successful presentation only. If unreferenced and
  unused it may be pruned; history remains ready but availability becomes
  `pruned/missing` until exact restore.
- **Workspace Manifest deletion:** never cascades. Deletion does not imply that
  a retained immutable revision body or Workspace applied/pending edge has
  disappeared. Only after the ADR 0079 authoritative retention inventory and every
  Workspace edge no longer references the Runtime can a later dry-run show the
  image as a candidate.
- **Workspace using an old image:** current desired binding is irrelevant; exact
  `last_successful_entry` or owned stopped/running container use protects the
  image. Pending adoption of a different desired revision is displayed and
  protects that desired revision independently. Runtime cleanup never performs
  adoption, and an attached Workspace requiring recreation remains blocked.
- **External image disappearance:** reads report missing without state repair.
  Existing binding remains truthful authority. `runtime restore` attempts exact
  reconstruction; mismatch/unavailable input fails without revision rewrite.
- **Failed build artifact:** only a journaled, label-verified candidate can be
  resumed or pruned. Unattributed images and BuildKit cache are not claimed.
- **Interrupted delete:** active Runtime directory is moved to one exact private
  quarantine only after every protection passes and the journal is durable.
  Readers report `retiring`; retry resumes forward. Do not reactivate partially
  retired authority automatically.
- **Interrupted prune:** journal records each exact candidate disposition.
  Already-absent material under the same started plan is idempotent; unrelated
  candidate-set drift before journal creation is stale and makes no change.
- **Shared image content:** remove only exact Tobari tags without force. Report
  content preserved when another tag shares it; never claim shared bytes
  reclaimed.
- **Disk usage:** exact logical source/snapshot bytes plus bounded image virtual
  bytes and certainty. Do not sum shared virtual sizes or expose global Docker
  inventory.
- **Last used:** `last_successful_entry.reconciled_at` is shown only as
  last-successfully-applied. Unless an independently approved exact usage
  receipt exists, last-used is `unknown`; passive container activity,
  filesystem metadata, migration, and absence do not manufacture usage facts.
  Review metadata never overrides exact current protection.

### Fault and recovery contract

Add task-specific stable faults in addition to generic mutation faults:

| Code | Phase/change state | Retry | Exact next command |
| --- | --- | --- | --- |
| `runtime_protected` | precondition/none | no | `runtime show` |
| `runtime_revision_referenced` | precondition/none | no | `runtime history` |
| `runtime_workspace_image_in_use` | precondition/none | no | `runtime show` |
| `runtime_external_image_in_use` | precondition/none | no | `runtime show` |
| `runtime_retirement_observation_unknown` | observation/none | no | `doctor` |
| `runtime_prune_plan_stale` | precondition/none | no | `runtime prune dry-run` |
| `runtime_revision_unavailable` | observation/none | no | `runtime restore` |
| `runtime_revision_unrestorable` | verification/none | no | `runtime history` |
| `runtime_restore_interrupted` | mutation/partial | no until read | `review runtimes` |
| `runtime_restore_outcome_unknown` | mutation/unknown | no until read | `review runtimes` |
| `invalid_runtime_restore_result_partial` | verification/partial | no | `runtime history` |
| `invalid_runtime_restore_result_confirmed` | verification/confirmed | no | `runtime history` |
| `runtime_retirement_interrupted` | mutation/partial | no until read | `runtime show` |
| `runtime_prune_interrupted` | mutation/partial | no until read | `runtime prune dry-run` |
| `invalid_runtime_retirement_result` | verification/confirmed or partial | no | `runtime show` |

- “Retry no” means the caller must run the read-only next action and follow its
  result; an exact journal may then authorize resume. Timing never grants replay.
- Before the journal's first destructive phase, cancellation is
  `canceled/none/retryable`. After any destructive phase, journal truth wins and
  cancellation is partial or confirmed, never generic retryable cancellation.
- After terminal commit, emit through the mutation-complete boundary. Output
  encoding/write failure reports confirmed state and read-only recovery; it
  never invites replay.
- Preserve valid structured adapter outcome faults before generic context
  cancellation. Collapse any unclassified mutation result to
  `unclassified_mutation_outcome`, unknown, non-retryable, with read-only
  reconciliation.
- Recovery commands are exact catalog paths only; do not append unchecked
  references or flags to fault metadata.

### Layer ownership

- **Domain:** Runtime/revision/plan opaque references; lifecycle availability,
  protection reasons, storage/last-used certainty, candidate/disposition,
  journal phase, task inputs/results, exact target validation, and invariants.
  No Docker or filesystem type enters domain.
- **Application:** separate list/show/history discovery, delete, prune plan,
  prune apply, restore, and refined build use cases. Own the complete protection
  join, plan equality, lifecycle-lock sequence, mutation intent/target/impact,
  partial-result preservation, and task identity validation. Ports are
  task-specific reads/effects, not one unrestricted Runtime executor.
- **Infrastructure:** strict owner-only manifests/journals/receipts; safe
  directory quarantine/removal; bounded source/snapshot byte observation;
  Workspace Manifest retention and Workspace applied/pending stores; exact Docker
  label/tag/content/container inspection; non-forced untag/removal; staged
  build/restore; atomic persistence and synchronization.
- **CLI/catalog:** complete command specs, roles/effects, reference flows, typed
  flags/defaults/conflicts, mutation contracts, human Review, schema-1 outputs,
  stable faults/recovery, mutation-complete output, completion, and generated
  projections. No lifecycle facts are reimplemented in handlers.

### Data and control flow

```text
runtime list/show/history
  -> validated task-owned observation
  -> derive runtime/revision opaque refs
  -> complete semantic result
  -> human or schema-1 projection

runtime prune dry-run
  -> complete Runtime + retained Manifest revisions
     + Workspace desired/applied/pending + Docker observation
  -> pure protection/candidate plan
  -> deterministic plan_ref + complete review result
  -> no write

delete / prune apply / restore
  -> Catalog typed input + reference validation + confirmation
  -> operation Intent + exact TargetRef + Impact
  -> installation lifecycle lock
  -> re-read and revalidate all task dimensions
  -> durable journal before first effect
  -> exact filesystem/Docker adapter steps
  -> authoritative receipt/result validation
  -> mutation-complete output boundary
  -> human or schema-1 projection
```

Lock ordering must be one durable rule, tentatively:

```text
installation lifecycle -> Runtime store/journal -> Workspace Manifest store
-> Workspace desired/applied store -> bounded Docker call
```

Implementation must verify and revise this order against existing call graphs
before code. Do not hold a lower-order lock while acquiring a higher-order one.

### Security and trust boundary

- The existing trusted-host Runtime boundary expands from create/build/read to
  exact private lifecycle journals, exact directory quarantine/removal, and
  exact Docker inspect/untag/remove/restore operations.
- It does not expand into Workspace home content, credentials, arbitrary host
  paths, arbitrary executables, daemon-global prune, Docker socket delegation,
  remote registries, or new provider/network APIs.
- User-authored Dockerfile and Docker output remain untrusted. Restore has the
  same possible external access as current build, bounded by the existing
  Dockerfile boundary; it grants no new Tobari network client.
- Build owner labels are trusted only because infrastructure supplies and then
  verifies them together with Runtime stable identity and content digest.
- Every destructive target is resolved from Tobari authority before Docker
  calls. A Docker label, tag, `.Id`, timestamp, ordinal, head, or displayed name
  alone never authorizes deletion.
- Public results are secret-free and visibly escape untrusted external text.
  Raw build logs remain bounded diagnostics and never enter stable receipts.

## Implementation dependency order

1. **WP01 + WP02 completion audit — satisfied upstream.** ADR 0079, integrated
   `docs/00` through `docs/04`, commits `07535a9`/`428812f`, and HEAD `52a53bc`
   record final Manifest retention, migration certainty, Runtime copy isolation,
   and desired/applied interfaces. WP03 does not reinterpret them.
2. **WP08 completion.** Wait for `WP08_IMPLEMENTATION_COMPLETE`, re-run its
   focused evidence on the integrated checkout, and consume its one recursive
   Catalog output/reference traversal without a Runtime-specific duplicate.
3. **WP03 ADR, Catalog/domain contract, and negative tests.** Promote the
   accepted Runtime lifecycle decision by revising ADR 0067. Implement the fixed
   `runtime.lifecycle` capability and pre-public schema reset; add reference
   kinds, command specs, effects, target
   bindings, impact, confirmation, faults, recursively derived output-reference
   paths, output schemas, and negative canaries for revision delete/gc/raw
   Docker/source-image delete.
4. **Pure lifecycle model.** Add availability, protection, candidate plan,
   storage/last-used certainty, journal/receipt phases, task identities, and
   conformance fixtures independent of presentation.
5. **Applied-state audit and lock foundation.** Consume exact Workspace
   `last_successful_entry`, pending-adoption, retained-Manifest, and migration
   evidence from ADR 0079's integrated implementation; fail closed when it is
   insufficient. Promote
   and test lock ordering across Runtime, Workspace Manifest, Workspace, and
   Docker.
6. **Build journal and availability reads.** Journal staged builds, enforce
   owner labels, detect no-change image loss/mismatch, and surface exact
   availability/storage/last-used state before enabling deletion.
7. **Restore slice.** Add exact snapshot rebuild, compatibility/digest equality,
   staged promotion, fault/recovery, and interruption tests.
8. **Prune planning and apply.** Reuse one pure candidate engine, add zero-write
   dry-run, deterministic plan reference, stale rejection, journaled apply,
   shared-image preservation, and machine/human results.
9. **Whole-Runtime delete.** Add protection barrier, quarantine, exact image and
   filesystem disposition, receipt/name-reuse semantics, and recovery.
10. **Presentation/docs/generated integration.** Remove routine Docker
   identities/paths, add semantic availability/storage output, promote durable
   contracts/ADR, update README/help/ledgers/site/fixtures, and remove this
   temporary packet only after all evidence.
11. **Repository and platform gates.** Run focused/race/integration scenarios,
    `task check`, `task security`, `task public:check`, and
    `task release:check`; record supported Docker platform observations.
12. **Commit and notify.** Record final interfaces, all gates, exact HEAD/status,
    packet retention/removal, and WP04 readiness. Notify the root orchestration
    thread with `WP03_IMPLEMENTATION_COMPLETE`; use
    `WP03_IMPLEMENTATION_BLOCKED` if an integration or gate fact prevents
    completion.

Do not implement prune/delete before the exact durable retained-Manifest inventory,
Workspace applied/pending evidence, migration gate, and lifecycle-lock
protection exist. Do not change public output in isolation from Catalog-derived
schema fixtures and the active conformance packet.

## Verification

- **Domain/unit:** opaque reference construction/validation; target mismatch;
  immutable revision/no-delete; zero revisions; head not authority; protection
  join; plan determinism; stale plan; explicit unknown/empty/zero; disposition;
  journal monotonicity; idempotent receipt; task/request dimension validation.
- **Catalog/contract:** roles/effects, required ref producer reachability,
  `target_id_input`, exact impact, confirm enums, input cardinality/default/
  conflict, recursive nested reference paths, complete delivery/coverage,
  schema-1 fields, exact recovery paths, root index byte limit, no fixed-target
  misuse, and removed/hypothetical command canaries.
- **Infrastructure filesystem:** owner-only/symlink/special/path traversal,
  quarantine atomics, source/snapshot byte totals, zero-revision delete,
  manifest-last/receipt persistence, fsync/write/cancellation/process-death at
  every phase, and same-name new-ID safety.
- **Docker:** exact labels/tags/content ID, missing/mismatched image, shared tags
  and layers, stopped/running owned and foreign containers, no force, no global
  prune, candidate validation failure, BuildKit exclusion, restore equality,
  mutable/unavailable base failure, and bounded hostile output.
- **Manifest/Workspace integration:** multiple bindings to one revision,
  different revisions, desired changed while old Workspace remains applied,
  pending adoption, stopped/running container, Manifest deletion followed by
  authoritative retention expiry and dry-run, concurrent Manifest mutation/
  deletion with build/entry/delete/prune, home/ID/applied-receipt/auth/root byte
  preservation, and unknown migrated applied evidence.
- **Mutation outcome:** pre-I/O cancellation, mid-Docker interruption,
  mid-filesystem interruption, terminal-commit cancellation, output encoding/
  write failure, read-before-retry, exact resume, already-complete receipt, and
  unclassified outcome collapse.
- **Presentation:** typed semantic fixture and answer key; zero-revision,
  empty-prune, protected, missing, shared, partial, unknown bytes/last-used, and
  same-name reuse; negative canaries prove no inference from head/name/order/
  tag/path/Docker ID.
- **Agent readiness:** one scoped help retrieval plus `runtime list` or prune
  dry-run produces exact refs and action argv; zero external processing,
  provider notation decoding, source inspection, and exploratory Docker calls.
- **Manual supported-platform observation:** Docker Desktop/Linux Engine as
  required by repository policy; record exact version, image-store mode,
  non-forced sharing behavior, interruption point, bounded logs, and synthetic
  assets.
- **Required profiles:** focused tests, race tests where shared state is
  involved, `task check`, `task security`, `task public:check`, and
  `task release:check`.
- **Generated/artifact checks:** catalog, capabilities, schemas, claims,
  architecture data/site, README/help examples, embedded runtime source snapshot
  equality when affected, public guard, and clean expected diff.

## Rollout, compatibility, and rollback

- This is a deliberate pre-public V1 contract completion. No released Runtime
  delete/prune contract exists, so no command alias, deprecated flag, or hidden
  fallback is added.
- Existing unpublished `runtime build --name NAME` and omission-based direct
  build selection are replaced by `runtime build --id <runtime-ref>` plus the
  read-only `review runtimes` human flow. There is no build-name alias or action-
  local rediscovery. `runtime create` produces the ref needed for the immediate
  next build.
- Public Runtime success JSON remains named schema 1 but is replaced atomically
  with semantic reference/availability/storage fields before first release.
  Consumers of unpublished provisional `id`, Docker `image`, `image_digest`,
  and `snapshot_path` fields must update; no dual output persists.
- Existing `RuntimeManifest` schema-1 records remain valid and are not rewritten
  by reads. Build/retirement journals and receipts are new strict schema-1
  sidecars created only by mutations.
- Workspace Manifest/Workspace migration, retained revision-body policy, and
  historical evidence certainty are owned by ADR 0079 and `docs/00` through
  `docs/04`. Runtime prune stays
  unavailable or fails closed while any Workspace is migration-unverified; an
  explicit unknown last-used diagnostic does not by itself weaken exact
  desired/applied/container protection.
- Safe rollback before any retirement mutation has run is code/catalog rollback.
  After a prune, retained snapshot plus exact restore is the rollback path. After
  confirmed whole-Runtime delete, source/history deletion is intentionally
  irreversible; Docker shared content is not a replacement for authority.
- Never roll back by reintroducing a raw Docker command, reading a retired
  manifest as active, reusing an old reference for a new name, or weakening a
  protection check.

## Documentation promotion

Before implementation completion, promote these durable conclusions:

- `docs/00_theses.md`: Runtime lifecycle closure; immutable revision history;
  image material as replaceable availability; stable reference authority.
- `docs/01_product_contract.md`: exact commands, roles/effects, human/JSON,
  confirmation, protection, restore, faults, recovery, and compatibility.
- `docs/02_architecture.md`: domain/app/infra/CLI ownership, applied-state join,
  lock order, journals, quarantine, and Docker boundaries.
- `docs/03_security_model.md` and threat model: exact deletion authority,
  shared Docker content, no force/global prune, protected negative targets,
  untrusted labels/output, and interruption.
- `docs/04_harness.md`: lifecycle fixtures, reference graph, zero-write dry-run,
  race/interruption/platform gates, and generated contract agreement.
- ADR 0067 revision/new ADR: whole Runtime retirement, no revision delete,
  reviewed prune, exact restore limit, state compatibility, and cross-packet
  Workspace Manifest/Workspace dependency.
- README and catalog-derived public/generated data only in the implementation
  change, never in this packet-authoring session.

## Fixed decisions and implementation audit inputs

The Product Owner fixed the following WP03 decisions: one
`runtime.lifecycle` capability; pre-public JSON replacement with no aliases;
WP08-owned recursive Catalog reference derivation; exact-or-fail restore with no
V1 archive; bounded idempotency receipts without generic GC; prune eligibility
for a fully unused head image because head is presentation; non-forced
shared-image preservation; and no V1 usage receipt, leaving `last_used` unknown.
Implementation may not reopen these choices.

The remaining audit/evidence inputs are:

1. Consume ADR 0079's complete current/retained Manifest revision inventory
   through one exact interface without inventing a narrower retention bound.
2. Consume ADR 0079 and the durable security/harness Docker migration evidence and
   cluster-stopped/no-attachment (or equivalent) precondition; unverified state
   keeps cleanup fail-closed.
3. Verify WP08's recursive Catalog paths on the integrated Runtime schemas.
4. Prove the exact lock order and journal/quarantine phase ordering.
5. Record supported-platform Docker shared-tag/container behavior while
   preserving the fixed no-force/block-or-preserve rule.
6. Promote and test the exact bounded receipt-retention mechanism without
   changing its fixed idempotency outcome.

The durable architecture assigns the root-scoped Git fallback to the bound
Workspace Manifest and applies session defaults to each later child session.
Neither is Runtime-retirement authority. If a future durable change adds an
authoritative Runtime revision edge, that contract must expose it through the
same complete retention/protection query before cleanup is enabled.

## Completion notification

After every implementation, test, durable-documentation, generated-contract,
and gate task completes, commit the integrated result and notify the root
orchestration thread `01a02c51-885b-7b80-a66f-05850f48ba4d` with exactly
`WP03_IMPLEMENTATION_COMPLETE`. If an upstream integration fact or required gate
prevents completion, notify it with exactly `WP03_IMPLEMENTATION_BLOCKED`.
Include final public/internal interfaces, all gate results, exact HEAD and fully
explained status, whether this temporary packet was removed or why retained,
and explicit WP04 implementation readiness.
