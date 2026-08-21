# Work Plan: Expose one Workspace HTTP service to the trusted host

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Extend the attachment pattern established by ADR 0055 without extending its
browser authority. Tobari materializes one dedicated read-only
`tobari-expose` helper and provides it an unpredictable attachment-local Unix
socket. A separate non-TTY Docker exec transports a closed service schema and
stream setup. The host validates the exact attachment and target port, records
a non-authoritative pending request, and emits a fixed cue. Only an explicit
`Allow once` inside Trusted Host Review creates a random host-loopback listener.

The host listener accepts only the unpredictable `.localhost` authority shown
to the user. It connects through the attachment-owned data path to exact
Workspace `127.0.0.1:<port>`, supports bounded HTTP/1.1 and WebSocket Upgrade,
and otherwise fails closed. The exposure registry belongs to the attachment,
not the foreground helper process, Workspace, Context, or cluster.

The terminal switch is a hard prerequisite. If its PTY experiment does not
ship, this packet returns for product review rather than substituting a second
terminal flow under the same design.

## Alternatives considered

### Host-side `tobari expose 3000`

This keeps authority visibly on the host but makes the user switch terminals
and requires Tobari to rediscover which attached Workspace and service the
number describes. The Workspace helper expresses the service target where it
is known while keeping approval on the host.

### Full `tobari` CLI inside the Workspace

Most host commands have no valid meaning inside the untrusted Workspace and
would imply authority the process does not possess. A purpose-specific helper
keeps both vocabulary and capability narrow.

### Docker port publishing

Publishing at container creation couples exposure to container lifecycle and
host daemon configuration, encourages broader address binding, and cannot
provide attachment-scoped host review. The host-owned loopback relay has the
required exact lifetime and authority.

### Reuse the browser socket as a generic attachment RPC bus

This saves a small amount of transport code but combines browser requests,
approval, listener creation, inventory, shutdown, and application streams
under one discriminator. Separate schemas and channels preserve reviewable
authority while allowing lower-level framing and owner-cleanup primitives to
be factored after their common invariants are proven.

## Design

### Public contract

- Workspace command: `tobari-expose <port>` where port is an exact validated
  non-privileged TCP port. No shell parsing or service-name lookup occurs.
- On request, stdout reports that trusted-host approval is pending. A fixed
  host cue says `Host review available - press Ctrl+] then r` without including
  untrusted service text or an actionable identifier.
- Trusted Host Review shows one typed request with fixed fields: Workspace,
  service target, host access, host-port selection, browser behavior, and
  lifetime. The only actions are `Allow once`, `Deny`, and `Back`.
- Successful approval returns one generated HTTP URL whose host is an
  unpredictable `workspace-<entropy>` label below `.localhost`. The entropy is
  host-generated and the displayed host port is selected by binding host IPv4
  loopback to an available port.
- The v1 helper also supports `tobari-expose list` and
  `tobari-expose stop <target-port>`. The catalog design slice must prove their
  exact roles, effects, and action target binding. If the target-port stop form
  conflicts with the governing mutation contract, implementation pauses for a
  product decision about an opaque exposure reference.
- Normal human output is the supported helper interface. No JSON format,
  automatic browser opening, host-port flag, service name, TLS, or raw relay is
  added in v1.
- Expected exit behavior: invalid local input fails before channel I/O;
  denied, withdrawn, unavailable-channel, protocol, and host-setup outcomes are
  distinct nonzero failures; accepted and same-active-target outcomes are zero;
  `list` with no exposures is a successful known-empty result; `stop` succeeds
  only after confirmed closure.
- Public discoverability must derive from one canonical executable contract.
  The full host `tobari` router remains absent from the Workspace.

Operation classification to confirm mechanically before mechanism code:

- exposure request: create one pending item in the fixed current-attachment
  request scope; creating the request does not itself change network access;
- review `Allow once`: create one exact attachment-owned exposure from one
  fresh opaque pending-request reference; access change is bounded and
  temporary;
- review `Deny` or `Back`: no exposure mutation;
- list: exhaustive read of the exact current-attachment exposure scope;
- stop: write one existing current-attachment exposure, with the final target
  binding selected by the catalog decision above.

### Layer changes

- Domain: typed port, pending request, unpredictable request and exposure
  references, reviewed decision, exact authority, passive state, result,
  lifetime, and state-transition invariants. Domain contains no socket, HTTP,
  terminal, or Docker types.
- Application: submit, withdraw, list, approve once, deny, stop, and correlate
  attachment closure through task-specific ports. It owns ordering and ensures
  approval precedes listener creation and listener closure precedes authority
  cleanup.
- Infrastructure: materialize and mount the helper, create the owner-only Unix
  socket and non-TTY control process, bind host IPv4 loopback, validate bounded
  HTTP authority, relay HTTP and WebSocket streams, generate the fixed 502, and
  implement exact shutdown. It makes no approval or persistence decision.
- CLI and catalog: represent the helper executable and Trusted Host Review
  source from one canonical contract, parse exact inputs once, render fixed
  request and result states, and keep host and helper routing separate.

### Data and control flow

```text
Workspace user
  -> tobari-expose 3000
  -> attachment-local exposure socket
  -> fixed non-TTY service control agent
  -> host decodes and validates exact attachment plus target port
  -> pending request with opaque host identity
  -> fixed host attention cue

host keyboard: Ctrl+] then r
  -> Trusted Host Review
  -> fresh pending-request read
  -> Allow once with exact opaque request reference
  -> application confirms attachment still owns request
  -> infrastructure binds 127.0.0.1:<random>
  -> exact workspace-<entropy>.localhost authority becomes active
  -> reviewed result returns to tobari-expose

host browser request
  -> host loopback listener
  -> bounded HTTP authority validation
  -> attachment-owned stream setup
  -> exact Workspace 127.0.0.1:3000
```

Browser and service channels may reuse a small reviewed owner-lifecycle and
bounded-frame implementation only after tests show identical invariants. They
do not share a socket, schema, authority registry, request union, or data path.

### Error and cancellation behavior

- Invalid argv, unavailable socket, malformed frame, stale attachment, and
  invalid port fail before a host listener exists.
- `Ctrl+C` while the helper waits sends one typed withdrawal when possible and
  exits nonzero. Attachment close implicitly withdraws every pending request.
- `Back` leaves the request pending and returns to the child. `Deny` resolves it
  denied. Neither opens a listener.
- A listener can be approved before the server starts. When a validated HTTP
  request cannot connect to the Workspace target, Tobari returns a bounded 502
  page: `Workspace service is not available yet. Start the service, then reload.`
- No active health probe runs. List states are limited to host listener open,
  currently relaying connections, and last Workspace connection failed.
- A failed host bind produces no active exposure and returns a typed failure to
  the pending helper. There is no automatic retry or fallback to another
  address.
- Stop rejects a stale or foreign target, closes the listener, causes active
  relays to terminate within a finite bound, then removes the exposure. It does
  not signal the development server.
- Attachment cancellation first prevents new connections, then closes
  listeners and active streams, then the control process and socket, and only
  then removes request and exposure authority. Cleanup errors cannot leave an
  authority record that reopens a route.
- Half-close, blocked writers, idle streams, upgraded connections, and host
  process cancellation receive exact bounded tests before timeout values are
  documented.

### Security and public boundary

- The Workspace can request only an exact non-privileged Workspace-loopback
  port. It cannot choose a host address, host port, hostname, protocol parser,
  destination address, attachment, or lifetime.
- Only host keyboard input can enter review and only a typed reviewed action can
  create the listener. The request channel is evidence, not authority.
- Host binding is exactly IPv4 `127.0.0.1`. The random `.localhost` label and
  port form a capability-like URL, but authorization still requires exact
  listener ownership and HTTP authority validation.
- Application bytes never enter normal logs, audit, policy evidence, terminal
  cues, or error copy. Only bounded metadata such as attachment identity,
  target port, passive state, counts, and secret-free fault codes may be
  recorded.
- Workspace HTTP output remains untrusted. The fixed 502 is Tobari-owned and is
  emitted only when no Workspace response has begun.
- The helper is versioned with Tobari runtime assets and mounted read-only. It
  carries no Docker socket, host filesystem mount, credential, policy writer,
  browser opener authority, or general executor.

## Implementation slices

1. Wait for the Trusted Host Review packet's PTY go decision and durable ADR.
   Then revise the attachment-helper thesis and write a service-exposure ADR
   before mechanism code.
2. Resolve and mechanically test the program-scoped canonical catalog,
   operation roles, effects, fixed or reference-bound targets, helper grammar,
   output, and stable failures. Return the stop selector for review if needed.
3. Add domain and application failing tests for request identity, approval,
   idempotence, attachment isolation, cancellation, stop, and owner cleanup.
4. Add the dedicated runtime asset and control protocol with hostile-frame,
   readiness, boundedness, and cleanup tests. Factor only proven common
   browser-channel lifecycle primitives.
5. Add the host HTTP and WebSocket relay behind the application-owned port;
   prove exact authority, 502 behavior, backpressure, concurrency, shutdown,
   and zero data logging.
6. Compose service requests into Trusted Host Review, add fixed cue and helper
   presentation, and validate representative development-server compatibility.
7. Promote the final contract through theses, product, architecture, security,
   harness, capability ledger, README, architecture site, and agent readiness.

## Verification

- Unit and contract tests: port and authority validation, request and exposure
  state machines, roles and effects, opaque references, exact helper grammar,
  output and exit mapping, current-attachment inventory, and stop identity.
- Negative side-effect tests: malformed or denied requests create zero
  listeners; Workspace bytes cannot open review or approve; wrong Host, stale
  request, wrong attachment, wrong port, replay, and duplicate frames cannot
  reach the Workspace server.
- Protocol and hostile-output tests: duplicate or unknown fields, versions,
  oversize, controls, partial frames, timeouts, reconnects, application ESC and
  line separators, request smuggling boundaries, and response-before-failure.
- Relay tests: HTTP request and response, WebSocket Upgrade and bidirectional
  messages, concurrent connections, half-close, backpressure, idle behavior,
  target refusal with fixed 502, stop, and attachment-close cleanup.
- Docker and runtime tests: exact read-only mount, owner-only unpredictable
  socket, separate non-TTY exec, environment, version reconciliation, no full
  host CLI, and custom Runtime independence.
- Compatibility observation: pinned Vite, Next.js, Storybook, and Jupyter plus
  a synthetic HTTP and WebSocket fixture using the exact unpredictable
  `.localhost` authority and Origin.
- Agent-readiness scenario and discovery budget: a human in an attached
  Workspace needs one helper command, one documented host prefix, and one
  approval; no Workspace-ID lookup, Docker command, output parser, or external
  reconstruction is required.
- Required profiles: focused Go and Python tests as applicable, local Docker
  integration, `task check`, `task security`, and `task public:check`.
- Generated checks: catalog-derived help, capability ledger, runtime asset
  manifest, architecture site, public docs, and `git diff --check`.

## Rollout and rollback

The capability is pre-public and stores no durable state. Safe rollback removes
the helper mount, exposure channel, host listeners, review source, catalog
entry, and capability entry together. Normal attachment shutdown remains the
cleanup path during mixed-version failure. No compatibility reader, dormant
socket, persisted approval, Docker published port, or hidden listener remains.

If the terminal switch, catalog model, Host or Origin compatibility, bounded
HTTP validation, or owner cleanup cannot meet the accepted contract, no partial
exposure ships. The existing workflow of running the server outside the
Workspace remains the fallback while the product trade-off returns for review.

## Documentation promotion

- Add a narrow thesis for binary-owned attachment helpers: request capability,
  typed non-authority, trusted-host approval, exact owner, and no generic bus.
- Add a service-exposure ADR defining catalog ownership, channel separation,
  approval, authority, HTTP and WebSocket bounds, passive state, and cleanup.
- Update the product contract with the Workspace helper transcript, Trusted
  Host Review source, host URL, commands, output, failures, and lifetime.
- Update architecture and security documents with the control and data planes,
  assets, threats, precedence, owner order, and why browser and service
  channels remain separate.
- Add claim-to-enforcement rows, Docker and relay profiles, capability-ledger
  status, and the relevant agent-readiness journey.
