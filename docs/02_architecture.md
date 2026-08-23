# Architecture

## System topology

```text
host
  tobari CLI ---- Docker CLI ---- Docker Engine
      +-- authenticated attachment TCP relay --> host 127.0.0.1:PORT
      +-- root A (Workspace Manifest-selected ro/rw) --> Tobari A -- guarded net A --+
      +-- root B (Workspace Manifest-selected ro/rw) --> Tobari B -- guarded net B --+--> tobari-gateway
                                                                      |
internal control network:                                      tobari-opa :8181
egress network:                              Gateway --> policy-allowed HTTPS
```

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
It is absent when no host session is attached. The experimental development override adds a
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
The pump preserves ordinary mitmproxy request/response streaming; the host
listener revalidates an active Allow for the requested port before connecting
to the same physical-host IPv4 loopback port. Workspace headers,
environment, paths, query, and DNS cannot select an epoch, relay port/token, or
target address. The capability projection inside the attached shell is an
advisory discovery surface and carries no routing secret or permission.

Workspace service exposure is a separate opposite-direction attachment branch.
The canonical base recipe cross-compiles a dedicated Linux `tobari-expose`
helper for Docker `TARGETARCH` with a pinned Go builder and an exact checked
source/module closure. Its main hardcodes the helper Program rather than
selecting authority from `argv[0]`. Before attachment, the host extracts the
helper and identity record from the verified source-derived base through a
bounded temporary container; validates source/API identity, SHA-256, regular
file type, safe mode, Linux ELF, and engine architecture; and atomically stores
one owner-only executable. Every selected Workspace, including one using a
managed custom Runtime, receives that executable as the same read-only
`/usr/local/bin/tobari-expose` mount. An unpredictable Workspace Unix socket
connects that helper to one fixed non-TTY control process; it can submit only
one non-privileged Workspace-loopback port, list current-attachment exposures,
or stop one unchanged opaque reference. The live host attachment
owns pending requests and exposes a distinct owner-only Unix rendezvous plus
atomic ephemeral record. A separate `tobari review services` process validates peer UID
and PID, nonce, attachment identity, and a fresh snapshot, but never owns the
listener or route lifetime. Allow once binds random host IPv4 loopback and a
bounded HTTP/1.1/WebSocket relay to exact Workspace loopback. This channel
shares no schema, socket, registry, authority, or data plane with browser login
or Host Loopback access.

Permission resume is a third, read-only attachment concern. The interactive
Tobari entry session created before the child is its canonical owner; Host
Loopback is only one capability on that session. Exactly one owner epoch may
exist for a Workspace Manifest/Workspace pair, concurrent borrower entries
share it, and service-exposure controller attachment IDs are ineligible. A
bounded private session registry joins one frozen schema-v1 Gateway principal
to canonical Workspace Manifest and Workspace IDs, attachment epoch, owner,
256-bit process-instance nonce, one closed platform ingestion transport, and
renewable lease. The owner PID and transport address are diagnostic only.
Trusted composition fixes native Linux to an owner-only Unix socket and macOS
Colima to a Darwin-kernel `127.0.0.1:0` listener reached by Gateway at exactly
`host.docker.internal`. Gateway accepts only its composed transport kind and
has no runtime probe, fallback, or downgrade. Transport kind, endpoint, nonce,
lease, and stable owner identity are exact record fields; nonce-first
constant-time authentication and host-side frame/deadline/concurrency/rate
bounds protect the channel, while endpoint and peer address grant no authority.
Gateway emits resume data only after that owner acknowledges the exact immutable
secret-free wait record and an exact post-acknowledgment registry read proves no
authority drift. The registry is mounted read-only into Gateway alone; its
nonce and endpoint never enter logs or public output.

The owner keeps the wait registry in memory and exposes one attachment-local
read-only Unix socket to `tobari-permission`. Observation delegates exact
effect evaluation and precedence to the canonical live OPA policy; it adds no
rule matcher, policy authority, persistent store, daemon, Workspace file, or
request replay. Teardown ends the authority and never rebinds waits to a new
attachment. Listener, heartbeat, and renewal failures close transport and
invalidate waits before exact bounded authority cleanup; an expired lease is
never renewed. Attachment cleanup faults remain typed secondary entry outcomes
and cannot overwrite the already-observed child exit status.

Attachment Grants are runtime-owned inputs to the complete per-request OPA projection and
are disjoint from Workspace Manifest `policy/domains` learned rules. Policy review binds
one grant to Workspace Manifest, project, epoch, target port, and exact effect. The route is
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
set. The experimental compile-time profile additionally activates the reviewed
AWS driver; runtime data cannot change profile membership.
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
Pup and Claude instead run from the selected Workspace Manifest image in fresh
mount-free containers. Pup binds the immutable image ID, observes bounded
semantic version syntax and executable digest without a compiled version
allowlist, then requires the fixed login, status, and native-state capture
contract. Claude runs exact 2.1.220 from the selected Workspace Manifest image in a fresh
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
  Workspace Manifest-authentication use cases with consumer-owned ports.
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

`cluster up`, `cluster status`, `cluster denials`, `policy candidates`,
`review permissions`, `policy allow`, `policy deny`, `policy rules`,
and `policy reset` remain a valid standard seam. `serve`, `auth login`, `auth import`,
`auth status`, and `auth logout` remain experimental-only seams
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

The Catalog spans both executable programs so the helper's produced service
reference can close through the host actions and the exposure reference can
close through helper stop. Validation and reference closure are global;
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

Experimental `serve` is a foreground CLI composition over the existing typed application
tasks. Before exposing a listener it obtains one valid installation snapshot:
cluster status, exhaustive Workspace inventory, bounded policy review, and
learned rules. Infrastructure owns an embedded no-external-asset HTTP surface
on `127.0.0.1:0`; it depends only on domain-shaped backend methods and makes no
policy decision. The CLI adapter delegates snapshot reads to `tobaricmd.Service`
and every browser Apply to the catalog-owned internal `policy apply-reviewed`
contract. The process lifetime owns the listener, session bearer, and browser
opener. Cancellation closes the surface without changing policy. Build-tagged
composition keeps its command, handler, and loopback infrastructure wiring out
of the standard profile.

### Workspace Manifest composition

Workspace Manifest is the user-facing stable desired definition of a
Workspace. A trusted Manifest has one stable WorkspaceManifestID and publishes
complete immutable desired revisions. WorkspaceManifestID plus semantic digest
is authority; generation is correlation only. A semantic no-op retains both
digest and generation. A→B→A publishes a later generation with the original A
digest. Retained storage may use `generation + digest` to distinguish those
receipts without changing action or validation authority.

The creation-time Boundary is the immutable capability envelope: direct source
access, policy mode, one normalized policy snapshot/revision, complete
destination/method ceilings, and native-readiness participation. The same
desired revision binds one exact installation-wide Runtime revision and
compatible image, plus the read-only agent profile, narrow session defaults,
and typed future-Workspace creation defaults. Fields remain one aggregate
because they share host owner and reuse scope, but activate separately: cluster
projection at `cluster up`, entry state at explicit Workspace entry, session
defaults for each later child session, and creation defaults once for a new
Workspace home. The standard Manifest locates no Auth Broker state.
Runtime source and revision history belong to the separate installation Runtime
catalog; a Workspace Manifest never owns or edits build source. Shell and Git settings are
late-bound session defaults, while bootstrap is a create-once default for
future Workspace homes. The manifest is not itself a mountable authority: policy
is mounted only into OPA and agent configuration is mounted read-only into the
work runtime. Tool-owned authentication remains in the per-Workspace home.
The Runtime adapter inventories at most 1,024 owner-only regular files and 256
owner-only directories under one safely opened source root, with 32 MiB per
file and 64 MiB total. It then streams canonical path/mode/size framing and the
same bytes copied into a private temporary snapshot through SHA-256 with one
fixed buffer. Source and opened-file identity/mode/size are revalidated around
each copy; Docker is invoked only after the complete snapshot exists. Public
source-contract faults carry only a bounded quoted relative path and reviewed
actual/limit or permission facts.
Runtime creation uses that same bounded inventory and drift-checked stream copy
when `--copy-source-from` selects a managed editable source. It stages source, a fresh stable
identity, empty history, and the manifest outside the visible Runtime catalog,
then publishes the complete standalone Runtime with one same-filesystem rename.
`standard` instead writes the canonical built-in starter into the same private
stage. Neither path persists source identity or lineage, reads an immutable revision
snapshot, builds an image, or changes a Workspace Manifest.
Only the experimental profile has encrypted Workspace Manifest vaults and projects a
project-bound handle.

Experimental Broker credential ownership is Workspace Manifest-wide. Every Workspace permanently bound to that
Workspace Manifest is eligible, but reconciliation issues a distinct project-bound handle
only on the Workspace's next matching entry; no running process is
rewritten. Replacement revokes the previous revision. Logout removes the
Workspace Manifest/provider credential and all handles immediately, while the next entry
recreates the work container without the environment projection and removes
only unchanged Tobari-owned complete files.

Every Workspace Manifest has a stable UUIDv7 identity, and every Workspace permanently
binds one Workspace Manifest. `(canonical root, Workspace Manifest ID)` selects one record, so the
same root may have multiple Manifest-bound Workspaces. `manifest default set` changes only
the default WorkspaceManifestID used when an invocation omits a selector; it does
not mutate existing records or Docker. Project runtime reconciliation resolves
the stored Workspace Manifest ID and uses that Workspace Manifest's runtime image and agent profile.
Tobari names the product and ownership boundary; Workspace is the resource
name. Domain/application and public lifecycle state use Workspace,
`workspace_id`, WorkspaceManifestID, and `project_root` consistently. Frozen
predecessor experimental Broker/Gateway protocol keys may retain `context_id`
or `project_id` until their separately governed build-profile migration; those
wire spellings do not become public aliases or Manifest desired/applied fields.

Argument-free `manifest create` is a CLI-owned input-completion workflow. When
persisted Workspace Manifests exist, CLI first reads the exhaustive local collection and
chooses one exact immutable source revision, with the default Manifest initially selected; zero
persisted Workspace Manifests skip that step. The application exposes the exact validated
copyable Manifest-owned snapshot. Review binds source WorkspaceManifestID,
semantic digest, and canonical body; infrastructure revalidates all three under
the lifecycle lock before publishing a fresh ID at generation 1. Copy is a
one-time initializer, not a target, parent, reference, live inheritance source,
or persisted lineage. It reads no Workspace/auth/permission/attachment/applied/
failure/observed/default state and causes no reconciliation. The workflow then
collects a name, source-access enum, and a complete default-plus-exact-override
method policy before calling the same application create boundary once. The
application owns typed composition and result correlation. Infrastructure
replaces the Workspace Manifest-owned method policy, filters only positive baseline
entries made unreachable by method Deny, normalizes the resulting immutable
snapshot, and persists its digest; CLI never edits policy files directly.
The ordinary interaction is Name -> Filesystem -> effective Network -> Review.
Optional Workspace-bootstrap editing calls application-owned read ports only
from Review; infrastructure returns typed resolved candidates and the final
action revalidates the selected semantic revision before mutation. The CLI
renders those facts but never reads host files or infers compatibility from
labels.
The interactive root may reuse this exact wizard only when Workspace Manifest observation
returns the display-only synthetic default. After creation, CLI composition
switches command identity before each further action: `manifest create`,
`cluster up`, and CWD Workspace entry each retain their
own catalog effect, fixed target, impact, application invoker, and mutation-
complete output boundary. Root never models those targets as one mutation and
never invokes the public CLI as a subprocess.

Omitted `runtime create --copy-source-from` is a smaller Base-first input-completion flow:
interactive text lists `standard` plus managed editable sources and skips the
chooser when only `standard` exists, while redirected and JSON omission binds
`standard` without a read. Omitted primary selectors for `runtime build` and `manifest runtime set` are a
smaller CLI-owned input-completion workflow. CLI composes existing Runtime and
Workspace Manifest read use cases into a terminal Review, binds an omitted default Workspace Manifest
to the exact name returned by `manifest show`, and passes only one selected
managed Runtime name or ready Runtime revision into the unchanged application
mutation. Workspace Manifest Runtime editing and confirmation are separate presentation
states: only a different selected binding enters the old-to-new Review with
Apply, while Back, unchanged selection, and cancellation remain read-only.
Review never reads Runtime source bytes and has no Docker or manifest write
port. Fully specified selectors bypass Review, while machine-readable or
non-interactive omission fails before the application mutation boundary.
The Runtime source chooser names the managed candidate as its current editable
source and does not present the latest successful head as copy identity.
Confirmed human results compose the separate catalog operations only through
exact next argv derived from the validated Runtime binding and default Manifest
selector; no renderer performs discovery or changes an effect.

`manifest delete` is a Workspace Manifest-catalog write serialized by the installation
lifecycle lock. Application maps the foundational/default/Workspace guards and
validates the terminal deletion result. Infrastructure checks durable Workspace
bindings by stable Workspace Manifest ID, then removes only the exact Workspace Manifest directory
and Workspace Manifest-ID authentication directory. It never selects another Workspace Manifest or
deletes a project root or shared image.

Workspace Manifest and project stores expose separate observation and ensure/mutation
paths. Observation never initializes directories, Manifests, default selectors,
vault state, policy, or lock files. A missing omitted Workspace Manifest becomes a typed
display-only synthetic default. Persisted state must match the exact V1
contract; only a validated create/write path may re-read under its mutation
lock and atomically initialize it. Project observation opens an existing
lock when present and otherwise remains lockless. A pre-existing validated
project journal is the narrow exception: observation acquires the recovery
lock, completes bounded cleanup, and then reads the remaining logical state.

A Workspace aggregate stores WorkspaceID, canonical ProjectRoot,
WorkspaceManifestID, create-once defaults, the last successful AppliedEntry,
and at most one latest reconciliation failure. DesiredEntry is projected from
the selected Manifest revision; observed runtime/attachment evidence is a
separate read model. Explicit entry is the only per-Workspace reconciliation
boundary. It observes before mutation, rejects pending adoption while attached,
resolves exact RuntimeID+semantic revision, and records AppliedEntry only after
runtime, health, network guards, endpoints, and principal publication verify.
A failed or canceled attempt retains the prior AppliedEntry and records the
attempted Manifest digest, entry digest, phase, and bounded change-state while
still holding the Workspace lifecycle lock. Routine UI derives Current and
Next entry from these typed facts; it never treats generation, image, labels,
timestamps, or container identity as desired/applied authority.

Runtime lifecycle consumers receive typed read ports for current and retained
Manifest Runtime references, last-successful AppliedEntry, pending adoption,
and observed Workspace/container references. These form the protection graph
seam. `AppliedEntry.reconciled_at` is not Runtime last-use authority, and this
decision does not implement Runtime delete/prune/restore.

Cluster state records one content-addressed projection revision and loaded
Workspace Manifest count, not an active enforcement Workspace Manifest. Standard `cluster up` builds
the projection from all authoritative Workspace Manifest policy sources,
validates each source and the whole candidate, publishes only a complete
owner-only directory, and starts exactly one Gateway and one OPA. Experimental
composition adds the provider projection and one locked Auth Broker.
Reconciliation is not ready on process health alone: OPA must
serve both the exact content-addressed aggregate revision and its decision
document. An already-ready identical revision is not republished. Cluster
reconciliation unlocks the broker through the host
root-key provider after its control endpoint is healthy and verifies the exact
Broker container. Policy
mutations serialize this same all-Manifest activation and preserve the previous
known-good revision on any failure.
Standard cluster status schema 1 projects Gateway and OPA. Experimental status
additionally projects Auth Broker, `auth_provider_projection`,
`auth_broker_state`, and `root_key_backend`.
Workspace Manifest report schema 1 exposes explicit Workspace Manifest
persistence state, nullable pre-authority ID/stores, the complete Workspace Manifest
shell-environment and Git identity policies plus `native_workspace`
authentication mode. Experimental output adds broker and installed-provider state without
returning a vault path/content, root key, primary secret, or handle. Public Linux backend values are `xdg_file`; the
infrastructure/doctor detail `linux_xdg_file` is not a public JSON enum. The
canonical schema/path/backend table is in
[Authentication handling](07_authentication.md#experimental-canonical-schemas-paths-and-backend-identifiers).

Domain projects validated Workspace Manifest reports and list items into typed routine
summaries: effective Access, routine-client availability after actual ceilings,
exact Runtime selection and action state, summarized shell/Git/bootstrap
defaults, and authentication ownership. CLI renders those summaries rather
than inferring readiness or continuation from labels. `manifest show --details`
projects the same application read into the complete sectioned diagnostic;
selection cannot add, remove, or reinterpret a typed field in JSON. The default
summary retains every effective method override, while host paths, stable IDs,
images, profiles, and revisions remain secondary. An inactive named Workspace Manifest is
never presented as the omitted current selection.

Workspace status similarly carries an internal typed routine summary and exact
Runtime selection resolved from the already selected Workspace Manifest manifest. Its
schema-2 JSON retains the complete Workspace Manifest lifecycle projection; the human
view suppresses healthy IDs, home paths, revisions, and unchanged bootstrap
state while retaining explicit runtime/session attention and exact recovery.

Workspace Manifest show presentation projects the same report through ADR 0071 lifecycle
classes. Boundary and Runtime binding are top-level timing groups. Shell/Git
and new-home AWS/EKS setup stay nested inside one Workspace-defaults group,
while login ownership remains outside those defaults. The renderer performs no
second observation and never turns the selected Runtime into an active-
Workspace claim or the create-only setup into existing-home state.

Narrow host projection is a separate composition concern, not a file or secret
mount. Domain owns each fixed scalar inventory, `default|inherit|literal`
invariants, complete reports, and Git's atomic name/email pair. Workspace Manifest
application use cases own the fixed-target writes; the owner-only Workspace Manifest store
persists only non-default policy. CLI owns two input-completion modes for the
same use cases: complete typed argv, or a terminal-only staged editor when the
entire setting group is omitted. The editor first reads typed complete current
state, writes no mutation before Apply, binds the returned Workspace Manifest name across
the human review, sends every distinct staged Shell row through one application
call and one atomic manifest replacement,
and never completes a partial direct invocation. Explicit-empty Workspace Manifest input
is rejected rather than collapsed into the omitted current-Workspace Manifest selector.

Workspace Manifest creation is the sole owner of Boundary defaults and initializes the
Runtime binding plus Workspace defaults. It resolves omitted source access to
`read-write` and omitted native readiness to `enabled`; it
normalizes and validates the complete Workspace Manifest method policy against the fixed
agent-ready compatibility baseline, binds its owner-only snapshot by SHA-256
revision, and atomically persists both
manifest and snapshot before returning. Creation may instead seed those values
from one exact Base revision. Infrastructure copies only Boundary, exact
Runtime binding, shell/Git defaults, future-Workspace bootstrap, and Advanced
policy source; it publishes the complete new Workspace Manifest with one catalog-local
rename and starts learned decisions empty. Workspace homes, authentication,
Attachment state, current selection, and Base identity never cross that
boundary. Observation and old-state readers never
invent either field. Root entry carries `source_access` into the exact project
runtime spec/hash; policy activation validates the Workspace Manifest policy snapshot, then for
enabled native readiness replaces every readiness rule in the binary's
append-only compatibility history with its current compile-time set.
One dedicated compile-time family catalog owns each pinned client version,
independently revised current readiness contract, and complete append-only
contract history. Aggregate construction and read-only cluster inspection share the
same deterministic content-revision calculation, so a binary catalog update
makes the previously active projection observably invalid until explicit
reconciliation.
The effective result enters the Tobari-owned system evaluator without rewriting
the snapshot. The fixed compatibility overlay is applied to the Workspace Manifest policy,
but its destination ceiling and method Deny decisions remain terminal. Disabled
readiness receives no overlay;
an omitted readiness value resolves to the explicit default without rewriting the
manifest. A read-only source is the same live direct bind
with Docker read-only authority: no writable alias is added, home and tmpfs
remain writable, and host or same-root read-write Workspace Manifest changes remain
observable.

The system evaluator resolves an exact method override or the Workspace Manifest policy
default before one combined exact-Deny tier containing trusted baseline Deny
and exact learned Deny with no internal ordering. Only after that tier may a
baseline grant, exact or reviewed single-segment-template learned Allow, or
Advanced Rego decide an unresolved ordinary request.
Terminal denial ends before candidate projection, external DNS, broker
resolution, and upstream I/O. Advanced modules may decide generic input only at
that unresolved tier; they cannot bypass the Workspace Manifest policy ceiling or exact
Deny, or redefine the scheme-aware exact learned identity.
The native-readiness capability uses a finite baseline coupled to Claude Code
2.1.220, Codex 0.147.0, GitHub CLI 2.96.0, TWG CLI
1.2.5, and pup 1.10.7 when the latter clients are supplied by a custom runtime. Named
compile-time native-authentication readiness bundles expand into exact HTTP and
declared semantic rules during aggregate generation; their names and executable
names never enter the persisted snapshot or evaluator. New snapshots omit
readiness rules, while legacy snapshot forms are removed before the current
binary set is added. Exact HTTP grants, GitHub CLI's exact GraphQL `query` /
`viewer` grant, TWG CLI's exact site-inventory, stable-manifest, token-revoke,
and GraphQL `query` / `me` grants, pup's exact US1 DCR-registration and token
POST grants, and one
direct-child evaluation template cover native model/account/bootstrap,
first-party capability discovery, bounded evaluation, and telemetry. Trusted
MCP endpoints are projected separately; Gateway buffers one bounded JSON-RPC
object and emits only method plus exact `tools/call` tool name to OPA. Bootstrap
and enumeration methods are baseline grants; action methods continue to exact
semantic review. Bodies, arguments, resource URIs, and responses never enter
policy or audit. Exact Deny precedes the baseline, which identifies Workspace Manifest
authority rather than a process.
The fixed agent-ready baseline is part of the default Workspace Manifest policy and is not
selectable as a named profile. Workspace Manifest creation supplies one complete method
default plus exact overrides; Method Allow enters the same Workspace Manifest-policy
baseline path as exact grants, after destination/method Deny and exact Deny
checks. GET receives no safe or read-only classification, and a deny-only or
GET-only posture is expressed by the Workspace Manifest method policy itself.

### HTTP authority scope, lifetime, owner, and precedence

The table below is the canonical technical inventory. Rows describe inputs to
two disjoint evaluator branches; sharing the Gateway and OPA transport does not
make authority transferable between them.

| Authority source | Scope | Lifetime | Owner | Precedence |
|---|---|---|---|---|
| Workspace Manifest destination and method Boundary | Workspace Manifest and ordinary external HTTP | Workspace Manifest lifetime | trusted host user at Workspace Manifest creation | terminal after principal/schema/Workspace Manifest validation and before every ordinary positive source |
| trusted baseline Deny | Workspace Manifest and exact ordinary effect | Workspace Manifest snapshot or binary compatibility revision | Tobari | member of one terminal exact-Deny tier with remembered exact Deny; neither member precedes the other |
| remembered exact Deny | Workspace Manifest, Workspace, and exact ordinary effect | until explicit reset | trusted host user | member of the same terminal exact-Deny tier; wins over every ordinary positive source |
| baseline grant | Workspace Manifest and exact or reviewed-template ordinary effect | Workspace Manifest snapshot revision | Tobari | Workspace Manifest-policy positive tier, inside the Boundary and below the exact-Deny tier |
| native readiness | enabled Workspace Manifest and current reviewed client-compatibility effects | installed binary's independently revised compatibility contract | Tobari contract, enabled by the host user at Workspace Manifest creation | Workspace Manifest-policy positive tier, inside the Boundary and below the exact-Deny tier |
| remembered Allow | Workspace Manifest, Workspace, and exact or explicitly reviewed one-segment-template ordinary effect | until explicit reset | trusted host user | unresolved Guided authority only; cannot exceed the Boundary or exact-Deny tier |
| Advanced Rego | one Advanced Workspace Manifest's generic ordinary request input | current owner-authored policy revision | advanced trusted host user | consulted only while ordinary authority remains unresolved; cannot exceed the Boundary or exact-Deny tier |
| ordinary default deny or exact-review result | unresolved valid ordinary effect | per request | Tobari evaluator | final ordinary result; missing or invalid identity instead receives the non-learnable fail-closed default |
| Attachment Deny | active principal-owned route, Workspace Manifest, Workspace, Attachment Epoch, and exact Host Loopback effect | owning attachment | trusted host user | first policy decision in the separate Host Loopback branch |
| Attachment Allow | same exact Host Loopback identity | owning attachment | trusted host user; route owned by the host attachment process | after Attachment Deny and before attachment review |
| exact attachment review | unresolved valid Host Loopback effect | owning attachment | Tobari evaluator and trusted host user | final Host Loopback result; review creates no durable learned rule |

Ordinary external HTTP follows one exact order:

```text
principal, schema, and Workspace Manifest validity
  -> terminal destination/method decision
  -> trusted baseline Deny + remembered exact Deny (one tier; no internal order)
  -> Workspace Manifest-policy positive authority (method Allow, baseline, native readiness)
  -> if unresolved: Guided remembered Allow or Advanced Rego
  -> fail closed or exact-review eligibility
```

Host Loopback follows its own complete order:

```text
active principal-owned route and Attachment Epoch
  -> Attachment Deny
  -> Attachment Allow
  -> exact attachment review
```

The Gateway resolves the active route and epoch before creating the OPA input,
and opens its authenticated one-shot bridge only after OPA Allow. The host
relay revalidates active route, Workspace Manifest/Workspace principal, epoch, target port,
and Attachment Allow before dialing `127.0.0.1`. Ordinary destination/method
ceilings, baseline and native rules, remembered Allow/exact Deny, and Advanced
Rego are inapplicable to this branch. Conversely, Attachment Deny, Allow, and
review are inapplicable to ordinary traffic. Route-owner teardown closes the
relay first, then removes route and grant projections, so stale policy bytes
cannot reach the physical host.

Project runtime infrastructure resolves only declared shell `inherit` entries
from the launching process at child-exec time and passes exact values to the
child environment. The default session appends `/bin/bash`; a typed direct
session instead appends the caller's exact argv after Docker's `--`, without
shell construction, expansion, or reparsing. In both cases the container keeps
the fixed `/usr/bin/sleep infinity` lifetime process as PID 1.
It always owns `PROMPT_COMMAND` so the chosen `PS1` survives image startup
behavior, but that hook is neither configurable nor inherited. Git inheritance
instead uses a purpose-built host adapter during stable-root reconciliation. It
runs at most two finite, bounded, one-attempt, fixed-key global-scope Git calls;
repository/worktree config cannot select another host file for that read. The
child receives only validated host config-directory paths and fixed controls,
never ambient `PATH`, loader, or shell-startup variables. A complete validated
pair is safely encoded after an `/etc/gitconfig` include in one size-bounded,
private per-Workspace system config. Its directory is mounted read-only and
selected by `GIT_CONFIG_SYSTEM`, leaving Workspace-global and repository-local
configuration at higher precedence. The report includes explicit `default`
state so presentation never infers policy from omitted manifest fields.

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
  opa/policy/tobari.rego
  opa/policy/tobari_test.rego
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
Workspace Manifest/project-principal registry and all-Manifest policy projection,
and invokes Docker through the runtime port. Standard Compose owns only Gateway,
OPA, shared networks, and CA volumes. Cluster startup ensures the verified
Gateway and agent-ready runtime images from embedded pinned recipes under
source-derived local tags, building each only when absent. The release resolver
is `embedded`; the development resolver selects its own source-hash local tags.
The runtime adapter
creates or reconciles each Workspace from its bound Workspace Manifest image and connects Gateway to
its dedicated network. After it has reconciled the Workspace guard, it records
the exact owned Workspace and Gateway endpoints in the schema-1 principal
registry. Before project container creation, the runtime issues
configured provider handles and renders only manifest-declared environment or
complete-file projections. A public-only CA volume is mounted read-only into
each Workspace, whose entrypoint builds an ephemeral CA bundle.

The same resolver owns a pure build-identity projection used by `version` and
cluster preflight. Both implementations fix APIs to canonical source and derive
image tags from embedded source bytes; release packaging injects no image
authority.
Neither implementation can consult CWD, project metadata, environment, or a
moving registry tag. Cluster preflight compares this projection before state
loading, asset materialization, journals, policy tests, or Docker calls.

Experimental Auth Broker follows the same canonical-source/runtime-input pattern as Gateway. Its
editable Python package, Dockerfile, tests, and bridge/protocol source live
under `authbroker/`; the Go binary embeds the checked Docker build inputs, not
the tests or contributor documentation, at
`internal/infra/runtimeassets/assets/authbroker/`.
`scripts/sync-authbroker-source.sh` refreshes the snapshot and
`scripts/check-authbroker-source.sh` rejects byte, membership, or Docker
`COPY` drift. The source and image
checks run the broker unit suite, prove that no provider CLI is installed, and
build the fixed non-root image. It is not published. The experimental
contributor resolver uses a source-hash local tag.

Both canonical sources declare component API V1. Source records only reviewed
parent inputs; generated owned-image outputs never enter `versions.env` or
release packaging.

Compose mounts owner-only host state
`auth/contexts` at `/var/lib/tobari-auth/contexts` and `auth/runtime` at
`/run/tobari-auth/runtime`; the control socket lives on private tmpfs. The
daemon listens on
`/run/tobari-auth/control/broker.sock` and
`/run/tobari-auth/runtime/broker.sock`. Control operations enter the container
through fixed `docker exec` argv and stdin. Gateway mounts only the runtime
directory read-only. The provider projection is generated atomically from the
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
the immutable standard or experimental active subset, and the
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
Workspace Manifest-associated data, owner/mode validation, bounded file reads, and atomic
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
lifecycle: Workspace Manifest validation, optional renewable-state validation, record
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
profile membership to the compiled Broker and Gateway registries. In
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
separate Workspace Manifest/tool configuration and is not part of login state. Repository
`.git/config` and Workspace-authored tool configuration remain
inside the project/Workspace boundary; ambient host provider configuration is
never read by these drivers. The
separate Workspace Manifest Git identity fallback contains only safely re-encoded
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
Contributor development resolves `builtin` to its local combined base.

The root resolver obtains the desired image from the stored Workspace Manifest identity's
strict manifest on each runtime reconciliation. A new Workspace Manifest selects
`builtin`. The resolved selector, rather than the source of the
default, is persisted on the Workspace only as the last successful
runtime-container image. Project metadata is not consulted for runtime
selection.

Project runtime path mapping is owned by the Docker adapter. The selected root
is mounted exactly once with the bound Workspace Manifest's immutable source access. If
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

Project metadata is not a runtime adapter. Tobari does not interpret
`.devcontainer` files, invoke the Dev Container CLI, or transfer container
creation to a second orchestrator. The supported customization adapter is the
installation-wide Runtime catalog: infrastructure snapshots one bounded
owner-only source tree, builds only the immutable snapshot, validates the
resulting image, and appends one successful semantic revision. A Workspace Manifest stores
an exact Runtime ID and revision; only the explicit Workspace Manifest Runtime mutation
changes that binding. Future import formats must attach to this Runtime boundary rather than
introduce project or Docker-tag authority.

Workspace Manifest Workspace bootstrap is a separate create-only projection boundary.
Domain owns the closed normalized snapshot and semantic revision; application
owns configure/refresh/remove intent and result correlation; infrastructure
alone reads fixed host AWS shared-config and kubeconfig paths, parses one closed
AWS profile and optional dependent EKS target, atomically replaces the Workspace Manifest
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
compare it with the selected Workspace Manifest bootstrap recipe without inspecting the
Workspace file.

The activation classes are therefore explicit:

```text
WorkspaceManifestID + desired semantic digest
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

The state-migration slice is deliberately separate from every current reader:
`internal/app/migrationcmd` owns the fixed installation-state task,
`internal/domain/tobari` owns its closed source identity and result, and
`internal/infra/dockerruntime/migration.go` is the only predecessor decoder.
It plans the complete predecessor collection under the lifecycle lock, requires
the cluster stopped and zero live attachments, prepares any exact managed
Runtime revision, writes a content-addressed private backup, and commits final
schema-2 Manifest/Workspace state plus the exact default selector. Current
readers never call the migration decoder. Retained Manifest receipt collision
accepts only the same WorkspaceManifestID and canonical body at the same
generation+digest path; other or partial artifacts fail closed.

The same owner-only journal enumerates and quarantines the complete
filesystem-side predecessor research-auth authority set. Central ciphertext
and lookup state moves first, so every later crash point leaves the predecessor
reader unable to resolve old handles even if inert config or Workspace
artifacts have not moved yet. Resume completes the exact set; rollback restores
it byte-for-byte and refuses fresh canonical state. macOS Keychain material is
untouched recovery material and is never queried by migration. Linux
filesystem root-key material moves with the set. Canonical readers do not know
the quarantine path, and public output is secret- and path-free.

## Lifecycle model

The MVP owns one shared cluster `tool_local` target with stable ID
`cluster-default` and many CWD-owned Workspace records. The root index
stores a canonical root, stable Workspace Manifest ID, and stable internal ID at
`$XDG_STATE_HOME/tobari/roots/<hash>.json`; each instance owns
`instances/<id>/state.json` and `instances/<id>/home`. The instance record
contains the stable ID, canonical root, permanent Workspace Manifest binding, last reconciled image, profile, optional create-time bootstrap revision, and
diagnostic container or network identifiers. Logical state, not Docker
inspection, defines whether a Workspace exists. Docker labels include:

```text
io.tobari.owner=default
io.tobari.component=tobari|gateway|opa
io.tobari.id=<stable ID when applicable>
io.tobari.role=work|network
io.tobari.version=<asset revision>
```

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
  -> tobari delete
Workspace absent
```

The child Bash `exit`, or any exit from `tobari -- COMMAND [ARG...]`, ends only
the foreground exec process and returns its exact status to the host; it does
not fall through to Bash or stop or delete the work container, logical
instance, root index, or per-Workspace home. There is no persisted stopped or
paused state. Detached `delete` removes the logical
Workspace; an attached exec makes ordinary deletion fail, and `delete --force`
is the explicit host-side override.

Explicit standard `cluster up` validates configuration, obtains and preflights the
Gateway image, locally ensures every required runtime image, builds and tests
the complete all-Manifest policy projection, reconciles exactly one OPA and one
Gateway, and
reconnects Gateway and OPA to their required shared networks plus Gateway to
every existing registered project network. It completes
only after OPA serves the exact aggregate revision and a defined decision
document.
Image preflight fails before the policy test, cluster journal, shared network,
or service-container mutation. Local Tobari-managed image development uses
`task build` and the source-hash development resolver instead of a public
cluster option.
On first use, root validates the canonical project root and observes a known
empty Workspace Manifest collection before rendering one typed recommended draft. Start
revalidates empty/default-absent under the Workspace Manifest lifecycle lock and invokes
the same Workspace Manifest-create application invoker; Customize seeds the ordinary
complete Workspace Manifest wizard. Neither branch duplicates Workspace Manifest persistence or
imports host configuration implicitly. Confirmed creation is durable; root then
invokes the same typed cluster reconciliation used by explicit `cluster up`.
Before creation, the application selects the closed generic Workspace-start
readiness profile from Doctor's typed observation inventory. Infrastructure
executes only fixed bounded Docker CLI/Engine-version/Workspace Manifest/Compose reads.
The application enforces Engine major version 24 and returns a typed context
receipt so the composed `cluster up` does not repeat the profile. Direct
`cluster up` performs it before the mutation invoker. No layer detects, opens,
starts, stops, or otherwise manages the Docker provider.
Cluster failure leaves the Workspace Manifest available for another root invocation. Once
ready, root proceeds directly with the exact ready Runtime revision reviewed by
Workspace Manifest creation; it has no post-create Runtime chooser. Customization is
prepared independently through Runtime create/build before selection. Existing
Workspace Manifests stay pinned until the explicit Workspace Manifest Runtime mutation replaces their
binding.

Outside that fresh-Workspace Manifest composition, root observes whether the configured
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
the lifecycle lock before the selected logical record is created or reused. It
then resolves the selected record's root-scoped Workspace Manifest Git fallback before
Docker calls and resolves the bound Workspace Manifest image. Standard reconciliation
uses an explicit empty authentication projection and neither inspects nor
creates experimental authentication state. Experimental reconciliation
requires the Broker to be ready and reconciles the Workspace Manifest's configured
project-bound handle projection. Both profiles then ensure one exact project
network and work container, connect Gateway with the `gateway` alias, reconcile
and verify both network-namespace guards, bind the exact owned Workspace source
endpoint plus Gateway endpoint to the host-issued Workspace Manifest/project principal,
wait for project readiness, and enter the container. The
logical creation and deletion boundaries use durable journals so an interruption
between the home, instance, index, runtime, and deletion steps is recoverable
without treating a partial file set as a second project. Runtime convergence
stores a hash of the bound Workspace Manifest's desired image identity, mounts, security,
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
Workspace and Workspace Manifest IDs are not trusted when echoed by a caller; Gateway
derives both from the kernel-observed Workspace source endpoint and the exact
host registry binding. Exact allow, deny, and reset
actions provide the deterministic portable activation path: each locks the
projection, tests the target Workspace Manifest's private source copy and the complete
all-Manifest candidate, verifies the exact OPA and bundle-volume ownership
labels, builds a revision-named archive through pinned OPA, atomically renames
it through a fixed networkless pinned publisher, and transfers the tested
owner-only host projection through a bounded owner-only archive and container
stdin into the bundle volume rather than giving container root a host bind,
waits for the running OPA to report the exact expected revision, and rolls back
on failure. Reducing or mixed authority first activates a deny-all transition
revision.
The target-copy test yields a one-shot in-memory receipt bound to the exact
preflight bytes, candidate source digest, Workspace Manifest policy directory, and source
transaction. Aggregate construction may consume it only for that same
candidate; a missing or mismatched receipt reruns the per-Workspace Manifest OPA test. The
complete aggregate candidate test, existing aggregate revalidation, activation
test, and authority-reduction fence are never skipped.
`delete` observes active Docker exec IDs before ordinary removal, then verifies
owner, ID, and role labels before removing the selected container and network;
an attached exec rejects ordinary deletion and `--force` skips only that guard.
It then removes only its XDG home and records. Container
or network loss is reconciled by the root operation; it never deletes logical
state. Cluster status and reconcile derive project counts and network joins
from the indexed CWD-owned records.
Status and list expose an `incomplete` runtime diagnostic when an indexed root
has lost its instance record; root entry refuses to rebuild that record and
delete remains the exact cleanup path.
Cluster removal is rejected until no instance record remains. Both `cluster
down` forms remove shared runtime resources while preserving encrypted Workspace Manifest
vaults and the installation root key. `--purge` additionally removes only the
shared CA and active policy-bundle volumes; cluster teardown is not credential
logout or revocation.

The root-index collection enforces one logical Workspace per `(canonical root,
Workspace Manifest ID)`. Its hash includes both values, and the project lock performs the
exact pair check again immediately before explicit creation. A repeated or
concurrent creation for one pair has one winner; a different Workspace Manifest may own a
separate stable Tobari, home, container, and network at the same root.
Parent/child roots may also overlap and run concurrently. Their host-file
effects are intentionally shared; the architecture does not add overlays,
checkout copies, root locks, or filesystem integrity isolation.

## Command catalog

`cli.Catalog` is the only registry for public paths, roles, effects, fixed
targets, inputs, outputs, failures, routing, and human/agent help. The root
operation is represented as a catalog-owned fixed current-directory target even
though it has no argv path words. Handlers receive parsed inputs and call one
application service. `tobari` and `delete` declare complete fixed-target
mutation impacts; `tobari` keeps its target fixed to the canonical CWD even
when its selected Workspace root is an ancestor. `status` resolves the same CWD
target. For those three lifecycle commands, the dispatcher normalizes the
prefix or command-local Workspace Manifest spelling into the command's one catalog input;
the typed parser rejects duplicates and explicit empty values. Application
resolves that selector to one validated manifest before CWD or Workspace
selection, and infrastructure receives the bound manifest rather than
rediscovering its display name. Status and force-delete preview retain that
stable Workspace Manifest identity through presentation and exact follow-up argv. `list`
reports IDs as diagnostic fields but no public lifecycle action consumes them.
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
port reads validated Workspace Manifest lists, Runtime lists, and managed Runtime history
from the existing infrastructure adapter. It does not invoke another command,
parse rendered output, reach Docker or the network, inspect arbitrary paths, or
mutate startup state. Internal catalog entries remain excluded from public
copies, lookup, routing, reference workflows, help, and completion.

In the experimental build, the four auth commands share one catalog-declared `authentication.broker`
capability. Login, import, and logout are fixed-target writes to the one
installation credential catalog; status is a Workspace Manifest-scoped read. The
application resolves the explicit or default Workspace Manifest before the infrastructure
port, validates mutation intent and impact before acquisition/vault I/O, and
accepts import material only through the declared stdin input. Terminal import
stdin is rejected before reading. Non-terminal bytes are read after public
argument/intent/mutation validation; infrastructure then validates the selected
existing Workspace Manifest, installed provider/acquisition mode, and broker readiness
before broker send. Provider IDs are human selectors validated against the
installed projection; they are not opaque action references or credential
authority. The compiled union includes GitHub, Datadog, OpenAI, Anthropic, and
AWS; AWS alone adds its method axis. Interactive omission opens the bounded
selector. Import, status, and logout remain
available for strict owner static manifests and Chatwork.

Standard `doctor` composes bounded read-only environment, Docker, policy, and
project-binding diagnostics. Experimental doctor adds provider, root-key/vault,
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
emphasis for catalog-rendered output. Attached Workspace child stdout has a
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

Human text is built in three independent stages. Task-owned typed results first
select a `humanOutput` document containing headings, rows, sections, empty-state
scope/bounds, and exact recovery. The style projection may then add only the
six semantic ANSI tokens; it cannot select, remove, reorder, or rename document
content. The terminal interaction state machine independently owns alternate-
screen entry, home-and-clear repaint, redraw eligibility, and main-screen/
cursor restoration. It never locates a prior frame by logical line count
because one line may occupy multiple physical rows after terminal wrapping.
Its reader absorbs idle VTIME polls without returning a render event, while
input, selection changes, completion, and cancellation remain observable state transitions. Catalog
selection supplies bare-namespace normalization and deterministic typo
suggestions, so routing and recovery do not create a second command registry.

## Gateway request flow

Standard Gateway establishes the source-bound Workspace Manifest/project principal,
normalizes the transparent authority, redacts authentication and cookies from
OPA/audit, asks OPA about the ordinary exact HTTP effect, and only after allow
forwards the original Workspace-owned credential in one upstream attempt. It
has no provider projection or Broker adapter. The Claude/Codex native-login
regression exercises this sequence, including Claude's authenticated profile
metadata request after token exchange.

### Experimental Broker augmentation

The experimental Gateway layer extends the same sequence as follows:

```text
client request headers
  -> establish the host-issued Workspace Manifest/project principal from the source
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
  -> classify only trusted Workspace Manifest-declared exact GraphQL endpoints
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
  -> route by trusted Workspace Manifest ID inside the Tobari-owned OPA package
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
Gateway recognizes distinctive standard catalog, tag, manifest, blob,
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
denial-shaped records, and returns every valid typed Workspace Manifest and project principal, host, port,
method, path, optional GraphQL operation/root or AWS wire-operation coordinate, reason, status,
exact-rule learnability, request identity, timestamp, the
trusted host policy directory, the unparsed-record count, and the exact review command. OPA computes
learnability only when version, cluster, Workspace Manifest, scheme, fixed port,
project-principal, and Workspace Manifest policy ceiling already pass, so an exact
Workspace Manifest/project/scheme/host/port/method/path rule, plus the GraphQL or AWS coordinate when
present, can close the request. `review permissions` and
`policy candidates` deterministically fold only that eligible retained evidence
by the structured Workspace Manifest/project/scheme/host/port/method/path and optional GraphQL or AWS
effect key. They emit one opaque
exact-rule reference, the latest evidence, and an observation count for each
pending effect; references remain stable across repeated denials. This pure
read projection also converges concurrent identical audit records without a
second persisted inbox or write race. They remove effects already covered by
the CLI-owned learned allow or deny data and trusted baseline deny rules.
Baseline denies remain audit-only. `review permissions` is the routine human text
workflow: its raw list stages exact decisions and its detail view alone stages
template authority, always over unchanged opaque candidate IDs for one Workspace Manifest,
then applies the complete typed set once. Apply or discard precedes
switching Workspace Manifest, keeping source promotion to one atomic domain generation;
redirected review is read-only. The optional raw-terminal `--watch` modifier
uses the same complete bounded application query on a one-second schedule with
bounded exponential backoff; it is not streaming delivery or a second policy
transport. The selector retains one alternate-screen presentation frame across
CLI-owned reads and quick-staging continuations, while the application use case
still owns every bounded read. It compares the complete typed report, selected
ID, staged-ID map, and notice before rendering, so an unchanged successful timer
refresh writes no new frame. Manual or automatic refresh intersects the staged ordered set with
the fresh queue by opaque ID, retains selection by ID, and preserves the last
valid snapshot on read failure. Final review repeats every exact scope,
effect, decision, and candidate ID before one explicit confirmation. The
infrastructure returns the revision only after the running OPA confirms it;
the typed application receipt preserves that revision and ordered decisions.
Watch tracks a process-memory union of successfully observed typed review-item
IDs. Only a later successful snapshot containing an unseen ID calls the narrow
terminal notifier once; the notifier writes fixed trusted ASCII OSC 9 or BEL,
or nothing for `off`, and never receives denial evidence. `auto` resolves from
exact iTerm2 identity or the presence of both protected cmux workspace and
surface identities inside infrastructure, and conservatively falls back to
BEL. Notification write failure does not change snapshot, staging,
Apply, retry, terminal passthrough, or policy state.
`policy rules` is the exhaustive current inventory of CLI-owned learned Allows
and exact Denies. `policy reset --id` removes exactly one such decision through
the same preflight, atomic-write, and OPA activation boundary, leaving the
matching effect at default deny so the retained denial can enter `review permissions`
again. It never edits baseline policy, grants permission, or retries a request.
Raw `cluster logs` remains the component-debugging interface.

`policy allow` resolves one exact candidate reference against retained
validated audit state without decoding it. Infrastructure reads the bounded,
owner-only `policy/domains/<canonical-host>/{allow,deny}.json` tree, preserves
every unchanged file byte-for-byte, appends one deterministic exact learned
rule to its host's `allow.json`, tests a private complete policy copy, swaps one
complete domain-source generation, and calls the existing OPA activation
boundary. Deny mutations use the matching `deny.json`; an unknown exact host is
created with both files in the staged generation.

Each authoritative domain source is strict schema 1. The directory name and
every embedded authority, endpoint, credential binding, and rule host must be
the same canonical lower-case host. Methods belong to the authority records
composed from that domain and cannot authorize another host. Wildcards and IP
literals are not source syntax. Guided Workspace Manifests contribute only the domain
tree; aggregate generation loads the current shared Rego evaluator and tests
from Tobari's embedded runtime assets. Advanced Workspace Manifests add exactly
`tobari.rego` and `tobari_test.rego`. Gateway runtime input uses exact schema 1
and rejects any other shape. The generated aggregate projection may contain a
single internal `data.json`; it is immutable execution input, never a Workspace Manifest
source. The aggregate projection is schema 1 and stores the composed sources below
`tobari_contexts[context_id]`; the Tobari-owned router is the only
`tobari.http` decision entrypoint. Each entry in `boundary.authorities` owns its
scheme, host, ports, and host-local methods;
`boundary.graphql_endpoints` declares exact protocol-classification points, and `rules` keeps
baseline denies, learned allows, and learned denies in separate collections.
Gateway and OPA share this structure; the CLI only owns the mutation of the
two learned collections and never rewrites the host-authored boundary.

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

Guided Workspace Manifests use one current Tobari-owned evaluator, projected once, with
Workspace Manifest-specific authorities, methods, ports, GraphQL endpoints, exact and
semantic baseline decisions, learned
decisions, and credential metadata supplied as data. Advanced source retains
the editable `package tobari.http` source contract, but projection rewrites it
to `tobari.contexts.c<uuid>.http`. Validation rejects source that claims the
cluster router, `tobari.system`, another Workspace Manifest package, or
`data.tobari_contexts`. This is namespace and routing enforcement within one
OPA process, not a claim of Rego process-level confidentiality.

The cluster router sends every input carrying a GraphQL, MCP, or AWS coordinate through the
current Tobari-owned system evaluator, even for an Advanced Workspace Manifest; the
Workspace Manifest's data still supplies its endpoints and rules. Only ordinary HTTP input
routes to editable Advanced Rego. This prevents older or custom policy source
from ignoring the new coordinate and accidentally authorizing it through one
coarse HTTP rule.

`policy deny` resolves the same exact candidate reference and appends one
Workspace Manifest/project-bound exact deny rule through the same aggregate preflight, atomic-write, and OPA
activation boundary. Exact denies are terminal and win over learned allows for
the same Workspace Manifest/project/scheme/host/port/method/path and optional protocol coordinate.

## Docker abstraction

Application code owns narrow ports such as `ResolveOrCreate`, `EnsureRuntime`,
`EnterRuntime`, `Inspect`, and `Delete`. The MVP infrastructure implementation
invokes the Docker CLI with fixed command structures and caller context. This
keeps Docker Engine API or Podman replacement possible without promising either
today. Arbitrary shell strings are never constructed; user commands are passed
as argv after Docker's `--`.

## Cancellation and errors

The command root installs signal-aware cancellation and propagates one context.
Pre-execution cancellation makes zero Docker calls. A child interactive session
exit status is preserved, and the CLI emits the session-closed/resume/delete
guidance on host stderr after the child returns. Child stdout remains owned by
the interactive process. The optional root delimiter and repeatable positional
input are catalog-owned; a missing command is rejected before lifecycle side
effects. Lifecycle operations return structured state after
confirmed completion; unclassified post-mutation errors are non-retryable and
direct the user to `status` for reconciliation.
The domain fault vocabulary separates causal identity (`kind` plus `code`) from
the closed command phase and strongest proved change state. Application and
infrastructure may strengthen state only from typed boundary evidence. Catalog
owns the published classification and rejects runtime disagreement; mutation
states `partial`, `confirmed`, and `unknown` recover first through a validated
read. CLI presentation projects these facts but never infers them.
Auth mutations use the same structured-outcome rule. A failed or cancelled
GitHub host driver leaves the previous Workspace Manifest credential unchanged; a
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
Workspace Manifest, logical project, and credential-revision correlation, then derive
activation state, exact re-entry actions, and mutation-change semantics.
Presentation never derives freshness from provider labels, process existence,
or row order.

## Architecture enforcement

- Go architecture lint preserves layer direction and a thin `cmd/tobari`.
- Each claim belongs to the lowest layer that can prove it without simulating
  the subject of the claim. Domain owns pure semantics, application owns use-
  case ordering, infrastructure owns exact adapter requests and filesystem
  contracts, and CLI owns parsing and presentation. A higher layer keeps only
  one representative composition canary; it does not repeat the lower layer's
  input or output matrix.
- Domain and application tests prove path, state, effect, and orchestration
  invariants without Docker.
- Infrastructure tests use a recording command runner.
- Gateway and Rego tests cover policy boundaries.
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
