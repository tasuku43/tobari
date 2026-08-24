# ADR 0049: Use attachment leases for ambient host loopback HTTP

- Status: Accepted
- Date: 2026-08-17
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, network policy, Workspace lifecycle, CLI, and harness
- Revises: Thesis 2 host-loopback boundary and Thesis 8 policy-decision lifetime
- Revised by: ADR 0083
- Superseded by: None

## Context

A coding agent often needs to observe a development server that the developer
already runs on the physical host, whether it was started natively or through
Docker Compose. That data-plane outcome does not require Docker control.
Mounting the host Docker socket or sharing its daemon would grant unrelated
control-plane authority and conflict with Workspace independence.

Physical-host loopback is nevertheless sensitive. Local services often rely on
loopback reachability instead of application authentication, and a port can
later be reused by another process. Existing learned policy rules persist until
reset and have no session constraint, so they cannot safely authorize this
destination.

The first named-service design required `tobari --host-http NAME=PORT`. Review
showed that this duplicated the later permission decision and made users
declare an ambient machine fact at entry time. The stable fact is simpler:
there is a physical host, while access to one exact effect remains policy-owned.

Workspace requests share one project network principal. Gateway can bind an
effect to the Context and Workspace, but cannot identify which shell, Docker
Exec, agent, or child process originated it. A session claim must therefore not
imply per-process isolation.

## Decision

Every interactive `tobari` attachment projects one constant Host Loopback
capability:

The initial hostname was `host.tobari.test`. ADR 0083 revises the current
public authority to exact `host.tobari.internal` and retains the old name only
as a terminal non-routable V1 retirement guard.

The URL port selects the same physical-host IPv4 loopback port. The first slice
supports plain HTTP on ports 1024 through 65535. No entry declaration, service
name, target address, Docker resource, or port discovery is accepted. Inside
the Workspace, `localhost` continues to mean the Workspace itself.

The trusted host process establishes an unguessable Attachment Epoch and owns
one authenticated relay. Capability projection and routing grant no authority.
The first exact effect passes through Gateway and OPA and remains denied until
interactive `review permissions` creates an exact Attachment Grant Allow or Deny.
The grant binds Context, Workspace, Attachment Epoch, host, target port, method,
and path. It is a separate domain and store from durable learned rules and can
never become a template, baseline, preset, or persistent policy rule.

Attachment lifetime is event-owned rather than clock-owned. It begins after the
reviewed projection is active and ends when the route-owning host attachment
ends. The relay closes before route and grant cleanup, making stale metadata
inert. Arbitrary durations, expiry timestamps, renewal, and idle-time authority
are excluded; timeouts bound I/O but do not define authorization lifetime.

The grant applies to every process in the Workspace while the owning attachment
is active. Concurrent attachments borrow the current epoch. A borrower cannot
extend its lifetime, remove it, transfer ownership, or remain authorized after
the owner exits. A later attachment can establish a fresh epoch.

Gateway and OPA authorize the exact method, path, and target port before
selecting the relay. The host relay independently requires its 256-bit token and
an active Allow for the same project, epoch, and target port before dialing the
fixed `127.0.0.1:<port>` target. A token alone is not host-loopback authority.

## Consequences

### Positive

- An agent can observe a host-run application without Docker control.
- The host is discoverable without startup ceremony, while access remains
  deny-by-default.
- A reused host port cannot inherit authority from an earlier attachment.
- Permission lifetime follows the developer interaction rather than a clock.
- Host access uses the same denial, trusted-host review, and retry journey as
  other supported HTTP effects.

### Negative

- Gateway, OPA input, denial audit, review, and attachment lifecycle gain a
  second authority lifetime.
- A separate host terminal is required to review while the attachment is open.
- Every Workspace process can use an active grant; per-agent isolation would
  require a different network principal.
- Each new owning attachment requires a fresh decision.
- The portable host-to-Gateway relay and crash reconciliation are
  security-critical infrastructure.

## Mechanical enforcement

- Closed domain values require `host_loopback` plus `attachment`, a valid epoch,
  constant hostname, and non-privileged target port.
- Candidate and grant opaque IDs bind every typed identity dimension.
- The catalog exposes no `--host-http`; parser tests reject the retired input.
- The Workspace receives one constant, secret-free `TOBARI_CAPABILITIES_JSON`
  projection. Relay coordinates never enter it, OPA input, audit, or review.
- A strict owner-only registry binds one project principal to one epoch, random
  relay port, and 256-bit token. Gateway ignores request-selected session
  headers, environment, and URL parameters.
- Gateway/OPA and relay tests prove denied, malformed, unauthenticated,
  unreviewed, wrong-port, stale, and post-detach requests perform zero target
  I/O.
- Relay tests prove loopback-only target construction, owner-before-authority
  teardown, concurrent borrower non-ownership, and grant cleanup.
- Policy tests prove attachment decisions cannot enter durable Allow/Deny
  stores, templates, presets, or the `policy rules` inventory.

## Compatibility and migration

The named `--host-http NAME=PORT` design was never released and is retired in
the same change. Existing public `tobari [--context NAME]` syntax remains. The
new capability adds an attachment-owned route and environment projection to
ordinary entry; no durable state migration is required, and rollback leaves no
Host Loopback Allow to clean up.

## Security and public-boundary impact

The initial Unix-socket relay hypothesis was rejected by runtime observation:
host-created Unix sockets in a bind-mounted directory were not visible inside
the Colima Docker VM. The authenticated TCP relay uses
`host.docker.internal`/`host-gateway` only as transport to a token- and
grant-protected listener, never as authorization.

This decision revises the private-destination boundary only for reviewed
physical-host loopback HTTP. It does not authorize host Docker, raw TCP,
private LAN, host networking, resident privileged processes, arbitrary Unix
sockets, caller-selected executables, or privileged ports. Repository tests and
documentation use synthetic identities, ports, paths, and epochs.
