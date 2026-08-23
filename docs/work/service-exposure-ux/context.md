# Work Context: Shorten reviewed Workspace service exposure

This file separates verified facts from evaluation, unknowns, and design
inference. Desired behavior is not described as current behavior.

## Baseline

- Historical predecessor-main observation, verified on 2026-08-23 after
  `git fetch origin main`.
- Local `HEAD`, `origin/main`, and `FETCH_HEAD` were all
  `6a26a3c274d2c2ce8dc8c59321ffb7ba67594b42`.
- `git status --short --branch` reported `## main...origin/main` with no
  existing worktree changes.
- The current service capability entered main in `1b2341c` and trusted-host
  review tasks were separated in `3c9bd15`.
- These facts describe the predecessor UX that motivated this packet. They do
  not freeze `6a26a3c` as the future Service implementation base.

### Shared integration checkout observation

- Re-observed on 2026-08-23 after WP01 and WP02 promotion. The shared checkout
  is on `codex/workspace-manifest-v1` at
  `52a53bcc69a0f2bdf9bf2a6782ecd98bacd8b0e1`; both `07535a9` and `428812f`
  are ancestors of that HEAD. `origin/main` remains the predecessor
  `6a26a3c274d2c2ce8dc8c59321ffb7ba67594b42` and is not the integration
  authority.
- The promoted authority is
  [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md)
  together with [Theses](../../00_theses.md),
  [Product contract](../../01_product_contract.md),
  [Architecture](../../02_architecture.md),
  [Security model](../../03_security_model.md), and
  [Harness](../../04_harness.md). Commit `07535a9` records the model/copy
  promotion and `428812f` records the associated ADR/security/migration
  quarantine evidence.
- The former WP01 and WP02 temporary packet directories contain no files and
  are not authority. `git status --short --branch` currently reports only the
  unrelated untracked `scripts/__pycache__/`; this packet sync does not touch
  it.
- This integrated observation still is not the future WP09 implementation
  baseline. The implementer must wait for the complete fixed upstream order
  and inspect the actual nominated integration HEAD and owned worktree after
  WP07 handoff.
- The current `tools/repoguard` private-network expression has an unanchored
  `.local` alternative inside full HTTP(S) locator matching, so a full generated
  `.localhost` URL is presently misclassified as a private hostname. This
  packet records the locator structurally as separate scheme, authority, and
  path and uses “confirmed exposure URL” in wireframes. No guard or production
  change belongs to this packet-only update.

## Verified current behavior

### Public journey and interaction

- `tobari-expose PORT` is a dedicated hardcoded helper Program mounted
  read-only inside an attached Workspace. It writes
  `Run on the host: tobari review services` to stderr and blocks until Allow,
  Deny, cancellation, or attachment closure.
- The control agent detects a closed helper connection and sends a typed cancel;
  the attachment owner withdraws pending requests belonging to that helper
  client. Pending lifetime is therefore attachment-bounded and currently also
  waiter-bound.
- A single-request TTY journey currently requires: invoke
  `tobari review services`, enter row `1`, enter `a`, then enter `y`. Deny uses
  the same selection/action/confirmation sequence. Back and a negative
  confirmation mutate nothing.
- `review services` reads once. An empty snapshot exits successfully; a
  successful Allow or Deny also exits rather than watching for later requests.
- Redirected `review services` is read-only and prints the exhaustive request
  snapshot. It does not accept `--format`; success output is text only.
- `tobari-expose list` reports active current-attachment exposures only.
  `tobari-expose stop EXPOSURE_REF` consumes the opaque reference unchanged.
- `status` does not read or display service requests or exposures. The session-
  closed stderr summary reads pending network permissions but does not report
  withdrawn service requests or closed service exposures.
- README explicitly says Tobari does not open the browser automatically and
  instructs the user to copy or use the numeric-loopback URL.

### Domain and Catalog identity

- In the current-main predecessor, `ServiceRequest` retains schema version,
  `srq_...` ID, attachment epoch, internal project and Context IDs, canonical
  root display value, exact target port, and state. A request snapshot accepts
  only `pending` rows.
- In that predecessor, `ServiceExposure` retains an `exp_...` ID, parent
  request ID, attachment, project and Context IDs, canonical root, target and
  host ports, exact URL, passive state, and connection count. These legacy
  names are verified current facts, not target V1 vocabulary.
- The exact active URL invariant is
  `http://127.0.0.1:<owner-bound-host-port>`.
- The helper request is a fixed `tool_local` create in
  `service-attachment-services` scope. It consumes no reference and produces
  a confirmed child `service-exposure` reference.
- `service allow --id` is a reference-bound create whose required parent input
  is one `service-request` reference. `service deny --id` and helper `stop` are
  reference-bound writes.
- The global Catalog validates reference closure across `tobari` and
  `tobari-expose`, while routing and help are filtered by Program. The helper
  cannot dispatch host review commands.
- Current scoped agent help declares public fields named `project_id` and
  `workspace`, even though the product contract says public Workspace identity
  and root facts use `workspace_id` and `project_root`. The command currently
  has no JSON success renderer, but these field names are already visible in
  its agent contract.
- `review services` has one `InteractiveWorkflowContract` selection reference
  kind (`service-request`) and action commands `service allow` and
  `service deny`. The current contract cannot describe one screen containing
  independent request and exposure action groups without extension.

### Promoted Workspace Manifest and applied Workspace authority

- The product owner has selected **Workspace Manifest** as the public durable
  desired declaration, routine **Manifest** presentation, `manifest` and
  `--manifest` CLI spelling, and `workspace_manifest_id` schema identity.
  Context has no target-V1 alias.
- Workspace Manifest is host-owned, CLI-managed, stable-ID authority. A
  project-owned YAML/JSON file, generic import, `apply -f`, Workspace input, or
  helper flag cannot supply or change the Manifest binding used by Service.
- The durable public resource budget is Workspace Manifest, Runtime, and
  Workspace. Service request and exposure remain attachment-owned ephemeral
  resources; they do not add a durable resource and are never Manifest desired
  or Workspace applied state.
- Target Service authority is the trusted tuple
  `(WorkspaceManifestID, WorkspaceID, AttachmentEpoch)` plus the request- or
  exposure-specific facts and opaque ref. The predecessor Context/project
  fields must be atomically replaced, never normalized or accepted as a
  fallback by ordinary readers.
- A first-use recommended Manifest draft is presentation only. It has no stable
  WorkspaceManifestID or attached Workspace and therefore cannot request,
  review, or own Service exposure.
- Each accepted Manifest mutation publishes one complete immutable desired
  revision while preserving the Manifest Boundary. That publication does not
  move or re-key a request/exposure, update its authority, reconcile the
  Workspace, or change an existing attachment; a Boundary change creates a
  different Manifest/Workspace binding rather than transferring Service state.
- Only explicit Workspace entry may reconcile Workspace runtime. If adoption
  requiring recreation is blocked by an attachment, the current attachment,
  development server, listeners, and exposures continue unchanged.
- CWD status consumes ADR 0079's desired, last-successfully-applied,
  observed, and bounded-failure facts read-only. Service attention is nested
  observed attachment state and cannot create a listener, repair rendezvous,
  record applied state, or clear a failure.
- The accepted pre-public migration direction retains predecessor Context UUID
  bytes as
  WorkspaceManifestID and ProjectInstance UUID bytes as WorkspaceID, but
  Service records are ephemeral and are not migration input or output. Exact
  migration attachment/cluster preconditions are governed by ADR 0079 and the
  durable security/architecture contracts.

WP01 is promoted and integrated rather than a future packet dependency. The
vocabulary and invariants above come from ADR 0079 and docs 00--04. Exact
Service-facing field layout, Catalog projection, attachment ownership protocol,
and any later upstream changes still require the fixed implementation-entry
re-baseline; this packet must consume the final integrated form rather than add
a parallel Service-local model.

### Confirmed derivation/copy and Runtime-retirement boundaries

- WP02's fixed copy semantics are promoted in ADR 0079 and the durable
  architecture/product contracts, with `07535a9` as integration evidence.
  Manifest copy is `manifest create --copy-from NAME --name NAME`; it reviews
  and revalidates the exact immutable current revision immediately before
  publishing a fresh WorkspaceManifestID at generation 1. It copies no
  Workspace, authentication, learned permission, attachment authority,
  desired/applied/failure/observed state, current selection, Service request,
  or active exposure, and starts no reconciliation.
- Manifest copy persists and displays no provenance, lineage, or
  `copied_from`. Service therefore cannot use a copy relation, source name, or
  common derivation type to locate an attachment. Existing requests/exposures
  remain solely with the source live attachment; the fresh Manifest has none.
- Runtime source copy is `runtime create --copy-source-from standard|NAME
  --name NAME`. It creates a fresh RuntimeID with empty history from current
  editable source, copies no Workspace or attachment state, and starts no
  reconciliation. `--base` has no alias. These choices add no Service authority
  field or Service workflow dependency.
- WP03 is also Product-Owner design-fixed but has no implementation-start
  notice. Runtime build becomes exact-reference-bound; retirement/prune/restore
  act on Runtime authority and execution material. They must not cascade Stop,
  delete, re-key, or recreate an attachment-owned Service exposure.
- An active attachment or observed running/stopped/foreign container belongs in
  WP03's complete fail-closed protection graph. Service state is not a cleanup
  target: URL, host port, connection count, owner record, or Service timestamp
  cannot authorize Runtime lifecycle mutation or manufacture `last_used`.
- Recursive nested output-reference producer/consumer derivation is a single
  Catalog-wide invariant coordinated with
  `catalog-domain-output-conformance`. Service may consume it but must not add a
  Service-only walker or validator.

### Attachment ownership and trust boundary

- The live attachment owner creates the request IDs, exposure IDs, random host
  listeners, control process, pending map, active-exposure map, relay streams,
  rendezvous socket, and ephemeral record.
- The reviewer validates regular owner-only records, peer PID and UID, nonce,
  owner PID, attachment identity, and a fresh owner snapshot. It never owns the
  listener or attachment lifetime.
- The Workspace helper can request only one exact non-privileged target port,
  list its current attachment, or stop one unchanged exposure reference. Its
  adapter explicitly rejects host request discovery, Allow, and Deny.
- Allow binds `tcp4` at exact `127.0.0.1:0`; the caller cannot select a host
  address or host port. The owner caps pending requests at 64, exposures at 16,
  and concurrent connections at 64.
- Owner cleanup marks the controller closed, removes pending and active maps,
  closes listeners and connections, replies to waiting clients, closes control
  and rendezvous channels, and removes ephemeral records and sockets.
- A reviewer action locates an owner by a fresh scan and opaque request ID.
  Duplicate matches fail as ambiguous; stale owners and requests fail closed.
- `liveServiceRecords` removes malformed or unreachable records and
  `ListServiceRequests` skips an owner when a fresh call fails. The public
  result then reports `Scope: live_attachments` and Catalog coverage
  `exhaustive` without an explicit unavailable-owner count. Whether concurrent
  owner loss is interpreted as scope shrink or bounded uncertainty is not
  represented in the typed result.

### HTTP, WebSocket, headers, cookies, and cleanup

- The host listener parses a bounded first HTTP/1.1 header before opening a
  Workspace stream. It requires exact Host authority, agreement from an
  absolute request target when present, exactly one Host field, no folded
  header lines, no duplicate Content-Length or Transfer-Encoding, and no
  ambiguous Content-Length plus Transfer-Encoding.
- Keepalive requests are revalidated against the same exact authority before
  forwarding. A wrong later authority closes without another Workspace stream.
- WebSocket bytes become opaque only after a validated Upgrade request and a
  `101 Switching Protocols` response. Any other response remains HTTP.
- The relay forwards request/response headers and bodies without rewriting
  Host, Origin, redirects, cookies, application headers, or content. It does
  not log application bytes.
- A target that cannot accept the validated request receives one fixed
  secret-free `502 Bad Gateway`; no health probe is performed.
- Stop and attachment exit close listeners and active streams. The development
  server is not signaled or stopped.

### Read-only observations performed for this packet

- `bin/tobari help review services --format agent` confirmed the current
  read-only/exhaustive, text-only contract and its opaque producer/consumer
  workflow.
- `go run ./cmd/tobari-expose help tobari-expose`, `help list`, and `help stop`
  confirmed the helper grammar, fixed target, and opaque reference flow.
- With fresh temporary XDG roots, redirected `bin/tobari review services`
  returned `No pending Workspace service requests.` with exit 0 and created no
  files beneath those roots.

## Product Owner-fixed V1 contract

These are accepted design facts, not current implementation claims:

- `review services` is a `RoleDiscover`/`EffectRead` shell over pending requests
  only. It may delegate from a trusted interactive text TTY to canonical exact
  Allow/Deny/Open use cases, but its declared output produces only
  `service-request` refs. Active exposures are discovered through separate
  `service status`; V1 adds no typed multi-selection groups.
- Safe raw TTY review uses one deliberate key on the complete effect card:
  `a`, `o`, `d`, or `b`. Multiple pending rows require one request selection
  before that card. A trusted interactive TTY without safe raw-key support uses
  `allow`, `open`, `deny`, or `back` plus Enter. Neither path adds a second yes
  prompt or a direct-command `--confirm` flag.
- Redirected and JSON review are read-only. `--watch` and `--notify` are valid
  only for trusted interactive text TTY use and conflict with JSON/redirected
  operation before reads or actions. Notification payloads are fixed,
  evidence-free cues.
- `service status` replaces predecessor `service requests` without an alias. It
  is the host producer for both request and exposure refs. Helper `status`
  returns current-attachment pending/active state; child-visible pending rows do
  not need a host mutation ref, while active rows carry exact helper-stop refs.
  CWD `status` consumes only bounded counts, attention, and observation state
  and exposes no Service refs, URLs, ports, or actions.
- Host Service reads have complete delivery and `bounded_window` collection
  coverage. One fixed registry anchor produces `complete`, `partial`, or
  `unavailable` plus bounded `observed_owner_count` and
  `unavailable_owner_count`. Known empty requires a successful scan with no
  in-scope owner. Unsafe metadata, duplicate authority, contradictory identity,
  and ambiguous refs fail the whole command. Reads never clean stale records.
- Exact actions take a fresh bounded resolution and revalidate owner, UID, PID,
  nonce, WorkspaceManifestID, WorkspaceID, AttachmentID/epoch,
  principal/controller, target, and supplied ref. Stale or duplicate matches
  fail closed.
- The only access locator is structurally `scheme = http`,
  `authority = svc-<128-bit-random-lowercase-label>.localhost:<random-port>`,
  and `path = /`.
  Its fresh hostname label is independent from the lifecycle exposure ref. The
  socket remains `tcp4 127.0.0.1:0`; no numeric/bare-localhost fallback,
  external DNS, LAN/wildcard/IPv6 bind, raw TCP, or caller host/port exists.
- Before Workspace I/O, relay validation requires the exact generated hostname
  and assigned port in Host and any absolute-form authority. Numeric loopback,
  bare localhost, sibling labels, wrong ports, absent/duplicate/malformed Host,
  and DNS-rebinding values fail. The accepted Host and other ordinary HTTP/
  WebSocket headers, cookies, Origin, redirects, bodies, and frames are then
  forwarded unchanged.
- The exact URL is attachment-lifetime access authority. The opaque exposure
  ref is lifecycle mutation authority. Neither is durable and neither is
  derived from the other outside the live owner.
- Allow is browser-free. The combined `o` transition finalizes Allow through
  the mutation-complete boundary, then supplies only its confirmed exposure ref
  to canonical Open. Open re-resolves the owner and derives the root URL; it
  accepts no URL/path/query/fragment/browser argv. Its outcomes are
  `open_not_dispatched`, `open_requested`, and `open_outcome_unknown` with the
  fixed retry rules in `plan.md`.
- Stop closes admission first, terminates or drains relays under one fixed
  bound, confirms closure, then removes owner state. Attachment teardown
  withdraws pending and closes exposures/streams. The best-effort terminal
  receipt carries bounded confirmed counts only, without IDs, URLs, ports,
  secrets, or application data.
- No Service state is durably migrated. Private host/helper protocol changes
  hard-cut over atomically; old attachments end and re-enter. WP01 migration
  already requires zero live attachments.
- WP08 owns the single recursive Catalog reference traversal. WP01/WP02/WP03/
  WP04/WP05/WP07 precede WP09 in the fixed implementation order. WP06 consumes
  only WP09's bounded summary after WP09 implementation; research-only WP04,
  WP05 Host Loopback, and WP07 permission-wait authority never enter Service.

## Evaluation

- The mechanism satisfies the main trust-boundary requirements. The largest
  routine cost is redundant host interaction and missing observation surfaces,
  not an authority defect requiring a new relay.
- Merging Permission and Service review into one authority-bearing queue would
  make durable staged Apply and attachment-local immediate decisions harder to
  distinguish and would require a heterogeneous reference/action model. A
  shared terminal presentation engine does not require a shared task result or
  mutation.
- The exact generated `.localhost` URL is safe to present and is access
  authority for the attachment lifetime, but it is not lifecycle mutation or
  approval authority. A browser opener is still an externally visible host
  side effect and must consume the independent active exposure reference rather
  than accept a caller URL or trust displayed text.
- A random host port alone does not isolate cookies because cookie scope ignores
  ports. A fresh per-exposure `.localhost` host plus exact Host enforcement is
  the fixed V1 browser-origin boundary, subject to supported-browser evidence.
- Automatic clipboard mutation is not necessary to close the supported
  outcome. It adds a host dependency and overwrites ambient user state; plain
  terminal output plus an explicit browser action is the smaller public
  contract.
- CWD `status` can integrate bounded Service attention without becoming a
  reference producer. Keeping refs and exact open/stop actions in Service
  discovery avoids a role change and aligns with the separately owned
  CWD-home contract.
- The current helper `list` outcome is not independent from a broader
  current-attachment service status task. Replacing it pre-public with
  `status` avoids parallel commands whose only difference is omission of
  pending state.
- A host-side direct `tobari expose PORT` could be trusted, but it cannot
  deterministically select one of several live attachment owners from CWD and
  port alone. Adding it now would either rediscover an action target or require
  a new attachment-reference discovery journey, so it does not shorten the
  agent-requested outcome.

## Relevant structure

- Entry point: `cmd/tobari-expose`, `internal/cli/service_exposure.go`, and the
  separate host `review services` leaf
- Domain rule: `internal/domain/tobari/service_exposure.go` and operation
  intent/target/impact contracts
- Application use case: `internal/app/serviceexposurecmd/service.go`
- Infrastructure boundary: attachment control and ownership in
  `workspace_service_exposure.go`, HTTP relay in
  `workspace_service_relay.go`, and host discovery in
  `service_review_rendezvous.go`
- CLI catalog or presentation: `service_exposure_catalog.go`, global
  Program-aware `cli.Catalog`, terminal review code, project status renderer,
  and session-close renderer
- Existing tests and harness checks: domain/application/CLI service tests,
  rendezvous/relay/asset tests, clean Docker integration,
  `.harness/capabilities.json`, the service-exposure harness row, and the
  agent-readiness service journey

## Constraints

- Workspace code, helper requests, application bytes, browser content, project
  roots, and display labels are untrusted data.
- Trusted-host review remains in a separate terminal. No child input prefix,
  alternate-screen restoration claim, or Workspace-terminal approval is
  introduced.
- Request and exposure action identity is opaque. Manifest name, project root,
  port, URL, attachment label, row position, revision, or proximity cannot
  replace it.
- The owner and lifetime remain the live attachment. Reviewer, browser, CWD
  status, Workspace Manifest, Workspace record, and cluster do not acquire
  ownership.
- Manifest revision publication, pending adoption, attached-blocked adoption,
  desired/applied drift, and bounded reconciliation failures cannot stop,
  recreate, transfer, or extend an exposure.
- RuntimeID/revision digest belongs to Manifest desired and Workspace applied
  entry state, not Service authority. Service relays only to the exact current
  attachment target and never rediscover or adopt a Runtime revision.
- Manifest/Runtime copy creates fresh authority and never copies or derives an
  attachment, request, exposure, listener, or observed Service row. Runtime
  retirement may treat live Workspace/container use as a blocker but may not
  cascade through it into Service cleanup.
- Physical host binding remains exact IPv4 `127.0.0.1`; Workspace target
  remains exact IPv4 `127.0.0.1` plus one reviewed non-privileged port.
- Public access authority is the exact generated per-exposure `.localhost`
  hostname plus owner-selected port. Numeric loopback and bare localhost are
  rejection cases, not fallback presentation.
- HTTP/1.1 and WebSocket Upgrade remain the closed transport union. Browser
  navigation never widens that data plane.
- Every read must remain observational on fresh state. Watch may poll bounded
  owner-only state but cannot create Tobari configuration, policy, Docker, or
  durable service state and cannot remove or repair stale owner records.
- Repository documentation is English. Synthetic roots, ports, URLs, HTTP
  data, and browser fixtures are required.
- The dedicated helper source snapshot and embedded runtime source must remain
  byte-checked; implementation must update both through the existing explicit
  sync flow rather than editing the snapshot independently.

## External facts

- RFC 9112, **HTTP/1.1**, RFC Editor, checked 2026-08-23:
  <https://www.rfc-editor.org/rfc/rfc9112.html>. HTTP/1.1 requires a Host field,
  rejects missing or multiple Host fields, defines request-target forms, and
  warns that inconsistent parsing creates request-smuggling risk. This supports
  retaining exact authority and strict framing checks before Workspace I/O.
- RFC 6455, **The WebSocket Protocol**, RFC Editor, checked 2026-08-23:
  <https://www.rfc-editor.org/rfc/rfc6455.html>. WebSocket begins with an
  HTTP Upgrade handshake; `101 Switching Protocols` completes the transition
  before bidirectional frames. This supports retaining the current
  validate-then-stream boundary.
- RFC 6761 section 6.3, **Special-Use Domain Names**, RFC Editor, checked
  2026-08-23: <https://www.rfc-editor.org/rfc/rfc6761.html#section-6.3>.
  `localhost.` and every name below `.localhost.` are special; IPv4/IPv6
  address queries may be assumed to resolve to the corresponding loopback, and
  resolution libraries should answer locally rather than send configured DNS
  queries. This supports per-exposure subdomain origins without external DNS;
  Tobari still enforces its own exact `tcp4 127.0.0.1` listener and Host check.
- HTTPWG, **Cookies: HTTP State Management Mechanism (RFC6265bis), section 8.5
  Weak Confidentiality**, checked 2026-08-23:
  <https://httpwg.org/http-extensions/draft-ietf-httpbis-rfc6265bis.html#section-8.5>.
  The current draft states that cookies do not provide isolation by port: a
  cookie readable or writable on one port is likewise available to another
  port of the same host. Therefore numeric-loopback URLs with only different
  ports are not an acceptable V1 browser-origin isolation boundary.
- RFC 8252 section 7.3, **OAuth 2.0 for Native Apps**, RFC Editor, checked
  2026-08-23: <https://www.rfc-editor.org/rfc/rfc8252.html#section-7.3>.
  Its loopback redirect use case is not the service-exposure contract, but it
  provides relevant reviewed precedent for operating-system-selected ports,
  loopback-only listeners, and bounded listener lifetime. It does not justify
  a public numeric Service URL; the Product Owner-fixed `.localhost` origin,
  RFC 6761, RFC6265bis, ADR 0074, and browser evidence govern that boundary.
- W3C, **Clipboard API and events**, Working Draft, checked 2026-08-23:
  <https://www.w3.org/TR/clipboard-apis/>. Clipboard access carries explicit
  privacy and user-activation concerns. This browser specification does not
  govern a native CLI, but it reinforces treating clipboard mutation as a
  separate user-visible capability rather than an invisible presentation
  detail. No clipboard API or external content is proposed.

## Unknowns requiring implementation-stage evidence

- [ ] After WP07 completes, record the actual integrated HEAD/worktree and final
      upstream WorkspaceManifestID, WorkspaceID, AttachmentID/epoch,
      principal/controller, registry, Catalog traversal, Runtime protection,
      build-profile, Host Loopback separation, permission wait, Service helper,
      JSON, migration, and WP06 summary interfaces. The current integrated
      checkout is evidence for promoted WP01/WP02 only, not that final WP09
      baseline.
- [ ] Fix and test the exact bounded owner-registry anchor protocol for owners
      that close concurrently, including when they count as unavailable versus
      closed, without cleanup during the read or silent scope shrinkage.
- [ ] Record the supported terminal/platform rule that distinguishes safe raw-
      key mode from full-line token fallback, including cancellation, writer
      failure, redirected streams, and terminal restoration.
- [ ] Record macOS and Linux opener evidence sufficient to distinguish provable
      pre-dispatch failure, successful dispatch request, and uncertain
      cancellation/timeout without claiming browser load.
- [ ] Record the supported browser floor and automated/manual evidence that two
      generated `.localhost` exposure hosts isolate cookies and that
      `Domain=localhost` cannot contaminate a sibling. Any contradiction is
      `WP09_BLOCKED`, not a local fallback decision.
- [ ] Select and prove the finite relay termination/drain bound and the maximum
      bounded cleanup counts without changing listener-first teardown.
- [ ] Freeze WP06's exact post-WP09 summary field placement and human rendering
      from the fixed counts/attention/observation semantics; it may not add
      refs, URLs, ports, or actions.
- [ ] Through the governing public-boundary review, fix the exact lowercase
      random-label grammar/length and narrow guard matcher for only Tobari's
      generated `svc-` authority. Prove arbitrary `.localhost`, bare localhost,
      `.local`, private hosts, caller URLs, and unrelated private-network text
      remain rejected.

## Fixed design rationale

- A watch already running on the host plus one explicit Allow action is the
  minimum safe steady-state handoff; no browser or clipboard automation is
  required for routine success.
- Returning the exact generated `.localhost` URL to both trusted-host and helper
  terminals reduces recovery cost while the independent opaque ref retains
  lifecycle mutation authority.
- `Allow once & open` is two visible ordered outcomes: exposure creation
  completes before browser dispatch, and opener failure leaves access active.
- CWD `status` remains a utility `EffectRead` because it consumes only a nested
  bounded summary, has no Service refs/actions, and creates or repairs nothing.

## Thesis evidence

- Repeated design decision or point of agent confusion: Permission review,
  Host Loopback, native-login browser opening, and Workspace service exposure
  all cross the host boundary but have different direction, owner, lifetime,
  and authority. Similar presentation repeatedly tempts a generic control bus.
- User outcome or friction observed in the minimal slice: the only pending
  Service request still requires row selection and a second confirmation, and
  the host must rerun review for every later request.
- Code workaround or exception being considered: folding Service rows into the
  Permission Inbox, trusting a displayed URL as an opener input, adding a
  caller-selected direct host command, or hiding live-service reads in the
  existing lifecycle renderer.
- Current thesis that resolves it: purpose-specific attachment helpers may
  request exact effects but cannot approve them; trusted-host review leaves are
  task-first but authority-separate; semantics precede presentation.
- Downstream impact: thesis consequences, product command tables, Catalog
  interactive workflow model, public vocabulary, service domain/application
  results, rendezvous protocol, host opener, status/exit presentation,
  security enforcement, agent readiness, helper source snapshot, README, site,
  and release/public schema checks.

## Other packets and decisions

- ADR 0073 remains a hard dependency: review stays in a separate host terminal.
- ADR 0074 must be revised rather than bypassed; it currently excludes browser
  opening and defines the single-shot review/list grammar.
- Commit `3c9bd15` and the removed `review-first-cli` packet are negative
  evidence against recreating one authority-bearing `review` selector.
- `docs/work/first-public-release-core/` and
  `docs/work/first-public-release-artifacts/` own the publication boundary.
  This change must land before their final generated snapshot/artifact freeze
  or be deferred until after that release; it cannot silently invalidate their
  command/schema evidence.
- `docs/work/context-source-access/` remains Boundary evidence, but Domain
  Model V1 supersedes its public Context vocabulary. A future Service status
  integration must preserve its security invariants without retaining the old
  noun or identifier.
- `docs/work/status-home/` owns the CWD project-home envelope and explicitly
  excludes request/exposure refs from `status`. This packet therefore supplies
  only bounded Service counts/attention/observation state there; it supplies no
  ref, URL, port, or action. WP06 consumes this summary only after WP09
  implementation.
- `docs/work/host-loopback-name/` owns the opposite-direction physical-host
  authority spelling and `host.tobari.internal`. Neither appears as a Service
  parameter, alias, hostname, authority field, or output label.
- `docs/work/permission-resume-handoff/` owns the separate ordinary-permission
  Workspace wait helper. It must not reuse `tobari-expose`, Service refs, or
  immediate Allow-once semantics; only task-neutral host review mechanics may
  be shared. Its wait IDs and durable permission policy never enter Service.
- ADR 0079 and docs 00--04 are the promoted authority for Workspace
  Manifest/Workspace identity, desired/applied/observed separation, explicit
  entry reconciliation, copy isolation, and migration. Service JSON/protocols
  must consume their `workspace_manifest_id` and `workspace_id` contracts
  without making Project root or Service state part of Manifest authority.
- ADR 0079's revision-body, Git-fallback, child-session, and Docker
  evidence representations do not change Service authority. WP09 consumes the
  final AttachmentID/epoch/controller interface after the fixed upstream order.
  WP01 migration already requires zero live attachments. A listener, URL,
  record, or reachable target is never applied-revision evidence.
- The promoted copy contract in ADR 0079 and docs 01--02 supplies negative
  integration requirements only: neither Manifest nor Runtime copy carries
  Service/attachment state, copy provenance is absent, and copy never invokes
  reconciliation. Service Exposure has no separate WP02 packet dependency.
- `docs/work/runtime-retirement/` is design-fixed. Its destructive lifecycle
  must fail closed on the relevant observed Workspace/container protection and
  must never cascade into Service Stop or attachment teardown. Service
  Exposure does not own Runtime protection planning, `last_used`, or retirement
  journals. Both packets may later touch Catalog reference schemas; they must
  share the Catalog-wide nested-reference invariant rather than land competing
  validators.
- `docs/work/catalog-domain-output-conformance/` is WP08 and owns the one
  recursive `OutputField.Fields`/`Items` traversal used for nested Service refs.
  WP09 waits for `WP08_IMPLEMENTATION_COMPLETE` and adds no local walker.
- `docs/work/build-profile-contract/` is research-only WP04. No research-only
  path, profile, authentication mechanism, state, or output participates in
  Service Exposure.
- The existing service helper source snapshot is a generated checked build
  input. Parallel changes under `internal/cli`, `internal/app`, or
  `internal/domain` can conflict mechanically with its exact closure.

## Reproduction or observation

```sh
git fetch origin main
git status --short --branch
git rev-parse HEAD origin/main FETCH_HEAD
bin/tobari help review services --format agent
go run ./cmd/tobari-expose help tobari-expose
go run ./cmd/tobari-expose help list
go run ./cmd/tobari-expose help stop
```

The empty-state read was observed with fresh temporary `XDG_CONFIG_HOME`,
`XDG_STATE_HOME`, and `XDG_DATA_HOME`; it exited 0, printed the known-empty
message, and left those roots empty. No Docker service, listener, browser, or
network request was started for this packet.

## Security and public-boundary notes

- Assets and side effects involved: attachment-local helper/control socket,
  owner-only host rendezvous, random loopback listeners, HTTP/WebSocket streams,
  optional exact browser navigation, terminal notification, and cleanup receipt.
- Credentials or confidential data involved: none by design. Application
  headers, cookies, bodies, WebSocket frames, and browser state may contain
  secrets and remain excluded from policy, logs, review, diagnostics, and
  fixtures.
- New dependencies, destinations, files, processes, or generated content: no
  new dependency or destination is selected. Browser open must reuse a narrow
  reviewed infrastructure primitive but own a Service-specific validation and
  application port. Helper source snapshots and generated public docs change
  only during future implementation.
- External schema provenance, publication rights, and drift evidence: only
  public RFC/W3C specifications are referenced; no external schema or copied
  third-party UI is required.
- Output delivery, collection coverage, pagination, timeout, retry,
  idempotency, and cancellation facts: CLI snapshots are complete and
  unpaged with `bounded_window` coverage; observation is explicitly complete,
  partial, or unavailable with bounded owner counts; watch repeats bounded
  snapshots with backoff; Allow and Open are separately classified; stop is
  exact and non-reconstructing; confirmed Allow is never replayable because
  Open failed.
- Publication and licensing concerns: none beyond updating the existing
  checked helper/runtime/public artifacts and retaining synthetic compatibility
  fixtures.
- ADR 0079 separation: standard authentication, learned permissions,
  Manifest desired/applied revisions, and Workspace reconciliation failures
  are not Service exposure state. Service code cannot use them as listener
  ownership, approval, migration, or cleanup inputs.
- Promoted copy/WP03 separation: copy never clones Service authority, and Runtime
  lifecycle never treats an exposure as disposable child state or as usage
  evidence. The only interaction is fail-closed protection of a live observed
  Workspace/container before Runtime destruction.

## Glossary

- **Workspace service**: one HTTP/WebSocket server on exact Workspace
  `127.0.0.1:PORT`.
- **Service request**: one pending, non-authoritative request for trusted-host
  review, identified by `service-request` reference.
- **Exposure**: one attachment-owned random host `127.0.0.1:PORT` listener and
  exact per-exposure `.localhost` access origin with relays, identified for
  lifecycle mutation by an independent `service-exposure` reference.
- **Service review snapshot**: one fresh host observation of pending requests,
  bounded owner counts, scope, and uncertainty. It produces request refs only
  and grants nothing.
- **Service status snapshot**: one bounded host observation of pending requests
  and active exposures. It is the host producer for both Service ref kinds.
- **Review presentation primitive**: shared terminal mechanics for typed
  snapshots, selection reconciliation, watch redraw, cues, and restoration. It
  owns no Permission or Service authority.
- **Purpose-limited Service opener**: a host action that accepts only an active
  exposure reference, re-derives its exact root URL from the owner, validates
  it, and asks the OS to open it. It accepts no caller URL.
- **Host rendezvous**: the private peer-authenticated path from reviewer to live
  attachment owner. It is internal and not public resource identity.
- **Service principal binding**: the trusted
  `WorkspaceManifestID` + `WorkspaceID` + `AttachmentID/epoch` + trusted
  principal/controller + exact target tuple inherited from the selected live
  attachment. Manifest revision, Runtime, Project root, ports, and labels are
  not mutation authority.
- **Per-exposure access origin**: the exact generated
  `svc-<128-bit-random-lowercase-label>.localhost:<random-port>` authority.
  It is attachment-lifetime access authority, not lifecycle mutation identity.
