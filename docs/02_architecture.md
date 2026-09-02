# Architecture

Build-surface and resolver authority follow [ADR 0082](decisions/0082-release-and-research-build-surfaces.md); this document describes the resulting topology and layer seams.
Physical-host loopback authority and migration follow
[ADR 0083](decisions/0083-name-the-physical-host-loopback-authority.md).
The task-owned CWD status snapshot and its zero-mutation observation budget
follow [ADR 0085](decisions/0085-make-status-the-cwd-home.md).
Task-scoped Configurator topology, managed-Home ownership, copied evidence, and
frozen submission follow
[ADR 0090](decisions/0090-agent-guided-configurator-first-use.md).
Tobari-owned terminal interaction uses the main-screen inline renderer defined
by [ADR 0091](decisions/0091-keep-tobari-owned-tui-inline.md).
Location-free Context authority and Workspace-only root ownership follow
[ADR 0092](decisions/0092-make-context-location-free.md).
Installation current-Context selection and CWD-only Workspace routing follow
[ADR 0093](decisions/0093-select-current-context-without-cwd.md).

## System topology

```text
host
  tobari CLI ---- Docker CLI ---- Docker Engine
      +-- authenticated attachment TCP relay --> host 127.0.0.1:PORT
      +-- task Configurator (task Runtime, task-owned managed Home) --> Internet
      |       +-- copied evidence -> one working source -> host-frozen submission
      +-- root A (Workspace Template-selected ro/rw) --> Tobari A -- guarded net A --+
      +-- root B (Workspace Template-selected ro/rw) --> Tobari B -- guarded net B --+--> tobari-gateway
                                                                      |
internal control network:                                      tobari-opa :8181
egress network:                              Gateway --> policy-allowed HTTPS
```

Configurator is a separate non-Workspace container and network role. Policy
assistance resolves the installation current Context or one exact
invocation-local override, then resolves that Context's recorded Runtime image
ID under the lifecycle/store fence; Runtime assistance resolves the installation-owned
standard Runtime and never reads CWD, Workspace, or Context.
The Runtime source being edited is never an execution selector. `container create` consumes
only that immutable ID, and post-create inspection compares Docker's actual
`.Image` identity rather than trusting the mutable selector retained as
correlation metadata. It never joins a Workspace, control, or Gateway egress
network. Its ordinary external route is valid only with its exact mount
contract: one complete task-owned managed Home as the only mutable
data bind, one binary-owned opener projected read-only, plus bounded tmpfs. One
task working source, guidance, and typed observed evidence are copies below
that Home. Project roots, host/other-Context Home,
active authority, writable Policy Memory, Docker socket, host network, and
arbitrary mounts/capabilities are absent. Runtime selection cannot create or
widen this role. Docker first creates the container with `network=none`, returns
its immutable resource ID, and exposes the ordinary-egress network only after
one atomic inspection proves that ID's complete role, mount, resource, process,
and label contract. Tobari then removes the built-in `none` endpoint, connects
the captured dedicated-network ID, starts only the dormant fixed lifetime, and
re-inspects that one active attachment before any helper or agent exec. Every later start, exec, cleanup, browser request, and
callback relay remains bound to the captured container/network IDs rather than
their human-readable names. The container starts in a bounded dormant state,
establishes the existing non-TTY browser control stream, and only then executes
the selected agent with Docker retaining its terminal. The Configurator bridge
selects only the chosen Codex or Claude Code driver and atomically revalidates
the same owned container immediately before host-browser opening and again
after accepting, but before relaying, a Codex callback. The attachment supervisor
observes both the Docker control process and the host request/response relay;
loss of either after readiness cancels the still-attached agent and is a
retained-material failure rather than silently degrading native login. Cleanup
uses bounded immutable-ID removal attempts. Failure to confirm every transient
removal is a separate partial cleanup fault, never a Home-only retained result.

Ordinary Workspace entry similarly resolves its label-owned work-container
name to one immutable Docker ID before starting permission, browser, or service
control. Every control exec and the attached shell/child uses that same ID;
name reassignment cannot split validation from execution.

Each Workspace joins only its dedicated internal network. OPA joins only the
shared internal control network. Gateway joins every Workspace network plus
control and egress. Standard has no Auth Broker service, provider projection,
credential mount, or credential helper. Its one session-scoped native-login
bridge is part of interactive entry: it mounts one binary-owned opener,
projects its attachment-local Unix socket through `BROWSER`, `GH_BROWSER`, and
`xdg-open`, selects one strict Claude Code, Codex, GitHub CLI, AWS CLI,
custom-runtime TWG, or custom-runtime pup authorization contract, derives a
non-privileged port from a callback-bearing loopback redirect, and relays one opaque callback
through a fixed Docker exec program plus that validated port to the exact
selected Workspace loopback. Pup is callback-bearing only on the four exact
ports compiled into 1.10.7. Claude's reviewed remote callback, GitHub's device
target, and TWG's device-verification target open no listener.
One bounded schema-v1 request over a dedicated non-TTY Docker exec control
stream is the only originator. The host independently validates the closed URL
union; arbitrary Workspace prose and terminal controls have no browser effect.
It is absent when no host session is attached. The research development override adds a
locked Auth Broker on control/egress plus its private runtime socket and trusted
host acquisition boundary. Tobari and control networks
use Docker's `internal` property; the egress network is the only network with
an external route.

An ambient Host Loopback attachment adds no direct Workspace route to the host
or Docker VM. The trusted host `tobari` process generates one Attachment Epoch,
listens on one random host TCP port whose pre-routing handshake requires one
256-bit attachment token and one policy-reviewed non-privileged target port,
and publishes one strict active-route registry in the XDG
configuration store. Gateway receives that registry through a read-only bind
mount, derives route and epoch identity from the source Workspace and constant
Tobari-owned hostname, and
asks OPA before selecting a Gateway-local one-shot TCP pump as the upstream.
The sole routable authority is exact `host.tobari.internal`; exact retired
`host.tobari.test` is terminal before ordinary routing for all V1. `.internal`,
synthetic DNS, and suffix matches never select the branch.
The pump preserves ordinary mitmproxy request/response streaming; the host
listener revalidates an active Allow for the requested port before connecting
to the same physical-host IPv4 loopback port. Workspace headers,
environment, paths, query, and DNS cannot select an epoch, relay port/token, or
target address. The capability projection inside the attached shell is an
advisory discovery surface and carries no routing secret or permission.

Workspace service exposure is a separate opposite-direction attachment branch.
The canonical base recipe cross-compiles dedicated Linux `tobari-expose` and
`tobari-permission` helpers for Docker `TARGETARCH` with a pinned Go builder and
one exact checked source/module closure. Each main hardcodes its helper Program
rather than selecting authority from `argv[0]`. Before attachment, the host
extracts both helpers and identity records from the verified source-derived
base through a bounded temporary container; validates their source/API
identity, SHA-256, regular file type, safe mode, Linux ELF, and engine
architecture; and atomically stores owner-only executables. Every selected
Workspace, including one using a managed custom Runtime, receives the same
read-only `/usr/local/bin/tobari-expose` and `/usr/local/bin/tobari-permission`
mounts. For service exposure, an unpredictable Workspace Unix socket connects
`tobari-expose` to one fixed non-TTY control process; it can submit only
one non-privileged Workspace-loopback port, read current-attachment pending and
active state, or stop one unchanged opaque reference. The distinct live
Service-controller attachment owns pending requests and exposes a separate
owner-only Unix rendezvous plus
atomic ephemeral record. A separate `tobari review services` process validates peer UID
and PID, nonce, Service attachment identity, final Context/Workspace identity,
and a fresh bounded snapshot, but never owns listener lifetime. Allow once
binds `tcp4 127.0.0.1:0`, generates an independent 128-bit lowercase
`.localhost` origin label, and starts a bounded HTTP/1.1/WebSocket relay to
exact Workspace loopback only after exact Host/framing validation. Host
`service open` re-resolves the exposure ref and delegates only the confirmed
root URL to a purpose-limited platform opener. This channel
shares no schema, socket, registry, authority, or data plane with browser login
Host Loopback, Permission Inbox/wait, or Context Policy Memory.

Permission resume is a third, read-only attachment concern. The interactive
Tobari entry session created before the child is its canonical owner; Host
Loopback is only one capability on that session. Exactly one owner epoch may
exist for a Context/Workspace pair, concurrent borrower entries
share it, and service-exposure controller attachment IDs are ineligible. A
bounded private session registry joins one frozen schema-v1 Gateway principal
to canonical Workspace Template and Workspace IDs, attachment epoch, owner,
256-bit process-instance nonce, one closed platform ingestion transport, and
renewable lease. The owner PID and transport address are diagnostic only.
Trusted composition fixes the Linux host adapter to an owner-only Unix socket
and the Darwin host adapter to a Darwin-kernel `127.0.0.1:0` listener reached
by Gateway at exactly `host.docker.internal`. Selection does not inspect the
Docker provider, context name, or context path. Gateway accepts only its
composed transport kind and has no runtime probe, fallback, or downgrade.
Colima remains the only supported and release-validated Darwin runtime; an
unvalidated provider whose host bridge is absent or different cannot complete
the exact acknowledgment and therefore receives no resume projection.
Transport reachability grants no authority. Transport kind, endpoint, nonce,
lease, and stable owner identity are exact record fields; nonce-first
constant-time authentication and host-side frame/deadline/concurrency/rate
bounds protect the channel, while endpoint and peer address grant no authority.
Gateway emits resume data only after that owner acknowledges the exact immutable
secret-free wait record. The post-acknowledgment registry read requires every
stable owner field to remain identical and accepts the lease only when it is
byte-identical or one valid renewal strictly advances both issue and expiry
times. A regressed lease, changed expiry at the same issue time, future issue,
expiry, or owner drift omits resume. The registry is mounted read-only into Gateway alone; its
nonce and endpoint never enter logs or public output.

The owner keeps the wait registry in memory and exposes one attachment-local
read-only Unix socket to `tobari-permission`. Observation delegates exact
effect evaluation and precedence to the canonical live OPA policy; it adds no
rule matcher, policy authority, persistent store, daemon, Workspace file, or
request replay. The host accepts an atomic OPA observation only when its
revision equals the strictly validated final active policy receipt at
`$XDG_STATE_HOME/tobari/workspace-authority-policy/active.json`; predecessor
`state.json` is never an authority or fallback, and a missing or unsafe final
receipt fails the wait unavailable instead of polling forever. Teardown ends
the authority and never rebinds waits to a new
attachment. Listener, heartbeat, and renewal failures close transport and
invalidate waits before exact bounded authority cleanup; an expired lease is
never renewed. Attachment cleanup faults remain typed secondary entry outcomes
and cannot overwrite the already-observed child exit status.

The Workspace-local socket and Python process are only an untrusted transport
adapter. The host creates one ephemeral response-signing key per entry channel;
the child receives only its verifier. The signed schema-1 response binds the
channel, canonical attachment and owner digest, wait ID, fresh request nonce,
and closed result or fault. The helper rejects a socket replacement, replay,
unsigned response, or any binding drift. Host code independently enforces at
most eight active waits, a bounded request window, the 4 KiB/1 KiB frames, and
the wait lease. Unexpected bridge exit or a host response-write failure closes
the channel, and checked teardown ends the control exec and verifies that its
socket is absent before canonical attachment authority is removed.

Attachment Grants are runtime-owned inputs to the complete per-request OPA projection and
are disjoint from Context Policy Memory. Policy review binds one grant to
Context, Workspace, epoch, target port, and exact effect. The route is
closed before grant and registry cleanup when entry returns or is canceled;
therefore stale projection data is inert. One attachment owns the route per
Workspace; concurrent attachments borrow its epoch without extending its
lifetime or inheriting ownership. The grant is explicitly available to every
process sharing that Workspace network principal.

The host adapter uses a fixed one-shot helper to configure only a verified
Gateway or Workspace network namespace. The helper receives root plus
`CAP_NET_ADMIN`, no mounts or secrets, and exits before entry. The Workspace
guard installs an exact default route through Gateway and rejects unexpected
on-link, UDP, and IPv6 paths. The Gateway guard redirects project TCP and DNS
to local listeners, keeps IPv4/IPv6 forwarding disabled, and drops its forward
chain. Neither resident process retains a network capability, and no host or
Docker-VM-global firewall state is changed.

Standard reviewed login drivers are the closed GitHub, pup, Codex, and Claude
set. The research compile-time surface additionally activates the reviewed
AWS driver; runtime data cannot change surface membership.
The shared host resolver for GitHub, AWS, and Codex rejects the first temporary, project-local, or
home-local PATH shadow without executing it and inspects a finite PATH-ordered
candidate set for the first canonical executable under an existing trusted
installation root. Each hashes that accepted executable outside the project, runs
fixed argv with a sanitized private state boundary, accepts only its bounded
browser/PTY/output contract, rechecks executable identity, and performs checked
cleanup. Codex additionally requires a stable observed product identity and
the exact compiled host-login/state contract. The verified child owns its
native loopback callback listener, browser request, PKCE state, callback, and
token exchange; Tobari owns none of those surfaces and validates only the
resulting private file state after exit. Its stream boundary recognizes
only the reviewed reset, muted, and accent SGR vocabulary, maps those meanings
to Tobari-owned styles for an interactive terminal, and emits no style when
`NO_COLOR` is present; unknown controls use the ordinary visible projection.
Pup and Claude instead run from the selected Workspace Template image in fresh
mount-free containers. Pup binds the immutable image ID, observes bounded
semantic version syntax and executable digest without a compiled version
allowlist, then requires the fixed login, status, and native-state capture
contract. Claude runs exact 2.1.220 from the selected Workspace Template image in a fresh
mount-free, project-free login container. Tobari hashes the copied executable
bytes on the host, validates the native OSC 8 authorization URL, and maps only
the exact opening, link, browser-result, and paste-prompt events to a fixed
Tobari UI. The no-newline prompt is emitted immediately; Claude owns its
non-echoing terminal input, and Tobari preserves the next real child status
line. Browser success hides the long URL, and opener failure retains the exact
validated manual fallback. The login container replaces the ordinary
Workspace entrypoint, whose CA wait requires a deliberately absent mount, with
fixed `/usr/bin/tini -- /usr/bin/sleep infinity`; this keeps PID 1 alive for the
bounded acquisition without weakening the mount-free boundary. All fixed and control-safe
pass-through lines use explicit CRLF while the child owns the raw Docker TTY;
they do not depend on terminal newline post-processing to return to column one.
Exact cursor hide/show events are consumed and closed with one Tobari-owned
cursor-show. The prompt includes fixed guidance that authorization may take a
moment after Enter, and successful child exit produces a fixed progress
transition before credential capture. The original terminal reader reaches
Docker unchanged so the child retains TTY identity. Tobari then extracts
access token, refresh token, expiry, the dynamically granted scope set, and the
non-secret subscription-type and rate-limit-tier labels
from the Linux file. It structurally validates OAuth scope tokens, requires the
grant to be a subset of the exact set observed in the authorization URL,
normalizes that set to Tobari order, canonicalizes those values with the
observed executable identity into its own V1 record, discards all other
provider-owned optional metadata, and removes the container before commit.
Workspace receives those two bounded labels and the scope set beside its
project-bound access handle, plus a fixed non-secret refresh sentinel needed
by the exact pinned client's local login-state check; the real renewable
session remains Broker-only. Workspace
reconciliation separately merges only the reviewed non-secret top-level
`hasCompletedOnboarding` boolean into Claude's private state, preserves all
other fields, and removes that field with the Anthropic projection. The merge
is unavailable to owner manifests. Owner
manifests remain static-only and cannot select these helpers or their dynamic
plans. Managed stores remain absent.

For HTTPS, ordinary DNS receives a bounded
synthetic non-public IPv4 answer and the direct TCP connection is redirected
to Gateway's local transparent listener. Gateway requires one consistent SNI
and HTTP authority, terminates client TLS with the installation CA, evaluates
the same decrypted HTTP request, and only after allow replaces the synthetic
destination, resolves and pins it, and creates a separate certificate-verified
TLS connection to the upstream. The path does not send plaintext HTTP over the
final Internet hop, and synthetic DNS never performs an external lookup.

## Four-layer dependency direction

```text
internal/cli  ------> internal/app
      |                    |
      |                    v
      +------------> internal/domain <------ internal/infra
```

- `internal/domain`: pure cluster/Tobari/Auth Broker specifications, provider
  and result schemas, state, paths, Gateway/OPA schemas, operation effects, and
  validation.
- `internal/app`: lifecycle, status, entry, diagnostics, doctor, and
  Workspace Template-authentication use cases with consumer-owned ports.
- `internal/infra`: Docker CLI runner, local state/config filesystem, embedded
  asset materialization, provider projection, host root-key storage, broker
  control, reviewed host credential drivers and companion, platform inspection, and
  terminal/environment capability adapters.
- `internal/cli`: the canonical catalog, typed argv parsing, rendering, signal
  handoff, shared semantic style presentation, and composition root.

Domain performs no I/O. Application imports neither infrastructure nor CLI.
Infrastructure satisfies ports structurally without importing application.
CLI is the only production composition root. `tools/archlint` enforces these
directions.

## User-facing composition

The internal separation between shared cluster reconciliation, project runtime
reconciliation, and policy activation must not become a setup tax for the
ordinary user. The product front door is the current project directory and the
bounded-autonomy outcome: enter a reusable isolated space, let the agent work,
and teach the minimum network permission after a useful denial. The CLI may
orchestrate or guide these internal steps in a future slice, but every such
path remains catalog-owned, effect-declared, and failure-before-side-effect
where applicable.

Fresh root remains the deterministic Tobari-owned Manual setup and canonical
default Template/Context composition. It never selects an agent or creates a
Configurator draft. `runtime assist --id` and `policy assist --context` create
or resume one owner-only task draft from explicit target authority and launch the same narrowed
Configurator execution plane, freeze one exact target, and return to the
target's canonical host mutation. Configurator and its helper do not create a
competing command registry or authority writer.

`cluster up`, `cluster status`, `cluster denials`, `policy candidates`,
`review permissions`, `policy allow`, `policy deny`, `policy rules`,
and `policy reset` remain a valid standard seam. `serve`, `auth login`, `auth import`,
`auth status`, and `auth logout` remain research-only seams
internal seams today. They are not permission to expose Docker, OPA, or opaque
resource identifiers as the routine mental model. `review permissions` is the
ordinary human-facing Permission Inbox: on a TTY it composes selection, typed
exact-or-template detail inspection, explicit template-Allow or exact staging,
and one final Apply of
the complete reviewed set. Staging grants no authority; redirected and
machine-readable review remains read-only. Detail actions cannot mutate from
the list, and final Apply is a command-owned fixed-target mutation that
revalidates every unchanged opaque review-item ID and reconstructs every
template proposal from fresh pending evidence and current exact rules.
Single-reference `policy
allow` and `policy deny` remain machine and recovery actions. `policy rules` is the exhaustive current learned-decision inventory;
on a TTY it composes selection, detail inspection, explicit reset confirmation,
and `policy reset` for one current decision, while redirected and
machine-readable inventory remains read-only. `policy candidates` is the
machine discovery surface. The catalog declares this
composition while preserving discover/act separation: the act still consumes
exactly one validated opaque reference or one declared fixed target. A
fixed-target read or write remains reference-free. A fixed-target create may
return confirmed opaque child-resource references, but consumes none and cannot
return the fixed creation-scope kind.

Interactive review availability comes from the final CLI composition's one TTY
probe, never from an optional predecessor command implementation. Runtime
review uses that probe to enter exact lifecycle recovery; a recovered failed
draft is projected as recovery, not as a no-change build with fabricated
history.

The Catalog spans the host Program and both helper Programs. `review services`
and `service status` produce request refs; host status and confirmed helper
create/status produce exposure refs; allow consumes a request and may produce
only its confirmed exposure child; deny consumes a request; open and both Stop
paths consume an exposure. The one recursive Catalog traversal derives all
nested reference edges. Validation and reference closure are global;
dispatch, human help, and scoped agent help are filtered by exact program.
The dedicated `tobari-permission` Program participates in that same global
Catalog but its required `permission_wait_id` is bounded plain text, not an
opaque reference. It has no producer, completion, discovery, or local output
traversal; the global recursive output/reference derivation remains the only
walker.
The pure `review` Catalog namespace has two host-only leaves. `review
permissions` delegates to the existing Permission Inbox read and fixed-target
staged Apply path. `review services` delegates directly to the fresh Service
snapshot and invokes canonical reference-bound Allow once or Deny immediately.
Bare namespace dispatch is generic Catalog help and calls neither application
port. The Workspace helper never receives host command routing.

Research `serve` is a foreground CLI composition over the existing typed application
tasks. Before exposing a listener it obtains one valid installation snapshot:
cluster status, exhaustive Workspace inventory, bounded policy review, and
learned rules. Infrastructure owns an embedded no-external-asset HTTP surface
on `127.0.0.1:0`; it depends only on domain-shaped backend methods and makes no
policy decision. The CLI adapter delegates snapshot reads to `tobaricmd.Service`
and every browser Apply to the catalog-owned internal `policy apply-reviewed`
contract. The process lifetime owns the listener, session bearer, and browser
opener. Cancellation closes the surface without changing policy. Build-tagged
composition keeps its command, handler, and loopback infrastructure wiring out
of the release surface.

### Workspace Template, Context, Policy Memory, and Workspace composition

The final authority store owns four distinct domain resources:

```text
Workspace Template (installation reusable desired revision)
        |
        | immutable binding
        v
Context (location-free ContextID + TemplateID)
        |
        +-- Policy Memory (Context lifetime)
        |
        v
Workspace (replaceable ProjectRoot-bound applied instance)
```

A Template has one stable WorkspaceTemplateID and complete immutable revisions.
TemplateID plus semantic digest authorizes content. One Context has one stable
ContextID and permanently binds to one TemplateID without a location. Multiple
Contexts may bind the same Template. A Workspace has one stable WorkspaceID,
binds one canonical ProjectRoot, and belongs to one Context. Policy Memory belongs to the Context, not the
Template or Workspace.

Concept-separated owner-only YAML below XDG configuration is the ordinary
editable desired source. Immutable typed concept objects plus one complete
generation manifest and atomic active pointer form the owner-only active
last-known-good snapshot consumed by runtime evaluation. Observation paths
never create directories, locks, selectors, journals, or resources. Apply and
every other active mutation re-observe under the installation lifecycle lock
and atomically publish one validated generation. Ordinary readers never adopt
predecessor serialization; the exact supported typed predecessor is available
only through the dedicated stale-bound installation migration port.

```text
$XDG_CONFIG_HOME/tobari/
  templates/<TemplateID>/{template.yaml,policy.yaml}
  contexts/<ContextID>/context.yaml
  runtimes/<RuntimeID>/{runtime.yaml,source/}

$XDG_STATE_HOME/tobari/
  runtimes/<RuntimeID>/{runtime.json,revisions/}
  authority/
    templates/<TemplateID>/<digest>.json
    contexts/<ContextID>/<digest>.json
    policy-memory/<ContextID>/<digest>.json
    workspaces/<WorkspaceID>/<digest>.json
    generations/<digest>.json
    active.json
    journal/
```

Template Plan snapshots the closed two-file source, compares Template-wide
`base_revision` with active authority, resolves one exact Runtime ID/revision,
and binds Context/Memory/Workspace impact into one opaque reference. Apply
consumes that reference unchanged and fences the same source bytes immediately
before active publication. It then
advances only source bookkeeping without clobbering intervening edits. Context
Plan/Apply binds and activates a draft or validates its immutable
ID/root/Template tuple and publishes no rebind.
Reads report `in_sync`, `modified`, `invalid`, or `missing`; absent desired
files do not delete active resources. Policy Memory and Workspaces remain
below XDG state and are unrepresentable in these files.

Template creation is a fixed-target draft create. It starts from the reviewed
standard complete body and may bind the existing read-only/read-write source
access choice plus one exact bounded HTTPS GraphQL endpoint at creation. The
endpoint is stored as the existing POST policy rule and remains under the
Boundary's destination and method ceilings. Template copy consumes one exact
`workspace-template-revision` reference and issues a fresh unpublished source
identity with no lineage. Template Apply consumes one
`workspace-template-change-plan` reference after a human or agent has edited
and reviewed the complete
desired Runtime/session/creation and static-policy source pair. Granular
Runtime, shell, Git, and bootstrap setters are not a parallel authority path.
Context creation consumes one Template reference and no CWD. `context use`
consumes one Context reference and changes only the installation selector.
Bare `tobari` selects an existing Workspace from CWD; when none exists there,
explicit create-here binds an unattached current Context, or publishes a
distinct Context from the default Template when current already owns a
Workspace. Workspace and Context deletion consume their own exact references.
Deleting an otherwise-unreferenced current Context publishes the same authority
generation with that Context removed and CurrentContextID absent; selection
alone is not a deletion blocker.
Names remain read-only discovery/presentation input and cannot authorize
mutation.

The installation owns one optional DefaultTemplateSelection and one optional
CurrentContextID. The former seeds fresh authority; the latter supplies the
default for Context-aware commands and an unattached exact Context for
new-Workspace entry. When it is already attached, explicit create-here uses the
default Template to create a distinct Context instead. Existing Workspace
selection and status derive from canonical CWD and then follow that Workspace's
permanent Context binding. No entry path rewrites CurrentContextID. An explicit
Context option overrides only one invocation.

### Independent activation axes

Template current, active Template-policy, current/active Context Policy Memory,
Workspace AppliedEntry, and live observation are independent:

- Template mutation publishes desired state only.
- `cluster up` validates and atomically activates one complete installation
  projection with separate Template-policy and Policy-Memory receipts per
  Context.
- root entry alone reconciles one Workspace entry slice; `context use` performs
  no Workspace or Docker reconciliation.
- session defaults affect only a newly handed-off child session.
- creation defaults affect only a newly created Workspace home.
- status, list, show, doctor, and completion are zero-mutation observations.

A Workspace entry plan derives from one validated Context snapshot and exact
Template revision. It rejects attached pending adoption before Docker, keeps
desired and last-successful AppliedEntry distinct, and records one bounded
failure or unknown outcome without replacing prior success. Runtime
reconciliation publishes AppliedEntry only after runtime, health, network,
principal, and endpoint checks succeed.

Runtime lifecycle observation joins current/retained Template Runtime
references, Workspace applied/pending/observed evidence, journals, receipts,
and bounded Docker facts. Build, restore, prune, and delete remain Runtime-owned
actions. Root never invokes them implicitly.

The cluster projection is per Context: its static slice derives from the bound
Template revision and its learned slice from current Policy Memory. OPA and
Gateway receive only the complete validated aggregate. A failed candidate
preserves the prior known-good aggregate. Research composition adds
Context-owned credential projection; the release surface has no Broker state.

### Root first-entry orchestration

Bare root is one composition of canonical boundaries, not a second authority
model. Fresh root validates CWD, shows the no-authority recommended draft, and
uses the deterministic Manual review. It starts no Configurator. Apply then
performs these checkpoints in order:

```text
Check requirements
Save setup
Prepare protection
Prepare Workspace
Enter Workspace
```

The semantic stage keys are
`check_requirements -> resolve_context -> prepare_protection ->
prepare_workspace -> enter_workspace`. The second key/label is presentation
only; it publishes a Template, selects the installation default, and creates a
Context through final authority ports.

Progress is invocation-local line-oriented stderr. It has exactly
`pending|running|succeeded|skipped|blocked|failed|unknown`; no percentage, ETA,
event resource, flag, or persisted preference exists. After an anti-flicker
threshold it shows the current stage, then bounded elapsed/wait details and
redirected heartbeat. External BuildKit/provider text is untrusted projection
and never decides stage or retry.

Every checkpoint proves only its own receipt. Root retains the canonical
cluster result, re-observes the exact post-cluster collection receipt and
default Template/Context authority, and passes that observation to entry.
Failure, cancellation, or output encoding after a confirmed mutation reports
retained facts, causal change state, and one Catalog-derived Next. Unknown
mutation outcome never grants replay.

Before child handoff, caller cancellation cancels the current canonical
operation, performs one bounded uncanceled classification, preserves confirmed
mutation state, prints one Next, and exits 130. After handoff, terminal streams
and status belong to the child. Cleanup runs under a distinct bounded context;
its failure may add one host-owned diagnostic but cannot replace child status.

Direct child argv exists only after the positional-only marker. Every argument
is passed unchanged, with no shell parsing, persistence, logging, or
reconstruction. Progress stays on host-owned stderr before handoff. A fresh
noninteractive invocation fails before mutation because trusted Manual review
requires the existing terminal contract.

Task-assistance child handoff is distinct from Workspace child handoff. Before
Configurator handoff, the host owns one fixed target and trust summary. During the child,
the selected agent owns its PTY; Tobari never interprets agent prose as a
request, confirmation, target, or result. There is no live host helper or host
filesystem mount. Agent exit returns control to host freeze, validation, and
review; failure may retain only confirmed task material in the Context Home.
Source publication or final Apply is never selected from child output.

Because this orchestration combines catalog dispatch with production-only
adapter construction and root-owned terminal/environment decisions, lower-layer
selector or application fixtures are not sufficient composition evidence. The
release binary owns the representative cold, exact-root, descendant-reuse, and
explicit-nested-create journey. Separately, a whole-public-catalog test fixes
the exact Program/path-to-handler mapping so a rewrite cannot leave tests on a
retired handler while routing the public command elsewhere.

### Recovery binding

Catalog recovery metadata and final status PrimaryNext/Attention are the only
public recovery graph. Root binds to them; it creates no parallel status model.
The repository-wide graph validator rejects unknown command paths, unchecked
required inputs, action rediscovery, nonretryable self-loops, closed reference
cycles, and mutation replay after unknown outcome. A typed non-command
condition such as `end_active_session` may be the terminal Next.

A missing authoritative Runtime execution image routes to `review runtimes`;
only Runtime discovery may produce the exact revision reference for restore.
Engine-unavailable faults give provider-neutral external-start guidance and
then `tobari`; Tobari never starts Docker Desktop, Colima, Podman, or another
provider. Reads remain zero mutation and never poll themselves.

### HTTP authority scope, lifetime, owner, and precedence

Template Boundary sets the terminal destination/method ceiling. Context Policy
Memory supplies exact remembered Allows and Denies within that Boundary.
Workspace/Project principal evidence selects one Context's combined policy.
Template copy therefore copies static reviewed setup but cannot copy learned
authority; Workspace replacement retains Context Policy Memory.

The policy evaluator is fixed Tobari-owned bundle material. Templates and
Contexts contribute only canonical typed data; no user-owned source path can
replace, extend, or point the aggregate at executable policy. Exact Deny
remains terminal over every positive source. Gateway redacts client
authentication and cookies before OPA/audit and forwards them unchanged only
after allow. Research credential resolution additionally requires the exact
Context, provider, revision, target, and header binding.
## Runtime assets

The Go binary embeds a versioned runtime tree:

```text
runtime/
  compose.yaml
  tobari/Dockerfile
  tobari/entrypoint.sh
  gateway/Dockerfile
  gateway/addon/tobari_gateway.py
  gateway/config.example.json
  authbroker/Dockerfile
  authbroker/broker.py
  authbroker/daemon.py
  authbroker/control.py
  opa/policy/tobari.rego              # embedded fixed evaluator (internal)
  opa/policy/tobari_test.rego         # embedded evaluator tests (internal)
```

The canonical Tobari base-image source now lives under `runtimes/base`. Its
Dockerfile and bootstrap are copied into the embedded
`internal/infra/runtimeassets/assets/tobari` snapshot by the explicit
`scripts/sync-runtime-base.sh` maintainer operation, and
`scripts/check-runtime-base.sh` fails if the snapshot drifts. The base metadata and
single digest/artifact lock are kept beside the source image. This keeps
the distributed CLI self-contained while avoiding two independently edited
base definitions.

Gateway follows the same monorepo pattern and is part of the CLI release unit. The
canonical Dockerfile, addon, entrypoint, and tests live under `gateway/`; the
Go binary embeds a checked snapshot of only the Dockerfile, `.dockerignore`,
and Dockerfile-declared image inputs at
`internal/infra/runtimeassets/assets/gateway/`. The explicit
`scripts/sync-gateway-source.sh` operation refreshes that snapshot and
`scripts/check-gateway-source.sh` rejects byte, membership, or Docker `COPY`
drift. Tests and contributor documentation stay canonical-only rather than
inflating the distributed CLI or its development image identity. Compose and OPA remain under
`internal/infra/runtimeassets/assets` because they are CLI-owned orchestration
and policy inputs, not Gateway image contents. Pull-request workflows validate
the canonical Linux amd64/arm64 build with cache-only output. No workflow
publishes a Tobari-owned image.

The root ensure operation materializes exact embedded bytes under the Tobari state directory,
writes generated non-secret runtime configuration, including the owner-only
Context-principal registry and all-Context policy projection,
and invokes Docker through the runtime port. Standard Compose owns only Gateway,
OPA, shared networks, and CA volumes. Cluster startup ensures the verified
Gateway and agent-ready runtime images from embedded pinned recipes under
source-derived local tags, building each only when absent. The release resolver
is `embedded` and the development resolver is `development`; both select the
same standard Runtime tag for equal exact embedded source identity. The channel
owns preparation and recovery, not a second image-name authority.
The runtime adapter
creates or reconciles each Workspace from its bound Workspace Template image and connects Gateway to
its dedicated network. After it has reconciled the Workspace guard, it records
the exact owned Workspace and Gateway endpoints in the schema-1 principal
registry. Before project container creation, the runtime issues
configured provider handles and renders only manifest-declared environment or
complete-file projections. A public-only CA volume is mounted read-only into
each Workspace, whose entrypoint builds an ephemeral CA bundle.

The same resolver owns a pure build-identity projection used by `version` and
cluster preflight. Both implementations fix APIs to canonical source and derive
the standard Runtime image name from the exact checked embedded source identity;
release packaging injects no image authority.
Neither implementation can consult CWD, project metadata, environment, or a
moving registry tag. Cluster preflight compares this projection before state
loading, asset materialization, journals, policy tests, or Docker calls.

Research Auth Broker follows the same canonical-source/runtime-input pattern as Gateway. Its
editable Python package, Dockerfile, tests, and bridge/protocol source live
under `authbroker/`; the Go binary embeds the checked Docker build inputs, not
the tests or contributor documentation, at
`internal/infra/runtimeassets/assets/authbroker/`.
`scripts/sync-authbroker-source.sh` refreshes the snapshot and
`scripts/check-authbroker-source.sh` rejects byte, membership, or Docker
`COPY` drift. The source and image
checks run the broker unit suite, prove that no provider CLI is installed, and
build the fixed non-root image. It is not published. The research
contributor resolver uses a source-hash local tag.

Both canonical sources declare component API V1. Source records only reviewed
parent inputs; generated owned-image outputs never enter `versions.env` or
release packaging.

Compose mounts only durable owner-only host state `auth/contexts` at
`/var/lib/tobari-auth/contexts`. The runtime socket crosses a Docker-owned
local volume backed by tmpfs, created with the host UID/GID and mode `0700`;
it is not a host bind mount or durable XDG state. Cluster down removes that
ephemeral volume even when the retained CA and policy volumes are preserved.
The control socket lives on container-private tmpfs. The daemon listens on
`/run/tobari-auth/control/broker.sock` and
`/run/tobari-auth/runtime/broker.sock`. Control operations enter the container
through fixed `docker exec` argv and stdin. Gateway mounts only the runtime
volume read-only. The provider projection is generated atomically from the
built-in documents plus owner-only XDG user manifests. Its dedicated parent
directory is mounted read-only into Gateway so atomic replacement selects a new
inode without recreating the service; neither the projection nor a provider
manifest contains a secret. Gateway caches a validated projection only while
its complete stat identity remains unchanged and fails closed, without a
last-known-good fallback, when a replacement is invalid.

The reviewed host-driver implementation union keeps GitHub, AWS, and Codex provider-native execution
on the trusted host. Each resolves and hashes one canonical executable from
conventional non-project trusted installation roots, uses only its fixed argv
and sanitized private state, and deletes temporary state on every outcome.
The Auth Broker domain owns the complete implementation provider-ID vocabulary,
the immutable standard or research active subset, and the
presentation-ordered reviewed-login subset with its exact helper binding.
The application service, CLI input enum, embedded-manifest loader, and fixed
infrastructure driver table derive from or prove parity with that closed
vocabulary. These immutable projections do not provide runtime registration;
the infrastructure table continues to own executable dispatch and payload
validation for each compiled driver.

Inside the Auth Broker, reviewed renewable bearer sessions use a second closed
compile-time registry keyed by the persisted credential kind. Its provider-local
adapters own only state parsing, freshness, the fixed refresh transport,
refreshed-state validation, and typed supplemental values. Broker core retains
handle and binding validation, the immutable record snapshot, per-record
single-flight lock, durable outcome-unknown barrier, provider-call ordering,
compare-and-swap, Vault commit, rotation, and revocation. Adapters receive no
Vault, root-key, handle-index, lock, barrier, executable, or registration
capability. AWS request signing and its trusted-host companion remain a
separate reviewed capability rather than optional methods on a common Provider
interface. Its closed mechanics registry owns only SigV4 request parsing,
companion request/result correlation, bounded temporary-credential decoding,
and request signing. Broker core retains the companion call, per-record lock,
durable no-replay barrier, snapshot comparison, Vault CAS, and response
framing; the mechanics object receives none of those authorities. Host
acquisition likewise remains a distinct trusted-host boundary
and shares only the existing strict credential-state envelope with Broker
runtime resolution.

The Anthropic envelope is deliberately not a copy of Claude's private file
schema. The acquisition boundary depends on four exact renewable fields and
two bounded non-secret entitlement labels from the pinned executable, while
Broker persists only a strict Tobari-owned record and supplies its reviewed
fixed client ID during refresh. All other additive or account-local Claude
metadata is discarded. Subscription and rate-limit values are structurally
bounded provider output, not compiled value lists, and cannot widen Broker
behavior. Scope names are likewise absent from the compiled contract: Broker refresh uses
the persisted canonical set and rejects a changed refresh response, while the
same non-secret set and the two native-client entitlement labels are projected
beside the Workspace handle. A fixed non-secret refresh sentinel prevents the
pinned client from misreporting that projection as expired but is never
recognized or resolved as a Broker handle.

The persisted credential-record union is also separated from Vault storage
authority. Closed record contracts own credential-kind membership, exact V1
record and binding shapes, bounded secret/state encoding, and record
construction. `VaultStore` alone owns root-key use, AES-GCM envelopes,
Workspace Template-associated data, owner/mode validation, bounded file reads, and atomic
replacement. Record contracts receive no key, path, file descriptor, cipher,
or persistence callback. Static records deliberately accept any validated
provider ID because strict owner manifests remain the supported declarative
extension boundary; AWS, Datadog, OpenAI, and Anthropic records bind one exact reviewed
provider and binding shape.

The control-socket login vocabulary is a third closed projection with only two
capability shapes: bounded static-secret login and bounded reviewed driver-state
login. Its immutable plans own exact request keys, payload-length field,
reviewed driver IDs, credential kind, and fixed record constructor. Dispatcher
parses through that union and Broker core performs one driver-state commit
lifecycle: Workspace Template validation, optional renewable-state validation, record
construction, Vault save, and prior-handle revocation. A plan receives neither
`BrokerState` nor `VaultStore` and cannot execute the host helper. The Go
trusted-host acquisition drivers remain on the other side of the control
socket and are not implementations of this protocol plan.

Gateway keeps its own closed, non-secret credential-profile registry for the
exact Anthropic, AWS, Datadog, and OpenAI projection constraints. Profiles see
only already parsed projection values and may validate fixed normalized
bindings, Workspace projections, helper identity, renewable classification,
and typed supplemental response metadata. They receive no HTTP request,
credential value, Broker caller, OPA client, socket, or mutation callback.
Generic schema validation continues to accept strict owner-authored static
providers. Gateway core continues to own candidate recognition, removal of all
secret-sensitive headers before introspection and policy, deny-before-resolution,
one same-revision post-allow action, and exact final header/signature application.

These Go and Python projections deliberately do not share a production
registry or generated adapter interface. A versioned test-only reviewed
provider capability fixture records only the translations that must agree
across component boundaries. Go tests bind provider IDs, acquisition mode and
helper, public manifest credential kind, and login membership to the domain
registry and embedded built-ins. Python tests bind control-login shape,
persisted record kind, renewable/signing/supplemental capabilities, and Gateway
surface membership to the compiled Broker and Gateway registries. In
particular, public `primary_secret` intentionally maps to persisted
`static_primary_secret`; identical spelling is not required where the trust
boundary owns a different representation. No production component reads this
fixture, and updating it cannot register a provider or grant adapter authority.

GitHub recognizes only its fixed device URL and requests one host-browser open
at most once after native confirmation. The standard runtime's exact-default-argv compatibility wrapper
uses the pinned real client with fixed GitHub.com HTTPS login and Git credential
setup argv; it inherits the attachment-scoped `GH_BROWSER` opener only for that path.
Every other GitHub CLI argv passes through unchanged.
OpenAI delegates browser opening and the dynamic authorization URL entirely to
the verified Codex child; Tobari visibly projects but never parses, reconstructs,
or opens that URL. Claude recognizes only its exact pinned OSC 8 authorization
event, opens that exact URL at most once, and otherwise treats changed provider
text as control-safe untrusted data. Its fixed UI never projects the pasted
authorization code and never guesses that an actual provider failure is input
echo. No URL,
executable, argument, environment key, or driver supplied by a provider
manifest, repository, Workspace, request, or project `PATH` can alter that
behavior. A Workspace copy is never an acquisition fallback. Request region is
separate Workspace Template/tool configuration and is not part of login state. Repository
`.git/config` and Workspace-authored tool configuration remain
inside the project/Workspace boundary; ambient host provider configuration is
never read by these drivers. The
separate Workspace Template Git identity fallback contains only safely re-encoded
`user.name` and `user.email`; it grants no authentication or signing authority.

Custom images are supported only when they preserve runtime API label
`io.tobari.runtime-api=1`, the `tobari` image user, and the exact built-in
entrypoint, including the `io.tobari.runtime-lifetime-command=sleep infinity`
capability needed for Tobari's fixed Workspace lifetime command. The intended
construction uses the resolver-selected immutable release digest (or the
contributor-local base) as `FROM`; a runtime-API label is a
compatibility assertion, not an image provenance or trust signature. The
selected image's `CMD` is ignored for Workspace lifetime: Docker create receives
the explicit `sleep infinity` command after the image. Tobari validates
compatibility before creating the project home, network, or container. Docker
create still supplies the invoking numeric UID/GID,
read-only root filesystem, dropped capabilities, fixed CPU/memory/PID/log
resource bounds, fixed mounts, guarded internal network, and health
check.

The canonical base under `runtimes/base` now includes Claude Code 2.1.220 and
Codex 0.147.0 beside the common tools. It downloads the pinned official agent
releases, verifies their per-architecture checksums, preserves the base user,
entrypoint, and lifetime command, and smoke-tests both version commands.
Claude may become the provider-only acquisition executable in
the separate mount-free login container; it never makes an ordinary Workspace
process or image executable a general host helper. Codex uses the official standalone package, which keeps its
CLI companion binaries and Linux sandbox resources together. Agent executables
and package resources live in image-owned `/usr/local/bin` and `/opt/tobari`
paths; `/var/lib/tobari` contains only per-Workspace home state and is safe to
replace with the persistent home bind. The base lock records each upstream
artifact's version, source, license-review state, architecture, checksum, and
size; there are no per-agent child images.

The combined base declares `NOASSERTION` and is permanently local-build-only.
`.github/workflows/runtime-base.yml` has read-only repository permission and
builds the multi-architecture source with cache-only output; it has no registry
permission, login, or push step. The released CLI materializes the same recipe
and builds it on the user's Docker host when its source-derived tag is absent.
Both final Workspace entry and an explicit managed `runtime build` call the
same preparation boundary. Managed build does so only after freezing its own
source snapshot and proving that snapshot names the exact canonical parent,
and before starting the managed BuildKit attempt; therefore it neither depends
on an earlier Workspace nor mistakes Docker Hub for parent authority.
Contributor development resolves `builtin` to the same source-addressed local
combined base selected by the embedded resolver when the checked source inputs
are equal. Final entry treats an owner-authoritative retained standard binding
as self-consistent only when its stable ID/name/ordinal,
`base-<source-id>` image, and revision derivation agree; this proves internal
binding consistency, not provenance of an arbitrary external image. It then
validates and executes the exact immutable local image ID. Only the current
compiled source identity is eligible for missing-image construction.

The root resolver obtains the desired image from the stored Workspace Template identity's
strict manifest on each runtime reconciliation. A new Workspace Template selects
`builtin`. The resolved selector, rather than the source of the
default, is persisted on the Workspace only as the last successful
runtime-container image. Project metadata is not consulted for runtime
selection. A binding-backed manifest may keep `builtin` as its stable selector;
an explicit portable selector must equal the binding's resolved material, or
domain validation rejects the contradiction before presentation.

Project runtime path mapping is owned by the Docker adapter. The selected root
is mounted exactly once with the bound Workspace Template's immutable source access. If
its canonical path is below the host
home, the adapter maps the host-home-relative suffix below `/var/lib/tobari`;
otherwise it uses the mirrored `/workspace` path. The per-Workspace home mount
is established before a nested project mount, and the runtime image contract
keeps executable and package assets in `/usr/local/bin` or `/opt/tobari`, not
below `/var/lib/tobari`.

The shared Gateway, OPA, and Auth Broker Compose services use fixed CPU, memory-plus-swap,
PID, and JSON-file log rotation bounds (`10m` per file, three files) so one
project cannot grow shared service resources without a cap. These are shared
service ceilings, not per-project fairness controls.

Before any shared-component or Workspace-helper container create, the Docker
adapter performs one timeout- and byte-bounded narrow image-metadata inspect.
It binds validation and execution to the returned immutable image ID and
rejects unreviewed `Config.Volumes`: OPA, Auth Broker, managed Runtimes, and the
helper image are volume-free, while Gateway may declare only the exact
inherited CA path shadowed by its reviewed Compose tmpfs.

Project metadata is not a runtime adapter. Tobari does not interpret
`.devcontainer` files, invoke the Dev Container CLI, or transfer container
creation to a second orchestrator. The supported customization adapter is the
installation-wide Runtime catalog: infrastructure snapshots one bounded
owner-only source tree, builds only the immutable snapshot, validates the
resulting image, and appends one successful semantic revision. Ownership
evidence is resolved first and compatibility is then checked against that
immutable image ID, never a mutable staging tag. No-change builds revalidate
the retained immutable ID, and publication recovery revalidates the journaled
image digest before its first phase mutation. A post-build compatibility
failure retains the exact failed journal and staging disposition as a partial
mutation; a no-change compatibility drift that is cleaned before Docker
mutation remains a precondition failure with no change. A Workspace Template stores
an exact Runtime ID and revision; only editing and applying the complete
Workspace Template source changes that binding. Future import formats must attach to this Runtime boundary rather than
introduce project or Docker-tag authority.

Workspace Template Workspace bootstrap is a separate create-only projection boundary.
Domain owns the closed normalized snapshot and semantic revision; application
owns configure/refresh/remove intent and result correlation; infrastructure
alone reads fixed host AWS shared-config and kubeconfig paths, parses one closed
AWS profile and optional dependent EKS target, atomically replaces the Workspace Template
manifest, and creates canonical private Workspace files. The EKS parser uses an
infrastructure-only pinned YAML dependency, resolves only one explicit context,
and converts the reviewed source to domain values; domain and CLI remain free of
the parser dependency. Projection runs between fresh home creation and logical
Workspace publication. Runtime reconciliation has no bootstrap write path.
Wizard discovery is a read-only sibling port: infrastructure parses each fixed
file once, shares the same resolver used by exact preparation, and returns
explicit available/unavailable candidates. Structural or source-safety failure
returns an explicit empty collection and no partial candidates. Candidate
ordering or proximity is never identity; available entries carry the complete
normalized snapshot and EKS results bind the exact AWS semantic revision.
The Workspace record stores only the applied bootstrap revision so status can
compare it with the selected Workspace Template bootstrap recipe without inspecting the
Workspace file.

The activation classes are therefore explicit:

```text
WorkspaceTemplateID + desired semantic digest
  ├─ Boundary revision            -> invariant source/network ceiling
  ├─ cluster projection revision  -> explicit cluster up
  ├─ entry revision               -> explicit Workspace entry; home preserved
  ├─ session-defaults revision    -> each later child session
  └─ creation-defaults revision   -> future Workspace creation only
```

Runtime build diagnostics use two deliberately separate paths. The
application's optional build-progress port carries a bounded, validated stage
vocabulary and selection-state metadata for CLI presentation. A purpose-bound
writer carries visible-projected Docker/BuildKit stdout and stderr to host
stderr as the build runs. Upstream prose never becomes a structured fault
field or a source of promotion-state inference; infrastructure decides state
from completed build, compatibility, digest, and atomic manifest operations.

The pre-release cutover has one deliberately narrow explicit installed-state
migration slice and no automatic predecessor decoder. Ordinary composition
injects one owner-only final Workspace authority store into
Template, Context, Workspace, Policy, Runtime-protection, principal/session, and
authentication consumers. A missing final store is exact empty authority only
when one non-decoding fixed-path legacy-presence guard also proves that no
declared predecessor authority exists. The guard is a precondition, never a
data source: it cannot supply an ID, policy rule, Runtime binding, principal,
session, or credential.

The fixed inventory is explicit rather than a directory walk. Predecessor
active Context/Workspace roots and journals, cluster reconciliation, and
migration roots are legacy-only. The final editable Context source root is
checked for absence only before first final publication, then its strict final
source adapter owns schema and path validation. Projection, principal, auth
(including the lazily created research Workspace-auth registry), Workspace profile/home, Host
Loopback, interactive-attachment, and service-exposure roots are checked for
absence only before first final publication because their final owners may
create them later. Each later owner validates its own exact schema; the
presence guard does not. WP03 Runtime catalog/material/lifecycle roots are not
legacy Workspace authority and remain available across the cut.

The exact supported typed `authority.json` returns a stable migration-required
fault to every ordinary reader. `installation migration plan` reads it through
a dedicated strict decoder and binds its byte digest together with the complete
predecessor custom-Runtime catalog/source tree and every Template Runtime
reference. Apply revalidates the opaque plan, stages concept sources and the
stable-ID Runtime config/state split outside canonical paths, publishes
immutable authority objects, commits the exact plan authority as optional
migration provenance inside the content-addressed target generation manifest,
swaps one verified active pointer, and retires the old roots through phase-aware
journals. Ordinary generations carry no migration provenance. Before transaction
cleanup Apply durably retains a plan/generation/revision accepted receipt in the
active authority journal; same-plan response-loss recovery requires exact equality
with the verified active-manifest provenance, and automatic receipt GC is
forbidden. Failure before acceptance restores
byte-identical predecessor config/state and authority; pending journals fence
ordinary reads until settlement. Every other legacy, unsafe, partial,
Advanced/Rego, or changing presence
returns reset-and-recreate guidance before Docker/OPA/Gateway/Broker mutation.
The root and cluster status application boundaries preserve this sentinel as
an observation/not-applicable fault. They do not collapse it into a transient
status read error, and their Catalog contracts expose the same terminating
read-only `doctor` route.

The canonical interactive-attachment and Host Loopback registries retain their
existing final lock, epoch, lease, nonce, liveness, and cleanup mechanisms.
Predecessor registry presence is unsupported legacy state rather than migration
input; it is neither translated nor automatically removed. The permanent
retired-host terminal guard remains independent of this clean-break rule.

## Lifecycle model

The MVP owns one shared cluster `tool_local` target with stable ID
`cluster-default`, concept-separated editable configuration, and one owner-only
active Workspace-authority generation selected by
`$XDG_STATE_HOME/tobari/authority/active.json`. The referenced manifest binds
immutable Template, Context, Context-owned Policy Memory, and optional Workspace
objects plus the installation default Template selection, the location-free
current Context selector, and independent
desired/applied/active receipts. Each Workspace owns its
separate persistent home and is permanently bound to one Context. Logical
authority, not Docker inspection, defines whether a Context or Workspace
exists. Docker labels include:

```text
io.tobari.owner=default
io.tobari.component=tobari|gateway|opa
io.tobari.id=<stable ID when applicable>
io.tobari.role=work|network
io.tobari.version=<asset revision>
```

Task assistance is a sibling of Workspace attachment, not part of fresh root:

```text
runtime assist --id RUNTIME_REF -> reserve Runtime-scoped task
  -> materialize target source + installation-owned per-agent Home
  -> prepare installation standard Runtime -> attach bounded Configurator
policy assist [--context CONTEXT_REF] -> resolve current or exact override
  -> reserve Context-scoped task
  -> revalidate Context/Template/Policy Memory under catalog fence
  -> materialize policy source in the complete Context Home
  -> prepare exact Context Runtime -> attach bounded Configurator
both
  -> agent exit -> cleanup -> freeze immutable target submission -> trusted review
  -> Runtime: source CAS publish -> separate runtime build
  -> Policy: Context-scoped Stage -> Template Plan -> final confirmation -> Apply
```

Attachment executes the selected pinned agent's ordinary interactive command
with one fixed positional initial request. The request tells it to read the
generated task guidance and copied evidence, explain the exact writable source,
and ask one concrete target question before editing. It carries no Project
source, confirmation, or Apply authority; task behavior remains in generated
files inside the managed Home.

The Configurator container, external route, and tmpfs are attachment state.
Metadata and working material are owner-only persistent state, not active
authority. Reservation metadata is committed before Home/source material.
For Runtime assistance the retained identity includes the target managed
Runtime's exact editable-source digest in addition to the execution Runtime.
Frozen task metadata records the last immutable submission, a separate durable
Apply-confirmed bit, and its settlement state. Until confirmation, recovery
may re-review the frozen submission without rerunning the agent. After
confirmation, recovery either resumes its exact canonical Apply or verifies
publication under the authority-owning fence and settles it; caller-supplied
new seed data is never publication evidence. Project+task+target discovery
permits only that exact retained generation before the next one is reserved.
Task recovery recognizes the exact deterministic V2 identity of a pre-release
aggregate receipt from bounded owner-only metadata before current task
validation. It excludes that receipt and neither adopts, migrates, nor deletes
it; every record that does not match the closed legacy aggregate identity still
fails current-schema validation closed.
Project and Template stage leases make one Project's pending policy Stage
unique. The Stage journal precedes canonical source mutation, retains exact
fingerprint and opaque Plan identity, and settles only after Apply. A surviving
policy Stage remains authoritative even when active policy already equals the
reviewed no-op, so task settlement cannot bypass canonical source bookkeeping. Runtime
source publication uses the same lifecycle/store fence and exact base digest.
Every Configurator task-scope, catalog, and Template-stage lease opens its
fixed child through one checked boundary. An existing child must already be an
owner-only 0600 regular file with one link; an absent child is created
exclusively and without following links where the platform supports it. The
opened descriptor and current path are then required to name the same file.
Alias, replacement, ownership, type, mode, or link-count failure occurs before
canonical source mutation and is never repaired implicitly.
When the final-authority Template Apply settlement journal remains, task
recovery returns that receipt's unchanged opaque Plan reference to the CLI;
the CLI re-enters canonical Template Apply directly and only then settles the
Stage and task receipt. It never derives a replacement Plan from already
published authority.
Context deletion takes the existing Catalog-to-Template-Stage lock order and
rejects while any retained Stage names that exact Context, whether or not Apply
is confirmed. This keeps the Context authority and complete Home alive through
the gap between pending-Plan discovery and canonical Apply; deletion becomes
eligible only after Stage settlement. When the eligible Context is the current
Context, the same atomic publication clears CurrentContextID rather than
requiring an unrelated Context selection first. A fresh eligible deletion runs
the application-owned generic Docker readiness check inside the infrastructure
planning fence before recording the durable aggregate-settlement decision. An
already-recorded same-action decision skips that fresh preflight and resumes
its exact settlement, so environmental failure cannot relabel partial state as
a no-change precondition. If the exact reference names only a valid unpublished
Context source, the same command removes that draft under the Catalog and Stage
fences without Docker readiness, Home retirement, or aggregate publication.
An unmaterialized Runtime task whose source authority has drifted is
first marked `retiring` durably, then cleaned idempotently and marked `retired`;
discovery completes an interrupted retirement before admitting another task.
Cleanup removes transient resources first through bounded immutable-ID attempts
and never guesses, retargets, or deletes a drifted Home. Unconfirmed transient
removal is typed partial state. Context deletion remains the complete Home
retirement boundary; Workspace deletion preserves it.
Workspace entry acquires shared exact Context/Workspace-root attachment leases before
runtime reconciliation and transfers them to the interactive session owner
until `Close`. Configurator materialization and Context/Workspace retirement
take those keys exclusively, so multiple Workspace borrowers may share one
managed Home while direct-egress Configurator or retirement cannot overlap it.
After Gateway/OPA and Workspace confirmation, `BeginFinalWorkspaceSession`
also acquires one installation-wide shared session fence for that borrower and
retains it through close. Gateway or cluster replacement takes the same global
key exclusively. Acquiring the global shared fence only after settlement
prevents entry from blocking its own no-live-session proof; the lifecycle fence
closes the settlement-to-session gap.
The root steady path treats authority as a candidate and invokes a current-only
entry port. Under the ordinary leases, that port performs one coherent
read-only Runtime/Workspace/protection preflight, deliberately excluding the
volatile attachment registry whose admission is owned by the shared lease. It
has no Runtime preparation, Docker reconciliation, or authority-publication
capability. An exactly missing canonical standard Runtime image or coherent
protection mismatch returns to the ordinary root readiness and
cluster/Workspace recovery flow. Unknown image observation and recovery-journal
residue fail closed; the current-only path leaves every recovery artifact
byte-exact, and cleanup remains owned by the ordinary recovery flow. In that ordinary
flow, Workspace entry takes the global session key exclusively before writing a
durable repair decision. It holds that fence through Workspace runtime
reconciliation, then releases it before Gateway settlement, which reacquires
the same key for its own no-session proof. The surrounding lifecycle fence
prevents another session from entering between those phases. A surviving
borrower therefore blocks repair even after the canonical attachment owner has
exited.
Task assistance first reserves only identity metadata, then re-reads the
exact selected Context, Project, Template revision, and Policy Memory revision
under the catalog fence while acquiring that same attachment lease. It creates
or updates managed Home material only after the lease is held, so deletion or a
live Workspace cannot be crossed by a stale selection.

Interactive session attachment is a separate, transient process state. The
runtime adapter starts the work container with the infrastructure-owned
`sleep infinity` lifetime process independently of the selected image `CMD`,
then enters it through one `docker exec -i -t ... /bin/bash` child session.
Exact commands use the same child-exec boundary.
The attached host process also owns the optional pinned-client login bridge.
Every invocation establishes or borrows one Host Loopback Attachment Epoch,
the strict route registry entry, advisory constant capability
projection in `TOBARI_CAPABILITIES_JSON`, and physical loopback relays. These objects are not logical
Workspace state. The owning invocation removes them when it exits; a borrower
cannot remove or extend them, and loses access when the owner exits.
Docker directly owns the attached shell's stdin, stdout, stderr, raw mode,
signals, resize behavior, and container PTY. On Unix, the optional structured
stdout presentation attaches the Docker CLI to a host-side PTY relay: the
relay preserves the child terminal identity, forwards input and window-size
changes, and reads the PTY master only to add bounded display-only syntax
colors. It does not parse input, change command meaning, or become authority;
the ordinary direct stream path remains the fallback. No attachment prefix is
reserved, including `Ctrl+]`, and Permission Inbox keeps its own trusted-host
terminal. Restoring an arbitrary child's alternate screen would require a
separately accepted terminal-emulation boundary rather than extending this
relay. A separate Docker exec starts one
fixed Unix-socket agent whose stdin/stdout carries bounded schema-v1 browser
requests and boolean responses. The binary-owned opener is mounted read-only at
the projected browser paths and emits no terminal output. The host rejects
malformed framing, duplicate or unknown fields, requests beyond the attachment
budget, and URLs outside the closed semantic union. One fresh compile-time
driver registry owns that union; each entry binds a stable driver ID to one
semantic target parser and open-only or callback-bearing mode. Selection scans
the complete registry and rejects ambiguous matches, so order cannot widen
authority. Each parser validates mandatory semantic fields independently from
an explicit finite set of reviewed optional selectors; total field count is
not a substitute for either check, and every unknown or duplicate field still
fails closed. The relay verifies the
selected container labels, binds the URL's validated
non-privileged loopback port, opens the reviewed host URL, and transports
callback bytes without parsing them. Device-flow confirmation remains entirely
inside the native CLI before it invokes the opener. The listener is nested inside this
attachment lifetime and never becomes Workspace or cluster state.

The state model is:

```text
Workspace absent
  -> tobari
Attached session + Workspace exists
  -> exit
Detached session + Workspace exists
  -> tobari
Attached session + Workspace exists
  -> exit
Detached session + Workspace exists
  -> workspace list
  -> workspace delete --id WORKSPACE_REF --confirm=delete
Workspace absent
```

The child Bash `exit`, or any exit from `tobari -- COMMAND [ARG...]`, ends only
the foreground exec process and returns its exact status to the host; it does
not fall through to Bash or stop or delete the work container, logical
Workspace record or per-Workspace home. There is no persisted stopped or
paused state. Reference-bound `workspace delete` removes the logical
Workspace; an attached exec makes ordinary deletion fail, and `--force` is the
explicit host-side override of only that guard.

Explicit standard `cluster up` validates configuration, locally materializes and
tests the complete all-Context policy projection, obtains and preflights the
exact Gateway image, and runs one bounded networkless probe that mounts every
required host source read-only and verifies its exact file or directory type.
Inside the fresh-action lifecycle fence, direct `cluster up` invokes the same
application-owned generic Docker readiness service as root entry. A failed CLI,
Engine, selected-context, Compose, or Engine-version observation therefore
returns a classified no-change precondition before any durable decision. An
already-durable same-action decision skips this fresh check so recovery cannot
be relabeled as no change.
Only a fresh task runs this probe, and it finishes before the final-authority
store publishes the durable cluster decision; exact recovery of an existing
decision bypasses preflight and resumes its journaled settlement. The mutation
then locally ensures every required runtime image, reconciles exactly one OPA
and one Gateway, and
reconnects Gateway and OPA to their required shared networks plus Gateway to
every existing registered project network. It completes
only after OPA serves the exact aggregate revision and a defined decision
document.
Policy/image/bind preparation failure precedes the cluster decision, cluster
journal, shared network, or named service-container mutation. Local
Tobari-managed image development uses
`task build` and the source-hash development resolver instead of a public
cluster option.
Policy testing uses the fixed bundle volume without weakening the later fresh-
resource fence. When preflight creates that volume, preflight owns exact
owner-verified removal on success, failure, and cancellation using a bounded
process-lifetime cleanup context. A volume that existed before preflight is
retained and rejected by the fresh-resource proof. Only the journaled cluster
activation creates the persistent bundle volume covered by rollback authority.
On first use, root validates the canonical Project root and observes known empty
final Template/Context authority before rendering one typed recommended draft.
The draft has no ID or persistence. Start and Customize both produce one
complete reviewed Template body, then call the same final default-pair
application boundary; neither CLI branch writes Template, default selection,
Context, or Policy Memory directly. The boundary publishes the default
Template, selects it, creates one location-free Context, and then binds its
Workspace to the canonical CWD under one final-authority lock.
Confirmed publication is durable; later cluster or entry failure never rolls it
back.

Before the first mutation, the application runs the closed generic
Workspace-start readiness port. Infrastructure executes only fixed bounded
Docker CLI/Engine-version/selected-context/Compose reads. The application
enforces Engine major version 24. No layer detects, opens, starts, stops, or
otherwise manages the Docker provider.

Root then calls the canonical final cluster reconciler with its own Catalog
Intent and retains the returned collection generation/revision. A bounded
read-only refresh must match that exact receipt plus the reviewed
Project/selected-Workspace/Context desired authority and selected Context
activation receipts before entry. This prevents cluster activation A→B from
being mistaken for caller drift while still stopping on real post-mutation
drift. The Workspace-entry application boundary alone reconciles the Workspace
AppliedEntry and establishes child ownership. Its infrastructure port first
prepares the exact Runtime binding selected by final authority, but only when
that binding is the canonical built-in standard Runtime and its source-addressed
local image is absent. The subsequent `PlanWorkspaceEntry` call remains
read-only and immutable. Custom managed Runtime material is never built by
entry. Root has no second Runtime selector, restorer, pruner, deleter, provider
repairer, or parallel status recovery model.

Shared Compose networks use Docker-assigned addresses and stable service
aliases; they carry no static-IP authority because their Compose definitions do
not declare user-configured subnets. Per-Workspace networks retain the exact
address authority required by transparent routing. Reconciliation therefore
uses alias-only connects for `tobari-control` and `tobari-egress`, and `--ip`
only for a Workspace-owned network. After replacement, the adapter observes the
new Docker-assigned shared addresses and durably replaces them in the active
settlement candidate journal before exact policy and recovery confirmation.

Invocation progress is a domain-validated ordered sequence of five stage/state
events projected by a line-oriented CLI renderer. It is process-local,
non-authoritative, and cannot affect nested application results. The entry port
emits only Workspace-prepared and handoff events because it owns those exact
checkpoints; root owns readiness, desired resolution, and cluster checkpoints.
Before constructing that progress journey for an existing Workspace, root may
classify exact-current desired, active, and applied authority as a direct
handoff candidate. One coherent read-only entry preflight must additionally
prove live Runtime, Workspace, cluster receipt, and protection current. The
current-only entry adapter runs that preflight with the exact resolution under
its ordinary locks without any preparation or reconciliation
authority. An exactly missing rebuildable canonical standard image and coherent
protection drift keep the five-stage reconciliation path; pending journals and
live uncertainty remain fail-closed and never become inferred readiness.
Template policy and Policy Memory remain independent required receipts, but
entry confirms both through one coherent live aggregate observation rather
than repeating the same Gateway/OPA proof for each axis.
The renderer applies bounded anti-flicker, elapsed, wait-reason, and heartbeat
timers and accepts no Docker/BuildKit/child text as a state transition. JSON
error mode composes this renderer with no writer; progress callbacks remain
non-authoritative no-ops and the error finalizer owns the complete stderr
document.

Outside that fresh final-authority composition, root observes whether the configured
cluster is running and whether its policy, Gateway, and principal projections
are valid for the current binary. Gateway projection observation includes the
live required Gateway/OPA shared-network joins, and principal projection
observation compares every registered endpoint with the exact currently owned
Workspace and Gateway network addresses. These are read-only Docker inspections;
they never reconnect a network. A ready observation proceeds unchanged; an
absent, stopped, or invalid observation composes the same exact typed
`cluster up` action before Workspace mutation.
An older active aggregate fails closed with exact `cluster up` recovery. It then reads the
canonical CWD's indexed Workspace candidates. An exact current-root record is
selected directly; when only ancestor records exist, the CLI presents every
containing root nearest-first and the application accepts either one validated
candidate or an explicit create-at-CWD choice. The choice is revalidated under
the lifecycle lock before the selected logical record is created or reused.
Selection retains the canonical invocation CWD independently from the selected
Project root: runtime binding and source ownership use the selected root, while
the attached process starts at the corresponding descendant path. It
then resolves the selected record's root-scoped Workspace Template Git fallback before
Docker calls and resolves the bound Workspace Template image. Standard reconciliation
uses an explicit empty authentication projection and neither inspects nor
creates research authentication state. Research reconciliation
requires the Broker to be ready and reconciles the Workspace Template's configured
project-bound handle projection. Both profiles then ensure one exact project
network and work container, connect Gateway with the `gateway` alias, reconcile
and verify both network-namespace guards, bind the exact owned Workspace source
endpoint plus Gateway endpoint to the host-issued Context principal,
wait for project readiness, and enter the container. The
logical creation and deletion boundaries use durable journals so an interruption
between the home, instance, index, runtime, and deletion steps is recoverable
without treating a partial file set as a second project. Runtime convergence
stores a hash of the bound Workspace Template's desired image identity, mounts, security,
environment, health contract, fixed resource contract, and profile revision on
the project container; drift recreates only that container and updates the
stored project image only after success. OPA runs with `--watch --bundle`
against one read-only Docker-managed bundle volume. Source edits become authority only
after explicit whole-projection validation and activation. The principal registry is a generic host-issued
contract: Docker currently supplies the owned network/endpoints observation, while a
future stronger runtime may supply the same binding through another adapter.
Only its dedicated directory is mounted read-only into Gateway; lifecycle
updates replace the registry file atomically inside that directory so a
single-file bind mount cannot strand Gateway on an old inode or expose the
neighboring credential configuration. Gateway caches a validated registry only
while its complete stat identity remains unchanged and fails closed, without a
last-known-good fallback, when a replacement is invalid.
Project runtime reconciliation first observes the exact owned container,
network, endpoint pair, desired runtime specification and source access,
connectivity, and health without mutating Docker. A complete match retains the
existing binding while the Gateway and Workspace guards are revalidated, then
atomically refreshes the derived current endpoints. Any drift takes the slower
path that closes the binding before a resource can change; interruption on that
path leaves the source unregistered for explicit cluster reconciliation.
Whole-projection Gateway settlement observes running Workspace principals from
their live endpoints. For an exact stopped Workspace it instead admits only the
already-published principal row from the prior active aggregate, after matching
the stopped container ID/labels/spec, its configured static address, the owned
network, the current Gateway attachment, and the owner-only registry row. The
settlement brackets that observation with the same registry snapshot and never
restarts the stopped Workspace. This closes stopped-ancestor nested creation
without treating a missing, created, dead, drifted, or unregistered container
as principal authority.
Context and Workspace IDs are not trusted when echoed by a caller; Gateway
derives both from the kernel-observed Workspace source endpoint and the exact
host registry binding. Exact allow, deny, and reset
actions provide the deterministic portable activation path: each locks the
projection, tests the target Workspace Template's private source copy and the complete
all-Context candidate, verifies the exact OPA and bundle-volume ownership
labels, builds a revision-named archive through pinned OPA, atomically renames
it through a fixed networkless pinned publisher, and transfers the tested
owner-only host projection through a bounded owner-only archive and container
stdin into the bundle volume rather than giving container root a host bind,
waits for the running OPA to report the exact expected revision, and rolls back
on failure. Reducing or mixed authority first activates a deny-all transition
revision.
The target-copy test yields a one-shot in-memory receipt bound to the exact
preflight bytes, candidate source digest, Context ID, bound Template policy
slice, and source transaction. Aggregate construction may consume it only for
that same candidate; a missing or mismatched receipt reruns the per-Context OPA test. The
complete aggregate candidate test, existing aggregate revalidation, activation
test, and authority-reduction fence are never skipped.
`workspace delete` consumes one exact opaque Workspace reference, observes
active Docker exec IDs before ordinary removal, then verifies owner, ID, and
role labels before removing the selected container and network; an attached
exec rejects ordinary deletion and `--force` skips only that guard. It then
removes only its XDG home and Workspace record. Container
or network loss is reconciled by the root operation; it never deletes logical
state. Cluster status and reconcile derive project counts and network joins
from the indexed CWD-owned records.
Status and `workspace list` expose bounded contradictory or incomplete state;
root entry refuses to invent authority and `workspace delete` remains the exact
reference-bound cleanup path only when discovery can produce that reference.
Cluster removal is rejected until no instance record remains. Both `cluster
down` forms remove shared runtime resources while preserving encrypted Workspace Template
vaults and the installation root key. `--purge` additionally removes only the
shared CA and active policy-bundle volumes; cluster teardown is not credential
logout or revocation. The stopped receipt orders purge above retained-volume
down: an exact completed purge may satisfy a later retained-volume request only
after runtime and volume absence are reconfirmed. Active journals remain bound
to the exact requested mode, and retained-volume completion can move to purge
only through the explicit upgrade settlement.

The authority collection enforces one Context per `(canonical root, Workspace
Template ID)` and at most one replaceable Workspace per Context. The lifecycle
lock checks the exact typed identities again immediately before creation. A
repeated or concurrent creation for one Context converges to one Workspace; a
different Workspace Template may own a separate Context and Workspace at the
same root.
Already-mounted parent/child roots may also overlap and run concurrently. Their
host-file effects are intentionally shared; the architecture does not add
overlays, checkout copies, or filesystem integrity isolation. Container
materialization is narrower: under the installation project lock, the runtime
enumerates and strictly inspects every owned work container immediately before
starting an absent or stopped target. A live read-write strict ancestor blocks
the descendant start with a typed precondition fault, because Docker records a
source path at create and resolves it at start rather than retaining the
host-selected inode. Same-root, descendant, read-only or stopped ancestor, and
already-running exact target observations remain admissible. Foreign,
malformed, duplicate, oversized, or contradictory Docker evidence fails
closed, and Tobari does not pause a sibling as an implicit second mutation.

## Command catalog

`cli.Catalog` is the only registry for public paths, roles, effects, fixed
targets, inputs, outputs, failures, routing, and human/agent help. The root
operation is represented as a catalog-owned fixed current-directory target even
though it has no argv path words. Handlers receive parsed inputs and call one
application service. Root `tobari` declares a complete CWD-owned mutation
impact; `workspace delete` is separately reference-bound to one discovered
Workspace. `tobari` keeps its target fixed to the canonical CWD even when its
selected Workspace root is an ancestor. `status` has a dedicated
task-owned read port: it canonicalizes CWD, selects the nearest existing
Workspace ProjectRoot without inferring a Context from location, then resolves
that Workspace's Context and applies the exact installation default within that
root. Its one snapshot revalidates root and
authority after bounded live observation; presentation performs no joins.
`status` has no selector, mutation lock, recovery route, or sibling-handler
call. The selected Workspace reference is its only produced reference.
`workspace list` reports its own exhaustive inventory independently.
The dependency-free terminal capability is an infrastructure
adapter used only by the CLI's human selector; a line-input fallback keeps
raw-mode availability out of the public command contract.

Catalog output fields recursively own every object child and array item,
including required/optional, nullable, enum, opaque-reference kind, and
semantic-scope facts. Object and field counts are bounded. The JSON renderer
validates its exact top-level envelope, schema version, nested keys, types,
enums, and nullability against that declaration before writing stdout; missing
and extra keys fail closed. Scoped agent-help schema 1 publishes the same
recursive declaration and exact success/error argv forms, including global
flag placement. Root help remains an index.

Produced opaque references come from one explicit bounded traversal of that
same output tree. Object children use dot paths and array items use `[]`, so
paths such as `metadata.owner_ref`, `ids[]`, and
`items[].runtime_ref` retain their declared structure. The resulting reference
kind/path pairs are the sole producer input to Catalog role validation,
required-reference reachability, scoped help, and grouped workflows. A command
does not rediscover nested references from a rendered document, Go reflection,
or a capability-specific walker. Consumed references continue to derive from
typed Catalog inputs.

Shell completion uses two public read-only utilities under `completion`. The
`completion zsh` handler emits a static adapter with no copied command registry.
On each Tab, that adapter invokes `completion candidates` with the bounded
one-based current-word index and exact shell-word vector. The CLI planner reads
public `cli.Catalog` paths and typed inputs for command hierarchy, flags,
booleans, finite values, conflicts, and directory directives. Inputs that need
local values declare one closed `InputCompletion` source in their catalog
contract. CLI maps that source to `internal/app/completioncmd`, whose narrow
port reads validated Workspace Template lists, Runtime lists, and managed Runtime history
from the existing infrastructure adapter. It does not invoke another command,
parse rendered output, reach Docker or the network, inspect arbitrary paths, or
mutate startup state. Internal catalog entries remain excluded from public
copies, lookup, routing, reference workflows, help, and completion.

In the research build, the four auth commands share one catalog-declared `authentication.broker`
capability. Login, import, and logout are fixed-target writes to the one
installation credential catalog; status is a Context-scoped read. The
application resolves the explicit or default Workspace Template before the infrastructure
port, validates mutation intent and impact before acquisition/vault I/O, and
accepts import material only through the declared stdin input. Terminal import
stdin is rejected before reading. Non-terminal bytes are read after public
argument/intent/mutation validation; infrastructure then validates the selected
existing Workspace Template, installed provider/acquisition mode, and broker readiness
before broker send. Provider IDs are human selectors validated against the
installed projection; they are not opaque action references or credential
authority. The compiled union includes GitHub, Datadog, OpenAI, Anthropic, and
AWS; AWS alone adds its method axis. Interactive omission opens the bounded
selector. Import, status, and logout remain
available for strict owner static manifests and Chatwork.

Standard `doctor` composes bounded read-only environment, Docker, policy, and
project-binding diagnostics. Research doctor adds provider, root-key/vault,
and broker checks. The application-owned
finite DAG schedules each infrastructure observation once its declared direct
prerequisites pass, continues independent branches, and creates typed blocked
rows for unready dependents. Infrastructure returns only pass, warning, or
failure observations; application code owns recovery and validates the fixed
complete order. Doctor JSON schema 1 and the text/TSV renderers project those
same facts. It fails the command with `diagnostic_failed` when any check fails,
and treats warnings alone as healthy. It observes rather than repairs: policy
checks validate bounded host source structure and do not start an OPA test
container, and doctor does not create a
root key, initialize or activate policy, start/reconcile/unlock the cluster, or
mutate provider, vault, credential, handle, or project-auth state.

Every catalog command that supports human text explicitly declares the shared
semantic-token presentation. The CLI presentation layer owns the exact
`text`, `muted`, `accent`, `success`, `warning`, and `danger` vocabulary and is
the only production location that maps those meanings to ANSI color or
emphasis for catalog-rendered output. That mapping uses named ANSI colors so
the terminal theme owns the concrete palette; fixed 256-color and truecolor
coordinates are rejected. Attached Workspace child stdout has a
separate infrastructure-owned syntax-color projection; it maps only bounded
JSON/YAML token classes and never changes task-owned catalog content. Command renderers select tokens by
information meaning; they do not own escape sequences or concrete colors.
The separate trusted-host login stream boundary may regenerate only its closed
reviewed upstream SGR vocabulary as the same semantic styles; it visibly
projects every unknown control and has dedicated conformance tests.
Infrastructure reports terminal
capability and the presence-only `NO_COLOR` environment preference. The CLI
combines those facts per output stream, keeping redirected and machine output
free of ANSI styling. Cursor-control sequences used by bounded interactive
selectors remain a separate terminal mechanism and do not define visual
styles.
The same layer owns lifecycle resource-card composition. Mutation renderers
project command outcome, resulting resource state, one primary continuation,
manual alternatives, and identity details as separate sections. Collection
renderers project the typed exhaustive count and use a human task anchor before
secondary canonical references. These renderers consume already-validated
task results and do not derive identity, ownership, readiness, or relationships
from their card structure.

Human text is built in three independent stages. Task-owned typed results first
select a `humanOutput` document containing headings, rows, sections, empty-state
scope/bounds, and exact recovery. The style projection may then add only the
six semantic ANSI tokens; it cannot select, remove, reorder, or rename document
content. The terminal interaction state machine independently owns terminal-size
observation, wrapped physical-row measurement, pre-save region reservation,
inline main-screen origin save/restore, below-origin repaint, redraw
eligibility, explicit raw-mode CRLF, and fail-closed cursor restoration. It
never enters DEC private mode 1049, clears scrollback, or locates a prior frame
by logical line count because one line may occupy multiple physical rows after
terminal wrapping. Stored dimensions fence every restore; resize, unknown-size
output, and oversized frames use complete append-only rendering. Non-ASCII
display width is conservatively over-budgeted. The
final frame remains in terminal history.
Its reader absorbs idle VTIME polls without returning a render event, while
input, selection changes, completion, and cancellation remain observable state transitions. Catalog
selection supplies bare-namespace normalization and deterministic typo
suggestions, so routing and recovery do not create a second command registry.
The deterministic CLI-experience evaluator combines these ownership checks
with public Catalog handler reachability. It rejects a presentation owner that
is not reached by the registered command and rejects retained predecessor
owners even when their focused tests pass. The shared progress module alone
owns the Braille frames and timing; task renderers supply only a truthful
active-work label and typed completion result.

## Gateway request flow

Standard Gateway establishes the source-bound Context principal,
normalizes the transparent authority, redacts authentication and cookies from
OPA/audit, asks OPA about the ordinary exact HTTP effect, and only after allow
forwards the original Workspace-owned credential in one upstream attempt. It
has no provider projection or Broker adapter. The Claude/Codex native-login
regression exercises this sequence, including Claude's authenticated profile
metadata request after token exchange.

### Research Broker augmentation

The research Gateway layer extends the same sequence as follows:

```text
client request headers
  -> establish the host-issued Context principal from the source
     endpoint at the header hook
  -> for transparent ingress, require one consistent SNI/HTTP authority and
     bind it as the requested server address instead of the synthetic target
  -> reject every malformed, misplaced, ambiguous, or binding-mismatched Tobari
     handle marker as credential_handle_invalid
  -> strictly recognize one valid broker handle from provider projection,
     remove it, and introspect its full non-secret binding
  -> at a declared header or AWS signing binding, reject any real Workspace
     credential as broker_auth_required before OPA or external I/O
  -> only when no declared binding and no Tobari handle marker exists, select
     Workspace-owned compatibility passthrough
  -> redact client authentication and cookie headers for OPA input
  -> classify only trusted Workspace Template-declared exact GraphQL endpoints
  -> structurally classify signed commercial AWS Query/JSON RPC without an AWS
     service model; reject claimed but unsupported or ambiguous RPC forms
  -> classify a validated EKS origin as Kubernetes, exact Git Smart HTTP
     transports as Git, and only distinctive OCI Distribution object routes
     as OCI; retain no object or pack body
  -> for ordinary HTTP, normalize body-free OPA input at the header hook
  -> for a declared GraphQL endpoint, require and retain one bounded request,
     parse the selected operation and canonical root fields, and normalize only
     those derived protocol coordinates with the HTTP input
  -> POST one fixed decision endpoint with finite timeout
  -> route by trusted Workspace Template ID inside the Tobari-owned OPA package
  -> require every declared GraphQL root coordinate and any broker-provider
     request to have its exact learned L7 rule rather
     than inheriting a broad static host/method allow
  -> require every classified AWS request to have its exact wire
     protocol/service/operation rule; never infer IAM or read/write semantics
  -> deny on any invalid/unavailable decision
  -> on a static brokered allow, resolve the same revision exactly once and
     replace only the declared destination header
  -> on a Datadog/OpenAI/Anthropic allow, select or refresh the same record once and
     apply only its reviewed bearer/supplemental-header result
  -> on an AWS allow, retain the authorized request within 8 MiB, obtain one
     private companion export, sign locally, and apply only those headers
  -> for the undeclared compatibility route, strip control headers and forward
     client authentication only after allow
  -> enable ordinary request-body streaming; forward an allowed buffered
     GraphQL or signed AWS body once
  -> resolve and pin the upstream address; reject unsafe dotted-host results
  -> stream the authorized upstream response from its headers
  -> emit redacted audit JSON
```

Ordinary body content is deliberately opaque to policy. Ordinary bodies are
neither retained nor hashed by Gateway. A trusted exact GraphQL endpoint is a
narrow pre-policy exception: Gateway accepts one positive length no greater
than 1 MiB, or an absent length without transfer/content encoding under the
fixed 8 MiB transport cap. It rejects a complete body over 1 MiB, parses one
strict UTF-8 JSON request, and sends only operation type and sorted canonical
root fields to OPA. It also accepts one body-free GET with a bounded strict
GraphQL parameter set, rejects mutation over GET, and removes every URL
parameter from OPA before policy. The original POST bytes are forwarded once
after allow; source text, operation name, aliases, fragment names, directives,
nested selections, arguments, variables, extensions, literal values, and body
hashes never enter policy, audit, learned state, or CLI output. Signed AWS RPC
on exact commercial AWS authorities is a separate structural exception. Query
RPC buffers one exact positive length of at most 8 MiB and retains only
`query`, the SigV4 service, and one `Action`; JSON RPC retains only `json`, the
SigV4 service, and one signed `X-Amz-Target`. The request supplies those tokens
dynamically. Gateway has no AWS schema or IAM/read-write classifier, does not
retain other form fields or JSON body fields, and rejects unsigned, ambiguous,
streaming, URL-query-mixed, or unsupported RPC forms before learning. Client authentication can be present on the forwarded
request but is absent from OPA input and audit output. No query or headers are
emitted in audit. Audit retains the path component, except that any path
containing a Tobari handle marker becomes `/[redacted-auth-handle]`. The
protocol-derived state-change value is a deterministic review projection of
validated identity and is never sent back as an authorization selector. Structural
URL/header handle rejections are non-learnable and cannot become policy
candidates. Any Tobari-looking handle marker either enters the exact valid
broker route or fails as `credential_handle_invalid`; only complete marker
absence permits configured fallback. A valid candidate is removed before
broker or OPA I/O and is never forwarded on failure. Broker introspection
returns no secret; policy denial performs no resolution, refresh, companion
call, or signing. The addon has no managed or arbitrary dynamic fallback and
never retries.

Protocol classifier admission is inventory-driven without becoming a runtime
registry. Canonical top-level classifier modules use the
`gateway/addon/<protocol>_request.py` shape, while a harness-owned matrix names
their Gateway entry functions and executable evidence. Routine lint discovers
those modules, requires an exact matrix row, rejects parser I/O, and requires
all collision, local-failure, projection, privacy, fallback, downstream, and
finite-corpus dimensions before a new module can pass. Production routing and
each protocol's parser types remain explicit; no common parser abstraction is
introduced.

The aggregate projection converts only an already validated commercial EKS
bootstrap origin into a Kubernetes protocol authority. Gateway parses the
standard resource path locally, sends only verb/resource/dry-run identity to
OPA, and streams the opaque object body only after exact authorization. It
does not load Kubernetes OpenAPI, enumerate CRDs, or call discovery endpoints
to classify a request.

Git Smart HTTP needs no endpoint catalog. Gateway recognizes only the exact
`info/refs?service=git-{upload,receive}-pack` discovery form and the matching
POST RPC path/media type. It projects repository and service, leaves the pack
stream opaque, and prevents classified traffic from re-entering ordinary HTTP
or AWS routing.

OCI Distribution likewise needs no registry catalog or repository schema.
Gateway recognizes distinctive release-surface catalog, tag, manifest, blob,
referrer, and upload routes under `/v2/`, then projects only repository,
action, and object identity. Cross-repository mount identity retains both its
digest and source repository. Bodies, authentication, and raw query values
remain opaque. The base `/v2/` probe and unsupported `/v2/*` paths stay on the
ordinary HTTP path to avoid claiming unrelated versioned APIs; once an object
route is classified, it cannot re-enter ordinary HTTP or AWS routing.

Denied audit records are also the policy-development feedback interface. A
learnable Gateway denial carries a fixed host-side `tobari review permissions`
navigation hint, and session closure may summarize the pending queue on host
stderr. These are advisory only: they contain no action reference and cannot
approve or retry a request. The learnable response tells the caller to keep the
current Workspace and agent session running, use a separate trusted-host
terminal, and retry in that same Workspace only after confirmed Apply. A
non-learnable response instead names the read-only `tobari cluster denials`
diagnostic and exposes no review command. `tobari cluster denials` parses one bounded Gateway
log window, isolates and counts malformed or otherwise unprojectable
denial-shaped records, and returns every valid typed Workspace Template and project principal, host, port,
method, path, optional GraphQL operation/root or AWS wire-operation coordinate, reason, status,
  exact-rule learnability, request identity, timestamp, the aggregate revision,
  evaluator identity, policy-data identity, the unparsed-record count, and the
  exact review command. OPA computes
learnability only when version, cluster, Workspace Template, scheme, fixed port,
project-principal, and Workspace Template policy ceiling already pass, so an exact
Context/scheme/host/port/method/path rule, plus the GraphQL or AWS coordinate when
present, can close the request. `review permissions` and
`policy candidates` deterministically fold only that eligible retained evidence
by the structured Context/scheme/host/port/method/path and optional GraphQL or AWS
effect key. They emit one opaque
exact-rule reference, the latest evidence, and an observation count for each
pending effect; references remain stable across repeated denials. This pure
read projection also converges concurrent identical audit records without a
second persisted inbox or write race. They remove effects already covered by
the CLI-owned learned allow or deny data and trusted baseline deny rules.
The staged reviewed set retains the exact typed candidate evidence selected
from that projection. Before its durable decision boundary, the final-authority
mutator rereads non-durable candidates while holding the lifecycle fence and
requires both an unchanged collection receipt and byte-identical authority.
That private evidence continues through independent settlement and publication
validation; only candidates present in the durable predecessor collection are
removed from the next collection. Once the exact effect decision is durable,
recovery uses its retained evidence instead of depending on a denial that may
disappear after successful activation.
The Permission Inbox adapter takes the installation's read-only lifecycle
observation lock, then reads the active decision journal and complete collection
as one coherent observation. An exact active reviewed-Apply decision becomes a
private typed snapshot recovery field bound to either its predecessor receipt
or its recorded-settlement successor receipt. It never enters candidate items
or serialization. A fresh TTY can pass only that unchanged set back to the
internal fixed-target action. Terminal journal evidence is deliberately not
projected as resumable; its recovery surface is the read-only learned-rule
inventory. When the lifecycle state is absent, observation performs a
zero-write read and rejects state that appears during the read instead of
creating a lock file.
Baseline denies remain audit-only. \`review permissions\` is the routine human
workflow over one validated final-authority snapshot. The CLI owns one
raw-terminal state machine with list, detail, and final-review states. It stages
only domain-valid decisions against unchanged opaque review-item IDs; exact
items can Allow or Deny, while path-template items can only Allow the typed
shape. Manual refresh returns to the application read boundary and discards the
prior staged map. Final confirmation exits raw mode before crossing either the
persistent reviewed-set mutation or the separate attachment-scoped mutation.
The two lifetimes cannot be mixed in one Apply. Redirected review remains a
read-only projection, and the release Catalog exposes no watch or notification
modifier. Infrastructure returns the revision only after running OPA confirms
it; the typed application receipt preserves that revision and ordered
decisions.
`policy rules` is the exhaustive current inventory of CLI-owned learned Allows
and exact Denies. `policy reset --id` removes exactly one such decision through
the same preflight, atomic-write, and OPA activation boundary, leaving the
matching effect at default deny so the retained denial can enter `review permissions`
again. It never edits baseline policy, grants permission, or retries a request.
Raw `cluster logs` remains the component-debugging interface.

`policy allow` resolves one exact candidate reference against retained
validated audit state without decoding it. Infrastructure appends one
deterministic exact learned rule to the Context-owned Policy Memory object,
publishes that immutable typed object through the same complete authority
generation transaction, tests a private complete policy projection, and calls
the existing OPA activation boundary. Deny and reset use the same transaction;
they never edit Template source.

Editable static policy is the closed `templates/<template-id>/policy.yaml`
document. The current `tobari.dev/template-policy/v1` compiler accepts only
`boundary.methods.deny` plus the sealed `semantic.protocols` and
`semantic.providers` inventories. Template Plan
and Apply validate the complete `template.yaml`/`policy.yaml` pair and publish
one immutable Template revision. Aggregate generation then joins that static
revision with separate Context Policy Memory and the fixed evaluator embedded
in Tobari. User-owned Template, Context, and configuration layouts contain no
executable policy source. Gateway runtime input uses exact schema 2
and rejects any other shape. The generated aggregate projection contains
immutable `data.json` plus private Tobari-owned router/evaluator modules; those
files are Docker-managed execution material, never Workspace Template or
Context source. The aggregate projection is schema 2 and stores the composed
typed data below `tobari_contexts[context_id]`; the Tobari-owned router is the only
`tobari.http` decision entrypoint. Each entry in `boundary.authorities` owns its
scheme, host, ports, and host-local methods;
`boundary.graphql_endpoints` declares exact protocol-classification points, and `rules` keeps
baseline denies, learned allows, and learned denies in separate collections.
Gateway and OPA share this structure. The CLI mutates learned collections only
through Policy Memory actions; reviewed Template Apply is the only current
writer of static Template authority.

The predecessor `v1alpha1` decoder is isolated behind the read-only
`template migration plan` and reference-bound `template migration apply`
ports. Migration stages and atomically replaces one complete desired source
directory, binds both byte fingerprints and the active revision, and never
publishes authority. Ordinary V1 readers cannot call the alpha decoder. A
subsequent ordinary Template Plan/Apply is the only activation path.

Routine mutations serialize through an in-process mutex and a cross-process
file lock. They prepare and fsync a complete sibling generation, record a
strict durable journal with original/candidate digests, move the prior
`domains/` generation to a recovery name, and rename the complete candidate
into place. A lockless reader therefore observes the old generation, a
fail-closed gap/journal, or the new generation, never a valid mixed pair.
Aggregate state records the candidate immutable projection revision before the
journal is finalized; recovery commits only when that revision is durable and
otherwise restores the validated original. Unexpected external edits make the
transaction ambiguous and fail closed instead of being overwritten.

Workspace Templates use one current Tobari-owned evaluator, projected once, with
Template-specific Method Boundary, closed HTTP/GraphQL/MCP/Git/OCI protocol
modules, closed AWS/Kubernetes provider modules, remembered decisions, and credential metadata
supplied as data. The evaluator owns the router and system packages; no
Template-specific executable module is accepted. The cluster router sends
every input carrying a GraphQL, MCP, AWS, Kubernetes, Git, or OCI coordinate
through the protocol-derived fixed evaluator, so classified traffic cannot
fall back to a coarse HTTP rule.

`policy deny` resolves the same exact candidate reference and appends one
Context-bound exact deny rule through the same aggregate preflight, atomic-write, and OPA
activation boundary. Exact denies are terminal and win over learned allows for
the same Context/scheme/host/port/method/path and optional protocol coordinate.

## Docker abstraction

Application code owns narrow ports such as `ResolveOrCreate`, `EnsureRuntime`,
`EnterRuntime`, `Inspect`, and `Delete`. The MVP infrastructure implementation
invokes the Docker CLI with fixed command structures and caller context. This
keeps Docker Engine API or Podman replacement possible without promising either
today. Arbitrary shell strings are never constructed; user commands are passed
as argv after Docker's `--`.

## Cancellation and errors

The command root separates four contexts: the current operation context, one
bounded settlement/classification context, the child context after handoff, and
one bounded cleanup context. Pre-execution cancellation makes zero Docker calls.
Before handoff, caller cancellation stops only the current canonical operation;
root then uses at most five seconds of cancellation-independent read-only
classification where a just-completed mutation receipt must be re-observed,
preserves confirmed or unknown structured state, prints one recovery, and exits
130. A late cancel cannot turn confirmed success into retry permission.

After handoff, the child owns terminal streams, signals, and exit status.
Attachment/service cleanup remains bounded under its existing owner; the CLI
may emit one host-owned cleanup-attention line after child exit, but cleanup
failure never replaces the child's exact status or triggers replay. The optional
root delimiter and repeatable positional input are Catalog-owned; a missing
command is rejected before lifecycle side effects. Lifecycle operations return
structured state after confirmed completion; unclassified post-mutation errors
are non-retryable and direct the user to a Catalog-validated read-only
reconciliation path.
The domain fault vocabulary separates causal identity (`kind` plus `code`) from
the closed command phase and strongest proved change state. Application and
infrastructure may strengthen state only from typed boundary evidence. Catalog
owns the published classification and rejects runtime disagreement; mutation
states `partial`, `confirmed`, and `unknown` recover first through a validated
read. CLI presentation projects these facts but never infers them.
One Catalog-wide traversal validates the release recovery graph from the same
command/fault metadata used by help and runtime dispatch. It resolves every
Next in-program, derives required-input and opaque-reference producer/consumer
edges, rejects nonretryable self-loops and closed cycles, and prevents a
read-only classifier from becoming an endless observation path. Status
continues to own typed PrimaryNext/Attention and non-command conditions; the
traversal adds no parallel status model or executable-string registry.
Auth mutations use the same structured-outcome rule. A failed or cancelled
GitHub host driver leaves the previous Context credential unchanged; a
host-login availability rejection carries one closed
infrastructure-owned diagnostic stage through the existing public fault code.
The stage vocabulary distinguishes driver dependency, executable lookup,
symlink/canonical-path/trusted-root checks, identity, a recognized ChatGPT app
bundle selection, and Codex executable/version observation without retaining a
raw cause, local path, digest, or provider output. An
`auth_mutation_outcome_unknown`, `unclassified_mutation_outcome`, or
`mutation_output_write_failed` result is non-retryable and directs the user to
`auth status` before another auth mutation. Confirmed login/import/logout output
is finalized before late cancellation can imply that replay is safe.
After a confirmed Auth Broker mutation, projection observation is advisory:
bounded registry, project, or binding-status failures become explicit
`unavailable` activation coverage and cannot turn confirmed `changed` or
`no_change` into replay permission. Infrastructure returns bounded, secret-free
observation facts only. `authcmd` and the Auth Broker domain validate request
Context, Workspace, and credential-revision correlation, then derive
activation state, exact re-entry actions, and mutation-change semantics.
Presentation never derives freshness from provider labels, process existence,
or row order.

## Architecture enforcement

- Go architecture lint preserves layer direction and a thin `cmd/tobari`.
- Whole-public-catalog tests fix the exact production handler for every public
  command in the release executable and both attachment helpers. New or
  rerouted paths must update that reviewed mapping, and focused CLI tests must
  execute their public mutation projection rather than stopping at the
  application service.
- Each claim belongs to the lowest layer that can prove it without simulating
  the subject of the claim. Domain owns pure semantics, application owns use-
  case ordering, infrastructure owns exact adapter requests and filesystem
  contracts, and CLI owns parsing and presentation. A higher layer keeps only
  one representative composition canary; it does not repeat the lower layer's
  input or output matrix.
- Domain and application tests prove path, state, effect, and orchestration
  invariants without Docker.
- Infrastructure tests use a recording command runner.
- Gateway and fixed-evaluator tests cover policy boundaries.
- Auth Broker, root-key, provider, companion, and Gateway integration tests
  cover locked startup, every closed vault record, project-bound handles,
  deny-before-action, bounded static/refresh/signing results,
  rotation/revocation, durable unknown-outcome barriers, and no fallback.
- Host-driver tests cover each fixed argv, canonical executable identity and
  digest recheck, private temporary home or PTY, bounded browser/output
  contract, checked cleanup, and cancellation settlement. Negative dependency
  and image tests prove managed profiles, arbitrary helpers, and provider CLIs
  inside Broker remain absent.
- Docker integration tests prove only actual topology, kernel enforcement,
  mounted/runtime isolation, live Gateway/OPA/Broker transport, watched policy
  activation, attachment-scoped host relay, and resource lifecycle. The fast
  harness rejects semantic command matrices and unreviewed growth in that
  scenario.
