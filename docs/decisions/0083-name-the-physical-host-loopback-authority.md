# ADR 0083: Name the physical-host loopback authority

- Status: Accepted
- Date: 2026-08-24
- Deciders: Tobari product owner and maintainers
- Scope: Product, DNS, Gateway TLS classification, attachment policy,
  migration, public boundary, and harness
- Revises: ADR 0049
- Related: ADR 0074, ADR 0079, ADR 0081, and ADR 0082
- Superseded by: None

## Context

ADR 0049 introduced one attachment-scoped HTTP capability for a Workspace to
reach the physical host's IPv4 loopback. Its original authority was
`host.tobari.test`. The mechanics are safe and useful, but `.test` describes
DNS testing while this is a routine reviewed development capability.

The name must not make a resolver return the Workspace's own loopback, depend
on a globally delegable suffix, expose Gateway transport as destination
identity, or turn a private suffix into permission. The cutover is pre-public,
but old transient route and grant material can still exist in development
state and contains authority-sensitive relay data.

Official sources checked for this decision establish distinct properties:

- [ICANN Board resolution 2024.07.29.06](https://www.icann.org/en/board-activities-and-meetings/materials/approved-resolutions-special-meeting-of-the-icann-board-29-07-2024-en)
  permanently withholds `.INTERNAL` from delegation in the DNS root for
  private-use applications.
- The [IANA Special-Use Domain Names registry](https://www.iana.org/assignments/special-use-domain-names)
  was last updated 2026-05-22 and does not list `internal.`. `.internal`
  therefore has no `localhost`-like resolver short-circuit contract.
- [RFC 2606](https://www.rfc-editor.org/rfc/rfc2606) reserves `.test` for
  testing and `.invalid` for names intended to be invalid.
- [RFC 6761](https://www.rfc-editor.org/rfc/rfc6761) gives `localhost.` and its
  descendants caller-loopback semantics. That conflicts with Tobari's need to
  route the name to Gateway synthetic DNS while keeping Workspace `localhost`
  Workspace-local.

Container runtime names such as Docker's `host.docker.internal`, Podman's
`host.containers.internal`, Colima's host mapping, and OrbStack's
`host.orb.internal` are useful mental-model precedents. They are infrastructure
contracts, not Tobari authority or portable resolver standards.

## Decision

The only public and routable Host Loopback hostname is:

```text
host.tobari.internal
```

The only supported URL shape remains plain HTTP on a non-privileged port:

```text
http://host.tobari.internal:{port}
```

The cutover from `host.tobari.test` is hard. There is no alias, CNAME,
redirect, translation, wildcard, suffix route, Host rewrite, or automatic
retry. Exact retired `host.tobari.test` remains a terminal non-learnable
authority for all V1 so it cannot fall through to ordinary external policy,
external DNS, Broker, upstream, or relay. Removing that negative guard requires
a separate ADR and negative safety evidence.

`.internal` is not authority. Host Loopback access is the conjunction of exact
hostname, port, method and path, the host-derived Workspace Manifest and
Workspace principal, a live canonical interactive Attachment Epoch, an exact
trusted-host Attachment Grant, and fresh route revalidation. Ordinary external
policy cannot decide this branch, and attachment authority cannot decide
ordinary external or sibling `.internal` traffic.

The exact public hostname is preserved as HTTP `Host` when the authenticated
relay dials physical `127.0.0.1`. The synthetic Gateway address and
`host.docker.internal` are routing transport only and never appear as public,
policy, review, or audit identity.

### Identity and schema cutover

`TOBARI_CAPABILITIES_JSON` remains its independent public/helper-visible
schema V1. Only the hostname value changes.

Private route and Attachment Grant registries hard-cut from schema V1 to
schema V2, contain only the new authority, and issue fresh opaque IDs. Route ID
V2 directly binds the exact hostname; there is no independent authority-
revision concept. ADR 0081's frozen compatibility key spellings remain exact:
private Host Loopback records continue to use `context`, `context_id`, and
`project_id` where that wire already does, while those values mean Workspace
Manifest and Workspace identity. They are not public aliases, and
`project_id` never names a project root.

Host Loopback remains a dependent capability of ADR 0081's canonical
interactive session. Route and grant stores remain separate from permission
ingestion endpoint, nonce, lease, acknowledgment, wait registry, and
Gateway-only transport/profile state. They also remain separate from ADR
0074's opposite-direction service-exposure owner.

### Migration and concurrency

Existing `migrate apply` is the only cutover owner. It adds no command and no
ordinary compatibility reader. Under the installation lifecycle lock it must:

1. prove the cluster is stopped;
2. acquire the canonical interactive-attachment lock and prove zero live
   canonical attachment owners;
3. keep that lock held while acquiring the Host Loopback lock;
4. accept only exact owner-only regular schema-V1 route and grant registries
   whose complete contents use `host.tobari.test`;
5. atomically replace them with empty schema-V2 registries; and
6. require explicit `cluster up` and a fresh Workspace entry, Attachment
   Epoch, denial, review, and grant.

The lock order is always `lifecycle -> interactive-attachment ->
host-loopback`. Entry creates the canonical session before Host Loopback, so
holding the interactive lock continuously from zero-owner proof through the
private registry cutover fences a concurrent entry. Host Loopback migration
must not read or interpret the WP07 permission-ingestion fields.

Routes, grants, relay tokens, candidates, and old opaque references are dropped,
not translated. Attachment startup, `cluster up`, status, list, show, and
doctor do not clean predecessor state. Manifest copy, Manifest revision
publication, Runtime lifecycle, learned policy, AppliedEntry, and observed
state neither own nor inherit attachment authority.

### TLS and CA

Host Loopback remains HTTP-only. New, retired, malformed, mismatched, and
unobservable TLS authority must be terminal before client leaf generation or
cache insertion and before passthrough, upstream, relay, or HTTP request hooks.
Absence of SNI, malformed SNI, encrypted/unobservable ClientHello authority,
and CONNECT/SNI drift fail closed.

The shared root CA is not rotated. A cached leaf is not authority. Only if an
exact owner-verified retired-host cache entry is actually observed may
`migrate apply` remove that one entry; broad cache deletion is forbidden.

## Considered alternatives

- Keep `host.tobari.test`: collision-resistant, but falsely presents a routine
  product capability as testing.
- Use `host.tobari.localhost`: rejected because resolvers and user agents may
  short-circuit it to the Workspace's own loopback and add secure-context
  semantics Tobari does not own.
- Use `host.tobari.invalid`: rejected because clients may fail before Tobari's
  synthetic resolver and terminal classifier.
- Use `host.tobari`: rejected because its suffix is globally delegable and
  search-path behavior is uncontrolled.
- Use `gateway.tobari.internal`: rejected because it names an implementation
  hop rather than the physical-host destination.
- Publish only a projection or use an IP: rejected because HTTP clients,
  policy, Host, and future SNI need one stable exact presentation identity.
- Route old and new names: rejected because it duplicates or widens attachment
  authority and introduces redirect/retry behavior into the security path.

## Consequences

- The user distinguishes Workspace `localhost`, physical-host
  `host.tobari.internal`, opposite-direction service exposure at numeric
  `127.0.0.1`, and infrastructure-only `host.docker.internal`.
- No public command, flag, resource, reference kind, policy lifetime, or trust
  boundary is added.
- Pre-public development state requires explicit migration and fresh review;
  stale references do not work.
- The permanent retired-name guard is deliberate negative compatibility and
  remains implementation surface throughout V1.

## Mechanical enforcement

- Domain tests fix the exact hostname, public capability schema V1, private
  registry schema V2, hostname-bound route/grant IDs, sibling and spelling
  rejection, and stale reference behavior.
- Migration tests fix the lock order, a deterministic competing-entry fence,
  stopped-cluster and zero-owner checks, exact schema/owner/hostname matching,
  empty V2 publication, crash recovery, no implicit cleanup, and secret-free
  output.
- Gateway and OPA tests fix exact branch separation, retired-name terminal
  handling, zero external-DNS/upstream/Broker/relay/retry calls, exact Host
  preservation, and no suffix/private-ceiling generalization.
- TLS tests fix pre-leaf rejection and zero cache growth for every terminal
  ClientHello shape. CA tests fix root digest stability and cache
  non-authority.
- Runtime integration covers the standard image's actual curl/libc, Python,
  applicable Go pure/cgo, and Node DNS plus HTTP clients. Every path proves
  Tobari synthetic DNS, zero external lookup, and exact Host preservation.
- Public guards allow the complete URI only for exact product-owned
  `host.tobari.internal` and reject sibling, wildcard, other `.internal`, and
  generic private-host URI exceptions.

## Validation

- focused domain, application, infrastructure, Gateway, OPA, CLI, migration,
  race, and documentation guards
- supported transparent-network and Runtime-client Docker canaries
- `task check`
- `task security`
- `task public:check`
- `task release:check`

## Reconsideration triggers

HTTPS Host Loopback, another protocol, persistent grants, a second hostname,
Host rewriting, root CA rotation, suffix authority, or removal/reintroduction
of the retired name requires a separate decision with resolver, TLS, migration,
and negative safety evidence.
