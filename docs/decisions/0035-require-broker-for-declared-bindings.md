# ADR 0035: Require the Auth Broker for declared provider bindings

- Status: Accepted
- Date: 2026-08-15
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, Gateway, harness, and compatibility
- Supersedes: [ADR 0009: Select tool-native passthrough by default](0009-defer-gateway-managed-credential-injection.md)
- Revises: [ADR 0019: Add a shared locked Auth Broker for Context credentials](0019-shared-locked-auth-broker.md), [ADR 0031: Restore reviewed broker provider plans](0031-restore-reviewed-broker-provider-plans.md)
- Superseded by: ADR 0044 for the standard profile; retained only by the experimental development profile

## Context

Gateway currently selects Workspace-owned passthrough whenever it sees no
Tobari handle marker. That lets a Workspace apply a real bearer token or an
already signed AWS credential at the same target and request shape that Tobari
declares as Broker-supported. The Broker is therefore optional at runtime even
after a provider binding has been reviewed and projected.

Denying known OAuth or token endpoints does not close this gap. A Workspace can
receive an already acquired credential through a file, environment variable,
prompt, or application output. Browser, device, callback, and token endpoints
also vary by reviewed client contract, and not every complete acquisition flow
is a stable Gateway-owned route set.

## Decision drivers

- Make the brokered credential boundary enforceable rather than caller-selected.
- Keep real credentials out of Workspaces for bindings Tobari claims to support.
- Preserve generic HTTP authorization and avoid command/process-name inference.
- Retain a bounded compatibility path for providers Tobari cannot yet describe.
- Keep owner manifests non-executable and unable to define policy or arbitrary
  acquisition routes.

## Decision

The normalized host-owned provider projection owns runtime credential routing.
Every exact projected HTTPS source-header/source-format binding is
Broker-required. The reviewed AWS signing target is likewise Broker-required.

For a declared binding Gateway accepts only the projected Tobari handle shape:

- a valid handle is removed, introspected, authorized by OPA, and resolved,
  refreshed, or used for request-local signing through the existing closed plan;
- a malformed, stale, copied, revoked, or mismatched handle returns
  `credential_handle_invalid`;
- a real Workspace-owned credential returns non-learnable HTTP 403
  `broker_auth_required` before OPA, Broker resolution, DNS, or upstream I/O.

The last case removes the matching secret-sensitive headers before failure.
Advanced Rego and learned policy cannot override it because credential routing
must be valid before there is an ordinary effect to authorize.

Passthrough remains only for requests that match no declared provider binding
and contain no Tobari-looking marker. It is named **Workspace-owned
compatibility**, not the universal default or an equivalent security mode. Its
real credential may be read by every process in that Workspace and is forwarded
only after OPA allow. Adding a provider binding intentionally moves that exact
request shape from compatibility to Broker-required behavior.

Tobari does not block `login` command names or process identities. Closed
acquisition-endpoint denial may be added as defense in depth only when a
reviewed built-in client contract supplies a complete stable route set. It is
not the Broker-enforcement mechanism, and owner manifests cannot declare such
routes.

## Consequences

### Positive

- A real credential cannot bypass the Broker at a binding Tobari claims.
- Acquisition and runtime use remain separate trust boundaries: reviewed host
  drivers acquire state, while Gateway enforces handle-only request use.
- Unsupported providers remain usable without claiming the stronger boundary.
- Policy continues to authorize generic HTTP effects rather than provider or
  command semantics.

### Negative

- Existing Workspace-owned login state stops working at newly or already
  declared bindings and must be replaced by Context Broker configuration plus
  Workspace re-entry.
- Tobari still cannot claim that no secret exists anywhere in a Workspace; the
  compatibility path and arbitrary readable project data remain.
- A projected handle is not a primary secret, but it is still a scoped bearer
  capability and should not be published, logged, or copied between Workspaces.

## Mechanical enforcement

- Gateway tests submit synthetic direct bearer/raw credentials at declared
  bindings and a direct SigV4 request at the reviewed AWS target, then assert
  `broker_auth_required`, removed secret headers, and zero fallback, Broker,
  OPA, DNS, or upstream calls.
- Existing tests continue to prove valid-handle introspection, policy-before-
  action, one same-revision static/renewable/signing result, and invalid-handle
  no-fallback behavior.
- A negative compatibility test proves a real credential at an undeclared
  target still selects passthrough and reaches OPA before possible forwarding.
- Canonical and embedded Gateway sources remain byte-identical.
- Harness claims and agent-readiness scenarios distinguish Broker-required
  declared bindings from Workspace-owned compatibility.

## Compatibility, migration, and recovery

This is a pre-public tightening with no persisted-state or argv migration.
`auth status` and `context show` add fixed declared/undeclared routing fields.
Configure the provider with `auth login` or `auth import`, inspect `auth status`,
and re-enter each affected Workspace so the current project-bound handle is
projected. A request must not be retried unchanged after
`broker_auth_required`.

## Security and public-boundary impact

No new dependency, executable, destination, provider API, or secret-bearing
fixture is added. The guarantee is intentionally scoped: declared bindings do
not accept Workspace-owned credentials. Tobari does not claim to detect every
credential byte or every external authentication endpoint.
