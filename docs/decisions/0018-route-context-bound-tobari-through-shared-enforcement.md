# ADR 0018: Route Context-bound Tobari through shared enforcement

- Status: Accepted
- Date: 2026-08-08
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, migration, and harness
- Supersedes: The single-active-Context and deferred per-Workspace routing decisions in ADR 0013; the active-Context runtime authority in ADR 0016
- Superseded by: None

## Context

An installation may need different runtime, policy, and credential boundaries
for work on the same canonical project root. Selecting one Context for the
whole cluster makes unrelated Tobari change authority together and cannot
represent that outcome. Adding a caller-provided Context field to HTTP input
would create a forgeable authority and adding one Gateway or OPA per Context
would discard the intentionally shared enforcement architecture.

## Decision

Each Context has a stable UUIDv7 `context_id`; its name is a human selector and
may not act as enforcement identity. Each logical Tobari has a stable UUIDv7
`project_id` and is uniquely selected by `(canonical root, context_id)`. Its
state permanently records that Context binding. There is no implicit Context
migration. Two Tobari may mount the same canonical root, and parent and child
roots may overlap. Their homes, work containers, internal networks, principals,
runtime authority, policy authority, and managed credentials are separate;
their mounted host files are deliberately shared and have no filesystem
integrity isolation.

The current Context means only the default used when a host command omits a
Context. `context use` atomically changes that default. It does not alter an
existing Tobari, reconcile shared enforcement to one Context, or start a
stopped cluster. `tobari --context NAME` chooses the Context for that invocation
without changing the default.

The installation continues to run exactly one Gateway and one OPA. The
host-owned principal registry schema 2 binds an exact Gateway local interface
address and network to `project_id`, `context_id`, Context name, and canonical
root. Gateway derives both trusted identities from the incoming connection's
local address. Headers, URLs, environment, session data, profile names, and
client project or Context values are never authority. Unknown, stale,
ambiguous, or inconsistent bindings fail before OPA or upstream I/O.

Gateway calls one fixed OPA decision endpoint with body-free input schema 4.
The trusted principal contains `cluster`, `context_id`, and `project_id`. OPA's
Tobari-owned router selects the Context policy by stable ID and denies unknown
or missing Contexts. It never falls back to the current or default Context.

Context policy and credential stores remain the sources of truth. Before
activation, the host creates one content-addressed cluster projection from all
Contexts under an owner-only temporary directory, validates every source policy
and the complete candidate with OPA, and only then atomically publishes it.
The revision covers Context manifests, policy data and modules, credential
metadata, and credential secret contents. Activation serializes mutations,
recreates only the shared OPA when policy changes, waits for health, and rolls
back the source and prior known-good projection on failure. Partial projections
are never mounted.

Guided Contexts share one Tobari-owned evaluation module and have Context data
under `data.tobari_contexts[context_id]`. Advanced source continues to declare
`package tobari.http`, but projection rewrites it into a reserved
`tobari.contexts.c<uuid>.http` namespace. User source may not claim the router,
system packages, another Context namespace, or aggregate data namespace. One
OPA is not claimed to provide process-level confidentiality between Rego
modules; the guarantees are validated routing, namespace separation, safe
activation, and explicit authority scope.

Gateway receives only a schema-2 principal registry and schema-2 credential
projection plus Context-scoped secret paths. Credential profiles are unique by
`(context_id, profile name)` and are checked against Context, project, and host
before a secret is read. OPA receives non-secret metadata only.

Denials, candidates, learned allows, exact denies, compactions, rule inventory,
audit, and opaque references retain Context and project identity. Reference-
bound mutations derive the complete target from one opaque reference; a
Context flag is never combined with the reference to construct authority.
`policy review` and `policy rules` are installation-wide views, so the fixed
denial recovery command remains `tobari policy review`. Human list, detail, and
confirmation views show Context, project root, and request. Machine output also
returns stable Context and project identities.

## Migration

Context manifest schema 3 assigns stable IDs. Cluster state schema 3 stores the
aggregate revision and Context count, not one active enforcement Context.
Project state schema 2 and root indexes bind Context identity. Existing project
IDs, canonical roots, and homes are retained only when legacy active marker,
cluster paths, Context manifest, project state, and root index consistently
identify one valid Context. The project is then fixed to that Context. Missing,
conflicting, interrupted, unsafe, or incomplete evidence fails closed with a
recovery diagnosis. Context-less historical policy evidence is inert and must
be re-observed; it is never guessed into an actionable Context.

## Consequences

- Runtime builds update only their owning Context. Bound Tobari reconcile that
  Context image on next entry while preserving identity and home.
- Context creation while a cluster is running requires an explicit `cluster
  up` to activate a new aggregate; creation and selection never start Docker.
- `cluster status` reports shared health, loaded Context count, aggregate
  revision, and principal/credential projection integrity rather than an
  active enforcement Context.
- Cluster down remains prohibited while any Context-bound Tobari exists.
- No Context move operation, overlay, checkout clone, root lock, session ban,
  or per-Context enforcement service is introduced.

## Mechanical enforcement

- Domain and state tests cover stable IDs, `(root, Context)` uniqueness,
  migration ambiguity, and Context-scoped opaque references.
- Gateway and OPA tests cover forged selectors, fixed-endpoint routing, unknown
  Context denial, project/Context credential binding, and Context-local rules.
- Aggregate tests cover reserved namespaces, secret-sensitive revisions,
  whole-candidate validation, known-good retention, and serialized updates.
- Docker integration creates same-root and overlapping-root Tobari in different
  Contexts, proves one Gateway/OPA, Context-local policy/runtime/credential
  behavior, installation-wide review/rules UX, restart restoration, safe
  deletion, and cluster-down refusal.

## Validation

- `task check`
- `task security`
- `task public:check`
- `task policy:test`
- `task gateway:test`
- `task integration:test`
