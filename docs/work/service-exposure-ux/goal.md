# Work Goal: Shorten reviewed Workspace service exposure

- Status: Accepted
- Decision state: Fixed by Product Owner
- Implementation state: Not started; implementation entry is gated by the
  fixed upstream order and a final re-baseline
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/00_theses.md` through `docs/04_harness.md`, ADR 0074,
  and ADR 0079
- Review/delete trigger: Delete after durable conclusions are promoted and the change completes
- Successor: None
- Owner: Tobari product, domain, and security maintainers
- Target: A future pre-public V1 implementation slice
- Related ADRs: ADR 0049, ADR 0055, ADR 0073, ADR 0074, and ADR 0079
- Fixed implementation order: WP01 + WP02 completion audit -> WP08 -> WP03 ->
  WP04 -> WP05 -> WP07 -> WP09
- Promoted WP01/WP02 authority:
  [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md),
  [Theses](../../00_theses.md), [Product contract](../../01_product_contract.md),
  [Architecture](../../02_architecture.md), [Security model](../../03_security_model.md),
  and [Harness](../../04_harness.md); promotion evidence is `07535a9` and
  `428812f`.
- Remaining upstream packets:
  completed WP08 Catalog/output contracts in [Architecture](../../02_architecture.md) and [Harness](../../04_harness.md),
  completed [ADR 0080 Runtime lifecycle](../../decisions/0080-close-the-managed-runtime-lifecycle.md),
  [ADR 0082 release and research build surfaces](../../decisions/0082-release-and-research-build-surfaces.md),
  [ADR 0083 Host Loopback authority](../../decisions/0083-name-the-physical-host-loopback-authority.md),
  the accepted [permission-resume and attachment-session contract](../../decisions/0081-observe-reviewed-permission-from-an-attached-workspace.md), and
  [Status Home](../status-home/goal.md)

## Outcome

A developer can request one HTTP/WebSocket service from an attached Workspace,
approve it once from a separate trusted-host terminal with the minimum safe
interaction, optionally open only the confirmed per-exposure `.localhost`
origin, see pending and active service state from the relevant status surfaces,
and stop the exact exposure from either side. The socket remains attachment-
owned, IPv4-loopback-only, opaque-reference-bound, and automatically cleaned up
when the owning attachment exits.

## Why now

The current capability has the correct trust boundary but a fragmented steady-
state experience. A single pending request requires the host user to invoke
`review services`, select the only row, choose Allow, and confirm again. There
is no watch mode, public JSON success format, CWD `status` integration, host-side
active-exposure action, or exit cleanup receipt. The helper blocks correctly
until a decision but its `list` command exposes only active listeners, so a
second observer cannot distinguish pending, active, unavailable, and absent
service state through one task.

The predecessor numeric-loopback URL also gives different exposures the same
cookie host. Current RFC6265bis states that cookies do not provide isolation by
port. V1 therefore needs one unpredictable `.localhost` hostname per exposure
while the actual listener remains bound only to `127.0.0.1`.

This packet keeps the accepted security mechanism and shortens only the task
sequence, typed state model, and presentation. It does not authorize
implementation in this session.

WP01 and WP02 have now been promoted into ADR 0079 and the governing durable
documents and integrated by evidence commits `07535a9` and `428812f`. The
predecessor-main facts in this packet remain UX evidence, but they are not a
frozen implementation base. Service implementation must still follow the full
fixed upstream order and then pass the explicit re-observation gate below.

## Non-goals

- Do not let a Workspace, helper, development server, terminal output, or
  browser content approve its own request.
- Do not merge Service review authority, references, persistence, or actions
  with the durable Permission Inbox.
- Do not add LAN binding, IPv6 binding, raw TCP, UDP, QUIC, TLS termination,
  HTTP/2, Docker publication, automatic port discovery, health probing, or a
  caller-selected host port as the default.
- Do not rewrite Host, Origin, redirects, cookies, headers, application
  content, or WebSocket frames.
- Do not publish a numeric-loopback or bare-`localhost` fallback URL, and do not
  strip or rewrite cookies as an origin-isolation substitute.
- Do not make browser opening automatic after ordinary Allow once, and do not
  treat terminal presentation, hyperlinks, or clipboard state as authority.
- Do not add a general attachment RPC, general URL opener, general clipboard
  writer, or full host `tobari` executable inside the Workspace.
- Do not put Service requests, exposures, attachment epochs, listeners, relays,
  or cleanup receipts into Workspace Manifest desired revisions, Workspace
  applied state, or bounded reconciliation failures.
- Do not move, stop, recreate, or re-key an existing exposure because its
  Workspace Manifest publishes a new revision or its Workspace has pending or
  attached-blocked entry adoption.
- Do not add a direct `tobari expose PORT` host shortcut until a separately
  reviewed task can bind one unique live attachment without rediscovery or
  guessed identity.
- Do not mix active exposures into the pending-request review selector or add
  Catalog typed multi-selection groups in V1.
- Do not add redundant `--confirm` flags to direct Allow/Deny/Open/Stop actions.
- Do not let Service discovery reads delete or repair stale owner records;
  cleanup has a separate owner.
- Do not edit, shadow, or pre-empt upstream production identity types,
  persisted schemas, Catalog entries, migration, desired/applied/observed
  model, attachment protocol, recursive-reference traversal, Runtime
  lifecycle, build profile, opposite-direction Host Loopback, or permission
  wait mechanisms.
- Do not make Manifest or Runtime copy lineage, provenance, source selection,
  retirement journals, usage evidence, or lifecycle commands part of Service
  request/exposure authority or presentation.
- Do not implement, change production code, tests, durable governing documents,
  generated files, release files, or public CLI contracts in this packet.
- Do not modify `tools/repoguard` in this packet. The current documentation uses
  structural locator components; the narrow generated-authority exception is a
  future governed WP09 implementation task with tests.
- WP05's `host.tobari.internal` and Host Loopback naming are the opposite
  Workspace-to-host direction. They are not a Service Exposure parameter,
  hostname, alias, authority field, or output label.

## Acceptance criteria

- [ ] WP01 + WP02 completion audit, WP08, WP03, WP04, WP05, and WP07 complete in
      that order. Immediately before WP09 implementation, the implementer
      fetches and records the actual integration HEAD/status, preserves all
      unrelated changes, and re-reads/re-tests every upstream identity, schema,
      Catalog, reference, attachment, Runtime-protection, status, and helper
      interface used here. A contradiction reports `WP09_BLOCKED` rather than
      changing the fixed design locally.
- [ ] `tobari-expose PORT` grants no access before host approval, emits pending
      guidance on stderr, blocks by default, and emits one final schema-1 success
      result containing the exact generated URL and confirmed child exposure
      reference; JSON stdout is never polluted by pending progress.
- [ ] `review services --watch` is accepted. Exactly one pending request shows
      its complete effect card directly; safe raw TTY keys are `a` Allow once,
      `o` Allow then Open, `d` Deny, and `b` Back, with no second `y`. Multiple
      requests require request selection followed by the same full card and one
      action key.
- [ ] Interactive text TTYs without safe raw-key support use the full line
      tokens `allow`, `open`, `deny`, and `back` plus Enter, with no second yes
      prompt. Direct exact Allow/Deny commands gain no redundant confirmation
      flag.
- [ ] Redirected operation and `--format json` are read-only. `--watch` and
      `--notify auto|osc9|bel|off` require a trusted interactive text TTY and
      conflict with JSON or redirected operation before any read or action.
      Notifications contain only generic evidence-free cues.
- [ ] `review services` remains a one-kind `service-request` selector and emits
      pending request refs only. Active exposures never enter that selector and
      V1 adds no Catalog typed multi-selection groups.
- [ ] `service status` replaces predecessor `service requests` with no alias and
      returns the host-wide bounded snapshot of pending requests and active
      exposures. It produces both `service-request` and `service-exposure` refs;
      helper `status` remains current-attachment scope and exposes stop refs
      only for active rows.
- [ ] Host Service reads use complete delivery with `bounded_window` coverage.
      One fixed owner-registry anchor distinguishes `complete`, `partial`, and
      `unavailable`, includes bounded `observed_owner_count` and
      `unavailable_owner_count`, and returns known empty only after a successful
      scan finds no in-scope owner.
- [ ] Unsafe path/type/mode, duplicate authority, contradictory owner identity,
      or ambiguous ref fails the command. Reads never clean stale records.
      Exact actions freshly re-resolve and revalidate owner, UID, PID, nonce,
      attachment, scope, and unchanged ref; stale or duplicate matches fail
      closed.
- [ ] The primary and only locator is structurally `scheme = http`,
      `authority = svc-<128-bit-random-lowercase-label>.localhost:<random-port>`,
      and `path = /`.
      The per-exposure label is fresh and independent from the opaque exposure
      ref. The listener still binds only `tcp4 127.0.0.1:0`; there is no numeric
      fallback, external DNS, LAN, wildcard, IPv6, raw TCP, or caller-selected
      host/address/port.
- [ ] Before Workspace I/O, every request validates exact canonical Host
      hostname plus assigned port and matching absolute-form authority. Numeric
      loopback, bare localhost, sibling label, wrong port, missing/duplicate/
      malformed Host, and DNS-rebinding authority are rejected. The accepted
      Host header is forwarded unchanged.
- [ ] HTTP/WebSocket headers, cookies, Origin, redirects, bodies, and frames are
      passed through only after exact authority/framing validation. Supported-
      browser tests prove cookies do not cross two generated origins and
      `Domain=localhost` cannot contaminate a sibling exposure. Contradictory
      supported-browser evidence reports `WP09_BLOCKED`; it never triggers a
      numeric fallback or ad-hoc cookie rewriting.
- [ ] The exact URL is attachment-lifetime access authority; the opaque
      exposure ref is lifecycle mutation authority. Neither is durable or
      derivable from the other outside the live owner.
- [ ] Ordinary Allow never opens. `o` finalizes Allow through the mutation-
      complete boundary, then calls the separate exposure-ref-bound Open use
      case. Open accepts no URL, path, query, fragment, or browser argv.
- [ ] Open returns `open_not_dispatched` with safe `service open` retry only
      when dispatch provably did not occur, `open_requested` after dispatch
      without claiming browser load, and non-retryable `open_outcome_unknown`
      when cancellation/timeout leaves dispatch uncertain. Allow success and
      URL/ref remain visible and non-replayable in every Open outcome.
- [ ] CWD `tobari status` consumes only bounded Service counts, attention, and
      observation state. It emits no Service ref, URL, port, or action, creates
      or repairs nothing, and never reconciles Workspace runtime.
- [ ] Stop closes listener admission first, terminates or drains relays within a
      fixed bound, confirms closure, then removes owner state. Attachment
      teardown withdraws pending and closes exposures/streams; its best-effort
      terminal receipt contains bounded counts only after confirmed steps and
      never IDs, URLs, ports, application data, or secrets.
- [ ] Request/exposure authority binds WorkspaceManifestID + WorkspaceID +
      AttachmentID/epoch + trusted principal/controller + exact target + exact
      request/exposure identity. Manifest revision/name, Runtime, ProjectRoot,
      row, target/host port, URL text, and display order are not mutation
      authority.
- [ ] Manifest copy transfers no request/exposure/attachment state. Runtime
      lifecycle never cascade-deletes an exposure; live attachment/observed
      Workspace state protects or blocks it. WP07 permission wait IDs/policy,
      WP06 summary consumers, WP04 research paths, and WP05
      `host.tobari.internal` never enter Service authority or output.
- [ ] There is no durable Service migration. The private protocol/schema
      hard-cutover is atomic across host and embedded helper source; old live
      attachments end and re-enter. WP01 migration's zero-live-attachment
      precondition means no request, exposure, listener, socket, or attachment
      state is translated or retained.
- [ ] First public Service success JSON is schema 1 after WP01 identity cutover,
      with no predecessor alias/reader. Host discovery, helper status, and CWD
      summary remain distinct schemas with exact absent/empty/partial/
      unavailable semantics.
- [ ] WP08's one recursive Catalog traversal derives every nested Service
      reference path. WP09 adds no Service-specific walker or competing
      producer/consumer registry.
- [ ] The future implementation updates the public guard through its governing
      contract with a narrow exact exception only for the generated Service
      authority: fixed `svc-` prefix, exact lowercase random-label grammar and
      length, exact `.localhost`, and validated port/locator structure. Tests
      reject arbitrary subdomains, bare localhost, `.local`, other private
      hosts, caller URLs, broad suffix/wildcard matches, and attempts to mask
      unrelated private-network text.
- [ ] Semantic, hostile Host/framing/cookie/origin, owner-race, TTY/raw/line/
      redirected/JSON, mutation-completion/cancellation, helper byte-equality,
      and Engine/Desktop/Colima readiness evidence passes, followed by
      `task check`, `task security`, `task public:check`, and
      `task release:check`.
- [ ] ADR 0074 and all thesis/product/architecture/security/harness consequences
      are promoted before mechanism. Completion removes this temporary packet,
      commits the implementation, and notifies the control thread with
      `WP09_IMPLEMENTATION_COMPLETE` or `WP09_BLOCKED`, final interfaces, gates,
      HEAD/status, retention, and WP06 readiness.

## Governing documents

- Thesis: North Star; Theses 0, 2, 5, 7, 8, and 9
- Product contract section: product outcome, public command inventory, helper
  command inventory, side effects, and pre-public V1 boundary
- Architecture or security invariant: purpose-specific attachment helpers,
  Program-aware Catalog, attachment ownership, separate host rendezvous,
  exact per-exposure `.localhost` Host validation over an IPv4-loopback-only
  listener, HTTP/WebSocket relay, one controlled side-effect boundary, and
  listener-first teardown
- Existing ADR: ADR 0074, constrained by ADR 0073's separate trusted-host
  terminal decision and ADR 0079's accepted identity/status/migration
  decisions

## Completion definition

The future implementation is complete only when every acceptance criterion has
evidence, the fixed Product Owner decisions remain intact, durable decisions
are promoted, temporary evidence is removed, the required gates pass, the
implementation is committed, the control thread is notified, and this
temporary packet is deleted from the final implementation tree.
