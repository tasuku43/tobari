# ADR 0006: Send generic HTTP input to OPA

- Status: Accepted
- Date: 2026-08-04
- Deciders: Tobari maintainers
- Scope: Product and architecture
- Supersedes: None
- Superseded by: None

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

Gateway sends OPA schema `2` containing a host-issued principal, a structured
request authority, method, path, query, redacted headers, bounded body
metadata, and an authorization object containing the requested credential
profile. OPA returns a small allow/deny document with required `allow`,
`reason`, `credential_profile`, `status_code`, and `learnable` fields. Gateway
owns normalization, body/authentication safety, transport checks, and audit;
OPA owns policy matching and the final authorization decision.

## Consequences

- Rego expresses service rules directly in HTTP vocabulary.
- Policy cannot claim provider-level semantics that are not in the request.
- Schema evolution requires an explicit version.

## Mechanical enforcement

Gateway unit tests snapshot normalized and redacted input. Rego tests cover
host, scheme, method, path, and credential binding. Invalid decisions fail
closed.

## Compatibility, security, and validation

OPA input schema `2` is a compatibility boundary. Secret headers and raw bodies
are excluded, and the old flat input shape is not accepted. Reconsider only
when repeated supported outcomes cannot be expressed or safely interpreted from
generic HTTP evidence.
