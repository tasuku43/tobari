# ADR 0057: Make pinned pup native login ready

- Status: Accepted
- Date: 2026-08-17
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, policy, runtime, and harness
- Revises: ADR 0048 and ADR 0055
- Related: ADR 0051 and ADR 0052
- Revised by: ADR 0065
- Superseded by: None

## Context

A live `pup auth login` from a custom Context runtime containing pup 1.10.7
produced one permission candidate for exact `POST
https://api.datadoghq.com:443/api/v2/oauth2/register`. Approving that observed
effect would still leave the fixed token exchange and native browser/callback
transport undeclared, so it would not close the login task.

Inspection of the pinned 1.10.7 source and binary shows a finite default US1
DCR/PKCE flow. It registers at the observed route, authorizes through
`https://app.datadoghq.com/oauth2/v1/authorize`, receives one callback at
`http://127.0.0.1:{8000,8080,8888,9000}/oauth/callback`, and exchanges or
refreshes at `POST https://api.datadoghq.com/oauth2/v1/token`. Successful
exchange persists the credential without a separate identity request.

The canonical base does not install pup. The selected custom runtime already
pins the client artifact, while ADR 0051 lets the trusted binary project
current readiness into existing exact agent-ready Contexts and ADR 0055 gives
native clients a dedicated browser/callback channel.

## Decision

Add compile-time readiness family `pup_ready`, coupled to pup 1.10.7. It grants
exactly:

- `POST https://api.datadoghq.com:443/api/v2/oauth2/register`; and
- `POST https://api.datadoghq.com:443/oauth2/v1/token`.

These are ordinary Context-wide effects. The family is review provenance, not
executable identity or an installation claim. It grants no Datadog product
API, telemetry, revoke, alternate site, custom gateway, route prefix, or other
method, host, or path.

Extend the dedicated Workspace browser union with pup's exact US1 authorization
contract. The trusted host accepts HTTPS only, canonical
`app.datadoghq.com`, exact `/oauth2/v1/authorize`, the seven reviewed query
keys, DCR client-ID/state/PKCE shapes, `S256`, and a sorted subset of the 110
default scopes compiled into pup 1.10.7. The redirect must be exact HTTP
`127.0.0.1`, exact `/oauth/callback`, and one of ports 8000, 8080, 8888, or
9000. Unknown keys, duplicate values, caller-added scopes, neighboring hosts or
paths, and alternate callback targets fail closed.

After full validation, Tobari binds that host-loopback port before opening the
browser and relays one opaque callback to the same port in the label-verified
selected Workspace. Pup retains state and PKCE validation, DCR interpretation,
token persistence, refresh, and result presentation. Tobari retains no browser
URL, dynamic client ID, authorization code, callback bytes, token, or account
identity.

Normal `tobari cluster up` projects the new binary-owned family and browser
asset/spec into existing exact agent-ready Contexts. It does not rewrite a
Context snapshot, recreate a Context, rebuild its custom image, or add pup to
the canonical base.

## Consequences

- A compatible custom runtime can complete default US1 `pup auth login` with
  one native invocation and provider browser consent, without policy review.
- The retained registration candidate is not approved; readiness replaces the
  fixed workflow effect through the baseline projection.
- Explicit added scopes, alternate Datadog sites, and future client drift remain
  denied or reviewable until the client pin and contract are reviewed together.
- Any later Datadog product call remains ordinary Context policy rather than
  authentication readiness.

## Mechanical enforcement

- The single readiness family catalog fixes `pup_ready` to version 1.10.7 and
  the two exact US1 POST effects.
- Domain and aggregate projection tests reject neighboring Datadog authorities,
  routes, methods, product operations, and semantic grants.
- URL tests fix all seven query fields, dynamic-field bounds, all four callback
  ports, the complete 110-scope ceiling, sorted-subset behavior, and hostile
  neighbors.
- A provider-specific relay test binds before browser open and forwards one
  opaque callback only to the selected owned Workspace.
- Runtime activation and repository gates inspect the current binary projection
  without approving the retained candidate or retaining live provider data.

## Compatibility and migration

No public command, flag, policy schema, Gateway protocol, credential format, or
Context image contract changes. The custom runtime must supply exact compatible
pup 1.10.7. Existing exact agent-ready Contexts receive the projection after
`tobari cluster up`; strict, offline, reviewed-exact, get-only-reviewed, and
custom preset origins receive no overlay.

## Security and public-boundary impact

The default agent-ready baseline widens by two exact unauthenticated Datadog
OAuth POST effects and one strictly validated host browser/callback shape. It
does not authorize a Datadog host, product API, arbitrary OAuth client or scope,
generic callback ingress, credential projection, or executable identity.
Fixtures use only synthetic values and contain no live client ID, state, code,
token, user identity, or authenticated transcript.

## Revision by ADR 0065

ADR 0065 replaces the incidental exact-seven-field constraint with seven
mandatory fields plus one reviewed optional `dd_oid` UUID-shaped organization
hint. Unknown fields, duplicates, malformed hints, and every authority, scope,
redirect, callback, or neighboring-target change still fail closed.
