# ADR 0054: Add semantic TWG current-user readiness

- Status: Accepted
- Date: 2026-08-17
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, policy, and harness
- Revises: ADR 0050
- Related: ADR 0044, ADR 0051, and ADR 0052
- Revised by: ADR 0056 and ADR 0058
- Superseded by: None

## Context

TWG CLI 1.2.5 obtained OAuth tokens through the effects already supplied by
`twg_ready`, then failed while resolving the authenticated Atlassian user. The
Gateway denial recorded `POST https://api.atlassian.com:443/graphql` as an
ordinary HTTP candidate because that endpoint was not yet declared for bounded
GraphQL classification.

Inspection of the pinned client established the fixed document shape without
retaining a token, account value, response, or live transcript. The selected
operation is a query named `TwgCLI_WhoAmIRich`; its only top-level field is
`me`, with account fields nested below it. The operation name and nested
selection are not policy identity.

Approving the route-wide HTTP candidate would authorize every body at the same
endpoint. Requiring review for this fixed current-user lookup would leave the
pinned native-login workflow incomplete.

## Decision

The TWG CLI 1.2.5 `twg_ready` bundle additionally declares exact endpoint
`POST https://api.atlassian.com:443/graphql` and grants only GraphQL operation
type `query` with top-level root field `me` there.

Gateway derives that bounded semantic coordinate from the request after
credential redaction. The GraphQL source, operation name, aliases, arguments,
variables, nested fields, response, and credential values remain outside OPA,
audit, candidates, and stored policy. The original request is forwarded
unchanged only after every top-level root is allowed.

The bundle grants no ordinary HTTP request at `/graphql`, mutation, sibling or
mixed root, Atlassian REST route, telemetry request, revoke, update, download,
or neighboring host. Exact Deny remains terminal.

## Consequences

- Pinned TWG login can perform its fixed post-token current-user lookup without
  approving a route-wide GraphQL candidate.
- Any changed operation type, root set, endpoint, or transport remains denied
  or reviewable under ordinary Context policy.
- The trusted binary readiness projection changes. An existing exact
  agent-ready Context receives it after explicit `tobari cluster up`; Context
  recreation is unnecessary.
- The earlier route-wide HTTP candidate is not approved or converted. A retry
  under the new declared endpoint is evaluated semantically.
- A later provider effect, if any, is observed and classified separately
  rather than inferred from login proximity.

## Mechanical enforcement

- Domain and aggregate tests pin the exact endpoint and `query` / `me` rule and
  reject ordinary HTTP, mutation, sibling roots, neighboring Atlassian hosts,
  and telemetry.
- The bounded Gateway parser replays the pinned client's synthetic current-user
  document and proves that only operation type and root field survive parsing.
- Existing all-roots OPA tests reject mixed-root documents unless every root is
  independently allowed and keep ordinary HTTP rules from matching GraphQL.
- Repository fixtures contain no credential, account identifier, authenticated
  response, or live transcript.

## Compatibility and migration

No public command, flag, preset schema, Gateway protocol, or Workspace state
format changes. Rebuild the trusted binary and run `tobari cluster up` to
activate the new aggregate revision. Existing Workspace homes and Context
snapshots are preserved.

## Security and public-boundary impact

The default baseline grows by one exact semantic current-user lookup for the
pinned custom-runtime client. It gains no route-wide GraphQL, business-data,
mutation, telemetry, artifact, or generic Atlassian authority.

## Revision by ADR 0056

ADR 0056 permits TWG's fixed current-user request to omit `Content-Length`
under the generic declared-GraphQL transport caps. The `query` / `me` policy
identity and all authority exclusions in this record are unchanged.

## Revision by ADR 0058

ADR 0058 adds exact TWG site inventory, stable CLI manifest, and OAuth revoke
effects beside this semantic identity lookup. The GraphQL `query` / `me`
boundary remains unchanged.
