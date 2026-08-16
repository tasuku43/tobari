# ADR 0047: Preserve native agent capability and govern semantic actions

- Status: Accepted
- Date: 2026-08-16

## Context

The first pinned Claude Code and Codex runs showed that treating every optional
first-party route as project traffic degrades the native client before the
agent performs a user operation. Claude loses account/capability metadata and
provider evaluation calls. Codex cannot initialize its built-in MCP apps
surface. Reviewing those transport requests as opaque HTTP effects asks the
user to approve implementation details while still failing to express the
actual external action.

Broad host/path allowlists are not acceptable. Provider routes evolve, MCP
shares one HTTP endpoint across bootstrap and tool execution, and request
payloads may contain secrets or untrusted external content.

## Decision

`builtin/agent-ready` preserves the native capability plane of the exact pinned
clients. Reviewed first-party model, account, bootstrap, discovery, telemetry,
and bounded provider-owned evaluation effects receive baseline authority.
Dynamic Claude evaluation matches one normalized direct-child identifier
segment; the observed SDK identifier is never retained.

MCP is a separate semantic policy protocol at an exact host-owned endpoint.
Gateway accepts only one bounded, unencoded `application/json` JSON-RPC 2.0
object. It classifies the exact method and, only for `tools/call`, the exact
tool name. Initialization, ping, and capability enumeration are baseline
methods. `tools/call` requires exact tool-name review; other action methods are
reviewed by exact method. Arguments, resource URIs, responses, and body bytes
never enter OPA, audit, denial output, or stored policy.

Batches, malformed or unknown shapes, oversized bodies, encoded bodies,
undeclared endpoints, and ambiguous GraphQL/MCP endpoint declarations fail
closed. Downloads, plugin acquisition, repository fetches, file transfer, and
self-update remain outside the baseline.

## Consequences

The agent can discover its supported capabilities without a permission inbox,
while actions that substitute for user operations remain reviewable at the
narrowest stable semantic identity. The Gateway must buffer bounded MCP request
bodies before policy, so MCP endpoints require explicit trusted projection and
regression tests. Client-version updates must review both the first-party route
matrix and MCP method matrix as one compatibility decision.
