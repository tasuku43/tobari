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
| Template Revision | Workspace Template | complete typed static desired body: fixed Boundary, baseline policy, Runtime/entry, session, and creation defaults | immutable retained history | immutable; semantic no-op publishes nothing | TemplateID + semantic digest recomputed from the complete body |
| Context | one Project × one Template binding | project-specific work setup | survives Workspace deletion; until explicit Context deletion | binding immutable in V1; desired revision derives from Template current | ContextID |
| Policy Memory | Context | learned decisions for that Project/Template relationship | survives Workspace replacement; ends/reset by explicit Context/policy action | mutable, independently revised and atomically activated | Context identity + policy semantic revision |
| Workspace | Context | one applied isolated instance and home | replaceable; ends at Workspace delete | applied receipt and bounded reconciliation/observation change | WorkspaceID + ContextID; observations are not authority |

V1 permits at most one Context for one canonical Project root and TemplateID.
This keeps the Context durable without adding a second human name: another
Context for the same Project means another Template. Template rebinding and
revision pinning are excluded; both would need explicit policy-memory
revalidation and a separate lifecycle decision.

One TemplateID also fixes one immutable source/network Boundary fingerprint,
including direct source access and terminal network ceilings. A Boundary change
is not a Template revision: it creates a fresh TemplateID and, for a Project, a
fresh ContextID with empty Policy Memory. Revisions may change reviewed baseline
and typed Runtime/session/creation defaults only inside that fingerprint.

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
| inspect one design/revision | `template show [--name NAME]` | discover/read | name is read-only discovery input; produces `workspace-template` and exact current `workspace-template-revision` refs |
| create a direct design | `template create --name NAME ...` | act/create, fixed Template collection | fresh TemplateID and generation 1 from complete reviewed direct input; no copy mode |
| fork static definition | `template copy --from TEMPLATE_REVISION_REF --name NAME` | act/create, reference-bound parent | consumes one exact immutable revision unchanged; fresh TemplateID/generation 1; copies no lower-lifetime state |
| choose omission default | `template default set --id TEMPLATE_REF` | act/write, reference bound | consumes one `workspace-template` ref; changes no Context or Workspace |
| delete a design | `template delete --id TEMPLATE_REF --confirm=delete` | act/write, reference bound | only with no default selection and no Context; produces no ref |
| list project bindings | `context list [--format text\|json]` | discover/read | exhaustive Contexts with `context` refs; human groups by Project and Template |
| inspect one binding | `context show --id CONTEXT_REF` | discover/read | consumes and re-emits one unchanged `context` ref; reports Template desired plus Policy Memory current/active separately |
| instantiate a design | `context create --template TEMPLATE_REF [--format text\|json]` | act/create, reference-bound parent | canonical CWD is Project scope; produces one fresh `context` ref; creates no Workspace or policy authority beyond empty memory |
| enter an explicit Context | `context enter --id CONTEXT_REF [-- ARGV...]` | act/create, reference-bound parent | creates or reconciles the Context's Workspace and produces the exact `workspace` ref |
| forget project binding | `context delete --id CONTEXT_REF --confirm=delete` | act/write, reference bound | requires no Workspace/attachment/research credential; deletes Policy Memory and unresolved candidates |
| enter default pair | `tobari [-- ARGV...]` | existing command-owned act/create | resolves and revalidates the exact `DefaultTemplateSelection` plus CWD pair; first-use may compose Context create, cluster reconcile, and Workspace entry after review |
| inspect default pair | `status` | discover/read, command-owned default pair | read-only Context desired, Policy Memory activation, Workspace applied/observed; optionally produces a `workspace` ref when one exists |
| list applied instances | `workspace list` | discover/read | exhaustive Workspace inventory producing exact `workspace` refs |
| inspect one instance | `workspace status --id WORKSPACE_REF` | discover/read | consumes and re-emits one unchanged `workspace` ref; never rediscovers by root, Template name, or order |
| remove one instance | `workspace delete --id WORKSPACE_REF --confirm=delete [--force]` | act/write, reference bound | removes exact Workspace, home, and native auth; preserves Context and Policy Memory |

`--manifest`, the `manifest` namespace, `manifest_id`, `workspace_manifest_id`,
`context use`, a default/current Context, `--template NAME` on root/status/delete,
and other root/name-selected mutation routes are absent. `context create
--template TEMPLATE_REF` remains the reference-bound parent input for the
distinct Context-creation outcome. Bare root/status own only the revalidated
default pair. Every explicit nondefault or destructive action consumes one
unchanged opaque ref. Existing Template configuration mutations replace
`--manifest NAME` with required `--id TEMPLATE_REF`; their typed activation
semantics remain unchanged. Bounded name completion remains read-only discovery
only.

The final reference graph is:

```text
template list/show -> workspace-template ref
  -> template default set / template delete / Template config writes
  -> context create -> context ref
       -> context show / context enter / context delete

template show -> workspace-template-revision ref
  -> template copy

context enter / status / workspace list / workspace status -> workspace ref
  -> workspace status / workspace delete

Context + Workspace denial -> policy-candidate ref
  -> policy allow / policy deny / staged review permissions + apply-reviewed

policy rules -> policy-rule ref
  -> policy reset

context list/show/create -> context ref
  -> research-only auth login/import/status/logout --context CONTEXT_REF

runtime list/show/history -> runtime / runtime-revision refs
  -> unchanged WP03 build/delete/restore and Template Runtime selection

permission_wait_id remains a non-reference attachment correlation value.
Host Loopback route/grant and service-exposure references remain
attachment-local and never become Template or Context references.
```

The Catalog must derive this exact producer/consumer inventory:

| Reference kind | Producers and fields | Consumers and inputs |
|---|---|---|
| `workspace-template` | `template list.items[].template_ref`; `template show.template_ref` | `template default set --id`; `template delete --id`; `context create --template`; Template-owned `config shell`, `config git`, bootstrap, and Runtime-binding writes through required `--id` |
| `workspace-template-revision` | `template show.current_revision.revision_ref` | `template copy --from` only |
| `context` | `context create.context_ref`; `context list.items[].context_ref`; `context show.context_ref`; research-only `auth status.context_ref` | `context show --id`; `context enter --id`; `context delete --id`; research-only `auth login/import/status/logout --context` |
| `workspace` | `context enter.workspace_ref`; bare `status.workspace_ref` when present; `workspace list.items[].workspace_ref`; `workspace status.workspace_ref` | `workspace status --id`; `workspace delete --id` |
| `policy-candidate` | `policy candidates.items[].id`; `review permissions.items[].id` | `policy allow --id`; `policy deny --id`; unchanged staged selection consumed by internal `policy apply-reviewed` |
| `policy-rule` | `policy rules.items[].id`; `policy apply-reviewed.decisions[].rule_id` | `policy reset --id` |
| `runtime` | unchanged WP03 `runtime list/show` and `review runtimes` result fields | unchanged `runtime build --id` and `runtime delete --id` |
| `runtime-revision` | unchanged WP03 `runtime history/show` revision fields | unchanged `runtime restore --id` and exact Template Runtime-selection input |
| `runtime-prune-plan` | unchanged `runtime prune dry-run` plan field | unchanged `runtime prune apply --plan` |

No action accepts a display name, root, generation, ordinal, image, container,
or reconstructed ID in place of these references. The only acts without an
input ref are complete command-owned fixed targets such as direct Template
creation, bare default-pair entry, and fixed reviewed policy-set Apply.

Research keeps WP04's exact five-path delta and no release path:

| Research path | Binding |
|---|---|
| `auth login --context CONTEXT_REF ...` | reference-bound create with Context as the sole parent; creates fresh Context credential authority |
| `auth import --context CONTEXT_REF ...` | same Context-parent-bound create; protected stdin remains the only secret ingress |
| `auth status --context CONTEXT_REF ...` | discover/read over the exact Context; returns the unchanged `context_ref` in its typed result |
| `auth logout --context CONTEXT_REF ...` | reference-bound write targeting that Context's credential authority and handles |
| `serve` | unchanged research-only trusted-host presentation; no authentication owner selection |

All four auth operations consume the exact Context ref from the existing
Context producer chain. There is no Template name/ID fallback, installation-
wide implicit status, migration UUID rebind, or standard-surface auth path.
Context creation copies no credential; Context deletion reports a read-only
`auth status` recovery until exact logout succeeds.

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
| bare root `status` JSON | schema 2 | schema 3; default-pair Context/Template/Workspace fields and optional workspace ref |
| `workspace list/status/delete` JSON | predecessor root list/status/delete schema 2/1 | new family-local schema 1 with workspace refs |
| `cluster status` JSON | schema 1 | schema 2 with Template count and separate active Template-policy/Policy-Memory receipts |
| policy candidates/review/rules JSON | schema 1 | schema 2 with Context, Template, and observing Workspace dimensions |
| `migrate apply` JSON | schema 2 | schema 3 |
| Gateway denial/wait record | WP07 schema 2 | schema 3 with ContextID, WorkspaceID, and TemplateID projection |
| permission helper result | schema 1 | unchanged schema 1 |
| research auth command JSON | schema 1 with Manifest owner | schema 2 with exact Context owner/ref correlation |
| research Broker vault/handle state | schema 1 with Manifest owner | schema 2 created fresh for Context owner; predecessor is quarantined, not read |
| Host Loopback capability projection | schema 1 | unchanged schema 1 |
| Host Loopback private route/grant registry | ADR 0083 schema 2 | unchanged schema 2, now binds ContextID + WorkspaceID |
| Runtime public schemas and refs | WP03 schema 1 | unchanged schema 1 |
| version, error, help, build-surface schemas | WP04/current | unchanged shapes and versions |

Only contracts whose keys or field semantics change advance. A renamed command
with a genuinely new family starts at schema 1; unchanged helper, Runtime,
version, error, help, build-surface, and capability shapes do not receive a
ceremonial bump.

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
  correlation only. Every revision validates the TemplateID's one immutable
  Boundary fingerprint; a Boundary difference is a new Template, not B.
- **Context desired:** a read-only resolution of the bound Template's current
  digest. A Template mutation writes no Context and no Workspace.
- **Workspace applied:** `AppliedEntry` records ContextID, TemplateID/digest,
  exact RuntimeID/revision, slice digests, and reconciliation receipt only after
  verified explicit entry. Failure preserves the previous receipt.
- **Template cluster projection:** `cluster up` validates the complete
  installation candidate but records one authoritative receipt per Context as
  `(ContextID, TemplateID, active Template policy-slice digest)`. A separate
  installation aggregate digest may prove atomic publication, but is never the
  Context or Workspace applied revision. A stale Context receipt blocks entry
  where required; reads do not repair it.
- **Policy Memory current/active:** each confirmed decision mutation publishes
  one complete Context Policy Memory semantic revision. `policy
  apply-reviewed`, `policy allow`, `policy deny`, and `policy reset` retain their
  existing explicit atomic hot-activation boundary; `cluster up` can reconcile
  the complete aggregate after migration or cluster absence. The authoritative
  receipt is independently `(ContextID, active Policy Memory digest)`. Neither
  operation publishes a Template revision or AppliedEntry.

Workspace `AppliedEntry` is the third, separate entry-slice receipt; it never
stands for either cluster receipt. Remembered decisions are independent from
Template baseline and are active only when their exact typed applicability and
the fixed Template Boundary admit them. Same-Template Boundary widening is
unrepresentable, so an inert remembered decision cannot regain authority from a
Boundary edit. A newly widened Boundary means a new Template and new Context
with empty Policy Memory.

### Error and cancellation behavior

- Template source drift, Template name reuse, Context/Project mismatch, stale
  policy reference, changed Boundary, unsafe attachment, missing Runtime, and
  mixed identity all fail closed before mutation where possible.
- Read-only `template show --name` may observe a reused display name, but every
  later action consumes its emitted opaque ref unchanged. `template copy`
  resolves exactly the referenced retained immutable TemplateID/digest/body;
  deletion, name reuse, current-pointer advance, or receipt drift cannot redirect
  it to another Template or revision.
- Failed or unknown Workspace reconciliation preserves last-successful applied
  state and records a bounded read-only recovery action. It never advances
  AppliedEntry optimistically.
- Direct Template create, Template copy, and Context create are independent
  idempotency-scoped creates;
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
   migration model. The final Template revision stores the complete typed
   static body and recomputes all slice/overall digests from it; copy and entry
   derivation consume no predecessor or parallel body.
3. Application use cases and typed read/mutation ports for Template copy,
   Context creation/lifecycle, Workspace entry/deletion, and policy activation.
   The dormant final-envelope mutator uses the installation lifecycle
   authority, bounded stage/read-back publication, one durable active effect
   decision, and one latest terminal receipt; it remains absent from current
   composition and public routing.
4. Owner-only stores, atomic migration/rollback, principal projection, and
   desired/applied/observed reconciliation adapters. Context entry reuses the
   installation lifecycle authority and the one final-envelope effect decision,
   confirms exact runtime/container plus independent activation receipts before
   AppliedEntry, and calls a session-owner port before releasing the lock. The
   concrete WP07 attachment bridge lands with final principal/session
   projection, not in the dormant coordinator concern.
5. Catalog hard cutover, CLI/human/JSON/status/default-selection behavior,
   completion/help/examples/site, and generated snapshots.
6. Security, migration, public-boundary, agent-readiness, and isolated runtime
   integration gates; durable promotion and temporary packet cleanup.

Each slice should remain buildable and reviewable. The parent may split delivery
only where transaction and authority boundaries remain coherent.

## Verification

- Unit and contract tests: identity, complete-body clone/mutation canaries,
  semantic digest/no-op/A→B→A, normal exact copy/entry derivation, Context
  uniqueness, multiple Contexts per Project, Template follow-current, Policy
  Memory isolation, Workspace preservation/deletion, and AppliedEntry timing.
- Advanced-source tests: the only executable source projection is the bounded
  exact `tobari.rego`/`tobari_test.rego` pair; missing, renamed, duplicate,
  extra, incomplete, and oversized sources fail before authority publication.
- Negative side-effect tests: stale/different Template authority, name reuse,
  cross-Context rule reuse, policy beyond Boundary, attached unsafe adoption,
  read-triggered mutation, and partial/mixed migration.
- Opaque-reference and complete-pagination tests: Catalog-wide Template,
  Context, Workspace, `policy-candidate`, `policy-rule`, and migration-plan
  producer/consumer graph.
- Catalog construction must validate the final command set and its derived exact
  producer/consumer inventory; no RoleUtility command may declare any reference
  input or output. Exact-input discover reads must re-emit the unchanged ref.
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
writer and adds exact source kind `workspace_manifest_v1` at the atomic public
cutover. The journal engine remains dormant until the same concern switches
every ordinary reader to final authority; an intermediate binary must not move
predecessor authority away from still-current Manifest readers.

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
5. verify that every retained immutable Manifest revision for one preserved ID
   carries the same source/network Boundary fingerprint and one exact complete
   typed predecessor body. Any differing, missing, corrupt, or ambiguous body
   fails closed before mutation; the migration neither splits authority nor
   invents a replacement ID. Transform each validated body into one complete
   static Template revision, preserve generation as correlation, recompute every
   slice/overall semantic digest after dynamic learned decisions are excluded,
   and preserve every exact Runtime reference required by WP03 protection;
6. publish one Context schema-1 binding and one complete Policy Memory schema-1
   revision from the exact predecessor decisions. Learned authority drops the
   predecessor WorkspaceID dimension and binds ContextID so it survives a later
   Workspace replacement; pending observations retain both ContextID and the
   observing WorkspaceID;
7. rewrite Workspace state to schema 3 with the fresh ContextID and preserved
   WorkspaceID/home bytes. Synthesize or preserve AppliedEntry only when an
   exact predecessor receipt and bounded `exact_owned` Docker observation agree
   on WorkspaceID, Template generation/digest, RuntimeID/revision, and resolved
   spec. Missing, mismatched, or unknown observation publishes no AppliedEntry
   and becomes explicit unverified state;
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

The physical order is monotonic: publish and read back the complete final
envelope while predecessor readers remain selected; move one exact cutoff as
the atomic reader selector; then quarantine subordinate predecessor sources.
Rollback restores subordinate sources first while final remains selected,
restores the cutoff last, and only then atomically retires the final envelope.
Every source backup is a same-parent private sibling so separate XDG config and
state filesystems never require a cross-filesystem rename. Process exclusion
uses a safely validated kernel-released advisory lock, so a stale lock pathname
cannot deadlock journal recovery. A rolled-back transaction is terminal rather
than implicitly reusable; a separately reviewed new transaction is required.

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
