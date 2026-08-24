# Work Plan: WP11 — Separate Workspace Template, Context, and Policy Memory

- Status: Proposed
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Take the four-concept model to owner review and sequencing:

```text
Workspace Template (stable reusable definition)
  └─ current Template Revision / Manifest (immutable semantic body)
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
| Template Revision / Manifest | Workspace Template | complete static desired definition | immutable retained history | immutable; semantic no-op publishes nothing | TemplateID + semantic digest |
| Context | one Project × one Template binding | project-specific work setup | survives Workspace deletion; until explicit Context deletion | binding stable in V1; desired pointer follows Template current | unresolved ContextID or exact typed pair |
| Policy Memory | Context | learned decisions for that Project/Template relationship | survives Workspace replacement; ends/reset by explicit Context/policy action | mutable, independently revised and atomically activated | Context identity + policy semantic revision |
| Workspace | Context | one applied isolated instance | replaceable; ends at Workspace delete | applied receipt and bounded reconciliation/observation change | WorkspaceID + Context identity; observations are not authority |

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

The exact CLI is intentionally not fixed by this packet; the following is the
semantic shape to evaluate:

- `template create --copy-from SOURCE --name TARGET` is a fixed-target create
  that forks one reviewed exact immutable current Template revision into a fresh
  independent TemplateID at generation 1. It copies no Context, Policy Memory,
  Workspace, home, auth, applied/failure/observed state, or default selection and
  performs no reconciliation.
- `context create --template TEMPLATE ...` creates the durable project-specific
  binding. It selects an existing Template; it does not copy or fork it. The
  command must bind one canonical Project and exactly one Template authority.
- Workspace entry selects one Context, resolves its desired Template revision,
  compares it with last-successful applied state and observation, and reconciles
  only after explicit user action.
- `workspace delete` removes the replaceable instance and preserves Context plus
  Policy Memory. `context delete` is a separate explicit destructive outcome
  whose exact policy-memory/home disposition requires owner decision.
- A default Template, if retained, means “Template proposed when creating a new
  Context.” It must not be described as the current Context or active Workspace.
  Whether a separate per-Project default Context exists remains open.
- Routine human output should lead with Context, its Template, and `Current` /
  `Next entry`; details expose exact Template and Policy Memory revisions without
  implying one combined revision.
- Structured output requires new schema versions rather than silently changing
  the meaning of `manifest_id`, policy ownership, or Workspace keys. Old and new
  identities must not dual-authorize.
- Copy and Context creation are `RoleAct`/`EffectCreate`; Template/Context
  discovery is read-only. Exact reference kinds and producer/consumer chains
  must be designed Catalog-wide before implementation.
- Delivery/coverage, structured fault codes, confirmation values, and next
  commands remain open and must be fixed in the governing contract.

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
- Context deletion must refuse or explicitly resolve live Workspace,
  attachment, Policy Memory, home, authentication, and audit ownership; no
  implicit cascade is assumed by this packet.

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
- Migration and rollback must be exclusive, journaled, content-checked, and
  atomic across Template, Context, Workspace, principal, and policy projection
  state. Old/new readers must never concurrently authorize the same bytes under
  different identity meanings.
- No credentials, external destinations, dependency, daemon, or arbitrary
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

This requires a pre-public hard-cutover migration if approved. Preserve existing
WorkspaceManifestID bytes only when the new Template authority semantics are
exactly equivalent; otherwise issue typed fresh identities and retain an
explicit mapping inside the migration transaction. Create Context authority
from exact current Manifest/project/Workspace and learned-policy evidence, never
from names, generations, roots alone, or observed containers. Preserve
last-successful AppliedEntry and Workspace home only when exact correlation is
proven; otherwise retain explicit pending/unverified state.

Migration must stop the cluster, require zero live attachments, preflight all
owners/schemas/digests, journal an owner-only backup, publish atomically, and
require explicit reconciliation. Rollback restores the complete predecessor
authority only if no fresh canonical state would be overwritten. Ordinary
readers accept one final schema; aliases and dual-authority readers are not the
default plan.

The parent must decide timing relative to the first public release. Deferring
WP11 after public V1 would require an explicit compatibility and deprecation
strategy rather than the pre-public hard cutover assumed here.

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
