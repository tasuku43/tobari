# ADR 0041: Project minimal Claude native entitlements

- Status: Accepted
- Date: 2026-08-16
- Deciders: Tobari maintainers
- Scope: Authentication, Workspace projection, Auth Broker protocol, Gateway profile, and harness
- Revises: ADR 0038
- Superseded by: None

## Context

ADR 0038 projected a project-bound Broker handle into Claude Code's
`accessToken`, an empty `refreshToken`, a fixed future expiry, and the granted
OAuth scopes. Inference and `claude auth status` worked, but the exact pinned
Claude Code 2.1.220 interactive UI reported `Login: Expired`, described the
account as `Claude API`, and selected the API-account default model instead of
the native account default.

A network-isolated pseudo-TTY matrix varied only synthetic credential fields
against the exact pinned executable and reused non-secret cached profile data.
Access-token spelling did not affect the result. Any non-empty fixed refresh
value removed the false expired state. Adding the native login's
`subscriptionType` and `rateLimitTier` restored the native account label and
account-selected default model. The experiment contained no provider token and
its raw account-local artifacts remain disposable outside the repository.

Giving the Workspace either provider access or refresh token would violate the
Broker boundary. Hard-coding known subscription or tier values would also turn
provider data into a brittle Tobari catalog.

## Decision

The isolated acquisition boundary retains two additional non-secret values
from exact Claude Code 2.1.220: `subscriptionType` and `rateLimitTier`. Tobari
requires each value to be non-empty, trimmed, control-free UTF-8 of at most 128
bytes, but compiles no value list. The strict canonical Anthropic session names
them `subscription_type` and `rate_limit_tier`. All other optional native-file
metadata remains discarded.

Auth Broker validates and encrypts those labels with the renewable session,
preserves them unchanged during token refresh, and returns them only as bounded
non-secret handle-issuance projection values. They do not participate in
credential selection, policy, binding, refresh authority, account identity, or
Gateway resolution.

The reviewed Workspace `.claude/.credentials.json` contains:

- the project-bound `tobari-h1_` handle as `accessToken`;
- fixed public `dummy-value` as `refreshToken`;
- the fixed future local expiry;
- the captured and canonicalized OAuth scope set;
- the captured subscription and rate-limit labels.

`dummy-value` is deliberately not a provider token and does not match the
Broker-handle grammar. Gateway can neither introspect nor resolve it. The real
renewable token stays encrypted inside Auth Broker. The fixed future expiry and
post-policy Broker refresh keep the pinned client from ordinarily attempting a
refresh with the sentinel; if it does, the value still conveys no authority.

There is no compatibility reader or migration. Existing pre-public Anthropic
records must be replaced by a fresh native login before their Workspace
projection can be issued under this exact schema.

## Consequences

### Positive

- Interactive Claude matches native account login semantics rather than
  reporting a false expired/API-account state.
- Account-selected model behavior remains provider-owned; Tobari does not set a
  model or compile subscription/tier names.
- Workspaces still receive no provider access token, refresh token, account ID,
  or renewable session.

### Negative

- The projection is coupled to two more private-file fields of exact Claude
  Code 2.1.220.
- Re-login is required when this strict pre-public record schema changes.
- A future Claude version may require a new reviewed native-state contract and
  another isolated client experiment.

## Mechanical enforcement

- Go capture tests require, bound, canonicalize, redact, and reject unsafe or
  missing entitlement labels while proving unrelated native metadata is absent.
- Python Broker tests require the exact canonical fields, preserve them across
  refresh, and return them in strict handle issuance without exposing secrets.
- Go control-protocol and exact-byte projection tests reject missing, extra, or
  unsafe metadata and fix the public sentinel and JSON escaping.
- Provider and Gateway profile tests keep the built-in projection closed and
  owner manifests unable to select these placeholders.
- A disposable real-client PTY matrix against Claude Code 2.1.220 verifies the
  false-expired cause and native account/default-model outcome without storing
  account data or credentials in repository evidence.

## Security and public-boundary impact

This decision projects two bounded non-secret provider labels and one fixed
public sentinel. It does not widen policy, target bindings, Gateway handle
recognition, refresh authority, or the public owner-manifest extension surface.
No live account identity, provider token, handle, or credential file enters the
repository.
