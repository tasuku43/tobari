# ADR 0058: Revise native readiness contracts independently

- Status: Accepted
- Date: 2026-08-17
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, policy, runtime, and harness
- Revises: ADR 0050, ADR 0051, and ADR 0054
- Related: ADR 0052 and ADR 0057
- Revised by: None
- Superseded by: None

## Context

After TWG CLI 1.2.5 login and current-user readiness were projected, routine
client lifecycle continued to produce exact candidates for `POST
https://api.atlassian.com:443/accessible-products`, `GET
https://teamwork-graph.atlassian.com:443/cli/manifest.json`, and `POST
https://auth.atlassian.com:443/oauth/revoke`. Binary inspection ties those
effects to site inventory, the stable CLI compatibility manifest, and token
logout respectively.

The readiness catalog already centralizes current exact grants, but it used the
pinned client version as both artifact identity and readiness-history identity.
When the same pinned client exposes a previously unobserved routine effect,
there is no independent revision to retain the old rule set for legacy
snapshot removal. Definitions also lived inside the broader preset
implementation, making the intended review surface less obvious.

Observed candidates cannot be promoted automatically. Workspace traffic is
untrusted evidence and may represent a user-selected product operation rather
than routine client readiness.

## Decision

Keep one binary-embedded declarative readiness catalog in a dedicated domain
source file. Each family declares:

- one stable family ID;
- one pinned client version;
- one positive append-only contract revision;
- exact HTTP and semantic grants; and
- any declared semantic endpoints.

Client version and contract revision are independent. A family selects exactly
one current contract revision, while every prior contract remains append-only
removal metadata. Aggregate generation removes the union of all historical
contract rules from a legacy exact agent-ready snapshot, then adds only the
selected current contracts. The effective aggregate digest still changes with
the exact rules, and an older running projection remains invalid until explicit
`cluster up`.

The catalog uses small constructors for canonical HTTPS/443 HTTP effects and
GraphQL query roots. These reduce repeated transport boilerplate but do not
infer methods, hosts, paths, scopes, effects, or provider identity. Adding an
authority still requires an explicit catalog row, a contract-revision change,
focused negative canaries, durable contract review, and repository gates.
Owner configuration and observed candidates cannot select or extend it.

Set `twg_ready` current contract revision to 2 for the same pinned TWG CLI
1.2.5. Revision 2 is revision 1 plus exactly:

- `POST https://api.atlassian.com:443/accessible-products`;
- `POST https://auth.atlassian.com:443/oauth/revoke`; and
- `GET https://teamwork-graph.atlassian.com:443/cli/manifest.json`.

The manifest grant covers one stable JSON document only. It grants no beta
manifest, installer, checksum, artifact, download, update execution, or path
prefix. Accessible-products grants no neighboring Atlassian REST operation.
Revoke grants no other OAuth route. All effects remain Context-wide rather than
TWG process identity, and exact Deny remains terminal.

## Consequences

- Routine TWG login/site selection, stable compatibility discovery, current-user
  lookup, token refresh, and logout no longer require permission review.
- A newly discovered effect for an unchanged client pin has a durable contract
  revision and safe historical-removal identity.
- Most future maintenance is one focused declarative contract edit plus its
  review evidence, rather than changing projection mechanics.
- Review is intentionally not automatic: product APIs and optional behavior
  remain denied or reviewable until classified.

## Mechanical enforcement

- Catalog tests require unique family IDs, positive unique contract revisions,
  one current revision with an explicit pinned client version, and non-empty
  current grants.
- TWG tests fix revision 1 history and revision 2's complete exact grant set.
- Neighbor canaries reject alternate method, REST path, beta manifest,
  installer, download, and unreviewed Atlassian authority.
- Snapshot/projection tests remove all historical forms, preserve immutable
  snapshot bytes, and add only current binary contracts.
- Aggregate revision, stale-entry recovery, exact-Deny precedence, security,
  and public-boundary gates remain unchanged.

## Compatibility and migration

No public command, flag, preset schema, Gateway protocol, Context manifest, or
credential format changes. Existing agent-ready Contexts receive revision 2
after rebuilding Tobari and running `tobari cluster up`; their snapshot,
custom runtime image, Workspace home, and credentials remain unchanged.

## Security and public-boundary impact

The default agent-ready baseline grows by three exact TWG lifecycle effects.
The declarative catalog is compiled trusted code, not a plugin, owner manifest,
remote registry, traffic learner, or executable allowlist. Repository evidence
contains only endpoint identities and synthetic fixtures; no token, site,
tenant, manifest response, or authenticated transcript is retained.
