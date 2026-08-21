# Work Context: Expose one Workspace HTTP service to the trusted host

## Current behavior

- A Workspace can reach an explicitly granted host-loopback service through
  `host.tobari.test`, but the host cannot expose a Workspace development server
  through a Tobari-owned command or URL.
- ADR 0055 gives each attachment a binary-owned read-only `tobari-open` asset,
  an unpredictable Unix socket in Workspace temporary runtime storage, and
  three attachment-local environment values. A separate non-TTY Docker exec
  runs a fixed transport agent; the host validates a closed authorization-URL
  union and owns cleanup.
- `internal/infra/dockerruntime/workspace_browser_channel.go` uses a bounded
  schema-v1 newline frame over the control process's stdin and stdout. The
  Workspace listener has transport responsibility only; it cannot decide
  browser authority.
- `internal/infra/runtimeassets/assets/browser/tobari-open` is materialized from
  Tobari's embedded runtime assets and mounted read-only. It is not installed
  by or versioned with a user-managed Context Runtime.
- ADR 0073 closed the proposed `Ctrl+]`, then `r` trusted-host review switch as
  a no-go because arbitrary alternate-screen restoration requires a terminal
  emulator/parser. Service exposure can no longer depend on that transition
  and remains returned for a separate product-flow decision; it still must not
  share policy persistence or Apply semantics.
- The root direct-command capability ends the attachment when its exact child
  exits. Therefore `tobari -- tobari-expose 3000` would immediately close an
  accepted attachment-owned exposure and is not a useful advertised workflow.

## Relevant structure

- Entry point: interactive root Workspace attachment and its future Trusted
  Host Review source selector
- Domain rule: attachment identity and epoch, Host Loopback capability bounds,
  typed session request, operation effect and impact, and exact project
  principal under `internal/domain/tobari`
- Application use case: Workspace attachment lifecycle and host review
  composition under `internal/app/tobaricmd`
- Infrastructure boundary: Docker attachment, non-TTY control processes,
  runtime assets, host browser bridge, loopback listeners, and bounded stream
  relays under `internal/infra/dockerruntime`
- CLI catalog or presentation: canonical command contracts, Trusted Host
  Review, fixed attention cue, and human output under `internal/cli`
- Existing tests and harness checks: browser-channel framing and cleanup,
  Host Loopback grants, attachment epochs, Docker container specs, terminal
  security, capability ledger, public guard, and architecture-site generation

## Constraints

- The Workspace, project content, development server, request data, and helper
  input are untrusted. They may request exact host access but cannot approve it,
  select a host listener address, choose a host port, or widen relay behavior.
- Service review is an attachment-scoped access mutation, not a learned network
  policy decision. It offers `Allow once`, not staging plus a durable Apply.
- The terminal switch remains human-owned. Workspace socket traffic, terminal
  output, OSC sequences, and the helper process cannot open or confirm review.
- The helper's attachment channel is an ambient capability available only to
  processes already inside that attached Workspace. The socket path must be
  unpredictable, owner-only, bounded, and removed during owner cleanup.
- The browser bridge and service-exposure channel may share reviewed transport
  and lifecycle primitives, but must retain separate schemas, sockets, typed
  authority, rate or concurrency budgets, and data planes. There is no generic
  method or operation discriminator.
- Host-side access is IPv4 loopback only. The Workspace target is exactly IPv4
  loopback plus one reviewed non-privileged port.
- The repository documentation locale is English. Tests use synthetic roots,
  hostnames, service content, and request bytes.

## External facts

- RFC 6761, "Special-Use Domain Names," checked 2026-08-21:
  <https://www.rfc-editor.org/rfc/rfc6761#section-6.3>. Names ending in
  `.localhost` are special-use loopback names; the product still binds and
  validates one exact host listener and authority rather than trusting DNS
  behavior as authorization.
- RFC 6455, "The WebSocket Protocol," checked 2026-08-21:
  <https://www.rfc-editor.org/rfc/rfc6455>. A WebSocket begins as an HTTP Upgrade
  request and then becomes a bidirectional stream, so exact authority
  validation must precede tunneling and shutdown must cover both directions.
- RFC 9110, "HTTP Semantics," checked 2026-08-21:
  <https://www.rfc-editor.org/rfc/rfc9110>. The host-facing relay must parse a
  bounded request authority before forwarding; it must not infer authorization
  from display labels or destination order.

## Unknowns

- [ ] Can the canonical catalog represent the public `tobari-expose`
      executable, including its helper-only routing and exact help, without
      making the full host CLI available inside the Workspace or creating a
      second registry?
- [ ] Does `tobari-expose stop 3000` satisfy the catalog's action target-binding
      invariant as an exact attachment-and-port selector, or must `list` and
      successful creation produce an opaque exposure reference for `stop`?
- [ ] Which fixed protocol owns pending request creation, approval response,
      active exposure inventory, stop, stream setup, and shutdown without
      turning into a generic request bus?
- [ ] Do the pinned representative versions of Vite, Next.js, Storybook, and
      Jupyter accept an unpredictable `.localhost` authority and browser Origin
      without application-specific rewriting?
- [ ] Which HTTP versions are supported in the first slice? HTTP/1.1 and
      WebSocket Upgrade are the expected minimum; HTTP/2, TLS termination, and
      HTTPS development servers require an explicit decision.
- [ ] What finite header limit, setup deadline, idle policy, connection budget,
      and byte-stream backpressure preserve ordinary development without an
      unbounded host resource?
- [ ] How should a pending request be visibly withdrawn if the helper receives
      `Ctrl+C` while Trusted Host Review is open?

## Thesis evidence

- Repeated design decision or point of agent confusion: native browser opening,
  Host Loopback access, Permission Inbox, and service exposure all involve the
  host, but each has distinct authority and lifetime. A generic bridge would
  erase the exact boundary that makes them reviewable.
- User outcome or friction observed in the minimal slice: users start the
  server inside the Workspace and naturally want to request its host-browser
  URL from the same shell, rather than switch to a host command and restate
  Workspace identity.
- Code workaround or exception being considered: installing the full host CLI
  in the Runtime, using Docker port publishing, or adding an untyped operation
  field to the browser channel.
- Current thesis that resolves it, or proposed thesis revision: the attachment
  may expose a finite set of binary-owned purpose-specific helpers whose
  requests are non-authoritative until the trusted host validates and approves
  one exact effect. Each helper keeps its own typed protocol and cleanup.
- Downstream impact: North Star and attachment thesis, product workflow,
  architecture channel ownership, security asset and threat tables, operation
  contracts, catalog and capability ledger, Docker harness, README, and agent
  readiness.

## Reproduction or observation

```sh
go test ./internal/infra/dockerruntime -run 'WorkspaceBrowser|HostLoopback|Attachment'
go run ./cmd/tobari help tobari
```

Before relay implementation, run pinned local fixtures for a minimal HTTP
server, WebSocket echo, Vite, Next.js, Storybook, and Jupyter. Record exact
request authority, Origin, redirect, cookie, upgrade, startup, stop, and
attachment-close observations. Do not record project content, credentials, or
real browser history.

## Security and public-boundary notes

- Assets and side effects involved: one embedded Workspace helper, one
  unpredictable Unix control socket, one fixed non-TTY control process, host
  loopback listeners, bounded HTTP parsing, bidirectional relay streams, host
  review state, and owner-ordered cleanup.
- Credentials or confidential data involved: none by design. Application HTTP
  content is untrusted and may contain secrets, so it must not enter logs,
  policy evidence, diagnostics, fixtures, or review copy.
- New dependencies, destinations, files, processes, or generated content: no
  dependency is accepted by the packet. A parser or relay dependency requires
  license, architecture, security, and public-boundary review before use.
- Output delivery, collection coverage, pagination, timeout, retry, idempotency,
  and cancellation facts: helper responses are one bounded complete result;
  the current-attachment list is exhaustive at one host snapshot; approval is
  one attempt with no automatic retry; repeating one active target is
  idempotent within the attachment; every network phase has finite setup and
  shutdown bounds; stream delivery is not a paginated CLI result.
- Publication and licensing concerns: primary protocol standards only; pinned
  representative tool observations must record versions and license-safe
  synthetic fixtures without copying third-party UI or project content.

## Glossary

- **Workspace service:** one server bound to Workspace loopback on an exact
  reviewed port.
- **Exposure:** one host-loopback listener and exact `.localhost` authority
  relayed to one Workspace service for one attachment.
- **Exposure helper:** the binary-owned `tobari-expose` executable mounted
  read-only inside an attached Workspace.
- **Control channel:** the attachment-local request and lifecycle path. It is
  not the application byte stream and grants no access by itself.
- **Data plane:** the bounded HTTP and upgraded WebSocket streams created only
  after host approval and exact authority validation.
- **Passive state:** a fact observed from relay ownership or connection events,
  not an active application health probe.
