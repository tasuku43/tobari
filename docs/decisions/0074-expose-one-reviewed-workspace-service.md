# ADR 0074: Expose one reviewed Workspace service

- Status: Accepted
- Date: 2026-08-22
- Deciders: Tobari maintainers
- Scope: Product, Catalog, attachment architecture, HTTP relay, security, and
  harness
- Revises: Catalog fixed-target create reference production
- Related: ADR 0049, ADR 0055, ADR 0073, ADR 0079, ADR 0081, ADR 0083, ADR 0084
- Revised by: WP09 Service Exposure UX, 2026-08-25
- Superseded by: None

## Context

Development servers normally listen inside a Workspace while their browser runs
on the trusted host. Docker publication would bind access to daemon state and
container lifecycle. Reusing the browser-login, Permission Inbox, permission-
wait, Host Loopback, or Context Policy Memory channels would merge unrelated
authority and let Workspace code influence trusted-host decisions.

A random port on one numeric host is not browser isolation: the
[current RFC6265bis draft](https://httpwg.org/http-extensions/draft-ietf-httpbis-rfc6265bis.html)
states that cookies are shared across ports on one host.
[RFC 6761](https://www.rfc-editor.org/rfc/rfc6761.html) reserves `localhost`
and names below `.localhost` for loopback handling.
Service access therefore needs a fresh hostname per exposure while the socket
remains IPv4 loopback-only.

## Decision

An interactive Workspace receives one Tobari-owned, engine-native
`tobari-expose` helper with a hardcoded Program. It is built from the checked
source closure into the verified base Runtime, extracted with source/API/digest
and Linux-architecture checks, and mounted read-only in standard and custom
Runtimes. It is not a host CLI alias and cannot select authority through
`argv[0]`.

`tobari-expose PORT` is one fixed-target create. It submits one exact
non-privileged Workspace-loopback port, prints pending guidance only on stderr,
blocks for trusted-host disposition, and emits one final schema-1 JSON exposure
only after confirmed Allow. `tobari-expose status` returns complete current-
attachment pending and active state; pending rows carry no host mutation ref.
`tobari-expose stop EXPOSURE_REF` consumes an active ref unchanged. The retired
helper `list` spelling has no alias.

Host tasks are:

- `review services [--watch] [--notify auto|osc9|bel|off] [--format text|json]`
  returns pending request refs only. One pending request shows its complete
  effect card directly; multiple requests require selection first. On a safe
  raw TTY, `a`, `o`, `d`, or `b` is the confirmation. Line fallback uses
  `allow`, `open`, `deny`, or `back` plus Enter. There is no second yes prompt.
  Redirected and JSON operation are read-only. Watch and notifications reject
  non-interactive/JSON operation before any Service read; notifications contain
  only a fixed generic cue.
- `service status [--format text|json]` replaces `service requests` with no
  alias. It returns one complete-delivery, bounded-window host snapshot of
  pending requests and active exposures.
- `service allow --id REQUEST_REF` and `service deny --id REQUEST_REF` retain
  reference-bound create/write semantics. `service open --id EXPOSURE_REF` and
  `service stop --id EXPOSURE_REF` consume only a live exposure ref. Direct
  exact actions need no redundant confirmation flag.

Review and status take one bounded owner-registry anchor. Successful scans with
no owners are known empty. Owner collection reports `complete`, `partial`, or
`unavailable` with bounded observed/unavailable counts. Reads never clean state.
Unsafe type/mode/path, duplicate authority, contradictory identity, or
ambiguous references fail the command. Exact actions re-anchor and revalidate
owner UID/PID, nonce, Service AttachmentID, final ContextID/WorkspaceID, and the
opaque ref immediately before mutation.

Allow binds only `tcp4 127.0.0.1:0` and generates two independent authorities:

- access authority: scheme `http`, authority
  `svc-<128-bit-random-lowercase-label>.localhost:<assigned-port>`, path `/`;
- lifecycle authority: opaque `exp_...` reference.

The relay accepts only the exact generated Host hostname and assigned port
before opening a Workspace stream. It rejects numeric loopback, bare localhost,
sibling labels, absent/duplicate/malformed Host, wrong ports, mismatched
absolute-form authority, DNS-rebinding values, folded headers, and ambiguous
framing. It publishes no numeric fallback. After validation it preserves the
accepted Host and ordinary HTTP/WebSocket headers, cookies, Origin, redirects,
and content unchanged. WebSocket relay begins only after a valid `101`.

Allow never opens a browser. Interactive `o` first crosses the confirmed Allow
mutation-complete boundary, then invokes the separate Open use case with only
the confirmed exposure ref. Open accepts no URL, path, query, fragment, or
browser argv and reports exactly `open_not_dispatched` (safe manual retry),
`open_requested` (dispatch, not page load), or `open_outcome_unknown` (no
automatic retry). Open failure never rolls back or makes Allow replayable.

Stop closes listener admission first, terminates and drains relays within a
fixed bound, confirms closure, then removes owner state. Attachment teardown
withdraws pending requests and closes exposures/streams in the same order. A
best-effort terminal receipt carries counts only; output loss changes no cleanup
authority.

Service authority is attachment-local and binds final ContextID, WorkspaceID,
its distinct Service AttachmentID epoch, trusted owner/controller, exact target,
and request/exposure identity. Context presentation and Project root are
non-authoritative display. Template copy, Context Policy Memory, Runtime,
Permission Inbox/wait state, canonical permission-ingestion transport, and Host
Loopback route/grant authority are neither copied nor reused. Runtime lifecycle
must fail closed around a live observed Workspace/exposure and never cascade-
delete it. CWD `status` remains owned by its separate task and may consume only
a typed bounded count/attention/observation summary with no Service refs, URLs,
ports, or actions.

There is no durable Service state migration. The private host/helper protocol
cuts over atomically; old live attachments must exit and re-enter. The first
public Service JSON schema is schema 1.

## Consequences

- Routine access is one Workspace request plus one trusted-host action. The
  trusted host, never Workspace code, creates temporary access.
- Per-exposure hostnames isolate browser origins despite random ports not
  isolating cookies. Pinned supported-browser tests are a release gate; a
  contradiction blocks Service release rather than enabling numeric fallback
  or ad-hoc cookie rewriting.
- Pending requests, exposures, connections, headers/messages, registry scans,
  setup, browser dispatch, shutdown, and cleanup all retain fixed bounds.
- A validated request whose Workspace target is unavailable receives one fixed,
  secret-free `502` and only passive status changes.
- `tobari -- tobari-expose 3000` remains non-persistent: child exit ends the
  owning attachment and its exposure.

## Mechanical enforcement

- Catalog tests prove the exact task partition, Program-filtered routing/help,
  fixed-create child refs, recursive nested reference graph, effects, mutation
  bindings, confirmations, conflicts, and predecessor absence.
- Domain/application/CLI fixtures prove final identity, absent/empty/partial/
  unavailable semantics, schema 1, helper stdout/stderr separation, raw/line/
  redirected/JSON behavior, Allow-then-Open ordering, mutation completion, and
  recovery output.
- Infrastructure tests prove owner-only registry/rendezvous identity, races,
  no read cleanup, fresh exact action resolution, tcp4 random loopback binding,
  hostile Host/framing rejection before Workspace I/O, header preservation,
  fixed 502, HTTP/WebSocket/backpressure, browser outcomes, stop ordering, and
  bounded cleanup receipts.
- Pinned Playwright Chromium proves two generated origins do not exchange
  host-only cookies and `Domain=localhost` cannot contaminate a sibling.
- Repoguard admits only the exact generated Service root-URL grammar, with
  positive and negative tests against broad localhost/private-network masking.
- Helper source byte equality, isolated Engine/Desktop/Colima integration, and
  repository security/public/release gates close packaging and platform claims.

## Reconsideration trigger

A supported browser violating the origin/cookie assumption blocks this design.
Broader protocols, another bind address, persistent approval, requested host
ports, arbitrary browser input, automatic discovery, application-specific
rewriting, or a generic attachment RPC require a separate reviewed decision.
