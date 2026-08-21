# Work Goal: Expose one Workspace HTTP service to the trusted host

- Status: Accepted
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/00_theses.md`, `docs/01_product_contract.md`, `docs/02_architecture.md`, `docs/03_security_model.md`, `docs/04_harness.md`, and the accepted ADRs required by this packet
- Review/delete trigger: Delete after durable conclusions are promoted and the change completes
- Successor: None
- Owner: Tobari maintainers
- Target: Pre-public self-use
- Related ADRs: ADR 0055 and ADR 0073
- Dependency status: ADR 0073 rejected the required inline terminal switch;
  this packet remains returned for a separate product-flow decision and cannot
  begin mechanism implementation as written.

## Outcome

From an attached Workspace, a user can run `tobari-expose 3000`, explicitly
approve the exact request in Trusted Host Review, and receive one unpredictable
host-loopback HTTP URL for the service on Workspace loopback port 3000. The
exposure is owned by the current attachment, remains separate from durable
network policy, and closes with that attachment.

## Why now

Running Vite, Next.js, Storybook, Jupyter, and similar development servers is a
routine local-development task. Tobari currently supports the opposite
direction, from a Workspace to a host service, but has no narrow path from the
host browser to a Workspace service. Requiring Docker publishing or an
unbounded relay makes the isolation model feel unsuitable for ordinary
development. ADR 0055 already proves the useful attachment pattern: a
binary-owned read-only Workspace helper, an unpredictable attachment-local
channel, a separate non-TTY control process, host-side validation, and exact
owner cleanup.

## Non-goals

- Do not add a host-side `tobari expose` command or make the full `tobari`
  executable available inside the Workspace.
- Do not add a generic Workspace-to-host RPC bus, arbitrary host command
  execution, Docker socket access, or a reusable plugin transport.
- Do not use Docker port publishing, bind a LAN address, accept `0.0.0.0`, or
  expose raw TCP or UDP in the first slice.
- Do not detect ports automatically, publish every listening port, choose a
  requested host port, or open a browser automatically.
- Do not persist approval in a Context, Workspace, learned network permission,
  or later attachment. There is no "always allow" choice.
- Do not poll the service or label it healthy. A connection attempt can have
  application-visible effects.
- Do not silently rewrite `Host`, `Origin`, redirects, cookies, WebSocket
  handshakes, or application content to manufacture compatibility.
- Do not implement this packet until the Trusted Host Review terminal contract
  has passed its bounded PTY experiment and is accepted durably.

## Acceptance criteria

- [ ] `tobari-expose 3000` is a Tobari-binary-owned, read-only attachment asset,
      not a Context Runtime customization and not an alias for the host CLI.
- [ ] The helper submits only one exact Workspace loopback TCP port in the
      non-privileged range through an unpredictable attachment-local channel;
      malformed, duplicate, oversized, stale, or cross-attachment requests fail
      closed before a listener or relay exists.
- [ ] A request cannot open Trusted Host Review or approve itself. The host
      emits a fixed attention cue, and the user must press `Ctrl+]`, then `r`.
- [ ] Trusted Host Review identifies the Workspace, exact target
      `127.0.0.1:<port>`, host-loopback-only result, automatically selected host
      port, no browser opening, and current-attachment lifetime before offering
      `Allow once`, `Deny`, and `Back`.
- [ ] `Allow once` creates exactly one listener on host `127.0.0.1` with an
      unpredictable `.localhost` hostname and random available port. Every
      request must present the exact hostname and port before any byte reaches
      the Workspace service.
- [ ] `Deny` and `Back` create no listener or data path. Cancellation while
      pending withdraws the request. Retrying after denial requires another
      explicit review.
- [ ] The foreground helper blocks only until the reviewed result is known and
      then returns. A separate attachment-owned helper owns accepted listeners
      and streams; the user's shell does not need to keep a background
      `tobari-expose` process alive.
- [ ] Repeating the same target port in the same attachment returns the same
      active URL without another approval. Different attachments cannot list,
      reuse, stop, or extend one another's exposure.
- [ ] `tobari-expose list` reports only current-attachment exposures using
      truthful passive states. `tobari-expose stop 3000` closes the matching
      listener and active relay connections without stopping the development
      server. Before implementation, its action identity must satisfy the
      repository's canonical catalog and mutation-binding invariants without a
      competing registry; if the approved port selector cannot do so, return
      the selector for product review rather than bypassing the invariant.
- [ ] When the host listener is open but the Workspace port cannot be reached,
      an HTTP request receives a fixed Tobari-owned 502 response explaining
      that the service is not available yet and may be retried after starting
      it. No active health polling occurs.
- [ ] HTTP requests and WebSocket upgrades work through a bounded relay. Host
      header parsing, header-size and setup deadlines, idle and shutdown
      behavior, half-close behavior, concurrency, backpressure, and cleanup are
      explicit and tested.
- [ ] Attachment shutdown closes listeners and active streams before removing
      request state, channel identity, and helper routes. No stale approval or
      socket path can reactivate an exposure.
- [ ] Representative pinned Vite, Next.js, Storybook, and Jupyter fixtures or
      runtime observations validate the unpredictable `.localhost` Host and
      Origin contract. An incompatibility returns for product review; it does
      not authorize silent rewriting.
- [ ] The public helper grammar and help have one canonical executable contract,
      predictable stdout, stderr, and exit status, and no hand-maintained
      parallel command registry.
- [ ] Theses, product, architecture, security, ADRs, capability ledger, harness,
      README, and agent-readiness evidence agree on scope, lifetime, owner,
      precedence, and non-authority.
- [ ] Focused domain, application, protocol, relay, Docker, CLI-contract, and
      hostile-input tests plus `task check`, `task security`, and
      `task public:check` pass.

## Governing documents

- Thesis: North Star; Theses 0, 3, 5, 7, and 8
- Product contract section: Primary operating loop, Workspace attachment,
  Host Loopback, Permission Inbox, and output and exit behavior
- Architecture or security invariant: four-layer dependency direction,
  controlled side-effect boundaries, trusted-host-only authority, exact
  attachment ownership, untrusted Workspace input, and complete cleanup
- Existing ADR: ADR 0055 for a dedicated Workspace browser channel; the
  Trusted Host Review terminal-ownership ADR is a prerequisite

## Completion definition

The work is complete when one representative HTTP service and WebSocket
service can be approved, reached, inspected, stopped, and cleaned up through
the product-shaped flow; hostile and cross-attachment requests cannot create
or retain host access; the second-executable catalog contract is mechanically
enforced; durable decisions are promoted into governing documents and ADRs;
all required gates pass; and this temporary packet is removed.
