# Work Plan: Shorten reviewed Workspace service exposure

- Status: Accepted
- Decision state: Fixed by Product Owner
- Implementation state: Not started
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Keep ADR 0074's owner, control channel, rendezvous, IPv4-loopback listener,
HTTP/WebSocket relay, and cleanup ownership. Replace the single-shot line UI
with a pending-request-only typed review snapshot and a reusable terminal
review presentation primitive shared mechanically with Permission Inbox.
`review permissions` and `review services` remain different tasks with
different result types, references, actions, confirmation semantics,
persistence, and authority.

Consume ADR 0079 and docs 00--04 as the promoted upstream identity, copy, and
lifecycle contract.
Request/exposure authority is keyed to the live attachment's trusted
WorkspaceManifestID, WorkspaceID, and attachment epoch. It is not a Manifest
revision slice, Workspace applied receipt, or reconciliation condition.

Do not implement this plan against predecessor main or treat the current
integrated checkout as the later WP09 baseline. The fixed order is WP01 + WP02 completion audit
-> WP08 -> WP03 -> WP04 -> WP05 -> WP07 -> WP09. Re-baseline every actual
upstream interface immediately before WP09 changes any production surface.

Consume promoted WP02 semantics from ADR 0079/docs 00--04 and WP03 as the
design-fixed isolation boundaries. Manifest or Runtime copy creates fresh independent authority and carries no Service or
attachment state. Runtime lifecycle actions protect observed live
Workspace/container use by failing closed; they do not cascade into Service
Stop or attachment teardown. Neither packet adds a Service authority dimension
or makes its production mechanism a prerequisite for the Service UX.

The helper request continues to wait by default because this gives the caller
the confirmed URL and stop reference without a second polling command. Pending
becomes a first-class observable state through helper status, host review,
host `service status`, and CWD summary. Review can watch before or after a
request and never mixes active exposures into its selector. One safe raw key or
one full line token on the complete effect card is confirmation. A separate
Allow-then-Open choice composes confirmed Allow with a purpose-limited
exposure-reference-bound browser action. Ordinary Allow never opens a browser.

Each confirmed exposure receives the sole access locator: `scheme = http`,
`authority = svc-<128-bit-random-lowercase-label>.localhost:<random-port>`, and
`path = /`. The
label is fresh per exposure and independent from its lifecycle ref. The socket
still binds `tcp4 127.0.0.1:0`, and the relay accepts and forwards only the exact
generated Host authority. Numeric loopback is never published as fallback.

Every host read snapshots one bounded owner-registry anchor, returns complete
delivery with `bounded_window` coverage and typed complete/partial/unavailable
observation, and performs no cleanup. Exact actions freshly re-resolve and
revalidate all owner and attachment authority before effects.

Terminal output is the mandatory URL handoff. Clipboard mutation is excluded.
Host and helper stop remain exact reference-bound actions. Attachment exit
returns a typed cleanup receipt used by the session-close presentation.

## Decision summary

| Question | Decision |
|---|---|
| Permission Inbox integration | Share presentation mechanics only; keep tasks, snapshots, refs, mutations, and lifetime separate |
| Service watch | Accept `review services --watch`; notification cues are fixed and evidence-free |
| Review selection | Pending `service-request` only; active exposures are never mixed in and V1 adds no typed multi-selection groups |
| Confirmation | Safe raw TTY uses `a/o/d/b`; line fallback uses `allow/open/deny/back` plus Enter; no second yes or direct-action `--confirm` |
| URL handoff | Always print exact URL on trusted-host and helper terminals; JSON retains it as a typed field |
| Access origin | Fresh `svc-<128-bit-random-lowercase-label>.localhost:<random-port>` only; no numeric/bare-localhost fallback |
| Clipboard | No default or public clipboard command in this slice |
| Browser | `service open --id`; `o` finalizes Allow, then invokes Open with only the confirmed exposure ref |
| Active inventory | `service status` is host-wide producer; helper `status` is current-attachment; review contains pending requests only |
| Observation | Complete delivery, `bounded_window`, fixed anchor, complete/partial/unavailable and bounded owner counts; reads never clean |
| Stop | Keep helper stop and add host `service stop --id`, both against the same ref kind and owner |
| Direct host expose | Excluded because CWD plus port cannot bind one unique live attachment |
| Host port | OS-selected random host port remains the only default and only first-slice mode |
| Retired reads | Replace host `service requests` and helper `list` with `service status` and helper `status`; no aliases |
| WP05 Host Loopback | Opposite direction; `host.tobari.internal` and its name are absent from Service parameters/output |
| Domain identity | WorkspaceManifestID + WorkspaceID + AttachmentID/epoch + principal/controller + target + exact resource identity |
| Manifest adoption | Desired revision changes and attached-blocked adoption leave current exposures unchanged |
| Migration | No durable Service migration; atomic private cutover after WP01's zero-live-attachment precondition |
| Integration | WP01+02 audit -> WP08 -> WP03 -> WP04 -> WP05 -> WP07 -> WP09, then fresh actual-interface re-baseline |
| Manifest/Runtime copy | Copy no request, exposure, attachment, observation, or reconciliation; use no lineage/provenance to select Service authority |
| Runtime lifecycle | Never cascade Service cleanup; active observed Workspace/container use is a fail-closed protection edge |
| Nested output refs | Consume WP08's one recursive Catalog traversal; add no Service-specific walker |

## Alternatives considered

### Keep the current single-shot review

This has the smallest code change and preserves the accepted security boundary,
but it does not address repeated command invocation, redundant one-row
selection, missing pending status, host-side active actions, JSON, or exit
feedback. It is the rollback behavior, not the recommended end state.

### Merge Service requests into Permission Inbox

A single visual inbox looks convenient, but Permission review stages durable
exact/template decisions and applies a complete set, while Service review acts
immediately on one attachment-local reference. A combined task would either
hide those semantics or create a heterogeneous authority-bearing selector.
The chosen design shares only the presentation state machine and keeps two
Catalog leaves.

### Add only `review services --watch`

Watch removes repeated host command entry, but the one-item
select/action/confirm sequence, active state, host stop/open recovery, public
field mismatch, and missing JSON remain. It is a useful implementation slice
but not the complete user outcome.

### Mix active exposures into the review selector

Rejected. Review is a pending-request decision task with one reference kind.
Mixing exposure Open/Stop rows would require heterogeneous selection authority
or V1 Catalog typed multi-selection groups. Host active discovery and actions
instead start from `service status`.

### Keep numeric `127.0.0.1` URLs and isolate by random port

Rejected. RFC6265bis states cookies do not provide isolation by port, so two
mutually untrusted services on one numeric host can share cookie scope. The
fixed per-exposure `.localhost` host separates browser origins while the socket
remains bound to `127.0.0.1`; numeric fallback and ad-hoc cookie rewriting are
both excluded.

### Automatically open the browser on every Allow

This saves one optional user action but conflates access creation with host
navigation and makes opener failure look like approval failure. It can also
create unwanted tabs for API servers. The chosen explicit `o` action keeps
ordinary Allow deterministic and terminal-first.

### Copy the URL automatically

Clipboard mutation is platform-dependent, overwrites ambient user data, and
adds another host side effect without closing an otherwise incomplete task.
Terminal output and explicit open are sufficient. A future clipboard action
would require its own user-visible contract and tests.

### Add `tobari expose PORT` on the host

This would be a trusted-host mutation, but the current runtime can have
multiple live attachment owners. CWD, Manifest name, Workspace label, and port
do not identify one exact owner. Rediscovery inside the action would violate
the action-binding rule; requiring an attachment reference would lengthen the
ordinary journey. It remains excluded.

### Make the helper return pending immediately

An asynchronous create would free the helper terminal but require a later
status/poll and change the fixed-target create result from a confirmed exposure
to a pending request. The current blocking call already withdraws safely on
cancel and delivers the final URL with zero joining. The chosen design adds a
parallel status read without changing default create completion.

## Concept count

### Public concepts retained or introduced

| Concept | Count | Reason |
|---|---:|---|
| Workspace service | 1 | The developer-owned target on Workspace loopback |
| Service request | 1 | Pending non-authoritative review resource with opaque ref |
| Service exposure | 1 | Active attachment-owned listener/relay resource with opaque ref |
| Service review | 1 task | Human task over typed request/exposure state; not a durable resource |

`pending`, `listening`, `relaying`, and `workspace_unavailable` are states, not
new resources. Browser open is an action on an exposure, not a Browser Session
resource. Host rendezvous, attachment epoch, controller, owner record, and
cleanup receipt remain internal concepts. The random `.localhost` label is one
immutable exposure field, not a public resource or lifecycle selector.

None of these are durable resources. ADR 0079's durable public concept
budget remains exactly Workspace Manifest, Runtime, and Workspace. Service
request/exposure are ephemeral attachment authority and are excluded from
Manifest desired/applied revisions and Workspace reconciliation history. They
store no Runtime binding or revision digest and cannot perform Runtime adoption.
Manifest/Runtime copy adds no Service concept, relation, or provenance field.
Runtime retirement protection is an external blocker observation, not Service
ownership or a Service lifecycle state.

### Concepts deliberately not created

- Unified Review Item or unified Review Decision
- Service name, route, publish profile, lease, or durable grant
- Browser target supplied by the caller
- Clipboard receipt
- Host-port reservation
- Docker-published service

## Owner, scope, lifetime, and mutability

| Thing | Owner | Scope | Lifetime | Mutability |
|---|---|---|---|---|
| Service request | live attachment owner | WorkspaceManifestID + WorkspaceID + attachment epoch + exact target port + opaque ref | until Allow, Deny, waiter withdrawal, or owner exit | state transition only through owner-validated decision/withdrawal |
| Exposure | live attachment owner | parent request identity + same trusted principal tuple + exact target + random host authority + opaque ref | until stop or owner exit | passive state/count changes; authority dimensions immutable |
| Review snapshot | host read task | explicitly observed live owners | one bounded observation | immutable result; watch replaces only with a fresh typed snapshot |
| Browser open | trusted host user through Service action | one active exposure ref and its owner-derived root URL | one attempt | no exposure mutation; no retained browser state |
| Cleanup receipt | attachment owner | one closing attachment | returned once after teardown | immutable counts/result |
| Permission decision | existing policy owner | unchanged durable permission policy scopes | unchanged | outside this task |

The reviewer, status command, browser, and helper do not own request or
exposure lifetime. A concurrent attachment cannot inherit or extend another
owner's exposure. Workspace Manifest revision publication and Workspace
desired/applied transitions do not mutate this table.
Manifest/Runtime copy does not clone any row. Runtime delete/prune/restore/build
cannot mutate a row; when the relevant Workspace/container is live, destructive
Runtime work fails closed in WP03's protection graph.

## Authority identity and presentation identity

### Authority identity

- request action: opaque `service-request` ref resolved to exact
  WorkspaceManifestID, WorkspaceID, attachment epoch, target port, and pending
  owner
- exposure action: opaque `service-exposure` ref resolved to exact owner,
  parent request, WorkspaceManifestID, WorkspaceID, attachment epoch, target
  port, bound host port, URL, and active state
- browser action: caller supplies only the exposure ref; owner-derived URL must
  validate against the exact structural exposure locator (`scheme = http`,
  generated authority, `path = /`) before the opener boundary
- every owner action revalidates live peer, nonce, attachment, snapshot, and
  unchanged ref immediately before side effect
- a recommended Manifest draft, Manifest name, revision digest, generation,
  ProjectRoot, or detached Workspace cannot create or substitute for the live
  attachment principal tuple
- copy source, derivation provenance, Runtime source/history, retirement plan,
  Docker observation, URL, host port, connection count, and timestamps cannot
  create or substitute for the attachment principal tuple
- the helper accepts no Manifest selector or project file; it receives the
  host-owned CLI-managed WorkspaceManifestID/WorkspaceID binding only through
  the trusted selected attachment
- access authority is the exact generated hostname plus assigned port; lifecycle
  mutation authority is the independent opaque exposure ref. Neither may be
  reconstructed from the other outside the live owner

### Presentation identity

- Human cards lead with canonical `project_root`, optional Manifest display
  name, Workspace service `127.0.0.1:PORT`, symbolic “confirmed exposure URL”,
  protocol, per-exposure-origin fact, random-port fact, and attachment lifetime.
- Public machine and Service protocol identity uses
  `workspace_manifest_id`, `workspace_id`, and `project_root`; authority also
  carries attachment epoch. Target V1 has no semantic `context_id`,
  `project_id`, `instance_id`, or dual-reader fallback.
- Human routine views do not lead with Workspace Manifest ID, Workspace ID,
  attachment epoch, request ref, or exposure ref, but exact actions and JSON
  retain the opaque ref and applicable scope facts.
- Same root label, same target port, same URL text, ordering, and indentation
  never create identity or transfer a staged/action selection.
- WP05's Host Loopback label and `host.tobari.internal` are the opposite
  Workspace-to-host direction and never appear in Service parameters, aliases,
  hostnames, authority, or output.

## Public contract

### Commands and roles

| Command | Role/effect | Contract after implementation |
|---|---|---|
| `tobari-expose PORT [--format text\|json]` | act, fixed-target create | Submit one exact target, emit pending guidance on stderr, wait for one result, and return a confirmed exposure on success |
| `tobari-expose status [--format text\|json]` | discover/read | Replace helper `list`; return complete current-attachment pending and active state with exact stop refs for active rows |
| `tobari-expose stop EXPOSURE_REF [--format text\|json]` | act, reference-bound write | Close one exact current-attachment exposure and relays |
| `tobari-expose help ...` | utility/read | Derive helper contracts from the Program-filtered global Catalog |
| `review services [--watch] [--notify auto\|osc9\|bel\|off] [--format text\|json]` | discover/read shell | Return bounded pending requests and request refs only; trusted interactive text TTY may delegate exact actions; redirected/JSON is read-only |
| `service status [--format text\|json]` | discover/read | Replace `service requests`; return host-wide bounded pending requests and active exposures with both ref kinds |
| `service allow --id REQUEST_REF [--format text\|json]` | act, reference-bound create | Preserve current Allow once semantics and return the exposure ref/URL |
| `service deny --id REQUEST_REF [--format text\|json]` | act, reference-bound write | Resolve one pending request without listener creation |
| `service open --id EXPOSURE_REF [--format text\|json]` | act, reference-bound write | Revalidate one active exposure and request one exact host-browser navigation; no access change |
| `service stop --id EXPOSURE_REF [--format text\|json]` | act, reference-bound write | Ask the live owner to close one exact exposure from the trusted host |
| `status [--manifest NAME] [--format text\|json]` | utility/read | Consume a matching CWD-scoped bounded counts/attention/observation summary with no Service refs, URLs, ports, actions, repair, or lifecycle mutation |

`service requests` and `tobari-expose list` are retired before first public V1
rather than retained as aliases. `service allow` remains browser-free.
`service open` accepts no URL, path, query, fragment, port, Workspace selector,
attachment selector, or browser executable. Direct action commands add no
redundant `--confirm` flags.

### Reference flow

| Reference kind | Producers | Consumers |
|---|---|---|
| `service-request` | `review services`, `service status` | `service allow`, `service deny` |
| `service-exposure` | helper create/status, `service status`, `service allow` | helper stop, `service open`, `service stop` |

The Catalog validates the global graph across both Programs. `review services`
retains exactly one selection reference kind, `service-request`; active Open/
Stop commands are discovered through `service status`. V1 neither weakens that
invariant nor adds typed multi-selection groups.
Nested producers/consumers use the Catalog-wide recursive schema traversal in
[Architecture](../../02_architecture.md) and [Harness](../../04_harness.md);
this slice supplies Service fixtures to that invariant and does not create a
parallel validator.

### Operation contracts

- Helper request remains fixed target
  `service-attachment-services/current-attachment-services`, `EffectCreate`,
  empty target inputs, access change `no`, and one confirmed child exposure
  ref.
- Allow remains `EffectCreate`, target kind `service-exposure`, required
  `parent_input=--id` of kind `service-request`, access change `yes`.
- Deny remains `EffectWrite`, target kind `service-request`, required
  `target_id_input=--id`, access change `no`.
- Open is `EffectWrite`, target kind `service-exposure`, required
  `target_id_input=--id`, notification `yes`, access change `no`, destructive
  `no`. The mutation policy permits only the narrow Service opener port.
- Host and helper stop are `EffectWrite`, target kind `service-exposure`,
  required opaque target input, access change `yes`, destructive `no`.
- Direct Allow/Deny/Open/Stop commands rely on their exact opaque target binding
  as deliberate invocation and declare no extra confirmation flag.
- `review services` and `service status` are complete delivery with
  `bounded_window` coverage over one fixed owner-registry anchor. They have no
  mutation target and create no files, locks, listeners, browser effects,
  record cleanup, or repair on fresh or stale state.
- `--watch`/`--notify` require trusted interactive text TTY operation and
  conflict with JSON or redirection before the owner registry is read.

### Human interaction contract

For one request with an already-running watch:

1. Workspace runs `tobari-expose 3000`; the helper displays `Pending trusted-host review` and waits.
2. Host watch receives a new typed ID and fixed evidence-free cue.
3. The screen directly shows the full card; user presses `a` for Allow once,
   `o` for Allow once then Open, `d` for Deny, or `b` for Back. That key is the
   explicit confirmation.
4. Owner revalidates the request and binds random `127.0.0.1:0`.
5. Both terminals receive the exact URL/ref/stop guidance. `o` then invokes
   the separate open action. The watch returns to the updated snapshot.

For one request without a watch, the host first invokes
`tobari review services`, then follows steps 3-5. With multiple requests, the
host first selects one typed row; selection is retained only while that exact
ID survives a fresh snapshot.

If safe raw-key input is unavailable on an otherwise trusted interactive text
TTY, the same card accepts only `allow`, `open`, `deny`, or `back` plus Enter.
It does not ask a second yes/no question. Redirected or JSON operation renders
the snapshot and never accepts an action. Active Open/Stop follow-up starts
from `service status`, not the pending-request selector.

The full card states:

- canonical project root and Manifest display identity;
- Workspace target `127.0.0.1:PORT`;
- host access uses the exact generated per-exposure
  `svc-<random-label>.localhost:<random-port>` origin over a listener bound only
  to `127.0.0.1`;
- HTTP/1.1 and WebSocket Upgrade only;
- exact generated Host and port are required before Workspace I/O; numeric,
  bare-localhost, sibling-label, wrong-port, malformed, and DNS-rebinding Host
  values are rejected;
- accepted Host, headers, cookies, Origin, redirects, and content are not
  rewritten;
- lifetime is the owning attachment;
- Allow, Allow & open, Deny, and Back consequences.

### JSON and typed state

Add public JSON V1 for Service reads and mutations. Use exact envelopes rather
than parsing text. Keep four task-owned shapes rather than one optional-field
envelope:

- `review services`: bounded pending `requests` only, producing request refs;
- `service status`: host-wide bounded `requests` and `exposures`, producing
  both host action ref kinds;
- helper `status`: current-attachment pending and active arrays, omitting host
  mutation refs from child-visible pending rows and retaining exposure refs for
  helper Stop;
- CWD `status`: counts, attention, and observation only, with no Service ref,
  URL, port, or action.

Host discovery results declare task identity, fixed anchor/scope,
`bounded_window` coverage, observation state `complete|partial|unavailable`,
bounded `observed_owner_count` and `unavailable_owner_count`, and always-present
task-owned arrays. Rows carry the applicable public `workspace_manifest_id`,
`workspace_id`, `project_root`, AttachmentID/epoch projection, target/state,
and only the opaque refs allowed for that task. Exposure rows include the exact
generated `.localhost` URL, random host port, connection count, and passive
state.

Known empty arrays are not `null`. Known empty is valid only when the anchored
registry scan succeeds and finds no in-scope owner. Partial/unavailable cannot
collapse to empty; unsafe or contradictory authority fails rather than becoming
a partial success.

The semantic requirements above are fixed, but the exact Go field locations,
schema composition, and Catalog envelope must be re-read from the landed WP01
implementation. This plan does not prescribe a parallel Service-only identity
schema to get ahead of that implementation.

Helper create JSON writes no partial object while pending; pending guidance
stays on stderr, and stdout receives one final exact success envelope. JSON
review remains read-only even on a TTY. Watch is human text/raw-terminal only.

### Status integration

- `tobari-expose status` is exact current-attachment scope and includes all
  pending target ports plus all active exposure rows.
- Host `service status` returns the installation-wide anchored snapshot; CWD
  `status` filters only the bounded summary by the already-resolved
  stable WorkspaceManifestID and WorkspaceID. It does not resolve by root,
  Manifest name, desired revision, or runtime label after the application
  result is formed.
- CWD status composes ADR 0079 desired, last-successfully-applied,
  observed, and bounded-failure facts without changing them. Service attention
  is an observed attachment sub-result only; it is never projected as Manifest
  desired/applied state or an adoption condition.
- Human CWD status adds a compact `Services` section only from validated typed
  facts: bounded pending/active counts, attention, and observation state. It
  emits no Service ref, URL, port, or action.
- Status JSON retains a typed bounded summary and observation uncertainty, not
  the Service discovery arrays. `review services`, `service status`, helper
  create, and helper `status` are the Catalog-declared reference producers.
- If the owner observation is unavailable, status says unavailable; it does not
  claim zero pending or active services.
- Status performs zero listener, control-socket, rendezvous repair, stale-record
  cleanup, principal, Docker, AppliedEntry, or failure-record mutation. Only
  explicit Workspace entry reconciles runtime; only Service Allow creates a
  listener.
- Session-close cleanup returns bounded counts only after pending withdrawal,
  listener admission closure, bounded relay termination/drain, and confirmed
  exposure closure. The best-effort stderr summary never prints IDs, URLs,
  ports, application data, secrets, or stale post-exit authority.

### Browser and terminal separation

- `service open` resolves only an exposure ref through the live owner and
  derives and validates the exact active per-exposure `.localhost` root URL
  before one narrow infrastructure call.
- The Workspace helper, application server, HTTP response, redirect, page
  script, terminal hyperlink, and caller cannot invoke this port.
- The opener receives only the root exposure URL, no application path, query,
  fragment, cookies, headers, or browser executable.
- `o` means two ordered canonical outcomes: Allow, then Open. Confirmed Allow
  is emitted/finalized before Open. Open failure is an ancillary presentation
  failure and cannot make Allow retryable or stop the exposure.
- A provable failure before dispatch is `open_not_dispatched` and may retry only
  `service open`. Successful dispatch is `open_requested`, never “loaded,” and
  has no automatic retry. Cancellation/timeout with uncertain dispatch is
  `open_outcome_unknown`, non-retryable. All retain the confirmed exposure
  URL/ref for manual use.
- `a` never calls the opener. JSON/machine Allow never calls it. Browser
  navigation grants no Workspace HTTP authority.
- Both terminals always show the URL. Plain text remains usable without ANSI,
  OSC 8, clipboard, or desktop integration.

## Common presentation primitive and separate tasks

Extract or extend one CLI-owned review presentation primitive for:

- alternate-screen and raw-mode ownership/restoration;
- typed snapshot equality and stable-ID reconciliation;
- list, single-card, detail, Back, and explicit action transitions;
- safe raw-key detection and full-line `allow|open|deny|back` fallback;
- watch timer/backoff and unchanged-snapshot repaint suppression;
- last-valid-screen retention with actions disabled on stale observation;
- fixed evidence-free OSC 9/BEL attention cues;
- semantic style tokens and hostile external-text projection;
- writer failure and cancellation restoration.

Do not put task semantics in that primitive. Inject a task-owned model with
typed rows, stable IDs, actions, details, and transition results. Permission
review continues to own staging, template detail, final Apply, and durable
receipts. Service review owns immediate pending-request Allow/Deny delegation,
optional post-Allow Open composition, no staging, and no Apply. Independent
active Open/Stop discovery starts at `service status`. Neither task imports the
other's domain types or action references.

## Layer changes

- Domain: add a task-owned Service review/status snapshot, exact observation
  uncertainty, public projection vocabulary, cleanup receipt, browser-open
  request/result, and invariants for the
  WorkspaceManifestID/WorkspaceID/AttachmentID-or-epoch/principal/controller/
  exact-target scope and state. Keep
  Manifest revision/applied types, sockets, HTTP, terminal, and browser types
  out of Service authority. Reuse the ADR 0079 identity value types only after
  the implementation-entry gate confirms their actual integrated shape; do not
  duplicate them in Service.
- Application: keep `serviceexposurecmd` as the task owner; add ports for fresh
  review/status snapshot, host exposure stop, exact exposure URL resolution and
  purpose-limited open, and attachment cleanup receipt. Validate task identity,
  scope, ref kind, intent, and impact before infrastructure calls.
- Infrastructure: extend the private rendezvous snapshot to include active
  exposures, fixed registry anchors, owner counts, and typed observation
  failures without cleaning on reads; add host stop/open owner calls;
  atomically replace predecessor Context/project fields with
  WorkspaceManifestID/WorkspaceID/attachment epoch; preserve
  peer/nonce/owner validation; generate an independent 128-bit lowercase DNS
  label per exposure; enforce exact Host/absolute authority before Workspace
  I/O; expose a narrow Service browser adapter; and return cleanup counts. Do
  not reuse the browser-login socket, schema,
  registry, or authority. Begin this principal/protocol edit only after the
  complete upstream order and final WP09 re-baseline gate.
- CLI and catalog: replace helper list with status, add formats/commands, replace
  host `service requests` with `service status`, retain one review selection
  kind without typed multi-selection groups, implement the shared review
  presentation primitive, add the CWD summary/exit projection, consume WP08's
  recursive reference graph, and keep Program-filtered routing.

## Data and control flow

```text
Workspace helper
  -> validate exact non-privileged target port
  -> purpose-specific attachment socket
  -> use trusted WorkspaceManifestID + WorkspaceID + AttachmentID/epoch
     + principal/controller
  -> live owner creates non-authoritative service-request ref
  -> helper emits pending guidance and waits

trusted-host review/watch
  -> snapshot one bounded owner-registry anchor
  -> peer/UID/PID/nonce/attachment validation
  -> fresh pending requests + complete/partial/unavailable owner counts
  -> typed selection by service-request ref only
  -> explicit Allow / Allow then Open / Deny / Back

service status
  -> snapshot one bounded owner-registry anchor without cleanup
  -> fresh pending requests + active exposures + observation uncertainty
  -> produce request refs for Allow/Deny and exposure refs for Open/Stop

Allow
  -> canonical reference-bound create intent and impact
  -> live owner revalidates request
  -> bind tcp4 127.0.0.1:0
  -> generate independent 128-bit lowercase label
  -> create exposure ref + exact svc-<label>.localhost URL
  -> finalize confirmed mutation output
  -> wake waiting helper

optional Open
  -> consume exposure ref unchanged
  -> live owner revalidates active exposure and re-derives URL
  -> validate exact generated .localhost root URL
  -> one purpose-limited host opener attempt

host HTTP/WebSocket
  -> exact generated hostname + assigned-port Host/absolute-form validation
  -> reject numeric/bare/sibling/rebinding authority
  -> preserve accepted Host and ordinary headers/cookies unchanged
  -> Workspace stream to exact 127.0.0.1:target
  -> WebSocket opaque bytes only after 101

Stop or attachment exit
  -> close listener admission
  -> terminate/drain relays under fixed bound and confirm closure
  -> remove exposure/request/control/rendezvous state
  -> return bounded count-only cleanup receipt for exit presentation

Manifest revision or adoption state change
  -> no Service mutation
  -> existing attachment/request/exposure identity remains unchanged
  -> attached-blocked entry performs no Docker or Service cleanup

CWD status
  -> read desired + last-applied + observed + bounded-failure facts
  -> join bounded counts/attention/observation by trusted IDs
  -> emit no Service ref, URL, port, or action
  -> create/repair nothing
```

## Error and cancellation behavior

- Invalid port/ref/format/watch/notify/TTY combinations fail before owner,
  listener, registry, or browser calls.
- Helper cancellation withdraws only that waiting request when possible. It
  never stops an already confirmed exposure whose success is being finalized.
- Watch cancellation restores raw mode, cursor, and main screen; it performs no
  decision. Timer refresh never acts.
- A refresh failure retains the last valid screen as visibly stale, disables
  action keys, applies bounded backoff, and retries reads only. Initial failure
  returns the typed read fault.
- Unsafe registry metadata, duplicate authority, contradictory owner identity,
  and ambiguous refs fail the read/action; they are never hidden as partial
  rows or repaired by the read.
- Same-label replacement IDs lose selection. A stale request returns to a fresh
  Service review snapshot; it is never retried automatically.
- Host bind failure creates no exposure. The pending request outcome is typed;
  the helper receives a stable failure and exact request-again recovery only
  when replay is safe.
- Workspace target refusal after valid host request retains the fixed 502 and
  passive `workspace_unavailable` state; it does not remove approval or probe.
- Open precondition failures (`not found`, `stale`, invalid URL) make zero
  browser calls. Provable pre-dispatch platform failure is
  `open_not_dispatched` with safe `service open` retry. Successful dispatch is
  `open_requested` and does not claim load. Cancellation/timeout with uncertain
  dispatch is `open_outcome_unknown`, non-retryable; the exact URL remains the
  manual recovery.
- Allow-and-open reports Allow as confirmed even when Open fails. The exposure
  remains active and the command never suggests re-running Allow.
- Stop closes the listener/relays before removing owner state. Unclassified
  post-action errors are non-retryable and reconcile through helper status,
  or `service status`; CWD status is only a non-authoritative summary.
- Short writes after a confirmed create/write use
  `mutation_output_write_failed` and read-only reconciliation. Late
  cancellation does not turn success into replay permission.
- Notifications use fixed payloads only. Request/root/port/URL/ref/ID or other
  external text never enters OSC control payloads.
- Manifest revision publication, pending adoption, and attached-blocked entry
  are not Service cancellation or cleanup events. They leave the current
  attachment/exposure unchanged and cannot be used as implicit Stop recovery.

## Compatibility and migration

- Treat the current predecessor-main commands and the promoted WP01/WP02
  integration commits as observations, not a future WP09 implementation pin.
  Reconfirm the final pre-public
  compatibility baseline after every fixed upstream packet completes and
  before assigning schema versions, field removals, or generated diffs.
- Service requests, exposures, owner records, sockets, and listeners are
  ephemeral. There is no durable service state to migrate.
- Replace the private control/rendezvous predecessor shape atomically in one
  binary and helper source closure. Its authority fields become
  WorkspaceManifestID, WorkspaceID, and attachment epoch with no Context,
  project, or instance compatibility fallback. A running old attachment is not
  upgraded in place; exit/re-entry closes it. New reviewers reject stale
  incompatible records without following paths or cleaning them during reads.
- Add public JSON V1 as the first success JSON contract for this capability;
  no legacy JSON reader or alias is introduced.
- Final public agent/JSON fields use `workspace_manifest_id`, `workspace_id`,
  and `project_root` before first public V1. The private Service protocol is
  part of the atomic principal-field cutover; it does not retain semantic
  `context_id`, `project_id`, or `instance_id` fields.
- ADR 0079 migration may retain predecessor Context UUID bytes as
  WorkspaceManifestID and ProjectInstance UUID bytes as WorkspaceID, but no
  request, exposure, listener, socket, owner record, or attachment epoch is
  migrated. New Service state is created only after attachment re-entry.
- WP01 migration already requires zero live attachments. End attachments
  through ordinary teardown, verify none remain, migrate durable identity,
  perform explicit post-migration activation, then re-enter and request again.
  Migration never translates or silently stops Service state.
- Retire host `service requests` and helper `list` in favor of their separate
  `service status` and helper `status` tasks with no aliases, fallbacks, or
  dormant handlers. Update every recovery action and reference workflow.
- Existing `tobari-expose PORT` and `stop EXPOSURE_REF` grammar remains source-
  compatible aside from the optional success-format flag.
- If implementation begins after the first public V1 freeze, reconsider these
  breaking choices and add an explicit compatibility decision before code.
- `manifest create --copy-from` publishes a fresh identity with no Workspace,
  attachment, Service request/exposure, applied/failure/observed state, or
  reconciliation. `runtime create --copy-source-from` likewise copies no
  attachment or Service state. No provenance/lineage field or `--base` alias is
  read as a compatibility bridge.
- Runtime build/restore/delete/prune migration and sidecar state do not migrate
  or clean Service state. Active attachment/exposure presence is preserved; a
  destructive Runtime action must instead fail closed through WP03's complete
  Workspace/container protection. Service observations do not supply
  `last_used` or migration evidence.

## Implementation-entry gate after all upstream packets

This gate is mandatory and precedes every production/test/schema/Catalog/
helper/public-guard/generated edit for Service Exposure:

1. Observe the fixed order: WP01 + WP02 completion audit, WP08 completion, WP03
   completion, WP04 completion, WP05 completion, WP07 completion, then WP09.
   Record each completion notification/evidence; design acceptance alone is not
   an implementation handoff. WP06 waits to consume WP09's summary.
2. Fetch `origin/main`, record `HEAD`, upstream/nominated integration commit,
   branch, and complete `git status`. Preserve every unrelated change. If a
   Service/identity/schema/Catalog/helper path is still changing or owned by
   another implementation, stop rather than merge assumptions into it.
3. Re-read `AGENTS.md`, ADR 0079, docs 00--04, the `07535a9`/`428812f`
   promotion evidence, and every final upstream interface used here:
   Workspace/Manifest identity, migration, promoted copy isolation, WP08
   recursive Catalog traversal, WP03 Runtime
   protection, WP04 build/research boundary, WP05 opposite-direction hostname/
   naming, WP07 permission-wait IDs/policy, attachment/controller/registry,
   Service domain/application/infrastructure, helper closure, JSON/status, and
   tests.
4. With fresh temporary XDG roots and no live mutation, rerun current binary or
   source help, empty Service review/status, JSON/schema fixtures, and any
   bounded read-only migration/status diagnostics needed to observe the landed
   contract. Do not create listeners, attachments, Docker state, or browser
   effects merely to pass the gate.
5. Record the evidence in `context.md` and compare it explicitly with the fixed
   authority tuple, task partition, reference graph, bounded observation,
   `.localhost` origin, Open outcomes, cleanup order, schema, and no-migration
   contract. A contradiction sends `WP09_BLOCKED` to the control thread; it is
   not authority to choose a local fallback.
6. Confirm one implementation owner and dependency order for every overlapping
   production/schema/Catalog/helper file. Do not partially reproduce or edit an
   upstream mechanism from WP09.

## Implementation dependency order

1. Pass the all-upstream gate and promote ADR 0074 plus thesis/product/
   architecture/security/harness consequences before mechanism.
2. Freeze presentation-independent fixtures and answer keys for anchored empty,
   one/many pending, active, complete/partial/unavailable, owner races,
   generated origins, Open outcomes, and cleanup. Add failing domain,
   application, Catalog, schema, TTY, relay, browser, and reference tests.
3. Add pure domain vocabulary for the fixed authority tuple, separate review/
   host-status/helper-status/CWD-summary shapes, bounded observation, exact
   access origin versus lifecycle ref, cleanup receipt, and Open outcomes.
4. Add application ports/use cases for anchored reads, fresh exact action
   re-resolution, Allow-before-Open ordering, host Stop, and cleanup ordering.
5. Extend infrastructure atomically: fixed owner-registry anchors with no read
   cleanup; principal/ref revalidation; independent random origin labels;
   exact Host/framing gate; listener-first bounded teardown; and purpose-limited
   platform opener.
6. Implement Catalog/CLI partition exactly: pending-only review with raw/line
   confirmation and watch/notify gating, host `service status`, helper `status`,
   direct exact actions without confirm flags, distinct schema-1 envelopes, and
   ref-free CWD summary, consuming WP08's traversal.
7. Hard-cut over the private host/helper protocol and embedded helper source in
   one change; end/re-enter old attachments and preserve source-byte equality.
8. Run supported-browser cookie/origin and Engine/Desktop/Colima readiness
   evidence. Any failed isolation assumption reports `WP09_BLOCKED` without
   numeric fallback or cookie rewriting.
9. Complete focused/repository gates, synchronize generated/public/release
   artifacts, commit, remove this temporary packet, and notify the control
   thread with `WP09_IMPLEMENTATION_COMPLETE` or `WP09_BLOCKED` plus final
   interfaces, gates, HEAD/status, retention, and WP06 readiness.

## Verification

- Unit and contract tests: states, scope, observation uncertainty, cleanup
  receipt, exact public vocabulary, command roles/effects, one request-selection
  kind, explicit absence of typed multi-selection groups, and raw/line one-step
  confirmation transitions.
- Negative side-effect tests: invalid/stale/foreign refs, same-label
  replacement, missing owner, wrong peer/nonce/attachment, malformed records,
  unavailable watch snapshot, unsafe registry metadata, duplicate authority,
  JSON/redirected watch/notify, and wrong URL create zero listener/browser
  calls and no read cleanup.
- Opaque-reference tests: request allow/deny and exposure open/stop round trips
  across host/helper Programs; no port, URL, root, row, or label reconstruction.
- Relay tests: require exact generated Host/absolute authority; reject numeric
  loopback, bare localhost, sibling labels, absent/duplicate/malformed Host,
  wrong port and DNS rebinding; retain framing/smuggling, keepalive, accepted
  Host/headers/cookies/Origin/redirect/content pass-through, WebSocket `101`,
  half-close, backpressure, limits, fixed 502, stop, and owner exit.
- Browser tests: exact owner-derived root URL only, no path/query/fragment,
  Allow-only zero calls, Allow-and-open ordering, opener failure after confirmed
  Allow, `open_not_dispatched|open_requested|open_outcome_unknown`, manual URL
  recovery, cancellation, no Workspace/browser channel reuse, cross-exposure
  cookie isolation, and `Domain=localhost` sibling-contamination rejection
  under the supported browser floor.
- Watch/presentation tests: raw and line fallback, single-item direct card,
  multiple-item selection, stable-ID reconciliation, stale action disablement,
  unchanged repaint suppression, bounded backoff, fixed cue payload, hostile
  fields, writer failure, and terminal restoration.
- JSON/schema tests: exact envelopes and nested keys, known-empty arrays,
  complete/partial/unavailable distinction, bounded observed/unavailable owner
  counts, distinct review/host-status/helper-status/CWD-summary schemas,
  WorkspaceManifestID/WorkspaceID/AttachmentID-or-epoch scope, public names,
  mutation receipts, and negative predecessor aliases.
- Status/exit tests: CWD identity filtering against ADR 0079 desired,
  applied, observed, and bounded-failure variants; zero-write fresh state;
  zero listener/rendezvous/record cleanup; concurrent owner close; pending-only
  review refs; host/helper status scoped refs; CWD no ref/URL/port/action; and
  post-cleanup bounded counts without stale authority.
- Adoption tests: Manifest revision publication and attached-blocked adoption
  make zero Service calls and preserve the current listener/relay; explicit
  Stop and attachment exit remain the only Service cleanup paths.
- Copy-isolation tests: Manifest copy and Runtime source copy create fresh IDs
  but no attachment epoch/controller, request, exposure, listener, observed
  Service row, copied provenance selector, or reconciliation call; the source
  attachment/exposure remains unchanged.
- Runtime-lifecycle integration tests: build/restore/delete/prune never call
  Service Stop or attachment cleanup, never re-key an exposure, fail closed
  when the WP03 protection graph observes a live Workspace/container, and do
  not infer `last_used` from Service URLs, traffic, connections, or timestamps.
- Migration tests: retained UUID bytes map only to WorkspaceManifestID and
  WorkspaceID; active Service state is not encoded, copied, dual-read, or
  silently stopped; WP01's zero-live-attachment precondition, atomic private
  protocol cutover, helper byte equality, and exit/re-entry are enforced.
- Asset/runtime tests: Program hardcoding, source/snapshot equality, pinned
  builder, engine architecture, owner-only host copy, read-only standard/custom
  mount, and live helper status/request/stop.
- Agent-readiness scenario: known path uses one helper task plus one host task
  or pre-running watch and one action; zero guesses, zero identifier
  reconstruction, zero automatic probe, and zero external processing.
- Human-handoff scorecard: compare current single-shot, watch, merged inbox,
  direct host expose, terminal URL, clipboard, ordinary Allow, and explicit
  Allow-and-open. Safety/certainty steps remain separately justified.
- Manual observation: synthetic Vite, Next.js, Storybook, Jupyter, HTTP, and
  WebSocket fixtures on supported Engine/Desktop/Colima and supported browsers;
  record no browser history, project content, cookie values, or credentials.
- Required profiles: focused Go tests, helper source check, integration,
  `task check`, `task security`, `task public:check`, and
  `task release:check`.
- Generated-diff or artifact checks: helper closure, Catalog help, capability
  ledger, schema ledger, architecture site and Japanese route parity where
  governed, public snapshot, narrow generated-authority public-guard exception
  positive/negative tests, and `git diff --check`.

## Rollout and rollback

This is a pre-public, ephemeral-state change. Rollout requires ending old
attachments and entering again with the matching helper/host binary. No
request, listener, stream, or pending result survives that transition.

Coordinate this rollout with ADR 0079 migration. Ending attachments is
ordinary Service teardown, not migration of Service state. A Manifest revision
change without migration does not require this teardown and must not be used to
trigger it.

Do not start this rollout until the complete fixed upstream order has finished
and the actual-interface gate has passed. Any overlapping Catalog/schema/helper
edit requires one sequenced owner; no upstream mechanism is partially
implemented by this Service slice.

Safe rollback removes watch/status/open/host-stop presentation and private
protocol additions together only before any matching attachment is active,
then restores the single-shot review surface. It must not restore or publish a
numeric-loopback access URL, broaden Host acceptance, leave a dormant browser
action/alias/incompatible owner record, or keep half of the origin hard-cutover.
Any active exposure closes through ordinary attachment teardown first.

## Documentation promotion

- Revise the North Star/Thesis 8 consequence to distinguish shared review
  presentation mechanics from separate authority tasks and to describe the
  shorter Service sequence.
- Revise product command/helper tables, public vocabulary, status/exit output,
  JSON, and browser behavior.
- Consume Workspace Manifest/Workspace terminology and the principal field
  cutover from ADR 0079 and docs 00--04; do not create a Service-specific alias or
  second identity registry.
- Revise architecture for typed Service snapshots, active host actions,
  cleanup receipts, and the separate purpose-limited opener port.
- Revise security for browser exact-ref validation, watch stale-state behavior,
  per-exposure origin/Host/cookie isolation, public uncertainty, and opener
  failure ordering.
- Revise ADR 0074; do not create a local exception that contradicts its current
  automatic-browser exclusion.
- Add/adjust harness claims for the common review primitive, JSON/status
  conformance, browser/origin separation, cleanup receipt, and public-guard
  enforcement.
- Update the public guard through its governing contract with one narrow exact-
  pattern exception for Tobari-generated Service authorities: fixed `svc-`
  prefix, exact lowercase random-label grammar/length, exact `.localhost`, and
  validated port/locator structure where applicable. Do not allow arbitrary
  subdomains, bare localhost, `.local`, private hosts, caller URLs, or a broad
  suffix/wildcard. Add positive and negative tests proving the exception cannot
  mask private-network text.
- Update README, agent readiness, architecture site, capability/schema ledgers,
  public/release contracts, and `add-capability` instructions if the Catalog
  interactive model changes.

## Remaining implementation-time evidence

- Final post-WP07 HEAD/worktree and exact upstream interface inventory.
- Exact anchored-owner close/unavailable race protocol and bounded counts.
- Supported safe-raw-key detection and full-line fallback behavior.
- Platform evidence distinguishing Open pre-dispatch failure, requested
  dispatch, and uncertain outcome.
- Supported-browser floor and cookie isolation evidence; contradiction is
  `WP09_BLOCKED`.
- Exact finite relay drain/termination bound and cleanup count ceilings.
- WP06's exact placement/rendering of the already-fixed ref-free summary.
- Exact narrow public-guard matcher grammar/length and tests, approved through
  the governing contract; it may recognize only the Tobari-generated exposure
  authority and must not mask other private-network text.
