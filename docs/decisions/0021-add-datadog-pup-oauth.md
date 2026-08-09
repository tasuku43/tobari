# ADR 0021: Add a closed Datadog pup OAuth credential plan

- Status: Accepted
- Date: 2026-08-09
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, external I/O, harness, public boundary, and release
- Extends: [ADR 0020: Add a host credential companion for renewable brokered credentials](0020-broker-reviewed-credential-plans.md)

## Context

The Datadog built-in projected a protected-stdin `DD_ACCESS_TOKEN`, although
pup 1.6.6 and the toolbox-pinned 1.10.5 both support OAuth Authorization Code
with PKCE S256, dynamic client registration, a loopback callback, renewable
tokens, and file-backed state. Consequently `tobari auth login datadog` failed
as unsupported and imported tokens could not refresh automatically.

pup does not expose its access token from release builds: `pup auth token` is
debug-only. Installing pup in Auth Broker would couple that image to a work
tool, while copying its normal keychain or home would mix ambient user state
with Context/project authority.

## Decision

Datadog becomes a schema-2 built-in `datadog_oauth_session` plan with helper
`pup-oauth`. `auth login datadog` resolves and hashes a trusted-host pup
executable, creates an owner-only temporary home, forces
`DD_TOKEN_STORAGE=file`, and runs exactly:

```text
pup --no-agent auth login --site datadoghq.com
```

The fixed plan uses pup's standard supported scope set. The user reviews and
accepts that set in Datadog's browser consent page. Arbitrary scopes, sites,
custom gateways, and named organizations remain outside this first plan.
pup owns PKCE, DCR, browser opening, the four-port loopback allowlist, code
exchange, and its five-minute login wait. Tobari accepts only the default US1
session, strict owner-only `client_*.json`, `tokens_*.json`, and `sessions.json`
shapes, then re-encodes the client/token tuple with the executable path/digest
as canonical opaque state. The temporary home is deleted on every outcome.

Workspace projection stays `DD_ACCESS_TOKEN=<project handle>` plus fixed
`DD_SITE=datadoghq.com`; no primary or refresh token enters Workspace. Gateway
removes the recognized handle, performs non-secret introspection, and asks OPA
about the exact ordinary HTTP effect before resolve.

After allow, Auth Broker parses the closed state and returns an access token
only when more than five minutes remain. Otherwise it takes the per-record
single-flight lock, persists an encrypted no-replay barrier, and performs one
30-second proxy-free, no-redirect HTTPS form POST to the exact fixed endpoint
`https://api.datadoghq.com/oauth2/v1/token`. DCR has no client secret; the
encrypted state supplies the bounded client ID and refresh token. Broker
strictly validates the response, atomically persists replacement canonical
state while preserving the grant revision, and only then returns the bearer.
An uncertain response leaves the durable barrier and requires explicit
Datadog re-login or logout. This is a closed compiled refresh implementation,
not manifest-selected network behavior or a generic OAuth client.

Existing valid static Datadog vault records remain readable and resolvable
with their persisted legacy binding. New `auth import datadog` is unsupported
because the installed built-in now declares helper acquisition. Normal
re-login rotates the record and handles.

## Consequences

- The Auth Broker image still contains no pup executable or user pup home.
- Login requires host pup; refresh does not require the companion or a resident
  pup process and remains automatic after OPA allow.
- Datadog is limited to the existing US1 authority and default organization.
- The Broker gains one exact outbound OAuth token endpoint and strict state
  parser. Ambient proxies, redirects, provider response text, and unbounded
  payloads are rejected.
- General OAuth, provider-authored endpoints, multiple accounts/sites, and
  scope selection remain unsupported.

## Mechanical enforcement

- Domain/Gateway tests accept only the exact schema-2 Datadog plan and reject
  dynamic credentials in owner manifests or other provider bindings.
- Host-driver tests fix executable identity, argv, environment, file modes and
  shapes, state canonicalization/redaction, and cleanup.
- Broker tests prove legacy compatibility, deny-before-resolve, no refresh for
  a valid lease, exact refresh request, same-revision state replacement,
  durable outcome-unknown behavior, and secret-free errors.
- Auth Broker image checks prove pup is absent; source/snapshot equality covers
  the fixed refresh module.
- Live OAuth consent and a post-expiry pup read are manual release checks and
  never supply repository fixtures.

## Validation

- `task check`
- `task security`
- `task authbroker:test`
- `task gateway:test`
- Manual `tobari auth login datadog`, Workspace re-entry, handle/site shape
  checks, a policy denial with zero token resolution, an allowed pup read, automatic
  refresh, logout, and stale-handle rejection.
