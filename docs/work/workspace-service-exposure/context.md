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
  and requires host review to remain in a separate terminal. The product owner
  selected `tobari review` as the unified trusted-host inbox. Permission
  requests retain their staged Apply semantics; service requests use immediate,
  attachment-local `Allow once` or `Deny` decisions.
- The root direct-command capability ends the attachment when its exact child
  exits. Therefore `tobari -- tobari-expose 3000` would immediately close an
  accepted attachment-owned exposure and is not a useful advertised workflow.
- The canonical Catalog is currently single-program and hard-codes `tobari` in
  usage, help, and dispatch. A separate helper catalog would break reference
  flow across helper discovery, host review, and stop. One global Catalog with
  an explicit program per command can validate the complete reference graph
  while filtering routing and help for each executable.
- Catalog mutation binding requires stop to consume the opaque exposure
  reference produced by creation or list. A target port is not the identity of
  an exposure and cannot distinguish the same port in another attachment.
- A bounded live-owner rendezvous prototype passed 20 macOS runs and Linux
  amd64 cross-compilation. It covered concurrent owners, atomic `0600` records
  under `0700` state, kernel peer identity, nonce, attachment and request
  revalidation, fresh snapshots, stale or forged cleanup without following
  record paths, owner exit during review, and distinct reviewer/owner lifetime.
- Pinned compatibility observations passed without Host, Origin, content, or
  application-configuration rewriting for Vite 8.2.2, Next.js 16.3.2,
  Storybook 10.5.10, and Jupyter Server 2.20.0 when host authority is exact
  `127.0.0.1:<random-port>`. Jupyter's default remote-access check rejected the
  previously proposed random `.localhost` authority with 403; the product owner
  therefore selected numeric loopback authority on 2026-08-22.

## Relevant structure

- Entry point: interactive root Workspace attachment plus a separate host
  `tobari review` process with Permission and Service request sources
- Domain rule: attachment identity and epoch, Host Loopback capability bounds,
  typed session request, operation effect and impact, and exact project
  principal under `internal/domain/tobari`
- Application use case: Workspace attachment lifecycle, live-review discovery,
  and host-reviewed service actions under task-specific application packages
- Infrastructure boundary: Docker attachment, non-TTY control processes,
  runtime assets, host browser bridge, loopback listeners, and bounded stream
  relays under `internal/infra/dockerruntime`
- CLI catalog or presentation: canonical helper and host-review command
  contracts, fixed host instruction, source-specific actions, and human output
  under `internal/cli`
- Existing tests and harness checks: browser-channel framing and cleanup,
  Host Loopback grants, attachment epochs, Docker container specs, terminal
  security, capability ledger, public guard, and architecture-site generation

## Constraints

- The Workspace, project content, development server, request data, and helper
  input are untrusted. They may request exact host access but cannot approve it,
  select a host listener address, choose a host port, or widen relay behavior.
- Service review is an attachment-scoped access mutation, not a learned network
  policy decision. It offers `Allow once`, not staging plus a durable Apply.
- The separate host process remains human-owned. Workspace socket traffic,
  terminal output, OSC sequences, registry data, and the helper process cannot
  open, select, or confirm a reviewed action.
- Each attachment owner publishes only bounded, secret-free routing metadata
  for an owner-only, unpredictable host Unix rendezvous socket. The registry is
  ephemeral discovery data, not authority or durable permission.
- `tobari review` reads a fresh exhaustive pending-request snapshot from live
  attachment owners. It never connects to the Workspace directly, and an
  opaque request reference—not a label, port, or list position—binds an action.
- The attachment process revalidates the request and remains the listener,
  stream, and lifetime owner. Review cannot create an exposure after that owner
  exits. Stale or forged registry records and identity mismatches fail closed.
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
  `.localhost` are special-use loopback names. Tobari considered this form but
  does not use it because Jupyter's default Host validation rejects the random
  hostname; exact numeric loopback authority preserves compatibility without
  application-specific rewriting.
- RFC 6455, "The WebSocket Protocol," checked 2026-08-21:
  <https://www.rfc-editor.org/rfc/rfc6455>. A WebSocket begins as an HTTP Upgrade
  request and then becomes a bidirectional stream, so exact authority
  validation must precede tunneling and shutdown must cover both directions.
- RFC 9110, "HTTP Semantics," checked 2026-08-21:
  <https://www.rfc-editor.org/rfc/rfc9110>. The host-facing relay must parse a
  bounded request authority before forwarding; it must not infer authorization
  from display labels or destination order.

## Unknowns

- [ ] Which closed Workspace-control and host-rendezvous protocols own pending
      request creation, fresh snapshot discovery, approval response, active
      exposure inventory, stop, stream setup, and shutdown without turning into
      a generic request bus?
- [ ] Which HTTP versions are supported in the first slice? HTTP/1.1 and
      WebSocket Upgrade are the expected minimum; HTTP/2, TLS termination, and
      HTTPS development servers require an explicit decision.
- [ ] What finite header limit, setup deadline, idle policy, connection budget,
      and byte-stream backpressure preserve ordinary development without an
      unbounded host resource?

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
  ephemeral host rendezvous registry, review state, and owner-ordered cleanup.
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
- **Exposure:** one host-loopback listener and exact
  `127.0.0.1:<random-port>` authority relayed to one Workspace service for one
  attachment, identified to actions by one opaque exposure reference.
- **Exposure helper:** the binary-owned `tobari-expose` executable mounted
  read-only inside an attached Workspace.
- **Control channel:** the attachment-local request and lifecycle path. It is
  not the application byte stream and grants no access by itself.
- **Host rendezvous:** an owner-only, unpredictable Unix socket plus bounded
  ephemeral routing record through which `tobari review` asks a live attachment
  owner for fresh request state and returns one typed decision. It is not the
  exposure owner or a durable policy store.
- **Data plane:** the bounded HTTP and upgraded WebSocket streams created only
  after host approval and exact authority validation.
- **Passive state:** a fact observed from relay ownership or connection events,
  not an active application health probe.
