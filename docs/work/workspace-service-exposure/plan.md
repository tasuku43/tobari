# Work Plan: Expose one Workspace HTTP service to the trusted host

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Extend the attachment pattern established by ADR 0055 without extending its
browser authority. The current embedded base-image source builds one dedicated
Linux `tobari-expose` executable for the Docker engine architecture from the
same global Catalog and domain code but with a hardcoded helper entrypoint. The
host extracts it from the verified source-derived base, checks regular-file
mode, hash, ELF architecture, source/protocol identity, and mounts it read-only
into every selected Workspace. It then provides an unpredictable
attachment-local Unix socket. A separate non-TTY Docker exec transports a
closed service schema and stream setup. The host validates the exact attachment
and target port, records a non-authoritative pending request, and tells the user
to run `tobari review` in a separate trusted-host terminal. Only an explicit
`Allow once` from that host review creates a random host-loopback listener.

The host listener accepts only exact authority `127.0.0.1:<random-port>` shown
to the user. It connects through the attachment-owned data path to exact
Workspace `127.0.0.1:<port>`, supports bounded HTTP/1.1 and WebSocket Upgrade,
and otherwise fails closed. The exposure registry belongs to the attachment,
not the foreground helper process, Workspace, Context, or cluster.

The attachment owner also exposes a separate owner-only, unpredictable host
Unix rendezvous socket and atomically publishes bounded, secret-free ephemeral
routing metadata. `tobari review` discovers live owners through that registry,
obtains a fresh typed snapshot, and returns an opaque-reference-bound decision.
It never talks to the Workspace directly or takes ownership of the listener.
ADR 0073 remains intact: no attachment input is intercepted and no review is
drawn over the child terminal.

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

The helper is not the host executable selected by argv0. Its dedicated main
hardcodes the helper program, so copying it or spoofing argv0 cannot route host
commands.

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
- On request, stdout reports that trusted-host approval is pending and says
  `Review on the host: tobari review` without including untrusted service text
  or an actionable identifier.
- The separate host `tobari review` surface has visibly distinct `Permission
  requests` and `Service requests` sources. Permission review retains exact or
  template staging and one Apply; service decisions are immediate, temporary,
  and never use Apply or persistence.
- Service review shows one typed request with fixed fields: Workspace,
  service target, host access, host-port selection, browser behavior, and
  lifetime. The only actions are `Allow once`, `Deny`, and `Back`.
- Successful approval returns one generated HTTP URL whose exact authority is
  `127.0.0.1:<random-port>`. The displayed host port is selected by binding host
  IPv4 loopback to an available port.
- The v1 helper also supports `tobari-expose list` and
  `tobari-expose stop <exposure-ref>`. Creation and list produce the same
  compact opaque exposure reference and render the exact stop command. Stop
  consumes that reference unchanged; it does not reconstruct identity from a
  port, URL, label, list order, or attachment position.
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
  request scope; creating the request does not itself change network access.
  This fixed-target create consumes no reference but may return confirmed child
  request or exposure references whose kinds differ from the attachment scope;
- review `Allow once`: create one exact attachment-owned exposure from one
  fresh opaque pending-request reference; access change is bounded and
  temporary;
- review `Deny`: write one exact pending request through its opaque reference,
  resolving it without an exposure; `Back` is a read-only UI transition and
  leaves the request pending;
- list: exhaustive read of the exact current-attachment exposure scope;
- stop: write one existing current-attachment exposure using the required
  opaque exposure-reference input as `target_id_input`.

### Layer changes

- Domain: typed port, pending request, unpredictable request and exposure
  references, reviewed decision, exact authority, passive state, result,
  lifetime, and state-transition invariants. Domain contains no socket, HTTP,
  terminal, or Docker types.
- Application: submit, withdraw, list, approve once, deny, stop, and correlate
  attachment closure through task-specific ports. It owns ordering and ensures
  approval precedes listener creation and listener closure precedes authority
  cleanup.
- Infrastructure: build the dedicated helper as a checked input of the
  canonical and embedded base source, validate engine architecture and exact
  helper identity, extract it through bounded Docker create/copy/remove into
  owner-only host state, mount it read-only into any selected Runtime, create
  the owner-only Workspace Unix socket and non-TTY control process, create the
  owner-only host rendezvous socket and ephemeral registry record, validate
  peer, nonce, and attachment identity, bind host IPv4 loopback, validate bounded HTTP
  authority, relay HTTP and WebSocket streams, generate the fixed 502, and
  implement exact shutdown. It makes no approval or persistence decision.
- CLI and catalog: represent the helper executable, live service-request
  discovery, opaque-reference-bound Allow/Deny acts, and the unified
  `tobari review` composition from canonical contracts. Existing `tobari policy
  review [--watch]` remains policy-specific and unchanged; no hidden parallel
  mutation path is added.

The Catalog revision is deliberately asymmetric. A fixed-target
`EffectCreate` may produce opaque references for confirmed child resources but
may not consume a reference or produce its fixed-target kind. Fixed-target read
and write operations remain reference-free. Global producer/consumer closure,
all mutation binding rules, and the confirmed mutation-output boundary remain
unchanged. ADR 0074 and contract tests must make this exception mechanical;
classification prose alone is insufficient.

### Data and control flow

```text
Workspace user
  -> tobari-expose 3000
  -> attachment-local exposure socket
  -> fixed non-TTY service control agent
  -> host decodes and validates exact attachment plus target port
  -> pending request with opaque host identity
  -> fixed instruction: run tobari review on the host

separate trusted-host terminal
  -> tobari review
  -> validates live owner registry and rendezvous identity
  -> fresh exhaustive pending-request snapshot
  -> Service requests
  -> Allow once with exact opaque request reference
  -> owning attachment revalidates request and lifetime
  -> infrastructure binds 127.0.0.1:<random>
  -> exact 127.0.0.1:<random> authority becomes active
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
  `Ctrl+C` in host review exits or returns locally without mutating a request.
- `Back` returns to the review source list and leaves the request pending.
  `Deny` resolves it denied. Neither opens a listener.
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
- Only a typed action from the separately invoked trusted-host review process
  can create the listener. Workspace bytes and registry metadata are evidence,
  not authority; the live attachment owner revalidates identity and request
  state immediately before the side effect.
- Host filesystem ownership and modes plus peer, nonce, and attachment checks
  protect rendezvous. Stale or forged records fail closed and are cleaned
  without following untrusted paths. The review process never inherits route
  lifetime ownership.
- Host binding is exactly IPv4 `127.0.0.1`. The random port is not authority by
  itself: access still requires exact listener ownership and exact
  `127.0.0.1:<port>` HTTP authority validation.
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

1. Treat ADR 0073 as the terminal boundary, prototype the cross-process host
   rendezvous, revise the attachment-helper thesis, and write a
   service-exposure ADR before mechanism code.
2. Add and mechanically test one program-aware canonical Catalog whose global
   reference graph spans host and helper commands while routing and help are
   filtered by program. Declare operation roles, effects, reference-bound
   targets, helper grammar, output, and stable failures. Revise the fixed-target
   create child-reference rule narrowly in governing contracts, Skill, Catalog
   validation, and negative canaries before relying on it.
3. Add domain and application failing tests for request identity, approval,
   idempotence, attachment isolation, cancellation, stop, and owner cleanup.
4. Add the dedicated runtime asset and control protocol with hostile-frame,
   readiness, boundedness, and cleanup tests. Build and extract the hardcoded
   Linux helper from the embedded base source for engine amd64 and arm64;
   reject Mach-O, wrong architecture, wrong source/protocol, non-regular files,
   and argv0 spoofing. Factor only proven common browser-channel lifecycle
   primitives.
5. Add the host HTTP and WebSocket relay behind the application-owned port;
   prove exact authority, 502 behavior, backpressure, concurrency, shutdown,
   and zero data logging.
6. Add canonical service discovery and reviewed actions, compose them into the
   separate-host `tobari review` source selector, add fixed helper instruction,
   and validate representative development-server compatibility.
7. Promote the final contract through theses, product, architecture, security,
   harness, public/release contracts, capability ledger, README, architecture
   site, and agent readiness.

## Verification

- Unit and contract tests: port and authority validation, request and exposure
  state machines, roles and effects, opaque references, exact helper grammar,
  output and exit mapping, current-attachment inventory, and stop identity.
- Negative side-effect tests: malformed or denied requests create zero
  listeners; Workspace bytes cannot open review or approve; forged/stale host
  registry records, wrong peer/nonce/Host/attachment/port, replay, and duplicate
  frames cannot reach the Workspace server.
- Protocol and hostile-output tests: duplicate or unknown fields, versions,
  oversize, controls, partial frames, timeouts, reconnects, application ESC and
  line separators, request smuggling boundaries, and response-before-failure.
- Relay tests: HTTP request and response, WebSocket Upgrade and bidirectional
  messages, concurrent connections, half-close, backpressure, idle behavior,
  target refusal with fixed 502, stop, and attachment-close cleanup.
- Docker and runtime tests: exact read-only mount, owner-only unpredictable
  socket, separate non-TTY exec, environment, version reconciliation, no full
  host CLI, dedicated hardcoded entrypoint, amd64/arm64 ELF validation,
  source/protocol identity, bounded extraction cleanup, and custom Runtime
  independence.
- Compatibility observation: pinned Vite, Next.js, Storybook, and Jupyter plus
  a synthetic HTTP and WebSocket fixture using exact numeric loopback authority
  and Origin.
- Agent-readiness scenario and discovery budget: a human in an attached
  Workspace needs one helper command, one exact host command, and one approval;
  no Workspace-ID lookup, Docker command, output parser, or external
  reconstruction is required.
- Required profiles: focused Go and Python tests as applicable, local Docker
  integration, `task check`, `task security`, `task release:check`, and
  `task public:check`.
- Generated checks: catalog-derived help, capability ledger, runtime asset
  manifest, architecture site, public docs, and `git diff --check`.

## Rollout and rollback

The capability is pre-public and stores no durable state. Safe rollback removes
the helper mount, exposure channel, host listeners, review source, catalog
entry, and capability entry together. Normal attachment shutdown remains the
cleanup path during mixed-version failure. No compatibility reader, dormant
socket, persisted approval, Docker published port, or hidden listener remains.

If the host rendezvous, catalog model, Host or Origin compatibility, bounded
HTTP validation, or owner cleanup cannot meet the accepted contract, no partial
exposure ships. The existing workflow of running the server outside the
Workspace remains the fallback while the product trade-off returns for review.

## Documentation promotion

- Add a narrow thesis for binary-owned attachment helpers: request capability,
  typed non-authority, trusted-host approval, exact owner, and no generic bus.
- Add a service-exposure ADR defining catalog ownership, channel separation,
  approval, authority, HTTP and WebSocket bounds, passive state, and cleanup.
- Update the product contract with the Workspace helper transcript, unified
  separate-host review sources, host URL, commands, output, failures, and
  lifetime.
- Update architecture and security documents with the control and data planes,
  assets, threats, precedence, owner order, and why browser and service
  channels remain separate.
- Update public/release contracts so the helper is a non-archive,
  engine-native checked base-image input built from the same release source;
  release archives continue to contain one host executable.
- Add claim-to-enforcement rows, Docker and relay profiles, capability-ledger
  status, and the relevant agent-readiness journey.
