# Work Plan: Make `status` the CWD-centric Workspace home

- Status: Fixed
- Decision state: Accepted by Product Owner
- Implementation state: Planned after all upstream implementations through
  WP09; no production, test, durable-document, Catalog, schema, generated, or
  release-file work has started
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)
- Higher/upstream decisions, in fixed order: promoted WP01+WP02
  [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md)
  and durable [docs/00](../../00_theses.md) through
  [docs/04](../../04_harness.md), the completed WP08 Catalog/output contracts
  in [Architecture](../../02_architecture.md) and [Harness](../../04_harness.md),
  completed [ADR 0080 Runtime lifecycle](../../decisions/0080-close-the-managed-runtime-lifecycle.md), completed WP04
  [ADR 0082 build surfaces](../../decisions/0082-release-and-research-build-surfaces.md), completed WP05
  [ADR 0083 Host Loopback authority](../../decisions/0083-name-the-physical-host-loopback-authority.md), WP07
  [first-use recovery](../first-use-progress-recovery/plan.md), and WP09
  [service exposure UX](../service-exposure-ux/plan.md)

## Chosen approach

Implement `status` only after WP01+WP02, WP08, WP03, WP04, WP05, WP07, and WP09
production implementations complete and the mandatory re-baseline gate
validates their actual integrated HEAD, schema, Catalog, ports, owner summaries,
behavior, and file ownership. Status is a presentation/read-model consumer, not
a second domain owner.

Status adds no parallel copy, Runtime lifecycle, wait-state, exposure-owner,
reference, journal, protection-planner, schema, or migration types.

The application result preserves independent facts for the selected Workspace:

```text
selection + desired + AppliedEntry + latest attempt + migration evidence
  + observed runtime + Runtime authority/material + attachment
  + cluster/session/creation slices + permission/service summaries
  -> closed orthogonal axes
  -> one structured primary Next + separately ordered Attention
```

Root selection happens before Manifest selection. An explicit Manifest or the
WP01 installation-level default selects one Workspace at that root. Other
Manifest-bound Workspaces
at exactly the same root are an exhaustive logical sibling summary, but only
the selected Workspace receives live runtime/policy/service observation.

Manifest activation slices remain separate:

- cluster projection: desired/applied/observed/failure, reconciled only by
  `cluster up`;
- entry: desired versus AppliedEntry and runtime observation, reconciled only
  by explicit Workspace entry;
- session defaults: current versus next child-session semantics as decided by
  WP 01 owners; and
- creation defaults: current desired versus the selected Workspace's create-once
  receipt, never an entry adoption request.

`--details` renders more of this same snapshot. It performs no additional I/O,
and JSON is always the complete projection.

For Runtime, keep these facts orthogonal to the Workspace entry axis:

```text
immutable revision authority: ready / not ready
local execution material:     available / missing / mismatched / unknown / pruned
historical last use:           unknown unless an exact approved receipt exists
```

Neither availability nor a Docker observation changes desired or AppliedEntry
authority. Status can recommend an owning lifecycle/review command but never
builds, restores, prunes, deletes, or calculates complete prune eligibility.

## Concept budget

- Public durable resources added by this packet: zero.
- Public durable resources consumed: exactly Workspace Manifest, Runtime, and
  Workspace.
- Subordinate public values consumed: Runtime revision and Project root.
- Public task outcome revised: one, `tobari status`.
- Public input added: one boolean, `--details`; selector spelling becomes the
  WP 01 final `--manifest` with no old alias.
- Internal status concepts: one task-owned `StatusHomeSnapshot`, upstream
  semantic fact types, one typed `StatusPrimaryNext`, and ordered typed
  `StatusAttentionItem` values. Headings and JSON nesting add no resources.
- Copy provenance/lineage concepts added: zero. Runtime lifecycle authority or
  protection concepts added: zero; status consumes their semantic projections.

## Selection, scope, and authority

Selection is deterministic and domain-owned:

1. Canonicalize CWD without discovering a Git root or reading source content.
2. Read and validate final-V1 root indexes once.
3. Select the nearest indexed ancestor ProjectRoot across all
   WorkspaceManifestIDs. With no index, canonical CWD is the prospective root.
4. Group every logical Workspace whose ProjectRoot exactly equals that root.
5. If `--manifest` is present, resolve that exact persisted host Manifest for
   this invocation. Otherwise resolve only the WP01 installation-level default
   established by `manifest default set --name`.
6. If no persisted default exists, return a typed recommended draft selection
   with no ID, revision, authority, or acting target while retaining every
   same-root logical Workspace as a sibling. Never infer selection from one or
   more existing Workspace bindings.
7. Select at most one Workspace with the key `(ProjectRoot,
   WorkspaceManifestID)`. Other group members are siblings and cannot replace
   the selected target because they are attached, healthy, or listed first.

ProjectRoot is a trusted subordinate value in the host Workspace record, not a
Project resource or action ID. WorkspaceManifestID and WorkspaceID are exact
authority identities. Manifest name, generation, Runtime ordinal, image tag,
Docker ID, attachment ordering, and presentation are not authority.

An explicit Manifest changes only invocation selection. It does not change the
installation default, choose another root, retarget a Workspace, publish
desired state, or hide same-root siblings. There is no project-local current
Manifest, `manifest use`, or hidden current selector.

## Central read model

### Desired Manifest state

For a persisted selection, load and validate:

- WorkspaceManifestID, current immutable Manifest generation and semantic
  digest;
- immutable Boundary revision and source access;
- entry revision and exact RuntimeID plus Runtime semantic revision digest;
- cluster projection input revision;
- session defaults revision; and
- creation defaults revision.

A recommended first-use draft is a separate no-authority value. It has no
WorkspaceManifestID, published revision, generation, or applied claim.

### Last successfully applied Workspace state

For an existing selected Workspace, load and validate:

- WorkspaceID, ProjectRoot, WorkspaceManifestID, and home identity/path;
- create-once applied defaults receipt;
- last successful AppliedEntry: Manifest/entry/Runtime/resolved-spec revisions
  and confirmed time; and
- bounded latest failure/unknown attempt, which may coexist with the prior
  successful AppliedEntry.

Status never updates, clears, migrates, or synthesizes these records. The
exact-predecessor migration in WP 01 may have produced no verified AppliedEntry;
that absence must remain explicit.

### Observed state

Observe only exact selected/installation-owned resources with a bounded call
plan:

- selected work container/network/spec/health/principal evidence;
- attachment count/epoch evidence;
- Gateway/OPA/shared networks and applied cluster projection identity;
- a fixed selected-Workspace denial window; and
- a pure authenticated selected-Workspace live-owner summary when available.

Docker labels are compared with trusted host identities and revisions; they do
not become authority by containing a matching string.

### Runtime readiness, availability, and usage certainty

For the desired and last-applied Runtime bindings, consume the owning Runtime
read projection without collapsing its dimensions:

- `ready` answers whether the immutable revision authority exists and validates;
- `availability` answers whether exact compatible local execution image
  material is present (`available`, `missing`, `mismatched`, `unknown`, or
  `pruned`, subject to the implemented WP 03 enum);
- selected observed container state answers what is running/stopped now; and
- `last_used` is `unknown` unless a separately approved exact usage receipt is
  present.

Do not infer ready from availability, availability from AppliedEntry, or
historical use from `reconciled_at`, pending adoption, a running/stopped
container, Docker timestamps, or image presence. Routine output never includes
a raw tag, Docker image/container ID, or private snapshot path.

Selected-scope protection relationships may identify why the displayed
revision participates in the current Manifest, a retained revision made
available by WP 01, pending adoption, the last successful AppliedEntry, or the
selected observed container. The object must say its coverage is
`selected_project_snapshot`, not installation-exhaustive. It cannot decide
prune/delete safety; WP 03's locked complete planner owns that conclusion.

### Latest failure

Preserve exact attempted desired identity, phase, code, change-state
classification, and occurrence/bound information. Do not display raw private
causes. A later failure never erases the last successful AppliedEntry. A status
observation cannot clear or rewrite failure evidence.

### Consistency anchors

Capture Manifest current pointer/digest, Workspace identity/AppliedEntry/latest
attempt, Runtime binding, cluster desired/applied identity, and attachment owner
identity before external observation. Revalidate relevant anchors afterward.
If a change could mix facts, retry the bounded snapshot once. Continued anchor
churn fails retryably with `status_snapshot_changed` and emits no partial
snapshot.

## Orthogonal Workspace facts

Derive and serialize separate axes; never persist or emit an `overall_status`,
`adoption_state`, or equivalent lossy authority:

| Axis | Closed values | Rule |
|---|---|---|
| `workspace_presence` | `absent`, `present` | Workspace existence only; absence is not duplicated in entry state |
| `entry_state` | `current`, `pending`, `blocked_attached`, `failed`, `unknown` | Present Workspace entry relation only |
| `observed_runtime_state` | `not_observed`, `absent`, `stopped`, `running`, `drifted`, `unknown` | Bounded observation; invalid/foreign ownership is a fault |
| `runtime_revision_authority` | `ready`, `not_ready`, `unknown` | WP03 immutable revision authority |
| `execution_material_availability` | `available`, `missing`, `mismatched`, `pruned`, `unknown` | WP03 exact local material |

`entry_state` precedence is exact:

1. `failed` only when the latest final attempt targets the currently desired
   entry identity and failed;
2. `blocked_attached` only when desired differs, exact adoption requires
   recreation, and the exact selected Workspace is attached;
3. `pending` when desired differs and neither prior rule is authoritative;
4. `current` only when desired and AppliedEntry match and required consistency
   evidence validates; and
5. `unknown` when the relation cannot be established.

An older failure remains `latest_attempt`/historical detail and cannot override
a newer desired identity. Current may coexist with unavailable or stopped;
pending may coexist with available or running. State-integrity/migration state
is emitted only if WP01 exposes a real domain value; migrated-unverified
evidence is an explicit bounded field. Invalid or incomplete authoritative
state otherwise fails the command.

Cluster-only, session-only, or creation-only changes do not alter entry state.
Session behavior is consumed from final WP01 implementation without a WP06
rule. The exact recovery task comes from the integrated Catalog; WP06 never
invents argv or replays a possibly completed mutation.

## Shared cluster projection

Status reports a separate cluster projection object:

- desired aggregate projection digest derived from every current eligible
  Manifest plus the trusted-binary readiness revision;
- last successfully applied aggregate digest;
- latest bounded cluster reconciliation failure/unknown attempt or null;
- bounded observation of exact owned Gateway/OPA/resources and their reported
  applied revision.

Desired, last applied, observed, and latest failure are separate cluster axes.
Presentation may summarize them but JSON stores no lossy cluster-wide authority
enum. Only `cluster up` reconciles any of them.

The selected Manifest's cluster input is not the cluster applied identity by
itself. A Manifest desired revision never changes running shared resources.
Only `cluster up` may reconcile and publish cluster applied evidence.

Cluster observation, permission evidence, and live-service owner observation
remain independent. A broken cluster may make denial evidence unavailable but
does not automatically invalidate a successfully authenticated live-owner
snapshot.

## Routine and detailed human presentation

Default human output leads with the user result and activation timing:

1. Workspace result;
2. Project;
3. Manifest;
4. Current;
5. Next entry;
6. Runtime observation and attachment;
7. Boundary/login;
8. Shared cluster;
9. primary Next; and
10. separately ordered Attention.

Routine output hides stable IDs, complete digests, home path, timestamps,
attempt phases, observation anchors, and sibling diagnostic rows unless needed
to distinguish a failure. It also hides Docker tags/IDs and private Runtime
snapshot paths in all routine cases. `--details` exposes semantic IDs,
revisions, certainty, coverage, and same-root logical rows from the same
snapshot, but not private Docker/path authority.

Desired facts are labelled `Desired` or `Next entry`. Only a validated
AppliedEntry is `Current applied`, and only bounded runtime inspection is
`Observed`. The renderer does not infer these meanings from names, generation,
order, image labels, or indentation.

The human headline is a convenience projection from the complete axes, not a
persisted or JSON authority. Domain/Application supplies both the structured
primary Next and ordered Attention; the renderer only formats them.

### Wireframes

These are semantic proposals, not frozen goldens. Exact wording and argv must
come from the task-owned fixture and final Catalog.

#### Fresh recommended draft

```text
○ No persisted default Manifest selected
  Project          /work/example · prospective root
  Manifest         Recommended draft · not saved · no authority ID
  Workspace        absent
  Next entry       available after the draft is saved
  Boundary         recommended values · not authority
  Shared cluster   unconfigured
  Attention        none
  Next             tobari — Start the first-use flow; nothing has been saved.
```

No JSON field may present the draft as `workspace_manifest_id`, persisted
desired revision, or current applied state.

If same-root Workspace bindings exist, the same no-authority draft remains
selected and they appear under `Other Workspaces`; neither one nor their order
creates a hidden selector.

#### Current and detached

```text
✓ Workspace current
  Project          /work/example
  Manifest         review · explicit selection
  Workspace        present · entry current
  Current          frontend@2 · last entry applied
  Next entry       frontend@2 · no entry change
  Runtime authority frontend@2 · ready
  Availability     available · exact local execution image
  Observed runtime running · attachment detached
  Last used        unknown
  Boundary         source read-only · fixed for this Manifest
  Login            Workspace-owned · sign-in not observed
  Shared cluster   current · compatibility valid
  Attention        none
  Next             tobari --manifest review — Enter the current Workspace.
```

#### Current and attached

```text
✓ Workspace current and attached
  Project          /work/example
  Manifest         default · installation default
  Workspace        present · entry current
  Current          standard@1 · last entry applied
  Next entry       standard@1 · no entry change
  Runtime authority standard@1 · ready
  Availability     available · exact local execution image
  Observed runtime running · attachment attached
  Last used        unknown
  Attention        0 permissions · 0 service requests · 2 active exposures
  Next             Continue in the attached Workspace.
```

`continue_attached` is typed non-command guidance with no command path or argv.
Agent tests validate exact guidance handling and never try to execute it.

#### Multiple Manifests at one Project root

```text
! Selected Manifest has no Workspace
  Project          /work/example
  Manifest         default · installation default
  Workspace        absent
  Next entry       create from the saved default Manifest
  Other Workspaces review · detached; write · attached
  Shared cluster   current
  Next             tobari --manifest default — Create and enter only the selected binding.
```

Sibling order/attachment never changes selection. Details shows exact
WorkspaceManifestID/WorkspaceID and logical receipts but performs no sibling
Docker observation.

#### Pending permission

```text
! Workspace needs review
  Manifest         default
  Current          standard@1 · last entry applied
  Next entry       standard@1 · no entry change
  Runtime authority standard@1 · ready
  Availability     available · exact local execution image
  Observed runtime running · attachment attached
  Last used        unknown
  Shared cluster   current
  Attention        2 permissions in last 200 log lines · 1 unparsed
                   3 learned rules · projection valid
                   services available · 0 pending · 1 active
  Next             tobari review permissions — Review selected-Workspace access.
```

#### Broken cluster, independent service observation

```text
! Shared cluster needs reconciliation
  Manifest         default
  Current          standard@1 · last entry applied
  Next entry       standard@1 · no entry change
  Runtime authority standard@1 · ready
  Availability     available · exact local execution image
  Observed runtime running · attachment detached
  Shared cluster   reconciliation failed · Gateway projection invalid
  Permissions      unavailable · cluster denial source not readable
  Services         available · 0 pending · 0 active
  Next             tobari cluster up — Reconcile shared resources explicitly.
```

#### Entry pending with a newer Runtime binding

```text
! Workspace entry update pending
  Project          /work/example
  Manifest         web · explicit selection
  Current          web@3 · last entry applied
  Next entry       web@4 · desired entry revision
  Runtime authority Current web@3 ready · Next web@4 ready
  Availability     Current available · Next available
  Workspace        present · entry pending
  Observed runtime web@3 running · attachment detached
  Last used        unknown
  Shared cluster   current
  Attention        none
  Next             tobari --manifest web — Reconcile on explicit entry.
```

#### Entry blocked while attached

```text
! Workspace update blocked while attached
  Manifest         web
  Current          web@3 · running unchanged
  Next entry       web@4 · requires runtime recreation
  Runtime authority Current web@3 ready · Next web@4 ready
  Availability     Current available · Next available
  Workspace        present · entry blocked_attached
  Observed runtime web@3 running · attachment attached
  Last used        unknown
  Last failure     none · no reconciliation started
  Next             End attached sessions, then run tobari --manifest web.
```

#### Reconciliation failed or unknown

```text
! Reconciliation failed
  Workspace        present · entry failed
  Current          web@3 · remains authoritative
  Next entry       web@4 · not applied
  Last attempt     failed at health verification · partial change
  Observed runtime unknown
  Next             Read-only diagnosis required; do not infer that web@4 is current.

! Reconciliation outcome unknown
  Workspace        present · entry unknown
  Current          web@3 · last confirmed success
  Next entry       web@4 · attempted, outcome not classified
  Observed runtime unknown
  Next             Do not replay entry until read-only observation classifies it.
```

Exact recovery paths for the final two cases bind only to tasks present in the
fully integrated Catalog after WP09.

#### Runtime revision ready but image missing

```text
! Runtime execution material unavailable
  Manifest         web
  Current          web@4 · last entry applied
  Next entry       web@4 · no desired entry change
  Runtime authority web@4 · ready
  Availability     missing · exact local execution image not found
  Workspace        present · entry current
  Observed runtime stopped
  Last used        unknown
  Protection       last applied · selected project snapshot only
  Next             tobari review runtimes — Select the exact recovery/build target.
```

The Next line must not reconstruct or emit an opaque Runtime/revision reference
from a display ID or digest. WP06 is fixed as no-refs and routes to the owning
WP08-conformant discover/review task.

## Aggregate JSON direction

The implementation review must freeze exact keys in `cli.Catalog`; this shape
states the required semantics without claiming current implementation:

```json
{
  "schema_version": 1,
  "status": {
    "task": "tobari.status",
    "scope": {
      "project_root": "/work/example",
      "root_selection": "indexed"
    },
    "manifest_selection": {
      "source": "installation_default",
      "state": "persisted",
      "workspace_manifest": "review",
      "workspace_manifest_id": "01912345-6789-7abc-8def-0123456789ab",
      "desired": {
        "manifest_generation": 4,
        "manifest_revision": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        "boundary_revision": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
        "cluster_projection_revision": "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
        "entry_revision": "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
        "session_defaults_revision": "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
        "creation_defaults_revision": "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "runtime_binding": {
          "runtime_id": "01912345-6789-7abc-8def-0123456789ac",
          "name": "frontend",
          "ordinal": 3,
          "revision": "sha256:1111111111111111111111111111111111111111111111111111111111111111"
        }
      }
    },
    "workspace": {
      "workspace_presence": "present",
      "entry_state": "pending",
      "workspace_id": "01912345-6789-7abc-8def-0123456789ad",
      "project_root": "/work/example",
      "workspace_home": "/var/lib/tobari-example/workspaces/example",
      "creation_applied": {
        "creation_defaults_revision": "sha256:2222222222222222222222222222222222222222222222222222222222222222"
      },
      "last_successful_entry": {
        "manifest_generation": 3,
        "manifest_revision": "sha256:3333333333333333333333333333333333333333333333333333333333333333",
        "entry_revision": "sha256:4444444444444444444444444444444444444444444444444444444444444444",
        "runtime_id": "01912345-6789-7abc-8def-0123456789ac",
        "runtime_revision": "sha256:5555555555555555555555555555555555555555555555555555555555555555",
        "resolved_spec_revision": "sha256:6666666666666666666666666666666666666666666666666666666666666666"
      },
      "latest_attempt": null
    },
    "runtime": {
      "desired_binding": {
        "runtime_id": "01912345-6789-7abc-8def-0123456789ac",
        "runtime_revision": "sha256:1111111111111111111111111111111111111111111111111111111111111111"
      },
      "last_applied_binding": {
        "runtime_id": "01912345-6789-7abc-8def-0123456789ac",
        "runtime_revision": "sha256:5555555555555555555555555555555555555555555555555555555555555555"
      },
      "runtime_revision_authority": "ready",
      "execution_material_availability": "available",
      "observed_runtime_state": "running",
      "attachment_state": "detached",
      "native_compatibility": {
        "state": "valid",
        "scope": "runtime_revision"
      },
      "last_used": {"certainty": "unknown", "at": null},
      "protection": {
        "coverage": "selected_project_snapshot",
        "reasons": ["current_manifest", "last_applied"]
      }
    },
    "observed_consistency": {
      "applied_spec_match": true,
      "principal": "valid",
      "observation": "available"
    },
    "same_root_workspaces": {
      "coverage": "exhaustive",
      "items": []
    },
    "boundary": {
      "source_access": "read-only",
      "source_semantics": "direct_live_view"
    },
    "login": {
      "owner": "workspace_home",
      "lifetime": "workspace",
      "validity": "not_observed"
    },
    "cluster_projection": {
      "desired": {
        "aggregate_revision": "sha256:7777777777777777777777777777777777777777777777777777777777777777"
      },
      "last_successful_applied": {
        "aggregate_revision": "sha256:7777777777777777777777777777777777777777777777777777777777777777"
      },
      "observed": {
        "state": "available",
        "aggregate_revision": "sha256:7777777777777777777777777777777777777777777777777777777777777777"
      },
      "latest_attempt": null
    },
    "attention": {
      "items": [
        {"order": 1, "kind": "permission_review", "observation": "available", "pending": 2, "coverage": "bounded_window", "window_lines": 200, "unparsed": 0},
        {"order": 2, "kind": "service_review", "observation": "available", "pending": 1, "active": 1, "coverage": "fresh_live_attachments"}
      ],
      "learned_rules": {"observation": "available", "stored": 3, "coverage": "exhaustive", "projection": "valid"}
    },
    "snapshot_evidence": {
      "degraded_sections": []
    },
    "primary_next": {
      "kind": "catalog_task",
      "command_path": ["tobari"],
      "inputs": {"manifest": "review"},
      "reason_code": "entry_pending"
    }
  }
}
```

Contract rules:

- A recommended draft has `manifest_selection.state="recommended_draft"`,
  `workspace_manifest_id=null`, and `desired=null`, plus a separately typed
  recommendation. It is not a fake persisted object. If no installation default
  exists, same-root siblings remain exhaustive and cannot supply selection.
- Optional receipts/failures are explicit null. Known empty collections and
  counts are present as `[]` and `0`. Unavailable is never coerced to zero.
- `latest_attempt` is separate from `entry_state`, carries its attempted desired
  identity/change-state, and affects `failed` only when it is the latest final
  attempt for the currently desired identity.
- Same-root items exclude the selected Workspace and carry only logical
  Manifest/Workspace identity plus durable receipt summary. They do not infer
  live state.
- Mixed nested coverage is explicit per attention section; top-level status is
  one scalar complete-delivery result.
- `attention.items` remains ordered and complete independently of
  `primary_next`. A higher safety prerequisite never erases pending permission
  or service facts.
- Validated opaque IDs/digests retain exact bytes. External names/paths pass
  visible projection.
- No final-V1 JSON key contains the retired public noun or legacy
  `project_id`/`instance_id` identity.
- No status JSON key stores or emits `base`, `copied_from`, `copy_source`,
  `parent`, `inherits_from`, `derived_from`, provenance, ancestry, or lineage.
  A copied resource is indistinguishable from any other independent resource
  with the same ordinary current state.
- Runtime readiness, image availability, and observed container state remain
  separate fields. `last_used` carries certainty and is unknown without exact
  receipt evidence; `reconciled_at` stays on AppliedEntry under its real name.
- Runtime protection coverage is explicitly selected-snapshot only. It does
  not claim prune/delete eligibility or enumerate the installation.
- Raw Docker tag/image/container IDs and private snapshot paths are absent from
  routine status JSON as well as human output. Status produces no opaque refs.
- No JSON field named or functioning as `overall_status`, `adoption_state`, or
  lossy cluster-wide state exists. A presentation headline is not serialized.
- `primary_next` contains a Catalog-known command path and typed inputs or one
  typed non-command guidance variant. It contains no stored `argv` or free-form
  command string; the common Catalog quoting/validation seam renders argv.
- Native compatibility is scoped Runtime metadata/evidence. It is not login
  validity or overall Workspace authority.

## Empty and degraded behavior

- **No persisted installation default:** prospective/selected ProjectRoot,
  recommended draft with no ID/revision/authority, no selected Workspace,
  exhaustive same-root siblings (possibly non-empty), null selected receipts/
  failures, zero Docker/live-owner calls, and zero writes.
- **Persisted Manifest, absent Workspace:** desired revision is valid authority;
  Workspace/observed/AppliedEntry are inapplicable; exact entry creates the
  `(ProjectRoot, WorkspaceManifestID)` Workspace.
- **Known empty Attention:** `items=[]` plus separately scoped source summaries;
  only authoritative known-zero counts are rendered as zero.
- **Not observed:** explicit enum/null where a fact was deliberately excluded,
  including native login validity or active exposures when owner protocol lacks
  them.
- **Unavailable:** valid bounded source could not be observed; aggregate is a
  complete degraded result with a safe action.
- **Pending:** desired entry differs from last success; it is not current and no
  mutation happens during status.
- **Blocked attached:** pending entry recreation plus attachment; no attempt or
  failure receipt is created by status.
- **Failed/unknown:** preserve Current from prior AppliedEntry and attempted
  Next separately; do not replay unknown/partial change.
- **Drifted:** desired and AppliedEntry agree but observation does not validate
  current runtime.
- **Migrated unverified:** do not synthesize AppliedEntry or overall state;
  preserve the explicit upstream bounded evidence field.
- **Anchor race or invalid data:** retry the complete snapshot once for anchor
  churn; continued churn or invalid/contradictory identity is a nonzero fault
  with no aggregate output.
- **Owner facts not observed:** permission/service/attachment values unavailable
  from the bounded authenticated summary are `not_observed`, never zero.

## Next-action precedence

Domain/Application selects one structured primary Next using this fixed safety
order; it separately returns every ordered Attention item. Renderers choose
neither:

1. Invalid/ambiguous identity, unsafe path, contradictory result, or continued
   anchor churn: command fault with Catalog-known read/help recovery, no status
   result.
2. Migration evidence or cluster prerequisite blocks the selected outcome:
   integrated migration recovery or `cluster up` as applicable.
3. Missing explicit persisted Manifest or installation default: use only the
   integrated Catalog's Manifest discovery/creation task. With no default,
   return the typed recommended draft; existing same-root siblings do not become
   selection.
4. Runtime revision/material is not ready for the intended entry: route to the
   owning reference producer such as `review runtimes`; never reconstruct a ref,
   use `runtime build --name`, or issue a raw Docker action.
5. Workspace/entry: absence → explicit entry/create; matching final failure →
   reviewed retry/diagnosis; attached recreation block → `wait_for_detach`;
   pending → explicit entry; unknown → read-only integrated recovery.
6. Permission/service attention may become primary only when no preceding
   safety/precondition wins. All other pending permission/service facts remain
   in ordered Attention.
7. Current detached → explicit entry/resume; current attached →
   `continue_attached`.

`wait_for_detach` and `continue_attached` are typed non-command guidance, not
paths and not empty argv. Command variants store exact Catalog path plus typed
inputs; only the common Catalog renderer validates/quotes argv. No action runs
automatically.

## CLI and Catalog contract

```text
status [--manifest NAME] [--details] [--format text|json]
```

- Capability: `tobari.lifecycle` unless the fully integrated ledger requires an
  explicit reviewed replacement.
- Role/effect: `RoleUtility` / `EffectRead`.
- Inputs: optional persisted Manifest selector, optional boolean `--details`
  default false, and text/JSON format.
- No fixed mutation target and no produced/consumed refs.
- Delivery `complete`; top-level coverage `not_applicable`.
- Fresh local result has no prerequisite. Optional live sections carry typed
  availability instead of preventing project orientation.
- Root agent help remains a bounded outcome index; exact status help owns every
  recursive field, enum, failure, coverage, primary Next, and Attention variant.
- Reject retired namespace/flag/field spellings with ordinary unknown input,
  not migration help or aliases.
- The fixed no-ref/utility contract means status cannot synthesize direct
  `runtime delete --id`, `runtime restore --id`, or `runtime build --id` argv.
  It routes to an exact reference-producing discover/review command and
  consumes WP08 recursive Catalog conformance.
- Status never invokes `runtime prune dry-run`, apply, delete, restore, or build
  internally. A safe explicit command may appear only as Next, and status never
  claims full lifecycle preconditions were proved.
- Reject `--context`, project-local current/`manifest use`, copy lineage/base,
  raw Runtime action selection, the WP05-retired host name, and WP04
  research-only auth/serve recovery in command/help/schema/Next/Attention.

## Docker and owner-call budget

| Status scope | Docker CLI maximum | Owner calls |
|---|---:|---:|
| No persisted installation default / recommended draft | 0 | 0 |
| Persisted selection with configured observation | `B_status`, frozen after all upstream implementations through WP09 | finite WP09-bounded selected-owner discovery/snapshot |

The predecessor-based provisional four-invocation decomposition is:

1. one Engine/version/availability probe;
2. one strict multi-container inspect for Gateway, OPA, and selected Workspace;
3. one strict multi-network inspect for shared control/egress and selected
   Project network; and
4. one fixed 200-line denial-log observation when applicable.

The container result must supply attachment and observed applied-spec evidence
without a second selected-Workspace inspect. Same-root siblings add no Docker
targets. `--details` adds no calls. Exhaustive installation diagnosis remains
`cluster status`.

The owner path needs a reviewed finite owner-record limit, one global deadline,
and at most one authenticated snapshot per selected matching owner. A revised
owner record/response may include WorkspaceID, WorkspaceManifestID,
AttachmentEpoch, pending count, and active count. It returns no refs, ports,
URLs, payloads, or credentials and never cleans records during status.

After all upstream implementations through WP09 complete, inspect their actual
bulk stores, Docker adapter, and authenticated owner summary; replay the
integrated binary in isolated state; and measure calls before freezing
`B_status`.
Do not keep four merely because it was correct for the predecessor. Fresh zero
calls, installation-size independence, no sibling scaling, and identical
routine/details/JSON work remain mandatory whatever finite number is approved.

WP 01's migration Docker evidence is a separate, stronger migration question;
this routine budget neither answers nor constrains it. Routine status also adds
no installation-wide WP 03 protection scan.

## Alternatives considered

### Preserve legacy public vocabulary

No longer a live alternative. WP 01 has fixed Workspace Manifest vocabulary and
no aliases. Retaining an old selector or field would contradict the higher
decision.

### Show only desired Manifest state

Rejected because it would call a pending or failed revision current and hide
the still-authoritative AppliedEntry and observed runtime.

### Show only live Docker state

Rejected because Docker IDs/labels are not logical authority and cannot explain
desired Next entry, prior success, failure target, or migration uncertainty.

### Compare complete Manifest generation as one adoption state

Rejected because activation is item-specific. Cluster/session/creation edits
must not falsely imply Workspace entry adoption.

### Serialize one overall/adoption status enum

Rejected because presence, entry relation, observed Runtime, immutable Runtime
authority, execution material, latest attempt, and migration evidence can vary
independently. One enum loses valid combinations such as current+stopped or
pending+running and invites presentation state to become false authority.

### Make status installation-wide

Rejected because unrelated roots obscure CWD intent, live observation scales
with installation size, and a single safe Next becomes ambiguous.

### Add `status home` or compose public command JSON

Rejected because it duplicates the lifecycle outcome or makes CLI renderers/
schemas semantic dependencies and creates a second registry/parser.

### Make `--details` perform deeper observation

Rejected because routine and agent facts/cost would diverge. Details is
presentation-only.

### Show copy provenance or group derived resources

Rejected by WP 02. Copy is a one-time initializer producing fresh independent
authority. A lineage field would invent persisted state, lifecycle coupling,
and protection edges explicitly excluded by the accepted design.

### Compute complete Runtime prune/delete safety in status

Rejected by WP 03. The destructive protection planner is installation-wide,
lock-sensitive, and fail-closed. Running it routinely would scale status with
installation size and risk presenting stale eligibility. Status shows only
selected-snapshot relationships; `runtime prune dry-run` owns the exact plan.

### Collapse Runtime `ready` and `available`

Rejected because immutable revision authority can remain ready when execution
material was pruned or externally removed. Conversely, Docker material cannot
establish Runtime authority. Desired/applied equality is a third independent
fact.

## Layer changes

- **Domain:** consume upstream exact IDs, revision slices, AppliedEntry,
  attempt/migration evidence, cluster axes, WP03 readiness/material certainty,
  WP07 wait state, and WP09 owner summary; add only the task-owned snapshot,
  orthogonal-axis validation, structured primary Next, and ordered Attention.
  Add no copy, Runtime-lifecycle, wait, or exposure-owner domain model.
- **Application:** one StatusHome use case owns root/Manifest/Workspace
  correlation, local/external read ordering, anchor revalidation, independent
  section degradation, and Next precedence.
- **Infrastructure:** reuse final upstream stores and WP09 authenticated owner
  summary read-only; add only the consumer adapter composition and bounded
  selected-target Docker/denial observation behind narrow ports.
- **CLI/Catalog:** final Manifest selector, exact aggregate JSON, routine/details
  renderers, retired-vocabulary rejection, agent help, and structured faults.

## Data and control flow

```text
typed argv + CWD
  -> Catalog parser (`--manifest`, never old alias)
  -> StatusHome application
     -> final-V1 local desired/applied/failure snapshot
     -> ProjectRoot-first Manifest/Workspace selection
     -> bounded selected runtime + shared projection observation
     -> bounded policy + authenticated owner summaries
     -> consistency-anchor revalidation
     -> orthogonal fact validation
     -> task-owned structured primary Next + ordered Attention
  -> one validated StatusHomeSnapshot
  -> routine/details text or complete JSON
```

No path invokes another CLI handler, parses rendered output, performs migration,
or receives an unrestricted executor/filesystem/network client.

## Error and cancellation behavior

- Validate Manifest selection and host scope before Docker/socket calls.
- Preserve one context and cancellation/deadline; emit no partial result.
- Expected missing/unreachable sources become typed observations only when the
  adapter can distinguish them authoritatively.
- Invalid/mismatched desired, AppliedEntry, attempt, Docker, cluster, policy, or
  owner results and unsafe paths fail closed; do not coerce them to unavailable.
- Retry a changed consistency anchor once within the same bounded operation;
  continued churn fails the whole command with no mixed snapshot.
- Status never writes a failure receipt, clears an unknown barrier, or treats
  cancellation as evidence that no prior entry action occurred.
- Output write failure is a retryable read-output failure and cannot change
  any desired/applied/attempt state.
- Raw private causes remain outside stdout/JSON.

## Security and public boundary

- Status reads final upstream trusted host evidence but has no authority to
  publish or mutate it.
- No credentials, tokens, cookies, account state, request headers/body/query,
  source content, service URL/port, or (under the chosen utility contract)
  opaque action references enter output.
- WorkspaceManifestID/WorkspaceID replace legacy principal names without
  widening authority. Cross-Manifest/root/Workspace mismatches fail.
- Manifest generation/name, Runtime ordinal/name, image tag, Docker ID, and
  project-supplied values never authorize policy or reconciliation.
- Active-exposure summary stays inside WP09's authenticated same-UID/peer-PID/
  nonce owner boundary and returns only its approved semantic summary.
- Status performs no store initialization, migration, repair, journal recovery,
  record cleanup, mutation-lock acquisition, Docker reconciliation, principal
  rewrite, socket creation, or other convergence.
- External printable meaning remains untrusted; structure is visibly projected.
- Runtime journal/receipt stores remain owner-only. Status receives only typed
  semantic projections, never journal paths, raw Docker identifiers, source or
  snapshot paths, and never treats observation as deletion authority.

## Authentication and readiness

- Standard authentication remains native and Workspace-owned. Status reads no
  credential file, provider output, token, account, or login endpoint and emits
  login validity only as `not_observed`.
- Native/agent-ready compatibility is Runtime-scoped metadata/evidence. It is
  neither authentication state nor an overall Workspace authority claim.
- WP04 research-only auth and `serve` surfaces are absent from release
  Catalog/status, primary Next, Attention, fixtures, and recovery.
- WP05's retired host name is never emitted or used as a fixture value.

## Implementation dependency order

1. Complete WP01 and its WP02 integration/audit.
2. Complete WP08 recursive Catalog/domain output conformance.
3. Complete WP03 Runtime retirement/readiness/material implementation.
4. Complete WP04 release/research build-profile contract.
5. Complete WP05 host-loopback authority rename.
6. Complete WP07 first-use wait/progress/recovery state.
7. Complete WP09 service-exposure owner-summary protocol.
8. Run the mandatory all-upstream re-baseline: record exact HEAD/branch/status
   and ownership; re-read final durable contracts/schema/Catalog/layers/tests;
   execute isolated read-only probes; compare selection/axes/JSON/wireframes;
   and measure Docker/owner calls. Block on any moving or ambiguous upstream.
9. Freeze status semantic corpus, answer keys, exact integrated field names,
   JSON, empty/
   degraded behavior, Next table, and measured call/owner budgets.
10. Add failing domain/Catalog/schema/selection/read-purity/presentation tests.
11. Implement task-owned snapshot/application joins over final upstream ports;
    do not add WP03 Runtime, WP07 wait, or WP09 owner types.
12. Implement measured fixed-budget selected-scope observation.
13. Replace Catalog/status human/JSON projection with the fixed contract.
14. Consume exact predecessor migration evidence and add post-migration status
   scenarios without adding a status migration reader.
15. Promote durable docs/capability/schema/site/help/harness changes after the
   governing decisions, then run all gates.
16. Remove this temporary packet and notify control with the required WP06
    implementation handoff and WP10 readiness.

## Verification

- Domain: exact task/root/Manifest/Workspace scope; separate presence, entry,
  observed Runtime, Runtime authority/material, migration evidence, and latest-
  attempt matrices; slice-specific activation; primary Next plus Attention.
- Catalog: `--manifest` only, exact recursive V1 JSON, no legacy keys/aliases,
  no refs, `RoleUtility`/`EffectRead`, complete delivery, root help budget.
- WP 02 negatives: no `--base`, build-by-name/omission, provenance, base,
  parent, lineage, or copy-triggered reconciliation in status/schema/help.
- WP 03 semantics: ready versus availability versus observed state; unknown
  last use; selected-snapshot/non-exhaustive protection; no raw Docker/private
  path; no lifecycle mutation; safe Next uses only Catalog-valid commands.
- Selection: prospective root, nearest ancestor, explicit/installation-default
  Manifest, no-default draft with existing siblings, no Workspace inference,
  same-root-only grouping.
- Read purity: zero state/store/journal/failure/AppliedEntry/principal/Docker/
  owner cleanup mutations in every success/failure state.
- Infrastructure: fresh 0 and post-WP09 integrated measured `B_status` Docker
  counts,
  fixed batches, no sibling or installation-wide protection-scan scaling,
  malformed/missing/canceled results, owner peer/nonce/scope/churn bounds.
- Migration consumer: retained ID bytes, verified AppliedEntry versus explicit
  unverified state, no old public fields, no credential reads, cluster
  reconciliation still explicit.
- Presentation: one typed fixture drives TTY, NO_COLOR, redirected, routine,
  details, and JSON; Current/Next negative inference; independent availability;
  non-command Next handling.
- Agent readiness: scoped help plus one status invocation closes routine
  interpretation with zero custom parsing; safe command or typed non-command
  action is exact.
- Required profiles: `task check`, `task security`, `task public:check`, and
  `task release:check`, plus relevant runtime/integration evidence.

## Compatibility, migration, rollout, and rollback

- Final public V1 replaces all legacy public command/flag/JSON vocabulary with
  Manifest/Workspace naming. No alias, warning command, deprecated field, dual
  schema, or ordinary compatibility reader remains.
- Status JSON is reset atomically to the final schema-1 aggregate after all
  upstream implementations through WP09 freeze exact consumed fields. It does
  not preserve the unpublished flat shape.
- Persisted-state migration belongs to WP 01's explicit exact predecessor
  `migrate apply`: retain legacy UUID bytes as WorkspaceManifestID/WorkspaceID,
  migrate non-secret authority, and synthesize AppliedEntry only with sufficient
  read-only Docker evidence. Otherwise status preserves explicit bounded
  migrated-unverified evidence without inventing overall pending/incomplete.
- Status itself performs no migration, implicit fill, state rewrite, or fallback.
- Migration preconditions and rollback follow WP 01's unresolved owner decision
  and content-addressed backup plan; this packet does not choose cluster-stop or
  attachment rules.
- Once public V1 ships, the unpublished predecessor path is retired by its
  owner rather than becoming a permanent status compatibility layer.
- WP 02 requires no provenance backfill or status migration. WP 03 keeps Runtime
  manifest schema and owns any journal/receipt sidecars; status neither creates
  nor migrates them.

## Documentation promotion

After every upstream implementation through WP09 and the integrated re-baseline
gate, during status implementation update:

- product: Manifest-only status command, Current/Next entry, exact aggregate
  JSON, empty draft, Next/recovery, and sibling scope;
- architecture: status consumer read model, orthogonal axes, slice-specific
  activation, structured primary Next/Attention, Runtime readiness/availability
  split, consistency anchors, selected observation, and measured call budgets;
- security: AppliedEntry/attempt observation, no credential inference, pure
  owner summary, no cleanup/reconciliation, and invalid/unavailable split;
- harness: desired/applied/observed/failure fixtures, migration consumer cases,
  no-default zero-write, `0/B_status` call budget, copy-lineage negatives,
  readiness/availability/unknown-use, no lifecycle mutation, negative
  inference, terminal parity, and agent-readiness;
- public surfaces: Catalog, capability/schema ledgers, README/site, completion,
  generated command/schema references, and old-vocabulary negative checks.
