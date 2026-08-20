# ADR 0065: Allow reviewed optional native-login selectors

- Status: Accepted
- Date: 2026-08-19
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, runtime, and harness
- Revises: ADR 0057
- Related: ADR 0048 and ADR 0055
- Revised by: None
- Superseded by: None

## Context

The pup 1.10.7 browser contract in ADR 0057 required exactly seven query
fields. A live first login succeeded and pup persisted the provider-confirmed
default organization UUID. On repeat `pup auth login`, the same pinned client
recalled that UUID and appended one `dd_oid` query field to the otherwise
unchanged authorization URL. Tobari rejected the request solely because its
query-map size became eight, before binding the host callback listener or
opening the browser.

The strict field count captured one first-login sample rather than the pinned
client's complete state-dependent semantics. Requiring logout or manual
callback transfer would keep the host boundary narrow but would make ordinary
repeat authentication destructive or operationally incomplete.

## Decision

Native-login URL contracts validate semantic fields, not an incidental total
field count. Every provider parser declares a closed query-field schema. Each
field states whether it is mandatory and supplies its singleton-value
validator; optional fields additionally document their security meaning.
Missing mandatory fields, unknown fields, duplicate values, malformed optional
values, and every change to authority, path, client, scope, redirect, callback,
or provider-specific fixed value continue to fail closed before a host effect.

A compatibility update inside an already accepted auxiliary-selector category
does not need a new thesis or ADR. It needs pinned-client evidence, one schema
entry, focused positive and negative tests, and an update to the relevant
authentication contract when the accepted syntax changes. A field that can
alter authority, site, OAuth client class, audience, resource, scope, redirect,
callback, readiness effects, credential handling, or another host side effect
crosses that boundary and requires a new or revised decision before code.

For pup 1.10.7, keep the seven existing fields mandatory and allow one optional
`dd_oid`. When present it must be exactly one 36-byte ASCII UUID-shaped value:
hex groups `8-4-4-4-12`, accepting upper- or lowercase hex. It is a transient
organization-routing hint. Tobari does not log, persist, normalize, compare,
or otherwise interpret it.

The optional hint can originate from pup's remembered default session or its
explicit `--org-uuid` input. This is accepted deliberately. It can affect which
Datadog organization the provider presents, but cannot change the fixed US1
site, OAuth client class, compiled scope ceiling, PKCE contract, redirect,
callback ports, readiness HTTP effects, or selected Workspace relay.

## Consequences

- First and repeat `pup auth login` use the same one-command native browser and
  callback outcome.
- A reviewed state-dependent additive selector no longer requires a brittle
  exact query-count match.
- A same-category optional compatibility field is normally one declarative
  schema entry plus focused evidence, not a new parser branch or repository-wide
  design exercise. This remains a closed contract, not general OAuth query
  passthrough.
- Caller-added scopes, alternate sites, arbitrary organization text, and
  neighboring OAuth parameters remain unsupported.

## Mechanical enforcement

- A package-private closed query schema centralizes known-field membership,
  requiredness, singleton cardinality, and validator dispatch. Pup and AWS use
  it while retaining provider-owned authority, scope, redirect, callback, and
  value semantics in their parsers.
- The pup schema declares seven mandatory fields plus optional `dd_oid` with
  its UUID validator.
- Generic schema tests reject empty or malformed definitions, missing mandatory
  fields, unknown fields, duplicate mandatory or optional values, and malformed
  values.
- Positive tests cover no hint, remembered lowercase UUID, uppercase UUID, all
  four callback ports, and the existing complete/reduced scope ceiling.
- Negative tests cover missing mandatory fields, empty/malformed/duplicate
  `dd_oid`, unknown extra fields, scope widening/order/duplication, alternate
  authorities and paths, and callback changes.
- The existing bridge test retains ownership verification, bind-before-open,
  one opaque callback, and listener cleanup.

## Compatibility and migration

No public command, policy schema, Gateway protocol, credential format, runtime
image contract, or readiness HTTP grant changes. Existing Workspaces receive
the parser behavior when entered through the updated Tobari binary. No pup
state migration or logout is required.

## Security and public-boundary impact

The host browser union grows by one bounded non-secret selector shape inside an
already reviewed exact pup authorization route. It gains no generic browser,
query, port-forwarding, credential, network, or Datadog API authority. Tests
use only synthetic UUIDs and retain no live URL, callback, organization, or
credential data.
