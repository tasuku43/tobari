# ADR 0084: Separate Workspace Templates, Contexts, and Policy Memory

- Status: Proposed
- Date: 2026-08-24
- Deciders: Tobari product owner and maintainers
- Scope: Product, domain, CLI, architecture, security, state, migration,
  harness, and public boundary
- Supersedes: ADR 0079 upon acceptance
- Revises: ADR 0080, ADR 0081, ADR 0082, and ADR 0083 only at their typed
  identity, protection, authentication-owner, and policy-owner seams
- Related: ADR 0066, ADR 0070, and WP11
- Superseded by: None

## Implementation-authority status

This Proposed ADR authorizes implementation and tests on the WP11 branch. It
does not claim that current public commands, JSON, persisted state, or ordinary
readers have cut over. ADR 0079 remains the current behavior until this decision
is implemented atomically, its governing documents are promoted, required gates
pass, and this ADR is accepted.

## Context

ADR 0079 correctly separated immutable desired revisions from last-successful
applied and currently observed Workspace facts. Implementation and product
review then exposed one remaining ownership mismatch: a reusable static design
and policy learned from one Project's runtime activity do not share owner,
scope, lifetime, or mutability. Calling both one Workspace Manifest makes a
class-like definition appear to grow from one instance's experience and makes
Workspace deletion semantics difficult to explain.

The product owner requires learned decisions to survive replacement of the
applied Workspace, while a reusable definition may be shared by several
Projects. The final pre-public V1 model therefore needs separate authorities
before status, first use, Host Loopback, and service-exposure work binds itself
to the ADR 0079 aggregate. The accepted design evidence is
`docs/work/workspace-template-context-policy-memory/`.

## Decision drivers

- Resource boundaries follow owner, scope, lifetime, mutability, and authority,
  not naming convenience or the current quantity of code.
- A reusable static design must not own Project-specific learned authority.
- Workspace replacement must preserve the durable Project/design relationship
  and its remembered policy until an explicit Context deletion.
- Mutable display names must not authorize an action after discovery.
- One stable Template identity must never widen remembered authority by changing
  its terminal source/network Boundary.
- Desired Template state, active static policy, active Policy Memory,
  last-successful Workspace entry, and observation remain independent facts.
- Workspace entry and `cluster up` remain the only mutation-bearing
  reconciliation boundaries; Tobari has no resident controller.
- The cutover is pre-public and atomic: no aliases, dual readers, or
  intermediate public schemas become compatibility obligations.

## Considered options

### Keep Workspace Manifest and change presentation only

This preserves the current implementation but leaves reusable static state and
Project-specific learned authority under one owner. Workspace deletion and copy
would continue to require exceptions that presentation cannot repair.

### Split Boundary and defaults into several reusable public resources

Source access, network ceiling, Runtime binding, and defaults have different
activation timing, but V1 has no independent owners or lifecycles that justify
separate selectors and deletion graphs. Typed slices inside one Template retain
the useful distinction without multiplying public resources.

### Make Context the static class and add a separate instance policy object

This preserves historical vocabulary but conflicts with the desired durable
meaning: Context is the Project-specific continuing relationship that survives
Workspace replacement. Reusing Context as the static class would leave that
relationship unnamed.

### Combine Template and Context and keep only Workspace separate

This reduces one noun but makes a Project-specific aggregate non-reusable and
turns static copying into a copy of dynamic memory. It cannot express several
Projects using the same reviewed setup without shared learned policy.

## Decision

V1 has four concepts in this area:

| Concept | Owner and scope | Lifetime | Mutability | Authority |
|---|---|---|---|---|
| Workspace Template | installation; reusable across Projects | until explicit Template deletion | stable identity with complete immutable revisions | WorkspaceTemplateID plus semantic revision digest |
| Context | one canonical ProjectRoot plus one WorkspaceTemplateID | survives Workspace deletion; ends at Context deletion | binding is immutable in V1 | ContextID |
| Policy Memory | one Context | exactly the Context lifetime | complete immutable remembered-decision revisions | ContextID plus semantic Policy Memory digest |
| Workspace | one Context's applied isolated instance and home | replaceable; ends at Workspace deletion | last-successful receipt and bounded reconciliation/observation change | WorkspaceID plus ContextID; observation is not authority |

`Workspace Template` is the public long noun and `template` is its CLI noun.
`Context` is the durable Project/Template binding and owner of `Policy Memory`.
Routine UI may say “remembered decisions”. `Workspace` remains the replaceable
applied instance. `Manifest` is retired from current public/domain resource
vocabulary and may describe only the private predecessor serialization during
migration.

### Identity and uniqueness

`WorkspaceTemplateID`, `ContextID`, and `WorkspaceID` are distinct opaque typed
identities. Names, roots, generations, ordinals, images, containers, labels, and
timestamps never authorize. Content authority is TemplateID plus semantic
Template digest or ContextID plus semantic Policy Memory digest. Generation is
monotonic correlation only.

Exactly one Context may exist for one `(canonical ProjectRoot,
WorkspaceTemplateID)` pair. A Project may have several Contexts only by binding
different Templates. Context has no second human name, cannot rebind to another
Template, and has no current/default/use selector in V1.

The installation owns one optional `DefaultTemplateSelection`. Bare `tobari`
and bare `status` own only the command-local default-Template/canonical-CWD pair
and revalidate that exact selection. An explicit nondefault action consumes an
opaque Context or Workspace reference unchanged. Mutable names are allowed only
for bounded read-only discovery and completion.

### Immutable Template Boundary and revisions

One WorkspaceTemplateID fixes one immutable fingerprint covering direct source
access and terminal network destination/method ceilings. A Boundary change
creates a fresh TemplateID and therefore a fresh ContextID with empty Policy
Memory. V1 has no rebind. Template revisions may change reviewed baseline policy
and typed Runtime, entry, session, and creation defaults only within the fixed
Boundary.

Each semantic Template mutation publishes one complete immutable revision.
TemplateID plus semantic digest authorizes content. A semantic no-op publishes
nothing and does not increment generation. A→B→A publishes a later generation
with A's original semantic digest. A retained storage receipt may use
`generation + digest`, but generation never becomes content authority.

Template copy and Context creation are separate outcomes. `template copy
--from TEMPLATE_REVISION_REF --name NAME` consumes one exact retained immutable
revision reviewed through `template show`; it issues a fresh TemplateID at
generation 1 and copies no Context, Policy Memory, Workspace, home,
authentication, candidate, attachment, applied, failure, observed, or default
state. It persists no lineage or provenance. `context create --template
TEMPLATE_REF` creates the unique Project binding with empty Policy Memory and no
credential or Workspace reconciliation.

### Independent desired, applied, and active receipts

- Template current is one immutable TemplateID/digest selected by its current
  pointer.
- Context desired is a read-only resolution of its bound Template's current
  digest. Template mutation writes no Context or Workspace.
- `cluster up` validates one complete installation candidate and records one
  static-policy receipt per Context:
  `(ContextID, TemplateID, active Template policy-slice digest)`.
- Policy review/apply records a separate receipt:
  `(ContextID, active Policy Memory digest)`.
- Workspace `AppliedEntry` is a third independent receipt for the exact entry
  slice and Runtime applied only after verified explicit entry.
- A complete installation aggregate digest may prove atomic publication, but it
  is never the Context or Workspace applied revision.
- Status, list, show, doctor, and completion observe only; they never publish,
  activate, repair, or reconcile.

Remembered Allows and exact Denies are independent from Template baseline and
are effective only when their exact typed applicability and the fixed Template
Boundary admit them. Same-Template Boundary widening is unrepresentable, so a
previously inert remembered decision cannot silently regain authority from a
static edit.

### Deletion and authentication ownership

| Delete | Removes | Preserves | Required blockers to clear |
|---|---|---|---|
| Workspace | exact Workspace/container state, home, native auth, pending Workspace observation, WorkspaceID | Context, Policy Memory, Project files, Template, Runtime | no live attachment; exact Workspace ref and confirmation |
| Context | Context binding, Policy Memory, unresolved candidates | Project files, Templates, Runtimes, bounded non-authorizing audit evidence | no Workspace, attachment, or research credential; exact research logout first |
| Template | exact Template identity/current/retained revisions | Projects, Runtimes, unrelated Contexts and Workspaces | no default selection and no Context; exact Template ref and confirmation |

Standard authentication remains Workspace-home owned and is removed with the
Workspace. In the research build only, Broker credential authority becomes
Context-owned and remains outside Template desired/applied state. The exact
five research paths from ADR 0082 remain; `auth login`, `auth import`, `auth
status`, and `auth logout` each consume one unchanged Context reference, while
`serve` has no authentication-owner selector. Login/import are Context-parent
creates, status is a discover/read that re-emits `context_ref`, and logout
targets that Context's credential authority. Release exposes none of these
paths. Context creation copies no credential and migration does not rebind
predecessor Broker state.

### Public command and reference graph

Discovery produces exact opaque references; actions consume them unchanged:

```text
template list/show -> workspace-template
  -> template default set / template delete / context create / Template config

template show -> workspace-template-revision
  -> template copy

context list/show/create -> context
  -> context show / context enter / context delete
  -> research auth login/import/status/logout

context enter / status / workspace list/status -> workspace
  -> workspace status / workspace delete

policy candidates / review permissions -> policy-candidate
  -> policy allow / policy deny

policy rules / apply-reviewed -> policy-rule
  -> policy reset
```

`template show`, `context show`, bare `status`, `workspace status`, and research
`auth status` are `RoleDiscover` when they produce or consume references; an
exact-input read re-emits the unchanged reference. No `RoleUtility` command has
a reference input or output. Runtime, Runtime revision, and Runtime prune-plan
reference kinds and mechanisms from ADR 0080 remain unchanged.

The hard cutover exposes `template`, `context`, and `workspace` families with
the accepted reference bindings. It exposes no `manifest` namespace,
`--manifest`, `manifest_id`, `workspace_manifest_id`, `context use`,
name-selected mutation, or compatibility alias. `--template NAME` is absent
from root/status/delete; `context create --template TEMPLATE_REF` remains the
typed parent input for the distinct Context-create task.

### Schema cutover

Only contracts whose keys or semantics change advance. Template, Context,
Policy Memory, and the new family command results start at schema 1. Workspace
state advances from predecessor schema 2 to schema 3. Bare status advances to
schema 3; cluster status and policy projections advance to schema 2; migration
advances to schema 3; denial/wait projections advance to schema 3; research auth
advances to schema 2. Unchanged Runtime, helper, Host Loopback public capability,
version, error, help, and build-surface contracts do not receive a ceremonial
bump.

Frozen private Gateway/OPA/Broker/Host Loopback compatibility spellings
`context_id`, `project_id`, and `context` remain versioned wire tokens, not
public/domain aliases. Their validated values become ContextID, WorkspaceID,
and Context presentation respectively. ADR 0083's private route/grant schema,
hostname, retirement guard, lock order, and attachment lifetime do not change.

## Consequences

### Positive

- Users can distinguish a reusable setup, a durable Project relationship, its
  learned memory, and a replaceable applied instance.
- Workspace replacement preserves useful learned decisions without sharing
  them across Projects or Template copies.
- Static copy cannot accidentally copy dynamic authority or state.
- Routine Current/Next output can expose three independent receipts without
  inventing one combined revision.
- Immutable Boundary and reference-bound actions close two normal name-reuse
  and authority-reactivation races.

### Negative

- The pre-public implementation receives another complete vocabulary, schema,
  state, principal, policy, help, fixture, and migration cutover.
- Context and Workspace acquire separate inventory and deletion lifecycles.
- Policy learning must be rebound from predecessor Workspace dimensions to the
  fresh Context identity.
- Intermediate branch commits may contain dormant final domain types while ADR
  0079 remains the only ordinary reader/writer contract.

### Risks and mitigations

- Partial cutover could allow old and new readers to interpret the same bytes
  as different authorities. Only `migrate apply` may publish final stores, and
  ordinary readers never dual-read.
- Mutable name reuse could redirect a reviewed action. Every explicit action
  consumes one unchanged opaque ref; names remain discovery-only.
- A Boundary edit could reactivate old learned authority. TemplateID fixes the
  Boundary fingerprint and any change requires a fresh Context with empty
  memory.
- One aggregate receipt could hide stale policy or entry state. Typed domain
  records and status fixtures keep Template-policy, Policy-Memory, and
  AppliedEntry receipts independent.

## Mechanical enforcement

- Distinct domain types validate WorkspaceTemplateID, ContextID, WorkspaceID,
  immutable Template revisions/history, fixed Boundary fingerprints, Context
  uniqueness, Policy Memory revisions, Workspace bindings, and AppliedEntry.
- Opaque-reference parsers are kind-specific and reject names, roots,
  generations, ordinals, images, containers, reconstructed IDs, and cross-kind
  values.
- Pure migration tests fix byte-preserved Template/Workspace IDs, fresh stable
  journaled Context IDs, retained-revision transformation, exact policy-memory
  mapping, default conversion, quarantine disposition, rollback eligibility,
  idempotence, and fail-closed mixed/ambiguous sources before I/O exists.
- Catalog construction must validate and its derived ProducedRefs/ConsumedRefs
  must contain the exact graph above with zero RoleUtility reference edges.
- Security, migration, public-vocabulary, source/snapshot equality, and isolated
  Docker integration guards become required before acceptance.

## Compatibility and migration

This is one pre-public hard cutover from exact predecessor commit
`0bbd9deb424814ab92eed0b816e2c565e4b8f6d3`. `migrate apply` remains the sole
writer and accepts one exact `workspace_manifest_v1` source. It requires the
installation lifecycle lock, stopped cluster, zero canonical attachments, and
the established research-quarantine and Host Loopback lock order.

Preflight enumerates all schema-2 Manifests/revisions, Workspaces,
AppliedEntry/pending/failure state, default selection, learned Allows/exact
Denies, pending candidates, principal projection, Runtime protection edge,
standard home, and research Broker filesystem authority. It validates exact
owner/type/mode/symlink/schema/digest/reference closure before mutation.

The transaction preserves each WorkspaceManifestID byte sequence as
WorkspaceTemplateID and each WorkspaceID byte sequence as WorkspaceID. It
generates and journals one fresh ContextID per exact `(canonical ProjectRoot,
WorkspaceTemplateID)` pair. Every retained revision under one preserved ID must
carry the same Boundary fingerprint; any different, missing, corrupt, or
ambiguous fingerprint fails before mutation. Valid revisions become static
Template revisions with generation retained as correlation and semantic digest
recomputed after dynamic learned decisions are excluded.

Each exact predecessor policy set becomes one complete Context-owned Policy
Memory revision. Confirmed authority drops the predecessor WorkspaceID
dimension; pending observations retain ContextID plus observing WorkspaceID.
Workspace state advances with the fresh ContextID and preserved WorkspaceID/home
bytes. AppliedEntry is retained or synthesized only from exact predecessor
receipts and bounded owned-Docker evidence; otherwise migration records explicit
pending/unverified state. DefaultManifestSelection becomes
DefaultTemplateSelection with the preserved ID.

Replay-capable predecessor research Broker filesystem authority is quarantined,
never rebound. macOS Keychain recovery material remains untouched; Linux
filesystem root-key material moves with the exact set. Standard Workspace homes
and native auth bytes are not read or transformed.

The journal records predecessor digests, preserved IDs, fresh Context mappings,
transformed revision receipts, and private backup identity. A second apply is
`changed:false` and cannot regenerate Context IDs or overwrite backup. Rollback
restores the byte-identical predecessor only when no fresh final authority
exists at canonical paths and never merges. Crash recovery exposes either the
complete predecessor reader set or complete final reader set, never a partial
policy/principal authority. Final ordinary readers accept only final schemas;
predecessor readers accept only predecessor schemas.

## Security and public-boundary impact

Policy Memory changes the owner of learned authorization state from a
Manifest/Workspace pair to exact ContextID. Cross-Context and cross-Project
candidate, rule, handle, receipt, and principal reuse fails closed. Template
Boundary remains terminal above baseline and remembered positive authority.

The decision adds no credential source, external destination, network
capability, daemon, arbitrary Template file, provider adapter, dependency, or
release-surface research path. WP07 permission transport/session lifetime,
WP03 Runtime mechanisms, WP04 compile-time surfaces, and ADR 0083 Host Loopback
authority remain intact; only their typed identity/protection/auth/policy seams
change.

## Validation

- focused pure domain and migration tests, including clone isolation and every
  invalid identity, cross-owner, Boundary, revision, receipt, and rollback case
- application/infrastructure/Catalog/public cutover tests in later concerns
- `task check`
- `task security`
- `task public:check`
- `task release:check`
- source/helper snapshot equality and repository hygiene
- agent-readiness for Template discovery/copy, Context creation/entry, policy
  learning, Workspace replacement, logout, and deletion
- isolated explicit-Docker-context integration without changing the default
  Docker context or cluster

## Reconsideration signals

- Independent V1 owners or lifetimes for source access, network Boundary,
  Runtime binding, shell/Git defaults, or bootstrap justify splitting Template.
- A supported need to rebind one Context to another Template requires a new
  learned-authority revalidation and migration decision.
- Repository-authored declarative configuration requires a separate file-format
  and trust-boundary ADR; it does not follow from the Template noun.
- Public V1 release retires this predecessor migration path through an explicit
  capability-retirement decision.
