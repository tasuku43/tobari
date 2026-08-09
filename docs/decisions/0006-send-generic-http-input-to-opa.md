# ADR 0006: Send generic HTTP input to OPA

- Status: Accepted
- Date: 2026-08-08
- Deciders: Tobari maintainers
- Scope: Product and architecture
- Supersedes: None
- Superseded by: [ADR 0022: Authorize declared GraphQL root fields](0022-authorize-declared-graphql-root-fields.md)

## Context

The enforcement point observes HTTP, while provider-specific operation names
would require incomplete and continuously changing semantic reconstruction.

## Decision drivers

- Authorize the effect that actually crosses a Tobari boundary
- Preserve provider independence
- Give policy authors request dimensions that are testable with Rego

## Considered options

- A versioned generic HTTP document
- Provider operation adapters in Gateway
- Host-and-port allowlists without method or path visibility

## Decision

Gateway sends body-free OPA schema `4` from the request-header hook to one fixed
decision endpoint. It contains a host-issued principal with stable Context and
project IDs, a structured request authority, method, path, query,
redacted headers, and an authorization object containing the requested
credential profile. OPA returns a small allow/deny document with required
`allow`, `reason`, `credential_profile`, `status_code`, and `learnable` fields. Gateway
owns normalization, authentication safety, authorization-before-stream
ordering, transport checks, and audit; OPA owns policy matching and the final
authorization decision.

## Consequences

- Rego expresses service rules directly in HTTP vocabulary.
- Policy cannot claim provider-level semantics that are not in the request.
- Body presence and content do not split exact policy candidates or learned
  rules; one exact route grant authorizes any body at that route.
- Schema evolution requires an explicit version.

## Mechanical enforcement

Gateway unit tests snapshot normalized and redacted body-free input and prove
streaming starts only after allow. Rego tests cover host, scheme, method, path,
credential binding, and body-independent decisions. Invalid decisions fail
closed.

## Compatibility, security, and validation

OPA input schema `4` is a compatibility boundary. Stable Context/project IDs
come only from the Gateway local-address registry, and unknown Contexts deny
inside the Tobari-owned router. Secret headers and all body
data are excluded, and the old input shapes are not accepted. Reconsider only
when repeated supported outcomes cannot be expressed or safely interpreted from
generic HTTP evidence.
