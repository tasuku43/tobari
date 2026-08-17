# ADR 0056: Accept bounded lengthless GraphQL requests

- Status: Accepted
- Date: 2026-08-17
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, Gateway, and harness
- Revises: ADR 0022
- Related: ADR 0054
- Revised by: None
- Superseded by: None

## Context

ADR 0022 required a positive `Content-Length` no greater than 1 MiB before
Gateway buffered a declared GraphQL request. A live TWG CLI 1.2.5 login
completed OAuth token exchange, then Gateway rejected its fixed current-user
request locally with status 400 and reason `GraphQL request requires one
positive Content-Length`. The client sent a valid unencoded JSON request
without that optional HTTP header, so semantic classification never ran.

Approving another policy effect cannot repair a pre-policy transport-envelope
rejection. Treating the endpoint as ordinary HTTP would discard the GraphQL
operation/root boundary established by ADR 0022.

## Decision

A declared GraphQL POST accepts either zero or one `Content-Length` header. If
present, it remains one positive decimal value no greater than 1 MiB and must
equal the complete body length. If absent, `Transfer-Encoding` and
`Content-Encoding` must also be absent.

Gateway disables request streaming for the declared endpoint. Mitmproxy's
fixed 8 MiB `body_size_limit` bounds the in-progress unknown-length buffer and
terminates an oversized request before the addon request hook, OPA, DNS, or
upstream I/O. At the request hook, Gateway independently requires the complete
body to be no greater than 1 MiB before JSON or GraphQL parsing. All existing
media-type, UTF-8, envelope, parser-complexity, operation-selection, all-roots,
credential-ordering, privacy, and byte-preservation rules remain unchanged.

This is a generic declared-GraphQL transport contract. It does not recognize
TWG, Atlassian, a process, or an operation name at Gateway.

## Consequences

- HTTP clients may omit an optional length while retaining exact semantic
  policy identity.
- A lengthless declared GraphQL request may occupy at most the shared 8 MiB
  transport cap while arriving, but any body over 1 MiB is rejected before
  parsing, policy, or upstream I/O.
- Transfer-encoded, compressed, duplicate/invalid-length, mismatched-length,
  and otherwise unsupported requests still fail closed and are not learnable.
- Ordinary HTTP, MCP, AWS, policy schema, and readiness grants do not change.

## Mechanical enforcement

- Parser tests accept absent length, retain exact present-length matching, and
  reject transfer encoding plus declared and actual over-limit bodies.
- Gateway lifecycle tests prove a lengthless request is buffered, contributes
  only operation type and canonical roots to policy, redacts authentication,
  and commits upstream only after allow.
- Runtime snapshot equality and the entrypoint asset test retain the fixed
  8 MiB mitmproxy transport cap.
- The pinned mitmproxy implementation's incremental body-size check is part of
  the reviewed Gateway artifact contract.

## Compatibility and migration

No public command, policy schema, audit schema, or Workspace state changes.
The Gateway image changes and requires `tobari cluster up`; Context and
Workspace recreation is unnecessary.

## Security and public-boundary impact

The accepted GraphQL transport subset grows only by omission of an optional
length header under two finite caps. No body, header value, token, operation
name, variable, nested field, or response enters OPA, audit, candidates,
stored policy, or CLI output.
