# ADR 0089: Compile closed semantic policy modules

- Status: Accepted
- Date: 2026-08-27
- Deciders: Tobari product owner and maintainers
- Scope: Product, domain, Gateway, policy source, Policy Memory, evaluator,
  migration, security, harness, and public boundary
- Revises: ADR 0022, ADR 0047, ADR 0075, and ADR 0088 at their policy-language
  and protocol-classification seams
- Related: ADR 0087
- Superseded by: None

## Context

Tobari already derives useful bounded identities for ordinary HTTP, GraphQL,
MCP, signed AWS RPC, Kubernetes API, Git Smart HTTP, and OCI Distribution.
Those identities currently share one flat domain union and only HTTP, GraphQL,
and MCP can contribute static Template authority. Kubernetes additionally
flattens a structured API coordinate into one string. ADR 0088 reserved a final
`tobari.dev/template-policy/v1` source language but deliberately left its
compiler and semantic provider language unimplemented.

The desired abstraction is not a provider SDK or a replacement for IAM, RBAC,
OpenAPI, GraphQL schemas, or service catalogs. Tobari authorizes only facts in
the validated request projection. Each protocol has different useful fields
and must retain ownership of their interpretation.

## Decision

### One sealed behavioral module contract

Tobari owns one compile-time semantic-module registry. A module owns:

- its stable ID and exact parent in an explicit refinement graph;
- its versioned closed request projection and validation;
- its static matcher validation and canonical identity;
- exact Policy Memory matching;
- conservative state-change evidence; and
- its private-field privacy exclusions.

The common policy engine selects a module and invokes those behaviors. It does
not interpret module fields. The registry is immutable compiled code, not a
plugin, configuration surface, registration API, or public provider adapter.
Gateway parser modules remain explicit and protocol-owned; the common contract
does not create a universal request parser.

The refinement graph is:

```text
protocols.http.generic
  +-- protocols.http.graphql
  +-- protocols.http.mcp
  +-- protocols.http.git
  +-- protocols.http.oci
  +-- providers.aws
  +-- providers.kubernetes
```

Every request begins with a validated HTTP transport projection. Trusted
endpoint declarations and distinctive bounded wire evidence may produce module
candidates. Exactly one unique most-specific candidate must remain. Ambiguity,
malformed classified traffic, or local classifier failure is terminal and
non-learnable. Numeric priority, registration order, confidence, caller tags,
external discovery, and fallback from a classified module to generic HTTP are
forbidden.

The research-only brokered AWS handle is resolved at the earlier credential-
binding trust boundary and supplies one already selected AWS request shape;
wire classifiers do not reinterpret that credential-bearing request. This is
not module priority: ordinary secret-free traffic still evaluates the complete
classifier inventory and rejects sibling claims as ambiguous.

### Request projections, not provider authorization models

All modules retain exact scheme, authority, port, HTTP method, and raw path in
the complete learned effect. Their refinements are:

- generic HTTP: no additional coordinate;
- GraphQL: `query|mutation` plus one canonical root field per learned effect;
- MCP: JSON-RPC method and an exact tool name only for `tools/call`;
- AWS: `query|json`, SigV4 service, exact protocol version or target namespace,
  and exact operation;
- Kubernetes resource: group, version, resource, optional namespace/name/
  subresource, verb, and exact dry-run mode;
- Kubernetes non-resource: exact path and verb;
- Git: upload-pack or receive-pack plus exact repository path;
- OCI: exact action, repository, and object coordinate.

AWS is intentionally final at this layer. Tobari does not load service models
or infer IAM action, ARN/resource, access level, read/write meaning,
idempotence, or retry safety. Kubernetes similarly has no RBAC, OpenAPI,
discovery, CRD schema, object-body, or impersonation semantics.

### Final Template policy source

`policy.yaml` schema `tobari.dev/template-policy/v1` has required `boundary`
and `semantic` containers. `boundary.methods.deny` is the only user-authored
method ceiling; it is a set of exact uppercase methods. The fixed evaluator
retains the public-HTTPS destination ceiling and exact non-public destinations
remain unrepresentable in V1.

`semantic` contains the Template agent profile and native-readiness choice,
then two closed taxonomies:

```text
semantic.protocols.http.generic.<allow|deny>.rules
semantic.protocols.http.graphql.<allow|deny>.rules
semantic.protocols.http.mcp.<allow|deny>.rules
semantic.protocols.http.git.<allow|deny>.rules
semantic.protocols.http.oci.<allow|deny>.rules
semantic.providers.aws.<allow|deny>.rules
semantic.providers.kubernetes.<allow|deny>.rules
```

GraphQL and MCP additionally own exact endpoint declarations needed for trusted
classification. Kubernetes endpoint classification remains derived only from
an already validated EKS bootstrap. Git and OCI retain their distinctive
self-identifying admission contracts. An absent module means known-none. A
present module requires both effects with explicit non-null rule arrays.

GraphQL V1 static rules retain the existing trusted-endpoint POST boundary.
Query-only GraphQL GET may still produce an exact Context Policy Memory effect,
but it cannot be widened into static Template authority without a later
GraphQL-specific decision.

Every rule owns exact scheme and port, `host` XOR `hosts`, and its module's
closed matcher. Generic HTTP additionally owns one method and `path` XOR the
single-full-segment `{id}` template. AWS owns `service` XOR `services`, exact
wire protocol, version/namespace, and a case-sensitive operation matcher that
is either exact or has one terminal `*`. Other initial module matchers are
exact over their typed projection; widening requires a later module-specific
decision rather than a generic wildcard.

Rule collections are semantic sets. Reordering does not change authority,
exact duplicates are invalid, and users author no rule IDs. Exact Allow/Deny
equality, fully Deny-shadowed Allow, and Method-Boundary-shadowed Allow are
compile errors. Exact Deny remains terminal at evaluation.

### Static and dynamic policy share one projection

Template static rules and Context Policy Memory consume the same validated
module request projection. Policy Memory remains exact and Context-owned;
static matchers may intentionally cover more than one exact request only where
their module contract says so. Neither path reparses payloads or implements a
parallel provider dialect. Candidate, rule, audit, OPA data, and CLI output
retain the selected module and its complete typed coordinate.

### Clean source migration

The transitional `tobari.dev/template-policy/v1alpha1` decoder remains
available only to an explicit non-activating source migration. Migration
requires an `in_sync` Template, binds exact source and active revisions, writes
one V1 `policy.yaml` with no semantic widening, and leaves active authority
unchanged until ordinary Template Plan/Apply. Alpha policies with exact
destination ceilings, method Allow overrides, or another shape that V1 cannot
represent are rejected with deterministic manual-edit guidance. Ordinary
read/Plan/Apply accepts only V1 after this cutover; relabeling alpha bytes is
invalid.
The public workflow is `template migration plan --id <template-ref>` followed
by `template migration apply --plan <opaque-ref>`. The plan reference binds the
active revision and exact alpha/V1 source fingerprints. Apply is a source-only
mutation with recoverable whole-directory publication; it never writes an
active Template generation or runs cluster reconciliation.

## Consequences

- All existing protocol-derived identities become first-class static and
  dynamic policy modules without a provider operation catalog.
- Kubernetes policy becomes structurally reviewable instead of stringly typed.
- A classified effect cannot gain authority from a generic HTTP rule.
- The final source cutover advances `policy.yaml` from alpha to V1, advances
  Policy read JSON to schema 3, cluster-denial JSON to schema 5, Gateway denial
  responses to schema 3, Gateway request/audit projection to schema 2, final
  OPA data and aggregate projection to schema 2, and generated Gateway
  configuration to version `v2`.
  The changed executable contract is named `tobari-evaluator-v2`. Evaluator and
  policy-data _identity envelopes_ retain schema 1 because their own field
  topology is unchanged and their content digests bind the exact new bytes.
  Workspace Template, Policy Memory, final authority collection, and generation
  store advance to schema 2 because their persisted policy coordinates changed.
  This is a pre-public clean break: a schema-1 generation is rejected as
  `legacy_state_present` with reset/recreate guidance. It is never decoded,
  inferred, or routed through the narrower `authority.json` migration. The
  explicit source migration is the only supported policy byte rewrite; no
  persisted policy is silently widened.
- Cross-language contract and hostile-parser testing grows, but every module
  has one owner and one review boundary.

## Mechanical enforcement

- Domain tests prove registry closure, graph acyclicity, unique-most-specific
  selection, module-owned validation/matching/canonicalization, and caller-
  immutable inventories.
- Source/compiler tests prove exact V1 topology, strict YAML, set semantics,
  host/service exclusivity, shadowing rejection, and non-activating alpha
  migration.
- Gateway admission tests prove complete module inventory, collision failure,
  privacy exclusions, no parser I/O, and no classified fallback.
- OPA tests prove exact and static module matching, terminal Deny precedence,
  boundary ceilings, and generic-rule non-matching for every classified module.
- Candidate, Policy Memory, audit, CLI, and retained-state tests prove every
  module coordinate round-trips without raw payload or secret data.
- Each module receives an independent review; `task gateway:test`,
  `task policy:test`, `task check`, and `task security` decide completion.

## Security and public-boundary impact

No provider SDK, service catalog, credential, external request, arbitrary
executable, or runtime extension boundary is added. Parser inputs remain
bounded and local. Bodies, arguments, variables, resource objects, pack data,
OCI payloads, authentication, cookies, and unmodeled query/header values remain
outside policy, audit, learned state, and CLI output.
