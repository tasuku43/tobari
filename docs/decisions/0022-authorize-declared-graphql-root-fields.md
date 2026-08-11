# ADR 0022: Authorize declared GraphQL root fields

- Status: Accepted
- Date: 2026-08-09
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, harness, and public boundary
- Supersedes: [ADR 0006: Send generic HTTP input to OPA](0006-send-generic-http-input-to-opa.md)
- Superseded by: None
- Revised by: ADR 0027 places GraphQL input, audit, and policy output inside
  exact V1

## Context

ADR 0006 made Context, project, authority, HTTP method, and path the complete
learned permission identity and deliberately excluded bodies. That is useful
for REST-shaped APIs whose paths distinguish resources and operations, but a
GraphQL service commonly sends unrelated query and mutation roots through the
same `POST` endpoint. One learned `POST /graphql` permission therefore grants a
materially broader capability than the HTTP presentation suggests.

Tobari's point is to judge the actual generic L7 effect without knowing the
calling CLI, agent, or provider operation catalog. Moving fixed GraphQL
documents into CLI commands or reconstructing provider outcomes would violate
that boundary. Conversely, guessing GraphQL from `Content-Type`, a path suffix,
or a caller header is bypassable because standard GraphQL POST and ordinary
JSON APIs both use `application/json`.

## Decision drivers

- Preserve CLI-, process-, and provider-independent boundary enforcement.
- Give one GraphQL root capability no more authority than its normalized L7 coordinate.
- Keep GraphQL source text, variables, arguments, and response data out of OPA, audit, learned policy, and CLI output.
- Preserve header-time authorization and streaming for ordinary HTTP requests.
- Fail closed when protocol classification, parsing, operation selection, or policy output is ambiguous.

## Considered options

### Keep method and path as the complete GraphQL identity

This preserves the old transport path but makes one approval cover every query
and mutation root served by that endpoint.

### Infer or register provider and CLI operations

Provider adapters or CLI-owned documents can attach business meaning, but they
make Tobari depend on tool details and an incomplete provider operation model.

### Authorize a trusted GraphQL endpoint by operation type and root field

Trusted policy data can declare exact GraphQL endpoints. Gateway can then use a
reviewed protocol parser to derive stable generic coordinates while treating
arguments and payload values like ordinary HTTP body data.

## Decision

Tobari extends its generic HTTP identity with an optional GraphQL coordinate.
Each Context policy may declare exact `(scheme, host, port, path)` GraphQL
endpoints. Requests to those endpoints never fall back to ordinary HTTP
authorization. Gateway accepts only the bounded GraphQL-over-HTTP POST subset,
selects the one executable query or mutation, recursively collects every
reachable canonical root field, and sends only the operation type and sorted
root names to OPA. Every distinct root field is an independent exact permission
under the existing Context, project, authority, method, and path dimensions;
the complete request is allowed only when all roots are allowed and none is
denied.

`operationName` is consumed only to select the operation and is not policy
identity. Aliases contribute their target field names. Root fragments and
inline fragments are traversed conservatively regardless of directives or
variables. Nested selections, fragment names, directives, arguments,
variables, extensions, literal values, and resolver meaning are not policy
identity and are never retained.

The first slice supports one strict UTF-8 `application/json` POST object with a
positive declared length of at most 1 MiB, one source document, and a selected
`query` or `mutation`. GET, subscription, batching, multipart upload,
persisted-query-only requests, nonempty extensions, compressed or
transfer-encoded bodies, and ambiguous documents fail locally and are not
learnable.

## Consequences

### Positive

- A user may approve `mutation.updateIssue` without approving unrelated mutation roots at the same endpoint.
- Any proxy-aware CLI or process receives the same enforcement without registration in Tobari.
- Ordinary non-GraphQL JSON and streaming requests retain the old fast path.

### Negative

- A Context must declare an exact endpoint before GraphQL-aware learning is available.
- Declared GraphQL requests are bounded and buffered before policy, so they do not support streaming uploads.
- Existing HTTP-only learned rules do not authorize declared GraphQL endpoints; users must review new root-specific denials.
- Gateway gains a pinned parser dependency and its supply-chain review obligations.

### Risks and mitigations

- Parser differentials could authorize the wrong root. Use a mature pinned parser, strict envelopes, conservative root collection, explicit complexity limits, and hostile conformance fixtures.
- A caller could try to avoid GraphQL classification. Classification comes only from trusted exact Context policy and cannot be selected by request data.
- Query text or variables could leak through diagnostics. OPA, audit, denial, policy, and CLI schemas admit only normalized operation type and root field values; canary tests reject the raw material.
- Buffering could exhaust shared memory. Require a positive unambiguous `Content-Length` no greater than 1 MiB before capture and keep the existing transport cap.

## Mechanical enforcement

- Policy-data validation admits only normalized, unique exact GraphQL endpoint declarations.
- Gateway tests fix envelope, length, media-type, parser, operation-selection, root-expansion, zero-upstream, byte-preservation, credential-ordering, and privacy behavior.
- OPA tests require every root, prevent HTTP rules from matching GraphQL, and keep exact deny precedence.
- Domain IDs, matching, candidate aggregation, learned allow/deny, reset, and CLI schemas bind one optional GraphQL coordinate.
- Compaction rejects GraphQL rules so path-prefix replacement cannot discard the root dimension.
- Dependency checks pin GraphQL-core by version and package hash, retain its MIT notice, and scan the changed Gateway image.

## Compatibility and migration

Gateway image API advances from 2 to 3. OPA input schema 5 and Context policy-data schema 2 gain strict additive
GraphQL members; the decision shape stays unchanged. GraphQL audit advances to
schema 3 and public policy outputs advance explicitly. Legacy HTTP rules keep
their existing IDs and meaning for ordinary HTTP. They are deliberately
ineligible for a declared GraphQL request. Legacy Context policy without a
GraphQL endpoint declaration remains ordinary HTTP policy until the trusted
owner opts into the new endpoint mode. Removing a declaration safely restores
the ordinary HTTP contract but does not reinterpret or compact retained
GraphQL rules.

## Security and public-boundary impact

GraphQL documents and variables become bounded transient Gateway inputs but do
not enter OPA, logs, audit, policy files, CLI output, credentials, or fixtures.
No new destination, credential, process, provider schema, or executable adapter
is added. GraphQL-core is an MIT-licensed Python dependency pinned with
integrity evidence in the Gateway image; source, license, vulnerability, and
multi-architecture image review are required.

## Validation

```sh
task gateway:test
task policy:test
task check
task security
task public:check
```

The Docker integration scenario also exercises a local synthetic GraphQL
endpoint when a Docker Engine is available.

## Reconsideration signals

Reconsider when supported tools require GraphQL GET, subscriptions, multipart
uploads, persisted documents, or target-aware argument policy; when the 1 MiB
bound rejects routine documents; or when parser/spec drift makes the accepted
subset incompatible with common clients.
