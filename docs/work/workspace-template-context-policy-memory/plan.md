# Work Plan: WP11 — Separate Workspace Template, Context, and Policy Memory

- Status: Proposed
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Take the four-concept model to owner review and sequencing:

```text
Workspace Template (stable reusable definition)
  └─ current Template Revision (immutable semantic body)
                         |
Project ----------------+----> Context (durable binding)
                                ├─ desired Template revision
                                └─ Policy Memory (mutable, separately revised)
                                           |
                                  explicit Workspace entry
                                           v
                                Workspace (replaceable instance)
                                  ├─ last-successful applied Template revision
                                  ├─ latest bounded reconciliation result
                                  └─ currently observed runtime facts
```

A Template defines the reusable static Boundary, baseline, Runtime binding, and
typed defaults. A Context binds one Project to one Template, follows the
Template's current immutable revision as desired, and owns project-specific
Policy Memory. A Workspace is the replaceable applied instance. Explicit entry
reconciles Template desired state; `cluster up` or the existing explicit policy
mutation boundary activates policy. Reads never reconcile.

The effective ordinary network authority is composed, not flattened:

```text
Template terminal Boundary
  ∩ (Template reviewed baseline + Context Policy Memory + bounded Advanced policy)
```

Terminal Deny and destination/method ceilings remain superior to learned
positive rules. Template revision and Policy Memory revision remain separate
authority axes even when routine UI presents them under one Context.

## Ownership model

| Concept | Owner | Scope | Lifetime | Mutability | Authority candidate |
|---|---|---|---|---|---|
| Workspace Template | installation/trusted host | reusable across Projects and Contexts | until explicit Template deletion | stable identity; current pointer advances | TemplateID |
| Template Revision | Workspace Template | complete static desired definition | immutable retained history | immutable; semantic no-op publishes nothing | TemplateID + semantic digest |
| Context | one Project × one Template binding | project-specific work setup | survives Workspace deletion; until explicit Context deletion | binding immutable in V1; desired revision derives from Template current | ContextID |
| Policy Memory | Context | learned decisions for that Project/Template relationship | survives Workspace replacement; ends/reset by explicit Context/policy action | mutable, independently revised and atomically activated | Context identity + policy semantic revision |
| Workspace | Context | one applied isolated instance and home | replaceable; ends at Workspace delete | applied receipt and bounded reconciliation/observation change | WorkspaceID + ContextID; observations are not authority |

V1 permits at most one Context for one canonical Project root and TemplateID.
This keeps the Context durable without adding a second human name: another
Context for the same Project means another Template. Template rebinding and
revision pinning are excluded; both would need explicit policy-memory
revalidation and a separate lifecycle decision.

## Alternatives considered

### Alternative A: Keep Workspace Manifest and improve presentation only

Retain one Manifest that owns static definition and learned policy, perhaps
labeling parts more clearly. This minimizes migration, but it leaves mutable
Project-specific learning under a reusable static definition and cannot explain
why Workspace deletion preserves only some Manifest-related state. Rejected as
the target because owner/scope/lifetime remain mismatched.

### Alternative B: Put learned policy on Workspace and omit Context

This has fewer concepts and binds learning directly to the observed instance.
It conflicts with the accepted requirement that deleting and recreating a
Workspace preserves learned policy. Moving policy during deletion would merely
create an unnamed durable owner, which is Context in substance. Rejected.

### Alternative C: Make Context a derived view only

Derive Context from `(Project, Template)` without durable identity or state.
This is viable only if nothing has an independent Context lifetime. Policy
Memory now does, and deletion/recovery/default-selection semantics require an
exact owner. Rejected as the leading model; retain as an identity simplification
to evaluate before assigning a ContextID.

### Alternative D: Snapshot Template into Context at creation

`context create --template` could copy a complete Template revision and then
diverge forever. This makes Context self-contained, but turns instantiation into
implicit copy, prevents reviewed Template improvements from becoming explicit
Next state, and recreates static definition ownership inside every Context. The
product owner selected follow-current desired state with entry-time adoption.
Rejected for V1.

### Alternative E: Eliminate Template and store one complete Context definition

This is the lowest public concept count. It fits per-project configuration but
loses the class-like reusable design and makes cross-project setup duplication
the normal operation. It also cannot give static copy and project instantiation
distinct meanings. Rejected.

## Design

### Public contract

The recommended hard-cutover command vocabulary is:

| Outcome | Final path/selector | Role/effect | Binding and result |
|---|---|---|---|
| list reusable designs | `template list` | discover/read | exhaustive installation collection; produces `workspace-template` refs |
| inspect one design | `template show [--name NAME]` | utility/read | explicit name or `DefaultTemplateSelection`; produces the same Template ref |
| create or statically fork | `template create [--copy-from NAME] --name NAME ...` | act/create, fixed Template collection | fresh TemplateID and generation 1; copy revalidates source ID/digest/body and copies no lower-lifetime state |
| choose omission default | `template default set --id TEMPLATE_REF` | act/write, reference bound | consumes one `workspace-template` ref; changes no Context or Workspace |
| delete a design | `template delete --id TEMPLATE_REF --confirm=delete` | act/write, reference bound | only with no default selection and no Context; produces no ref |
| list project bindings | `context list [--format text\|json]` | discover/read | exhaustive Contexts with `context` refs; human groups by Project and Template |
| inspect one binding | `context show --id CONTEXT_REF` | utility/read | consumes one `context` ref and reports Template desired plus Policy Memory current/active separately |
| instantiate a design | `context create --template TEMPLATE_REF [--format text\|json]` | act/create, reference-bound parent | canonical CWD is Project scope; produces one fresh `context` ref; creates no Workspace or policy authority beyond empty memory |
| forget project binding | `context delete --id CONTEXT_REF --confirm=delete` | act/write, reference bound | requires no Workspace/attachment/research credential; deletes Policy Memory and unresolved candidates |
| enter current Project | `tobari [--template NAME] [-- ARGV...]` | existing composed act/create | resolves exact Context by CWD + Template; first-use may compose Context create, cluster reconcile, and Workspace entry after review |
| inspect current Project | `status [--template NAME]` | utility/read | read-only Context desired, Policy Memory activation, Workspace applied/observed |
| remove applied instance | `delete [--template NAME] [--force]` | existing fixed-target act/write | removes Workspace, home, and native auth; preserves Context and Policy Memory |

`--manifest`, the `manifest` namespace, `manifest_id`, `workspace_manifest_id`,
`context use`, and a default/current Context are absent. Root `--template NAME`
is a human selection convenience over the unique CWD/Template pair; action
commands that can destroy or rebind authority consume opaque refs. Existing
Template configuration commands move their owner selector from `--manifest`
to `--template`; their typed activation semantics remain unchanged.

The final reference graph is:

```text
template list/show -> workspace-template ref
  -> template default set / template delete
  -> context create -> context ref
       -> context show / context delete

context + Workspace denial -> policy-review-item ref
  -> policy allow / deny / reset / apply-reviewed

runtime list/show/history -> runtime / runtime-revision refs
  -> unchanged WP03 build/delete/restore and Template Runtime selection

permission_wait_id remains a non-reference attachment correlation value.
Host Loopback route/grant and service-exposure references remain
attachment-local and never become Template or Context references.
```

Routine human output leads with Context, Template, `Current entry`, `Next
entry`, and `Policy Memory`; details expose TemplateID/digest, ContextID,
WorkspaceID, and current/active Policy Memory revisions. It never presents one
combined Context revision.

### Schema hard cutover

| Contract | Predecessor | Final V1 |
|---|---:|---:|
| persisted Workspace Template + immutable revision store | Workspace Manifest schema 2 | Template schema 1 |
| persisted Context + Policy Memory store | absent / Manifest-owned policy | Context schema 1 + Policy Memory schema 1 |
| persisted Workspace state | schema 2 | schema 3 with ContextID and Template applied digest |
| `template` and `context` command JSON | Manifest schema 2 / absent | family-local schema 1 |
| root `status` and `list` JSON | schema 2 | schema 3 |
| policy candidates/review/rules JSON | schema 1 | schema 2 with Context, Template, and observing Workspace dimensions |
| `migrate apply` JSON | schema 2 | schema 3 |
| Gateway denial/wait record | WP07 schema 2 | schema 3 with ContextID, WorkspaceID, and TemplateID projection |
| permission helper result | schema 1 | unchanged schema 1 |
| Host Loopback capability projection | schema 1 | unchanged schema 1 |
| Host Loopback private route/grant registry | ADR 0083 schema 2 | unchanged schema 2, now binds ContextID + WorkspaceID |
| Runtime public schemas and refs | WP03 schema 1 | unchanged schema 1 |
| version, error, help, build-surface schemas | WP04/current | unchanged shapes and versions |

Frozen principal/Gateway/OPA compatibility keys `context_id`, `project_id`, and
`context` remain internal wire tokens. After the atomic cutover their validated
values are ContextID, WorkspaceID, and Context presentation respectively; they
do not expose a public alias or Project-root identity. TemplateID is resolved
through the trusted Context registry and appears explicitly only in new public
schema-3 projections where interpretation requires it. Old and new readers do
not coexist.

All reads use complete delivery; installation collections are exhaustive.
Creates and writes use standard catalog mutation faults. `context create` is
idempotent only for an exact unchanged published Context receipt and otherwise
returns `context_exists` or `context_collection_changed`. Context/Template
delete require literal confirmation and recover through their read-only
list/show paths. Entry failure recovers through `status`; policy activation
failure through `policy rules` or `cluster status`. No recovery action is a
mutation.

### Layer changes

- Domain: introduce or rename Template stable identity and immutable revision;
  define Context aggregate, Policy Memory identity/revision, Workspace binding,
  desired/applied invariants, deletion preservation, and authority composition.
- Application: keep Template copy, Context creation/default selection, Workspace
  entry/deletion, policy learning/reset, and cluster activation as separate
  use cases with smallest owned ports.
- Infrastructure: separate Template revision store from Context/Policy Memory
  stores; migrate exact existing authority; update principal and aggregate
  projection production; retain atomic activation and owner-only storage.
- CLI and catalog: perform one reviewed hard vocabulary/schema cutover only
  after the parent schedules it; derive all help, completion, roles, reference
  flow, human/JSON output, recovery, site, and examples from the final Catalog.

### Data and control flow

```text
Template mutation
  -> publish immutable revision
  -> Context observes new desired digest (no Workspace mutation)
  -> routine read shows Current != Next

Explicit Workspace entry
  -> resolve exact Context + desired TemplateID/digest
  -> read Policy Memory revision and current cluster activation
  -> observe Workspace/attachment state
  -> validate boundary, Runtime, principal, policy, and safe adoption
  -> mutate/recreate only required Workspace resources
  -> verify health and authority
  -> commit last-successful AppliedEntry
  -> preserve Context and Policy Memory on Workspace delete

Policy review/apply
  -> bind denial/candidate to exact Context + Workspace observation
  -> stage no authority
  -> revalidate unchanged references and Template ceiling
  -> atomically publish/activate one Policy Memory revision
  -> do not publish a new Template revision
```

### Independent activation axes

- **Template current:** one immutable TemplateID/digest selected by the
  Template's current pointer. A semantic no-op publishes nothing; A→B→A keeps a
  later generation with A's original semantic digest. Generation remains
  correlation only.
- **Context desired:** a read-only resolution of the bound Template's current
  digest. A Template mutation writes no Context and no Workspace.
- **Workspace applied:** `AppliedEntry` records ContextID, TemplateID/digest,
  exact RuntimeID/revision, slice digests, and reconciliation receipt only after
  verified explicit entry. Failure preserves the previous receipt.
- **Template cluster projection:** `cluster up` validates and activates the
  complete all-Template static Boundary/baseline projection. A stale active
  projection blocks entry where required; reads do not repair it.
- **Policy Memory current/active:** each confirmed decision mutation publishes
  one complete Context Policy Memory semantic revision. `policy
  apply-reviewed`, `policy allow`, `policy deny`, and `policy reset` retain their
  existing explicit atomic hot-activation boundary; `cluster up` can reconcile
  the complete aggregate after migration or cluster absence. Neither operation
  publishes a Template revision or AppliedEntry.

When a Template Boundary narrows, now-ineligible remembered decisions remain
visible but inert; they are not silently deleted. A later reviewed Template
change may make an exact remembered decision effective again only after the
explicit Template/cluster activation boundary. Policy Memory never creates a
destination or method outside the active terminal Template Boundary.

### Error and cancellation behavior

- Template source drift, Template name reuse, Context/Project mismatch, stale
  policy reference, changed Boundary, unsafe attachment, missing Runtime, and
  mixed identity all fail closed before mutation where possible.
- Failed or unknown Workspace reconciliation preserves last-successful applied
  state and records a bounded read-only recovery action. It never advances
  AppliedEntry optimistically.
- Template copy and Context create are independent/idempotency-scoped creates;
  neither reconciles a Workspace or cluster.
- Cancellation before publication or mutation reports zero change. A confirmed
  result remains confirmed if presentation later fails.
- Context deletion refuses a live Workspace or attachment and any configured
  research credential. After explicit Workspace deletion and research logout,
  it deletes the exact Context, Policy Memory, and unresolved candidates;
  Project files, Template, Runtime, and non-authorizing bounded audit evidence
  remain. It cannot discover or delete a Workspace home indirectly.

### Security and public boundary

- Moving learned policy to Context changes a policy authority owner and
  principal/reference binding; it is a security-contract change, not storage
  refactoring.
- Context/Policy Memory authority must include exact trusted identity. Project
  paths, Template names, display names, generations, and Docker labels cannot
  authorize learned rules.
- Template terminal Boundary constrains Policy Memory. Rebinding a Context to a
  different Template is excluded from V1 unless a separate migration/revalidation
  design proves that remembered decisions cannot widen authority.
- Cross-Context and cross-Project rule/denial/handle reuse fails closed. Multiple
  Contexts on one Project do not share Policy Memory implicitly.
- Standard native authentication remains owned by one Workspace home and is
  removed with that Workspace. Research Broker credentials, when compiled, are
  Context-owned but remain outside Template desired/applied state and require
  explicit logout before Context deletion. Template copy and Context creation
  copy neither form.
- WP07 permission wait remains attachment-memory observation. Its canonical
  session binds ContextID + WorkspaceID; a wait record also projects TemplateID
  for interpretation, but no wait, lease, nonce, transport, result, or helper
  state enters Context or Policy Memory.
- ADR 0083 Host Loopback remains exact attachment authority at
  `host.tobari.internal`. Its final route/grant identity binds ContextID +
  WorkspaceID + Attachment Epoch + exact effect; hostname, migration lock order,
  HTTP-only rule, terminal retired-name guard, and schema split are unchanged.
- Migration and rollback must be exclusive, journaled, content-checked, and
  atomic across Template, Context, Workspace, principal, and policy projection
  state. Old/new readers must never concurrently authorize the same bytes under
  different identity meanings.
- No external destination, dependency, daemon, or arbitrary
  Template file authority are added by this design.

## Implementation slices

1. Owner sequencing, vocabulary decision, revised thesis/ADR, public contract,
   identity/reference graph, and failing contract tests.
2. Template revision and Context/Policy Memory domain invariants plus pure
   migration model.
3. Application use cases and typed read/mutation ports for Template copy,
   Context creation/lifecycle, Workspace entry/deletion, and policy activation.
4. Owner-only stores, atomic migration/rollback, principal projection, and
   desired/applied/observed reconciliation adapters.
5. Catalog hard cutover, CLI/human/JSON/status/default-selection behavior,
   completion/help/examples/site, and generated snapshots.
6. Security, migration, public-boundary, agent-readiness, and isolated runtime
   integration gates; durable promotion and temporary packet cleanup.

Each slice should remain buildable and reviewable. The parent may split delivery
only where transaction and authority boundaries remain coherent.

## Verification

- Unit and contract tests: identity, semantic digest/no-op/A→B→A, Context
  uniqueness, multiple Contexts per Project, Template follow-current, Policy
  Memory isolation, Workspace preservation/deletion, and AppliedEntry timing.
- Negative side-effect tests: stale/different Template authority, name reuse,
  cross-Context rule reuse, policy beyond Boundary, attached unsafe adoption,
  read-triggered mutation, and partial/mixed migration.
- Opaque-reference and complete-pagination tests: Catalog-wide Template,
  Context, Workspace, policy item, and migration plan producer/consumer graph.
- Structured output, hostile-output, and recovery tests: new schema versions,
  absent/current/pending/failed/unknown/drifted states, exact next commands,
  secret-free errors, and no inference from labels or ordering.
- Agent-readiness scenario and discovery-round-trip count: create/select Context,
  enter Workspace, deny/review/apply/retry, delete/recreate Workspace, and prove
  retained Policy Memory without external parsing.
- Human-handoff scorecard: not applicable unless setup/authentication scope is
  changed; standard authentication ownership should remain unchanged.
- Manual observation: isolated Docker context only, proving entry-time adoption,
  no read reconciliation, policy hot activation, and Workspace recreation.
- Required profiles: `task check`, `task security`, `task public:check`, relevant
  isolated `task integration:test` / `task runtime:test`, and `task
  release:check` if first-release material is affected.
- Generated-diff or artifact checks: Catalog-derived help/completion, embedded
  source snapshots, architecture site routes/links, and public-vocabulary
  negative guards.

## Rollout and rollback

This is a pre-public hard-cutover from exact predecessor `0bbd9deb`; it is not a
published compatibility migration. Existing `migrate apply` remains the sole
writer and adds exact source kind `workspace_manifest_v1`.

Under the installation lifecycle lock, stopped cluster, zero canonical
attachments, and the existing research-quarantine/Host-Loopback lock order, the
transaction must:

1. enumerate every schema-2 Workspace Manifest/revision, logical Workspace,
   AppliedEntry/pending/failure record, default selection, learned Allow/exact
   Deny, pending candidate, principal projection, Runtime protection edge,
   standard Workspace home, and research Broker filesystem authority;
2. validate owner/type/mode/symlink/schema/digest/reference closure and one exact
   `(ProjectRoot, WorkspaceManifestID)` pair per Workspace before mutation;
3. preserve each WorkspaceManifestID byte sequence as WorkspaceTemplateID and
   each WorkspaceID byte sequence as WorkspaceID;
4. generate and journal one fresh ContextID for each exact pair, rejecting
   duplicate, dangling, ambiguous, name-only, or observation-only association;
5. transform every retained immutable Manifest revision into the corresponding
   static Template revision, preserving generation as correlation, recomputing
   semantic digest after dynamic learned decisions are excluded, and preserving
   every exact Runtime reference required by WP03 protection;
6. publish one Context schema-1 binding and one complete Policy Memory schema-1
   revision from the exact predecessor decisions. Learned authority drops the
   predecessor WorkspaceID dimension and binds ContextID so it survives a later
   Workspace replacement; pending observations retain both ContextID and the
   observing WorkspaceID;
7. rewrite Workspace state to schema 3 with the fresh ContextID and preserved
   WorkspaceID/home bytes. Synthesize or preserve AppliedEntry only from exact
   predecessor receipt plus bounded owned-Docker evidence; otherwise publish an
   explicit pending/unverified state;
8. convert DefaultManifestSelection to DefaultTemplateSelection with the exact
   preserved TemplateID;
9. quarantine rather than rebind any replay-capable research Broker authority,
   leaving macOS Keychain recovery material untouched and moving Linux
   filesystem root-key material with the exact set; standard Workspace-home
   bytes are neither read nor transformed;
10. atomically publish final stores and require `cluster up` followed by explicit
    Workspace entry. Ordinary final readers accept only Template/Context/
    Workspace schemas and cannot discover predecessor authority.

The migration journal contains the exact predecessor digests, TemplateID byte
preservation, generated ContextID mapping, preserved WorkspaceIDs, transformed
revision receipts, and private backup identity. A second apply returns
`changed:false` and never regenerates ContextIDs or overwrites backup. Rollback
restores the complete byte-identical predecessor only when no fresh Template,
Context, Workspace, Policy Memory, home/auth, principal, or cluster authority
exists at canonical final paths; it never merges. Crash recovery exposes either
the complete predecessor reader set or the complete final reader set, never a
partially resolvable policy/principal combination.

Manifest/`--manifest` aliases, dual schema readers, post-release deprecation,
and fallback reconstruction are explicitly excluded.

## Documentation promotion

- Revise or supersede ADR 0079 with Template/Context/Policy Memory ownership,
  identities, activation axes, deletion, and migration.
- Revise Thesis 9 and propagate consequences through Thesis 0, Thesis 4, and
  Thesis 8.
- Update product public vocabulary, command table, selection/default/deletion,
  status, JSON, faults, and migration contracts.
- Update architecture aggregate/store/principal/reconciliation and Runtime
  protection graph contracts.
- Update security learned-policy ownership, terminal Boundary precedence,
  cross-Context isolation, trust boundaries, and rollback.
- Update harness claims, Catalog/reference guards, migration fixtures,
  agent-readiness scenarios, site/help/examples, and relevant repository Skill.
