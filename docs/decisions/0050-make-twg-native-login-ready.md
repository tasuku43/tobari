# ADR 0050: Make pinned TWG native login ready

- Status: Accepted
- Date: 2026-08-17
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, policy, runtime, and harness
- Revises: ADR 0039, ADR 0046, and ADR 0048
- Related: ADR 0044
- Revised by: ADR 0051, ADR 0053, ADR 0054, ADR 0055, and ADR 0058
- Superseded by: None

## Context

TWG CLI can be supplied by a custom Context runtime, but its native OAuth
device login still crosses the same two boundaries as the pinned canonical
clients: exact Workspace HTTP effects and purpose-limited host browser
navigation. A live TWG CLI 1.2.5 attempt was denied at exact `POST
https://auth.atlassian.com:443/oauth/device/code`. A bounded replay with
isolated temporary state then emitted the exact framing `Visit:
https://auth.atlassian.com/oauth/activate?user_code=<provider-value>` before
polling the fixed token route. No provider value or transcript was retained.

Granting an Atlassian host, route prefix, GraphQL endpoint, or generic browser
URL would exceed native-login readiness. Requiring users to review these fixed
transport details would leave TWG outside the existing agent-ready promise.

## Decision

The built-in agent-ready source adds compile-time bundle `twg_ready`, coupled
to TWG CLI 1.2.5. It expands into exactly:

- `POST https://auth.atlassian.com:443/oauth/device/code`; and
- `POST https://auth.atlassian.com:443/oauth/token`.

The bundle name and executable name never enter persisted policy. The effects
are Context-wide, exact Deny remains terminal, and custom presets cannot select
the bundle. Atlassian GraphQL, REST, telemetry, update, download, revoke, and
neighboring OAuth effects receive no baseline authority.

The attached-session observer accepts only one complete bounded line with exact
`Visit: ` framing followed by HTTPS host `auth.atlassian.com`, path
`/oauth/activate`, and exactly one non-empty bounded unreserved `user_code`
query value. It rejects alternate case, userinfo, explicit ports, raw paths,
other or duplicate query keys, encoding, fragments, extra prose, ambiguous
URLs, replay, and oversized lines. After verifying the selected owned
Workspace, Tobari opens the URL once through the strict host browser adapter.
It creates no callback listener and never logs, persists, or interprets the
provider value.

`twg_ready` is a network-and-browser compatibility contract when TWG CLI 1.2.5
is present. It does not add TWG to Tobari's canonical base runtime or promise
artifact installation. The custom runtime remains responsible for the pinned
executable and its integrity.

## Consequences

- A new agent-ready Context whose custom runtime supplies TWG CLI 1.2.5 can
  enter native device login without reviewing fixed OAuth transport details.
- Every process in that Context can call the two exact authentication effects.
- Prompt, URL, or client-version drift fails closed and leaves TWG's visible
  fallback available.
- The built-in preset revision changes. Existing immutable Context snapshots
  do not receive `twg_ready` automatically.

## Mechanical enforcement

- Domain tests pin the bundle ID, client version, two exact rules, and absence
  of neighboring Atlassian baseline authority.
- Observer and browser tests preserve child bytes, enforce exact framing and
  URL semantics, deduplicate replay, bind ownership, and prove zero callback
  listener calls.
- Agent-readiness fixtures keep GraphQL, REST, revoke, update, download,
  telemetry, alternate hosts, paths, and query shapes outside the baseline.
- Repository fixtures contain only synthetic user codes and no token, account
  identifier, authenticated response, or live transcript.

## Compatibility and migration

No public command, flag, preset schema, Gateway protocol, or Workspace state
format changes. Existing Contexts retain their stored preset revision. Users
may apply a one-off exact policy review in the current Context, or create a new
Context and rebuild/select the custom runtime to receive `twg_ready`.

## Security and public-boundary impact

The default baseline grows by two exact unauthenticated authentication POSTs,
and the host opener gains one strict provider verification URL shape. It gains
no Atlassian business-operation, refresh-neighbor, revoke, callback, arbitrary
browser, process-identity, credential, or artifact-acquisition authority.

## Revision by ADR 0051

ADR 0051 replaces this record's Context-recreation migration behavior.
`twg_ready` is now projected from the installed trusted binary for existing and
future exact agent-ready Contexts; the immutable snapshot is neither rewritten
nor used to retain an older readiness set.

## Revision by ADR 0053

ADR 0053 stages the strict TWG activation URL in bounded session memory and
opens it only after TWG emits its exact post-confirmation fallback line. The
earlier pre-confirmation open and never-retain-the-URL behavior no longer
applies; the staged URL is cleared after confirmation, ambiguity, or session
end and is never logged or persisted.

## Revision by ADR 0054

ADR 0054 adds the fixed post-token current-user lookup required by TWG CLI
1.2.5: GraphQL `query` root `me` at exact `POST
https://api.atlassian.com:443/graphql`. The earlier exclusion of all Atlassian
GraphQL authority is replaced by this single semantic rule; ordinary HTTP,
mutation, sibling or mixed roots, REST, telemetry, and neighboring effects
remain outside the baseline.

## Revision by ADR 0058

ADR 0058 adds TWG CLI 1.2.5's exact site discovery, stable CLI manifest, and
OAuth revoke effects. It also gives readiness contracts an append-only revision
independent from the pinned client version.
