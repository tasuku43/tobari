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
field count. Every provider parser keeps an explicit set of mandatory fields
and may admit only individually reviewed optional fields with their own name,
cardinality, value shape, and security meaning. Missing mandatory fields,
unknown fields, duplicate values, malformed optional values, and every change
to authority, path, client, scope, redirect, callback, or provider-specific
fixed value continue to fail closed before a host effect.

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
- Adding another optional field still requires a contract and test change; this
  is not a general OAuth query passthrough.
- Caller-added scopes, alternate sites, arbitrary organization text, and
  neighboring OAuth parameters remain unsupported.

## Mechanical enforcement

- The pup parser checks every query key against the seven mandatory names plus
  `dd_oid`, then validates mandatory cardinality/value semantics and optional
  UUID cardinality/shape independently.
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
