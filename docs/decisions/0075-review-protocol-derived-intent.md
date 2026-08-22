# ADR 0075: Review protocol-derived network intent

- Status: Accepted
- Date: 2026-08-22
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, harness, and public boundary
- Supersedes: None
- Superseded by: None
- Revises: ADR 0022 and ADR 0056

## Context

HTTP authority, method, and path are useful exact coordinates, but several
standard protocols multiplex unrelated operations through coordinates that
look identical at ordinary HTTP level. GraphQL and signed AWS RPC already
demonstrate the two safe cases: the wire request either carries a standardized
operation coordinate, or Tobari retains `unknown` rather than loading a
provider business catalog.

The next requested outcomes are GraphQL transport compatibility, Kubernetes
API resource verbs, Git Smart HTTP service identity, and OCI Distribution
repository actions. All four can be derived from standardized request
structure without GraphQL schemas, Kubernetes OpenAPI discovery, provider
operation catalogs, or exploratory calls.

## Decision

Tobari names this boundary **protocol-derived intent**. One bounded classifier
may refine the existing Context/project/authority/method/path identity only
with fields explicitly carried by a reviewed wire protocol. The complete
refined coordinate remains the exact learned Allow or Deny identity and never
falls back to an ordinary HTTP rule after classification.

Review also exposes one conservative state-change value:

- `not_expected`: the protocol contract excludes application-state mutation;
- `possible`: the protocol operation may change application state;
- `interactive`: the operation grants an interactive or tunneling capability;
- `unknown`: the protocol does not prove one of the preceding classifications.

This value is derived from a validated coordinate. It is not stored as an
independent selector, cannot combine unrelated operations, and cannot turn
`unknown` into safe authority. Data disclosure sensitivity remains a separate
unclaimed dimension.

The accepted slices are:

1. declared GraphQL POST plus bounded body-free query-only GET, retaining only
   operation type and canonical root fields;
2. a trusted Kubernetes API authority, retaining resource verb, group,
   version, namespace, resource, optional name/subresource, exact dry-run mode,
   while impersonation headers fail closed in the initial contract;
3. Git Smart HTTP, retaining repository path and upload-pack or receive-pack
   service from exact protocol paths/query and media types;
4. distinctive OCI Distribution object routes, retaining repository, action,
   and object coordinate from standardized `/v2/` catalog, tag, manifest,
   blob, referrer, and upload shapes.

Trusted declarations for GraphQL and Kubernetes come only from Context-owned
policy or an already validated typed EKS bootstrap. Request data cannot declare
an arbitrary endpoint to be GraphQL or Kubernetes. Git and OCI object requests
are self-identifying only when their complete distinctive path/query/media-type
contract is present. OCI's base `/v2/` probe, token routes, and unsupported
`/v2/*` paths remain ordinary HTTP to avoid collisions with unrelated versioned
APIs. Ambiguous or malformed classified traffic fails locally and is not
learnable.

## Consequences

- Review shows materially narrower intent without provider schemas.
- CRDs, arbitrary GraphQL root fields, arbitrary Git repositories, and
  arbitrary OCI repositories do not require embedded catalogs.
- Query-like operations may still disclose sensitive data; `not_expected` is
  not labeled safe or low-risk.
- Protocol parsers and their limits become security-critical Gateway code.
- Existing ordinary HTTP rules cannot authorize a newly classified effect.
- A standard-looking prefix alone is insufficient classification evidence;
  exact distinctive routes must survive collision canaries.

## Mechanical enforcement

- Domain validation owns every closed protocol field, exact matching, opaque
  ID material, and derived state-change value.
- `.harness/protocol_classifier_admission.json` is the exact classifier-source
  inventory. A dependency-free gate discovers every top-level
  `gateway/addon/*_request.py` module and rejects an unregistered or stale row.
- Every row must resolve executable evidence for positive classification,
  collision resistance, malformed local failure, minimal policy projection,
  privacy exclusion, no ordinary-HTTP fallback, zero downstream, and a finite
  deterministic adversarial corpus of at least three cases.
- The same gate rejects parser imports or direct calls that add network, file,
  dynamic-code, or executable I/O. Optional randomized fuzzing may supplement
  the deterministic corpus but is not completion evidence.
- OPA tests prove protocol rules match exactly and broad HTTP rules do not.
- CLI contracts expose exact coordinates and state-change evidence without raw
  payloads, query documents, credentials, or impersonated identities.
- Runtime snapshot equality and `task check` prevent source drift.

## Security and public-boundary impact

No provider schema, credential, live API discovery, response interpretation,
or external executable is added. Parsers operate locally with finite input and
field bounds. Git pack data, OCI bodies, Kubernetes object bodies, GraphQL
source/variables, authentication, cookies, and impersonation values remain
outside OPA, learned policy, audit, and CLI output.

## Validation

```sh
python3 scripts/protocol_classifier_admission.py
task gateway:test
task policy:test
task check
task security
```
