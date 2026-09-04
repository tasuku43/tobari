# ADR 0084: Separate Workspace Templates, Contexts, and Policy Memory

- Status: Accepted
- Date: 2026-08-24
- Deciders: Tobari product owner and maintainers
- Scope: Product, domain, CLI, architecture, security, state, migration,
  harness, and public boundary
- Supersedes: ADR 0079 and ADR 0070
- Revises: ADR 0027, ADR 0080, ADR 0081, ADR 0082, and ADR 0083 at their
  pre-release compatibility, typed identity, protection,
  authentication-owner, and policy-owner seams
- Related: ADR 0066 and WP11
- Revised by: ADR 0087 at the executable-policy and evaluator-identity seam;
  ADR 0088 at the persistence, desired-source, Boundary revision, draft, and
  explicit installed-state migration seams; ADR 0092 at Context location and
  Workspace ownership seams; ADR 0093 at current Context selection and CWD
  routing seams
- Superseded by: None

ADR 0088 makes Template and Context desired sources concept-separated files
and active authority a concept-object generation selected by an atomic pointer,
not one monolithic envelope. Method Boundary changes are planned moving-head
revisions, create/copy issue drafts, and the exact supported typed predecessor
has one explicit `installation migration plan/apply` path. Older immutable-
Boundary, direct-active create/copy, and no-public-migration statements below
are decision history rather than current contract.

## Implementation-authority status

This Accepted ADR authorizes implementation and tests on the WP11 branch. The
public vocabulary, Catalog, JSON, persisted state, and ordinary readers still
move together in one atomic product commit; an intermediate dirty snapshot or
partial commit is not a supported composition.

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
| Context | one location-free WorkspaceTemplateID binding and one managed Home | survives Workspace deletion; ends at Context deletion | binding is immutable; Home contains tool-owned mutable state | ContextID |
| Policy Memory | one Context | exactly the Context lifetime | complete immutable remembered-decision revisions | ContextID plus semantic Policy Memory digest |
| Workspace | one Context's applied isolated instance that mounts the Context Home | replaceable; ends at Workspace deletion | last-successful receipt and bounded reconciliation/observation change | WorkspaceID plus ContextID; observation is not authority |

`Workspace Template` is the public long noun and `template` is its CLI noun.
`Context` is the durable location-free Template binding and owner of `Policy Memory`.
Routine UI may say “remembered decisions”. `Workspace` remains the replaceable
applied instance. `Manifest` is retired from current public/domain resource
vocabulary and may describe only the private predecessor serialization during
migration.

### Identity and uniqueness

`WorkspaceTemplateID`, `ContextID`, and `WorkspaceID` are distinct opaque typed
identities. Names, generations, ordinals, images, containers, labels, and
timestamps never authorize. Content authority is TemplateID plus semantic
Template digest or ContextID plus semantic Policy Memory digest. Generation is
monotonic correlation only.

ContextID uniqueness is the only Context uniqueness rule. Multiple Contexts may
bind the same WorkspaceTemplateID. Context has no ProjectRoot, second human
name, or mutable Template binding. ADR 0093 adds one installation-owned current
Context selector without adding location to Context itself.

The installation owns one optional `DefaultTemplateSelection` and, per ADR
0093, one optional current Context selector. CWD selects only Workspace
candidates; an existing Workspace supplies its permanent Context binding.
Context-aware commands use the current selector unless one exact reference
overrides that invocation. Mutable names are allowed only for bounded
read-only discovery and completion.

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

The revision owns one complete typed body: immutable source/network Boundary,
baseline policy, exact Runtime binding, entry defaults, session defaults, and
creation defaults. Boundary, policy, entry, session, creation, and overall
semantic digests are recomputed from that body during validation. Digests are
receipts and activation identities, not a substitute body or permission to read
the predecessor Manifest store. Copy, entry derivation, and cluster projection
must therefore be derivable from the one validated final revision alone.
The final authority retains only canonical typed policy data. The evaluator is
Tobari-owned and is materialized from embedded runtime assets; user-owned
Template, Context, and configuration state contains no executable policy
source. Persisted V1 Advanced markers are rejected by the bounded clean-break
guard described in ADR 0087 rather than decoded or translated.

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
  -> context show / context use / context delete
  -> research auth login/import/status/logout

bare tobari / status / workspace list/status -> workspace
  -> workspace status / workspace delete

policy candidates / review permissions -> policy-candidate
  -> policy allow / policy deny

policy rules / apply-reviewed -> policy-rule
  -> policy reset
```

The internal fixed-target `policy apply-reviewed` action is an `EffectCreate`:
one non-empty confirmed set is the fixed creation scope and every reviewed
decision creates one resulting active `policy-rule` child reference, even when
the same atomic transition compacts exact source rules. A same-set terminal
replay returns that original confirmed create result without repeating effects.

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
is outside the pre-release clean-break surface; denial/wait projections advance
to schema 3; research auth advances to schema 2. Runtime
list/show/history/build/restore/delete schemas and the Runtime,
Runtime-revision, and Runtime-prune-plan reference kinds remain unchanged. The
Runtime prune plan JSON alone advances to schema 2 because its protection-edge
keys and reasons identify final Template, Context, and Workspace authority and
reject every predecessor Manifest spelling. Unchanged helper, Host Loopback
public capability, version, error, help, and build-surface contracts do not
receive a ceremonial bump.

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
- Planned complete-revision Apply and reference-bound actions close normal
  name-reuse, stale-source, and authority-reactivation races. Boundary changes
  are reviewed moving-head Template revisions rather than in-place mutations.

### Negative

- The pre-public implementation receives another complete vocabulary, schema,
  state, principal, policy, help, fixture, and clean-break cutover.
- Context and Workspace acquire separate inventory and deletion lifecycles.
- Existing development state is not retained; users explicitly reset and
  recreate under the final model.
- Intermediate branch commits may contain dormant final domain types but may not
  expose a partial public composition.

### Risks and mitigations

- Partial cutover could allow old and new readers to interpret the same bytes
  as different authorities. Ordinary composition selects only the final owner
  store, never dual-reads, and rejects bounded legacy presence before final
  initialization or mutation.
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
- Clean-break tests fix genuinely fresh final-empty authority, bounded legacy
  presence and ambiguity rejection, zero predecessor influence, zero mutation,
  and explicit reset-and-recreate guidance.
- Catalog construction must validate and its derived ProducedRefs/ConsumedRefs
  must contain the exact graph above with zero RoleUtility reference edges.
- Security, clean-break, public-vocabulary, source/snapshot equality, and
  isolated Docker integration guards are required before completion.

## Pre-release compatibility and clean break

Tobari has no public release, so this cutover deliberately carries no
development-state compatibility obligation. Ordinary readers select only the
bounded owner-only final authority envelope. When that envelope is absent, a
bounded fixed-path presence guard must also prove no declared predecessor
Manifest, Workspace, Policy, Broker, principal, route/grant, or session
authority before reporting exact empty final state and permitting first
Template/Context creation.

The closed inventory has two lifetimes. Legacy-only roots are `contexts`,
`roots`, `instances`, the predecessor auth-project registry, `state.json`,
`project-journal.json`, `cluster-reconcile.json`, and the config/state
`migrations` roots; they remain absent for the life of a final installation.
Roots reused by final adapters are the cluster projection and principal
registry roots, config/state auth roots, Workspace profile/home root, Host
Loopback and interactive-attachment registries, and service-exposure state;
they must be absent at first final-envelope creation, after which only their
owning final adapters may accept their exact final schemas. WP03 Runtime
catalog, material, and runtime-lifecycle roots are preserved final authority
and are deliberately outside this inventory.

The guard is not a compatibility reader. It observes only path, owner, type,
mode, and stability facts needed to classify presence. Legacy, unsafe, partial,
or changing presence returns a typed reset-and-recreate fault with zero
mutation. The final binary does not decode, transform, back up, quarantine,
restore, rename, delete, or adopt that content. It preserves no predecessor ID,
learned rule, pending candidate, Runtime protection edge, Workspace home,
principal, attachment, Broker credential, or Keychain/root-key relationship.
Destructive reset remains a separate explicit user action.

The public `migrate apply` predecessor capability from ADR 0070 is retired and
the WP11 migration/rollback engine is not selected by public composition. Fresh
final operations retain owner-only bounded reads, lifecycle serialization,
durable task recovery, atomic whole-envelope publication, and coherent
principal/policy/authentication settlement; removing predecessor migration does
not weaken those final-state guarantees.

This exception ends at the first public release. It neither decides nor waives
future compatibility obligations. Any incompatible released-state change must
first receive an explicit release-policy and migration/compatibility decision
based on supported user data.

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

- focused pure domain and clean-break tests, including clone isolation and every
  invalid identity, cross-owner, Boundary, revision, receipt, legacy-presence,
  and final recovery case
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
