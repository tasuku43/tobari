# Project Theses

These theses govern Tobari. When implementation pressure conflicts with them,
revise the thesis and its enforcement explicitly instead of adding a bypass.

## North Star

**Tobari gives a coding agent an execution boundary in advance, then lets it
move freely inside that boundary. By making isolation startup, understanding a
denied operation, and granting the minimum required permission extremely easy,
it makes the safe execution path a more natural choice than running the agent
directly on the host.**

Every HTTP and HTTPS effect that crosses that boundary is authorized as a
normalized request by one shared OPA-backed Gateway. A Context may start with a
finite reviewed exact baseline composed from its immutable preset snapshot and
its independently selected trusted-binary native-client readiness capability;
every effect outside that composition is denied by default. The preset's
offline, destination, and method guardrails remain terminal over readiness.
Trusted policy may declare an exact GraphQL endpoint whose generic L7 identity extends
the HTTP coordinates with one operation type and root field per effect.

The primary users are developers who run Claude Code, Codex, shells, tests, and
other arbitrary programs against project roots. Success has two inseparable
parts: each Tobari can host concurrent processes, cannot reach the Internet or
another Tobari directly, never receives a real host-managed credential, and is
selected from the canonical current directory rather than a user-managed name,
root flag, or container identifier; and the user can reach that boundary
without becoming a Docker or policy operator. In the standard profile, users
authenticate the pinned agent CLI inside the Workspace and its tool-owned state
persists only in that Workspace home; host credentials are never inherited.
An attached standard session may bridge only the closed reviewed native browser
login union for pinned Claude Code, Codex, GitHub CLI, AWS CLI,
custom-runtime TWG, and custom-runtime pup.
The native CLI invokes one attachment-scoped opener through `BROWSER`,
`GH_BROWSER`, or `xdg-open`; after bounded semantic authorization-URL validation,
the host opens that URL. Terminal presentation is never authority. Only
the reviewed Codex, GitHub, AWS SSO, and pup callback variants relay one opaque localhost
callback on the validated URL-selected port to the selected Workspace's
client-owned loopback listener; Claude's remote callback and GitHub's device
target and TWG's strict device-verification target create no listener. Opener framing is not URL
authority. The bridge is session-scoped and grants no general host browser,
port-forwarding, credential, or network authority. Output observation
must preserve terminal identity as well as bytes: Docker retains a real PTY,
raw-input ownership, and resize propagation while Tobari reads only a relayed
copy. A manual-code target is staged only in bounded session memory and opens
after the client-owned confirmation transition, giving the user time to copy
the visible code; URL/callback flows with no Workspace-to-browser code transfer
open immediately. Tobari does not intercept child input or own clipboard
shortcuts.
One installation-local standard cluster shares one Gateway, one OPA, an atomic
all-Context policy projection, and CA state without sharing Workspace homes or
runtime networks. The repository-only experimental profile may additionally
compile the Auth Broker research boundary. Host-issued Context/project
principals bind mutable learned permissions to the exact current-directory
network that originated a request; they never select a credential on their own.

The first testable slice is one local mock upstream reached through the
Gateway: an allowed request succeeds, a denied request does not reach upstream,
direct egress fails, and an OPA outage fails closed.

The core product loop is progressive policy learning: work freely in Tobari,
observe a denied boundary effect as secret-free evidence, receive a fixed
host-side review cue, keep the current Workspace and agent session running,
review the pending permission from a separate trusted-host terminal,
approve the minimum rule, and retry in that same session. The normal path does
not require writing OPA or Rego by hand:
interactive `policy review` presents a Permission Inbox, keeps one distinct
HTTP path exact, proposes a single-segment `{id}` template after a second
compatible distinct path, stages an explicit template-Allow, observed-exact
Allow, or pending-exact Deny from detail, and applies the reviewed set once;
its non-interactive and machine-readable path remains read-only. Staging grants
no authority. Final Apply revalidates every unchanged opaque candidate, tests
one complete all-Context candidate, and hot-activates one revision without
restarting active Tobari or the shared OPA. Single-reference `policy allow` and
`policy deny` remain available to machines and recovery workflows.
Successful Apply reports the authoritative active revision and the ordered
stored-rule receipts; it never asks the caller to re-enter the Workspace.
A separate `policy rules` view makes the complete current learned Allow and
exact Deny decisions visible, and its TTY flow can explicitly reset one
decision to default deny. Reset never grants or retries; it makes the retained
effect eligible for `policy review` again.
A foreground experimental `tobari serve` may present the same typed cluster, Workspace,
Permission Inbox, and learned-rule tasks through a host-browser Operator
Console. It is a trusted-host presentation alternative, not a second policy
engine or remote control plane: the process owns one random IPv4-loopback
session, browser staging grants no authority, and Apply delegates the complete
reviewed set to the same fixed-target mutation and authoritative receipt used by
the terminal flow.
A useful denial shortens that loop without silently
expanding authority. The loop is part of the product's adoption boundary: if
the safe path is harder than running the agent on the host, users will bypass
the isolation that the boundary is meant to provide.

## Thesis 0: Bounded autonomy must be easier than host execution

Tobari's security value depends on adoption. The product is not primarily a
container lifecycle wrapper; it is a way for a user to define an agent's
activity boundary before work begins, then let the agent move quickly inside a
chosen root, home, and network policy without per-command monitoring or
unrestricted host authority. Starting the isolated environment, understanding
a denied operation, and adjusting the boundary by granting the minimum
required permission must require less operational knowledge than running the
same agent directly on the host. The workflow is opt-in and reversible: not
using `tobari` leaves host execution unchanged, while `delete` and
`cluster down` remove Tobari-owned state through exact ownership checks.

### Consequences

- The human journey is CWD-first. Docker resource names, stable IDs, network
  topology, and OPA syntax are implementation or advanced-policy details, not
  routine setup inputs.
- The ordinary entry path should have one obvious next action, reuse an
  existing Workspace without reconfiguration, and make the first successful
  agent session valuable before policy customization is required. The user
  sets the boundary once; the agent is not supervised action by action inside
  it.
- The default Context carries the pinned supported coding clients and a finite
  reviewed baseline for their native capability effects. Requiring the user
  to reverse-engineer client bootstrap, model, account-status, inference, or
  telemetry or first-party capability discovery before useful work is an
  adoption failure. MCP action, downloads, file transfer, acquisition,
  self-update, and project-originated destinations remain outside that
  baseline.
- A denied network effect should provide enough secret-free context and a
  concrete host-side review action to reach one exact, tested permission. The
  safe action may remain opaque-reference-bound internally, but the human
  presentation must not make reference plumbing or OPA syntax the user's
  primary task.
- Host-authored Rego, raw logs, Docker diagnostics, and provider-specific
  details remain available as advanced paths; they do not define the routine
  agent workflow.
- The human first-use route starts with the CWD-first `tobari` entry. On an
  interactive terminal with no persisted Context, that one route owns the
  ordinary Context review, composes the separately cataloged `cluster up`
  action after confirmed creation, and offers the already-selected standard
  runtime or optional Dockerfile preparation before the first Workspace.
  Read-only `doctor`, status/list inspection, and
  opaque-ID policy actions remain recovery or machine paths rather than steps
  in the normal journey. On a TTY, `policy review` is the complete human
  review-to-decision flow, while `policy rules` is the complete human
  inventory-and-reset flow; neither requires a second JSON review.
- Experimental `tobari serve` is the dense host-browser alternative for the same inspection
  and review outcome. It starts no daemon, accepts no remote bind address, and
  cannot bypass typed review staging, fresh validation, or final confirmation.
  It is compiled only by `task build:dev`; the standard and release command
  catalogs omit it while the interface is evaluated.
- Explicit `context create`, `cluster up`, and runtime commands remain
  independently supported compatibility and automation surfaces. They do not
  require a human to remember their order for the ordinary first-use route.

### Mechanical enforcement

- Agent-readiness validation records the first-use path and the
  denial-to-retry path, including discovery rounds and undeclared external
  processing, rather than checking only that each command is individually
  correct.
- Human CLI help and README lead with the bounded-autonomy outcome and an
  exact next action; scoped agent help retains the complete catalog contract.
- Integration tests prove that isolation is opt-in, reusable, recoverable, and
  customizable through the deny-review-allow-retry loop without direct egress;
  they also prove that resetting a learned decision restores default deny and
  makes the same retained effect reviewable again.

## Thesis 1: Authorize effects at the isolation boundary

Tobari authorizes the HTTP request that actually attempts to leave a Tobari.
It does not infer intent from a command name, process name, shell text, or agent
brand.

### Consequences

- Any process that uses an ordinary HTTP or HTTPS socket receives enforcement
  through the guarded transparent path; command names and ambient or projected
  proxy-environment settings do not select policy behavior.
- Gateway sends generic HTTP attributes and, only for a trusted exact GraphQL
  endpoint, normalized GraphQL operation type and root-field attributes to OPA
  rather than service-specific operations.
- Ordinary request bodies are payload, not permission identity. Gateway
  authorizes the project, authority, method, and path from request headers
  before forwarding body bytes; body presence and content do not split
  approval candidates. A declared GraphQL endpoint is the narrow exception:
  Gateway buffers one strictly bounded body before policy and derives only its
  selected operation type and canonical root fields as additional identity.
- Allowed ordinary request and response bodies stream through Gateway. A
  declared GraphQL request is forwarded byte-for-byte after allow and its
  response still streams. Raw bodies, body hashes, GraphQL source, operation
  names, variables, arguments, aliases, fragment names, directives, nested
  selections, and literal values never enter OPA input, retained policy
  evidence, learned rules, or audit records.
- Provider-specific HTTP semantics are not part of policy. The first supported
  Auth Broker provider uses a declarative exact host/header contract for
  GitHub.com, but Gateway and OPA still authorize only the normalized HTTP
  effect rather than a GitHub operation.
- HTTP methods are evidence supplied to policy, not a CLI effect classifier.

### Mechanical enforcement

- Gateway unit tests fix the OPA input schema, secret-header redaction,
  trusted GraphQL endpoint classification, bounded parser behavior, and
  authorization-before-forward ordering.
- Rego tests exercise host, port, method, path, scheme, project-principal,
  ordinary body-independent decisions, and exact GraphQL operation/root-field
  boundaries.
- Docker integration tests use curl and Python rather than a named coding agent.

## Thesis 2: Network topology is an enforcement mechanism

Each Tobari has exactly one internal Docker network path: its dedicated Gateway
interface. Ordinary HTTP/HTTPS sockets follow a guarded default route and
terminate at the transparent listener. Gateway alone joins every Tobari network plus the shared
control and egress networks; OPA joins only control. Kernel forwarding remains
disabled, so neither path is direct egress.

### Consequences

- Proxy bypass has no route to an external upstream.
- A non-recursive synthetic DNS listener supplies only the bounded address
  needed to reach transparent HTTP/TLS inspection. It performs no external
  lookup before policy allow.
- Tobari cannot reach OPA, Gateway management interfaces, or another Tobari.
- Gateway or OPA failure denies outbound traffic.
- Each work container receives fixed CPU, memory (including total memory plus
  swap), process-count, and
  container-log bounds so one untrusted workload cannot consume those shared
  resources without limit. Disk quota and network bandwidth remain separate
  unclaimed controls.
- Host networking, privileged mode, Docker socket mounts, and capabilities in
  resident Workspace or Gateway processes are forbidden. A fixed short-lived
  helper may receive only `CAP_NET_ADMIN` while sharing one verified container
  network namespace; it receives no mounts, secrets, executable selector, or
  Docker socket and exits before user entry.
- Reviewed native loopback login is the separate no-capability exception: while
  one interactive host session is attached, a strict pinned-client observer may
  bind only a validated Codex, GitHub CLI, AWS CLI, or custom-runtime pup authorization
  URL's reviewed non-privileged
  host-loopback port, open only that provider's reviewed authorization shape,
  and relay one opaque callback to the same port in the exact owned Workspace
  loopback. It closes on completion or session exit and is not a generic ingress
  path.
- Reviewed Host Loopback access is a distinct Workspace-to-host exception.
  Every interactive attachment projects the constant
  `http://host.tobari.test:{port}` capability for physical-host IPv4 loopback
  HTTP on non-privileged ports; no entry declaration is required and the
  projection grants no authority. An exact HTTP effect becomes reachable only
  through Gateway and OPA after trusted-host review creates an Attachment Grant. The route and
  grant belong to one unguessable host-derived Attachment Epoch, apply to every
  process in that Workspace, and cannot become a durable learned rule,
  template, preset, arbitrary time lease, raw TCP route, or Docker authority.
  Closing the owning attachment closes the physical relay before removing its
  registry and policy projection. Inactive, privileged-port, or mismatched
  requests are terminally denied without host-loopback I/O. A concurrent
  attachment may borrow the current epoch but cannot extend it or inherit ownership.

### Mechanical enforcement

- The runtime adapter constructs one labeled internal network per Tobari, a
  shared internal control network, and a separate egress network.
- Runtime reconciliation installs and verifies exact Tobari-owned Workspace
  and Gateway namespace guards before user entry. Gateway forwarding sysctls
  remain disabled and its forward chain drops unconditionally.
- Integration tests prove transparent interception without proxy environment,
  zero pre-policy DNS/upstream calls, source-principal isolation, direct egress,
  direct OPA access, and traffic during Gateway or OPA failure do not succeed.
- Runtime specification tests reject privileged mode, host networking, Docker
  socket mounts, broad host-home mounts, and missing fixed resource bounds.
- Runtime integration inspects CPU, memory, PID, and log limits after creation
  and after recovery.
- Image-selection tests require a locally available runtime-API-compatible
  image before any per-Tobari resource is created; image configuration cannot
  replace the CLI-owned isolation arguments. Runtime API compatibility includes
  the bootstrap needed to execute Tobari's fixed Workspace lifetime command.

## Thesis 3: Native Workspace authentication is standard; brokering is experimental

Tobari does not inherit host authentication material. The standard profile
declares no provider bindings, provider projection, credential handles, vault,
root key, companion, or Auth Broker service. A pinned agent CLI performs its
native login inside the Workspace and owns the resulting state in that
Workspace's persistent home. Gateway removes client authentication and cookies
from OPA input and audit, asks policy about the ordinary HTTP effect, and
forwards the original values only after allow.
For pinned Codex, GitHub CLI, and AWS CLI, native local parity also includes ADR 0046's
attached-session browser/callback bridge, ADR 0048's GitHub host-open-only
device flow, and ADR 0053's provider-confirmed manual-code handoff. Each client
still owns OAuth state, exchange, and credential
persistence, while callback-bearing clients own callback parsing and Codex also
owns PKCE. Tobari validates one provider-specific browser target, transports an
opaque callback only when that target declares the reviewed loopback shape, and
never logs or persists the target, callback, or device code. Manual-code targets
may exist only in bounded session memory until confirmation, ambiguity, or
session end; Tobari continues to observe output only.

The experimental development profile retains the closed Broker research path.
That route stores one Context-owned credential or
renewable provider session in an authenticated encrypted vault. Each eligible
Workspace receives only a distinct random handle or handle-only client shim
bound to its stable Context, project, provider, credential revision, and exact
HTTP binding. Gateway resolves, refreshes, or signs through one closed reviewed
provider plan only after OPA allows the ordinary HTTP effect. Workspace-owned
authentication remains the only standard path; Broker-required declarations
exist only when the experimental capability profile is compiled.

### Consequences

- Host home, host CLI configuration files, keychains, SSH agents, and
  credential environment variables are never mounted or copied into Tobari.
  A thesis-declared narrow projection may instead read and re-encode only its
  fixed non-secret scalar allowlist. It never transfers the source file, path,
  include directive, executable setting, credential, or an undeclared key.
- Tool-owned credential state is available to every process in the same
  Tobari by design, survives runtime-container recreation, and is removed by
  the explicit Tobari delete operation.
- Client authentication and cookie values are redacted from OPA input, Gateway
  audit, denial projections, and CLI output. In the experimental profile, a declared binding rejects a real
  Workspace credential before OPA as non-learnable `broker_auth_required`; a
  valid handle selects the broker route and one exact post-allow action. Only an
  undeclared binding may select Workspace-owned compatibility passthrough after
  policy allow. Proxy and Tobari control headers are not forwarded upstream.
- Only in the experimental profile, one shared Auth Broker joins the internal control network and a provider
  egress path limited to compiled reviewed refresh plans, never a Workspace
  network. Workspaces and
  OPA cannot address its runtime socket; only Gateway mounts that socket.
  Gateway alone owns ordinary upstream egress after policy allow. Provider
  acquisition runs through fixed trusted-host drivers except that Anthropic
  uses one fresh mount-free container from the explicitly selected compatible
  Context image as a provider-only acquisition authority. Auth control and
  the encrypted companion use bounded fixed operations and never expose a
  public broker or host TCP API.
- The broker starts locked and retains the installation root key only in
  memory. Cluster reconciliation unlocks it with key bytes transferred through
  stdin. Context vaults are keyed by stable Context ID and bind their version
  and Context ID as authenticated data.
- All Tobari-owned schemas and component APIs are V1 before first publication.
  Readers accept exactly V1 and reject every other version; there is no legacy
  migration or compatibility path. Owner manifests and the normalized
  projection share that version while reviewed built-ins use typed closed
  plans within it. Owner manifests still contain no secrets, executable shell,
  refresh logic, or signer and remain single-secret protected-stdin imports.
- Provider-native executables do not enter the Auth Broker image. Reviewed
  host acquisition drivers execute fixed argv against verified host CLI
  identities in private temporary homes with sanitized environments. The
  Anthropic driver instead executes exact Claude Code in a fresh selected-
  Context-image container with no mounts, project state, persistent home,
  Broker socket, or Docker socket; that image sees only its own provider login.
  Claude account entitlement labels needed by the pinned native client are a
  narrow non-secret part of the Tobari storage contract: acquisition extracts
  the bounded access token, refresh token, expiry, dynamically granted scope
  set, subscription type, and rate-limit tier into a versioned Tobari-owned
  record. Every other provider-owned optional field is discarded. Scope and
  entitlement names are provider output, not compiled Tobari catalogs:
  acquisition bounds their OAuth syntax, requires the granted set to be a
  subset of the observed authorization request, and normalizes ordering.
  Browser
  targets, callback behavior, output framing, cleanup, versions where the
  client contract is pinned, and cancellation are closed per provider.
  The experimental built-in set is GitHub, Datadog, OpenAI/Codex,
  Anthropic/Claude, Chatwork, and AWS. A release or standard build cannot
  activate any of these Broker plans through configuration, environment, or
  runtime input. Capability maturity is one compile-time profile rather than a set of
  per-feature escape hatches. No manifest-selected helper, arbitrary
  OAuth client, executable adapter, provider SDK inference, or provider
  business-operation command is supported.
- The macOS root-key provider stores one installation key in Keychain. Linux
  uses an owner-only XDG state file and makes no host-user-compromise claim.
- A recognized malformed, copied, stale, or mismatched Tobari handle fails
  closed and is never forwarded upstream. Login and logout rotate or revoke
  every associated project handle; existing sessions receive a concrete
  re-entry action because their environment cannot change retroactively.
- A handle is not the primary credential, but it is a scoped bearer capability:
  copying it does not broaden its Context/project/binding authority, and users
  should not publish or log it. A real credential at a declared header or AWS
  signing binding fails before OPA, broker resolution, DNS, or upstream I/O.
- Brokered login does not itself grant network authority. OPA remains the sole
  authority for Context, project, scheme, host, port, method, and path. A
  brokered request receives only authority already present in the Context's
  immutable snapshot, selected binary readiness overlay, or learned rules. The
  default agent-ready preset includes
  finite compile-time `claude_ready`, `codex_ready`, `gh_ready`, `twg_ready`,
  and `pup_ready` authentication bundles coupled to reviewed client versions and expanded only into
  exact Context-wide effects. TWG's bundle includes only its exact device
  exchange, site inventory, stable CLI manifest, token revoke, and declared
  GraphQL `query` / `me` current-user lookup. Pup's
  bundle includes only exact US1 DCR registration and token exchange/refresh.
  Credentials and bundle names never widen runtime
  authority, and all unmatched effects remain reviewable.
- Built-in broker implementations are a closed typed union of static secrets,
  reviewed renewable sessions, fixed supplemental-header application, and the
  experimental AWS request-local signer. They exist only in the experimental
  profile. Owner manifests remain strict static-primary-secret,
  non-secret, non-executable local data and cannot select helpers, dynamic
  records, refresh, signing, policy, arbitrary routes, or provider business
  operations.

### Mechanical enforcement

- Runtime mount tests reject host-home and managed-secret mounts. Gateway tests
  use canary secrets to prove redaction, broker-required declared bindings,
  zero-I/O direct-credential rejection, deny-before-resolution, exact
  replacement, and compatibility client-header forwarding only after allow.
- Integration tests prove one Tobari's tool-owned state persists through runtime
  recovery, is unavailable to another Tobari, and is removed by exact delete.
  Broker tests prove encrypted Context ownership, project-specific handles,
  restart locking, rotation, revocation, and canary-free output.
- Acquisition tests fix every reviewed host executable identity, argv,
  environment, conventional non-project installation-root selection,
  control-safe visible output, checked private-home cleanup, purpose-limited
  browser/callback behavior, cancellation, and provider-specific version/state
  contracts.
- Companion, Broker, Gateway, dependency, image-content, state, catalog, and
  runtime tests prove the compiled plan union, encrypted task correlation,
  durable outcome-unknown barriers, policy-before-refresh/signing, exact one-
  attempt application, and absence of arbitrary execution fallbacks.
- One versioned test-only capability projection proves the intentional
  translations between the Go built-in/acquisition vocabulary and the Python
  Broker/Gateway closed unions. Production components never load that fixture,
  so parity enforcement does not become runtime registration authority.
- Narrow-projection tests fix every allowed scalar, bounded host read, private
  re-encoding target, precedence rule, and hostile source-file/key canary; no
  projection test treats identity as authentication authority.

## Thesis 4: One shared cluster hosts multiple CWD-owned Tobari

MVP manages one installation-local enforcement cluster and multiple logical
Tobari. The standard shared cluster contains exactly one Gateway and one OPA;
the experimental development profile adds one Auth Broker. The cluster uses a host-issued
Context/project-principal boundary: stable Tobari and Context IDs are not
trusted merely because they appear in caller data, but the host binds both to
the exact Gateway network interface that received the request. Context policy,
learned permissions, and broker handles cannot cross that Context/project
binding. A Tobari is selected
from the canonical current directory: an exact indexed root is reused directly;
when only ancestor roots exist, the interactive root command presents every
containing root nearest-first and an explicit create-here option. A new nested
root is never implicit. Every selected Tobari binds exactly one read-write
root with its Context-selected access, one dedicated internal network, and one persistent XDG-owned home
directory.

### Consequences

- `cluster up` explicitly creates or reconciles the shared enforcement runtime.
  Interactive `tobari` may compose that exact separately declared mutation
  whenever the selected shared projection is not ready; on first use it does so
  only after the ordinary Context wizard confirms creation. It then
  resolves an existing canonical-root record or creates one at the current
  directory, reconciles only the project runtime, and enters the work container.
- The logical record owns a generated stable internal ID, canonical root,
  last reconciled compatible runtime image, profile, XDG home, and diagnostic
  runtime identifiers. Container or network loss never changes logical
  existence.
- The selected project root remains the only writable project mount. For a root
  below the host home, its relative path is preserved below the container's
  `/var/lib/tobari` home; home-external roots retain the `/workspace` mirror.
  The host home is never mounted wholesale, and runtime image assets remain
  outside the mutable home boundary.
- Workspace existence and interactive session attachment are separate states:
  `tobari` attaches a session to an existing or newly created Workspace, while
  `exit` detaches only the session. The Workspace remains existing and
  reusable until the host runs `tobari delete`. A delete with an attached
  session is rejected unless the host explicitly uses `--force`; there is no
  user-facing stopped or paused Workspace state.
- `status` and `delete` resolve the nearest canonical root. The interactive
  `tobari` entry path resolves an exact root directly or requires an explicit
  choice among containing ancestor roots before it can create a nested root.
  `list` shows local roots and diagnostics without turning an ID into a normal
  user input.
- A canonical root plus stable Context identity is a unique Workspace key.
  Repeated or concurrent explicit creation for that pair must yield one logical
  record and a typed already-exists outcome for losing callers. The same root
  may have independent Tobari in different Contexts.
- Each logical Tobari is permanently bound to one stable Context identity. That
  Context is the only runtime-image authority for its creation and
  runtime-container reconciliation; project metadata records the last
  successful image for diagnostics but does not silently override the binding.
- The selected image is an environment and tool source, not the Workspace
  lifetime owner. Tobari starts the work container with its own fixed lifetime
  command; an image `CMD` such as `claude` cannot make a child-agent exit stop
  the Workspace. The base runtime carries the common work tools plus the pinned
  Codex and Claude Code clients required by the supported agent-ready Context.
  Their binaries remain outside the mutable Workspace home and their versions
  form one reviewed contract with the default policy baseline.
- Context creation review exposes the selected standard image. Creating a
  runtime recipe does not replace it: a pending or invalid recipe blocks root
  entry with exact build/inspection recovery, and only successful
  `runtime build` promotion changes the selected image.
- `tobari delete` is the only routine operation that ends a logical Tobari;
  ending a shell or losing a runtime resource leaves it existing.
- Every process in a Tobari may modify or delete every file below that Tobari's
  mounted root.
- The host-owned principal registry is the only source of project authority at
  Gateway. It binds the exact owned Workspace source endpoint and Gateway
  endpoint on one dedicated project network to the stable Context/project;
  session headers, names, SNI, URLs, environment, and profiles cannot select a
  project.
- A missing, malformed, ambiguous, or stale principal binding denies before
  policy evaluation and upstream I/O. Network recreation reconciles the
  binding before the project can use the proxy.
- Multiple clusters, overlays, clone modes, root locks, and change approval are
  non-goals. Overlapping roots intentionally expose the same mounted host-file
  mutations even when their Tobari belong to different Contexts; Tobari does
  not claim filesystem integrity isolation between them.
- A project root cannot be the filesystem root, the user's home or its
  ancestor, or any XDG configuration, state, or shared-profile management
  path. A policy source repository remains an ordinary allowed project root;
  active policy is published separately by explicit trusted-host operations.

### Mechanical enforcement

- Root-index and instance-state tests prove canonical path resolution, ordered
  ancestor-candidate selection, explicit nested-root creation, stable IDs,
  one-Workspace-per-canonical-root enforcement, atomic state updates, journal
  recovery at multi-file boundaries, and fail-closed handling of corrupt or
  unsupported-version state.
- State and Docker labels identify the one installation-owned cluster and each
  exact logical Tobari resource.
- Lifecycle integration tests create multiple roots, prove network separation,
  enter and recover the same root repeatedly, preserve the Workspace after
  session exit, and delete only the selected instance without growing owned
  resources.
- Context-only image selection tests prove project metadata cannot silently
  override the execution boundary for new or existing Workspaces before Docker
  mutation.

## Thesis 5: Lifecycle changes are bounded and ownership-labeled

Tobari manages only resources with its exact installation labels. Reads and
mutations are explicit catalog operations, and destructive cleanup never uses a
name prefix or broad Docker query as authority.

### Consequences

- Root resolution, logical creation, runtime reconciliation, and deletion are
  idempotent within declared state. Concurrent mutations serialize through the
  XDG state lock and atomic durable records.
- Session attachment is transient process state, not a persisted lifecycle
  resource. The only public session transition is `exit`; the only routine
  transition to Workspace absence is the host-side `delete` action, guarded by
  active-session detection and optionally overridden with `--force`.
- `delete` removes one exact container, network, home directory, and instance
  records when no session is attached. An attached session rejects ordinary
  deletion and `--force` explicitly overrides that guard. It can continue
  after partial runtime cleanup and never selects by a Docker name or prefix.
- `cluster down` refuses to remove shared enforcement while any Tobari remains;
  `--purge` affects only shared CA and active policy-bundle state after the
  cluster is empty.
- Docker CLI is behind an infrastructure port so another engine can replace it
  later without changing application outcomes.
- The project runtime spec hash includes the fixed resource contract, so an old
  or drifted container is recreated before reuse.
- The project runtime spec hash includes the fixed Workspace lifetime command,
  and image compatibility is rejected before project runtime resources are
  mutated.
- Shared Gateway and OPA services use the same fixed JSON log rotation bounds;
  the experimental Auth Broker follows those bounds as well. A project cannot
  fill their host-side Docker logs without a cap.
- Shared Gateway and OPA services carry fixed CPU, memory-plus-swap, and PID
  bounds; the experimental Auth Broker does too. Those limits protect the
  Docker VM but do not promise per-project fairness inside shared services.
- `status`, `list`, and `doctor` never reconcile Docker or create/delete
  runtime resources. They may perform bounded journal cleanup before selecting
  logical state so an interrupted multi-file write cannot remain authoritative.
  The root command is the single deliberate ensure-and-enter runtime
  reconciliation path.
- `doctor` owns one finite, topologically ordered check graph in application
  code. It observes every ready check, reports every inventory member, and
  marks an unready dependent as blocked by exactly one direct non-passing
  prerequisite; infrastructure cannot invent blocked results or recovery.
  Policy diagnosis reads bounded owner-controlled source structure on the host
  and never creates an OPA test container.
- Every catalog-declared read is observational on first use and during ordinary
  operation: it creates no Tobari-owned configuration, state, lock, policy,
  credential, key, vault, or Docker resource. Missing state remains explicit
  absence or a display-only synthetic default without stable authority. The
  sole read-side mutation is bounded cleanup of a pre-existing validated
  interruption journal, which may create the project recovery lock only to
  serialize that cleanup; reads never create the journal itself.

### Mechanical enforcement

- Domain resource specifications carry a fixed ownership label.
- Application tests prove validation and CWD-local target resolution precede
  Docker calls, ambiguous entry requires an explicit choice, and cleanup
  selects exact resources. They also prove ordinary deletion observes attached
  sessions before the destructive boundary and `--force` is the explicit
  override. CLI tests prove session-exit guidance stays on host stderr,
  separate from child stdout.
- The catalog declares every read/create/write effect and mutation impact.

## Thesis 6: Fail closed with bounded evidence

Unknown configuration, malformed OPA responses, timeouts, and unclassified
Gateway errors do not authorize traffic.

### Consequences

- Default policy denies.
- OPA and upstream calls have finite timeouts. Mitmproxy retains the fixed
  8 MiB advertised-body cap: a request or response whose `Content-Length`
  exceeds it is rejected before the ordinary addon header hook.
- After header-time authorization, ordinary request and response bodies use
  mitmproxy's streaming path rather than full-body retention. A trusted
  declared GraphQL endpoint is the only narrow exception: it
  accepts either one unambiguous positive length of at most 1 MiB or no length
  and no transfer/content encoding. The fixed 8 MiB transport cap bounds an
  arriving lengthless buffer; Gateway then rejects any complete body over 1
  MiB before parsing generic operation/root identity or policy. It forwards the
  original bytes once after allow. Unknown-length ordinary bodies remain
  streaming; unsupported GraphQL forms fail closed.
- Audit logs contain route metadata, never body content, body hashes, or secret
  values.

### Mechanical enforcement

- Gateway unit tests cover timeout, invalid decision, authorization-before-
  stream ordering, ordinary body-free policy input, bounded GraphQL-derived
  identity, and secret redaction. Runtime-asset
  tests keep the advertised-body cap present; integration proves an over-limit
  declared body stops at Gateway and allowed chunked request/SSE response bytes
  arrive incrementally.
- Rego tests start from deny and add explicit allow rules.
- Structured logs are scanned for secret and body canaries.

## Thesis 7: Claims must be executable

Documentation is complete only when the same boundary is covered by a type,
test, lint, policy test, or integration scenario.

### Consequences

- `cli.Catalog` remains the public command source of truth.
- The four-layer dependency direction remains in force.
- Every executable identifies its source version/commit, compiled runtime-image
  resolver channel, source-required component APIs, and the APIs
  selected by that resolver. Missing source metadata or an API mismatch is
  never presented as compatible.
- Tobari publishes no OCI images. Released and development CLIs derive local
  Gateway and agent-ready image identities from their embedded pinned source,
  build missing images on the user's Docker host, and validate them before
  mutation. Auth Broker remains local to experimental builds.
- `task check` is the implementation completion gate; security and public
  changes also run their named profiles.
- Docker integration is a separate explicit profile because it requires a
  working Engine, but CI runs it on supported Linux runners.

### Mechanical enforcement

- `tools/archlint`, catalog contract tests, Go unit tests, Gateway tests, OPA
  tests, and Docker integration tests cover distinct boundaries.
- Embedded-release and development build fixtures prove that source-derived
  local identities cannot cross resolver channels, and cluster preflight
  rejects an API mismatch before state or Docker mutation.
- `.harness/capabilities.json` classifies every supported and excluded outcome.
- CI delegates to repository scripts rather than duplicating commands.

## Thesis 8: Denial is a safe policy-development interface

The default-deny experience is successful only when a developer can understand
what boundary effect was rejected and deliberately teach Tobari the minimum new
rule from the trusted host without turning ordinary agent work into a policy
administration project.

### Consequences

- Every HTTP denial emits bounded structured audit metadata including host, port,
  method, path, decision, reason, and whether a reviewed learned rule can resolve
  the denial without weakening orthogonal invariants.
- Every denial also declares its closed destination kind and authority
  lifetime. Ordinary upstream candidates remain persistent learned-policy
  proposals. A Host Loopback candidate instead binds its Context, project,
  Attachment Epoch, target port, and exact HTTP effect and can
  produce only an attachment-scoped decision. Matching host, port, method,
  path, or display text cannot convert one lifetime into the other.
- A learnable denial returns a fixed, secret-free host-side review command to
  the agent; a completed session also summarizes the pending queue on host
  stderr. Neither notification can mutate policy or trigger a retry.
- Interactive `policy review` is the installation-wide human Permission Inbox
  over retained queues from every Context. Selection and detail inspection may
  stage several explicit exact or single-segment-template choices, but staging grants
  no authority and cancellation discards the whole staged set. A choice is
  accepted only from its exact detail screen. One final Apply confirmation
  binds the complete typed snapshot and applies the staged set as one
  command-owned installation policy decision-set mutation. Every opaque
  exact candidate or path-template proposal ID is retained unchanged and
  revalidated against fresh evidence. A manual refresh reconciles staged
  choices only by typed review-item ID: retained
  IDs keep their decision and order, stale IDs lose Apply eligibility, and a
  matching display label never transfers authority to a replacement ID.
  Its list groups by validated stable Context/project identity, presents that
  scope once per group, and leads each selectable row with the exact effect or
  typed template plus bounded observation evidence. Display labels, adjacency, and
  indentation never create policy identity.
  Redirected and
  machine-readable `policy review` remains read-only. `policy candidates`
  remains the machine discovery surface. `policy rules` is the exhaustive inventory of
  current CLI-owned learned decisions; `policy reset --id` removes exactly one
  Allow or exact Deny and returns that effect to default deny. The discovery
  queue turns only learnable retained denials into unique exact
  Context/project/scheme/host/port/method/path proposals with stable opaque IDs, the latest
  retained observation time, and a retained-window observation count; the current-rule
  inventory exposes the separate rule IDs used for reset.
  Active Attachment Grants are a separate exhaustive runtime authority
  inventory and never appear as durable `policy rules`. Interactive review
  labels them as Workspace-wide for the current attachment and offers only
  attachment-scoped Allow or Deny. Detach revokes them without `policy reset`.
- `tobari cluster denials` remains the lower-level diagnostic step: it projects
  validated denial records, reports the editable host-side policy directory,
  and points to the Permission Inbox. Raw component logs remain available.
- `policy allow --id` and `policy deny --id` are explicit trusted-host actions
  that consume one candidate ID unchanged. `policy reset --id` consumes one
  current `policy-rule` ID unchanged. Allow preflights and atomically records
  one exact learned rule; deny records one exact project-bound terminal rule;
  reset removes one current learned rule and leaves the same request at
  default deny. All three activate through the same portable OPA boundary, and
  an exact deny wins over a learned allow for the same request.
- Learned authority is exact unless the human explicitly approves a typed
  single-segment `{id}` template after two distinct compatible HTTP paths.
  Repeated observations of one path remain one example. Templates preserve
  every other request dimension and literal segment; prefix, wildcard, regex,
  multi-segment, GraphQL, and ambiguous proposals remain invalid.
- OPA watches one revisioned complete bundle mounted read-only from an exact
  owner-labeled Docker-managed volume. Exact allow, deny, reset,
  and reviewed-set actions test a private complete policy copy, atomically
  replace one complete Context `policy/domains/` generation, build a
  revision-named archive through pinned OPA,
  atomically rename it through a fixed pinned publisher, and
  report success only after the running OPA proves the expected revision is
  active. Authority-reducing or mixed changes first confirm a complete
  deny-all transition revision and restore the prior known-good revision on
  failure. The trusted host remains the only policy writer.
- Audit evidence never includes credential values, cookies, raw bodies, or raw
  response data.
- Tobari never changes permission from observation alone. Every learned rule or
  reset remains an explicit trusted-host mutation. Machine
  actions stay opaque-reference-bound; interactive reviewed-set Apply is one
  fixed-target mutation whose typed contents are unchanged opaque references
  selected from typed detail screens. Every candidate must pass `opa test`;
  finite examples and canaries detect declared
  regressions but do not prove safety for every unknown future request.

### Mechanical enforcement

- Gateway tests assert both useful denial dimensions and absence of secret/body
  canaries.
- Integration tests deny known requests, retrieve their typed audit records
  through the CLI, assert the structured agent navigation and host-only session
  summary, stage exact Allow and Deny choices through the human queue, apply
  them with one activation, exercise the allowed exact rule, and retain a
  denied boundary without restarting any Tobari or OPA.
- Host Loopback tests bind the constant capability to a host-derived Attachment
  Epoch, prove agent-visible discovery does not grant access, apply one exact
  attachment decision through the same review boundary, and assert zero
  host-loopback I/O for malformed, undeclared, stale, denied, OPA-unavailable,
  and post-detach requests. Relay tests prove the reviewed port cannot select a
  sibling port, route-first teardown, borrower non-ownership, and Workspace-wide authority.
- README makes the observe-review-decide-retry loop the primary operating
  workflow, keeps routine permission growth free of hand-authored OPA/Rego,
  and keeps tested host editing as the advanced escape hatch.

## Thesis 9: Every Tobari belongs to one logical Context

Users should choose one understandable execution setup, not assemble an agent
profile, runtime image, policy directory, and credential configuration from
unrelated paths.
Tobari therefore presents a named Context as the immutable host-owned
capability envelope for an agent's direct source access, snapshotted network
guardrail, configuration, runtime, and credential exposure. The Context manifest
is a host-owned composition record; it does not collapse the physical trust
boundaries between read-only agent data, OPA policy, and Gateway-only secret
stores. Each Context has a stable opaque identity; its name is a human selector,
not authority. Each Tobari permanently records one Context identity, and the
host derives that binding for Gateway and OPA from its network principal.

The standard installation runs one shared Gateway and one shared OPA for every
Context. An experimental development installation additionally runs one locked
Auth Broker.
The current Context is only the default when a host invocation omits a Context;
changing it cannot retarget or mutate existing Tobari or shared enforcement.
Tool-native authentication state remains below each Workspace home and is not a
Context secret. In the experimental profile, a brokered credential is owned once by the stable Context and
enables every permanently bound Workspace to receive a different project-bound
handle on its next reconciliation. A declared provider binding is handle-only;
Workspace-owned passthrough remains only for undeclared bindings. Neither a
provider name nor a handle selects authority without the trusted principal and
OPA allow.

### Consequences

- `context list`, `context show`, `context create`, `context delete`, and
  `context use` are the host-facing composition surface. `context use` changes only the omitted-
  Context default. `tobari --context NAME` chooses an invocation Context
  without changing that default.
- Every persisted Context fixes `read-only` or `read-write` access for its one
  direct source bind and one normalized `builtin/<name>` or `custom/<name>`
  policy-preset origin plus SHA-256 snapshot revision. The snapshot fixes the
  owner-selected guardrail and non-readiness baseline plus an enabled or
  disabled native-readiness capability. Readiness independently selects the
  current trusted-binary overlay. Argument-free creation on an interactive
  text terminal uses one continuous five-step frame to collect the Context
  name, direct source access, one complete method policy, and optional typed
  Workspace bootstrap, then reviews the complete composition before its single
  mutation. It does not leave and re-enter the interaction frame between
  fields. Its method-Deny choices
  remove only now-unreachable positive baseline rules from the immutable
  selected-preset snapshot; destination ceilings and exact Denies remain
  unchanged. Any explicit create input selects deterministic direct mode.
  Creation owns the `read-write`,
  `builtin/agent-ready`, and enabled-readiness omission defaults. A missing
  readiness field in legacy state preserves historical behavior: enabled only
  for `builtin/agent-ready`, disabled otherwise. Readers never rewrite old
  state, source-preset edits never change an existing
  Context, and a different envelope requires a new Context. A binary readiness
  update is a reviewed compatibility update rather than an envelope change and
  requires no Context recreation. The current binary readiness catalog is part
  of the aggregate content identity: observation reports an older active
  projection as invalid, and root entry fails closed with the explicit
  `cluster up` recovery instead of entering against stale authority.
- Context deletion is an explicit destructive catalog mutation. The
  foundational `default` Context, the current Context, and every Context still
  referenced by a logical Workspace are rejected before removal. Successful
  deletion removes the exact owner-only Context and Context-ID authentication
  stores, preserves project roots and shared runtime images, and requires an
  explicit `cluster up` when shared state exists.
- Source access describes only the direct live source bind. Read-only does not
  make the writable home or tmpfs read-only and does not provide a snapshot;
  host or same-root read-write Context changes remain observable.
- `config shell` and `config git` own the Context's narrow non-secret host
  projections. A complete setting group is deterministic for agents and
  scripts; wholly omitted setting flags open a terminal-only staged editor.
  Shell presents the complete fixed inventory and commits every distinct
  staged row through one atomic Apply; Git presents its complete source choice
  and uses the same stage/Apply vocabulary. Partial, redirected, and JSON
  wizard attempts fail before mutation. `config shell` retains only `PS1`, `TERM`,
  `COLORTERM`, and `NO_COLOR`; a V1 Context inherits exported `PS1` by default.
  An absent export retains Tobari's built-in prompt. `config git`
  owns one atomic `user.name`/`user.email` fallback and defaults to no
  projection so personal identity is opt-in. Git inheritance reads only those
  two host-global values for the stable Workspace root. No credential,
  executable startup hook, host shell or Git file, include directive, helper,
  signing setting, arbitrary environment name, or arbitrary Git key crosses
  either boundary.
- `runtime init` and `runtime build` are the host-facing runtime customization
  surface. The selected Context owns one fixed `runtime/Dockerfile`; build
  validates the resulting image and promotes it into that Context without
  requiring a second image-selection command. Only Tobari bound to that Context
  observe the promoted image on their next entry while preserving their home.
- Context creation initializes an owner-only policy store, references a
  read-only agent profile, and records the compatible Tobari runtime image.
  Auth Broker vault state remains separately keyed by stable Context ID rather
  than referenced from the manifest. Context creation never accepts a secret
  value in an argument, environment variable, or manifest.
- Context policy source is grouped by exact canonical lower-case host at
  `policy/domains/<host>/allow.json` and `deny.json`. The allow document owns
  that host's authorities, methods, GraphQL endpoints, credential bindings,
  and learned Allows; the deny document owns baseline and learned exact
  Denies. Directory and embedded hosts must agree, methods never flow between
  hosts, deny precedence remains terminal, and wildcard/IP/ambiguous host
  syntax is unsupported. Context source has no `data.json`; a generated
  immutable OPA projection may use that filename internally.
- Auth login/import affects one explicit or current Context and makes the
  Context-wide Workspace eligibility explicit. Login does not rewrite running
  Workspaces; their next matching entry issues project-bound handles and
  recreates only a changed work container while preserving home.
- Context source changes become active only through an explicit `cluster up` or
  policy mutation. The host serializes source mutation across processes,
  validates and swaps a complete domain generation under a durable recovery
  journal, then generates and validates one atomic projection of every Context
  for the shared OPA and Gateway. Incomplete, raced, or ambiguous source state
  fails closed; a failed candidate preserves the complete prior known-good
  source generation and projection.
- The ordinary guided mode keeps deny/review/allow exact permission growth as
  the default. Advanced mode keeps trusted-host Rego and tests available for
  policy that cannot be expressed as an exact learned rule. The projection
  namespaces Advanced modules and prevents them from claiming the Tobari-owned
  router or system packages.
- The preset destination ceiling and complete method policy are owned by the
  Tobari system evaluator and precede baseline data, exact learned policy, and
  Advanced Rego. Every HTTP method resolves from one `allow`, `exact_review`,
  or `deny` default plus exact overrides. A terminal destination or method Deny
  denial produces no candidate and performs no external DNS, broker resolution,
  or upstream call. Advanced Rego may further constrain generic input but
  cannot grant beyond the guardrail or redefine learned permission identity.
- Enabled native readiness preserves the pinned Claude Code 2.1.220 and Codex
  0.147.0 native capability plane, the pinned GitHub CLI 2.96.0 native
  authentication bootstrap, TWG CLI 1.2.5 native login readiness, and pup
  1.10.7 US1 native login readiness when those clients are supplied by a
  custom runtime. Compile-time `claude_ready`, `codex_ready`, `gh_ready`,
  `twg_ready`, and `pup_ready` bundles expand from the trusted
  binary into the exact authentication effects required for routine native
  login. One dedicated compile-time catalog gives each pinned client an
  independent append-only readiness contract revision. Aggregate generation
  replaces historical snapshotted readiness rules with the current set; bundle
  and executable names are never policy identity.
  Model execution, account state, bootstrap, first-party capability discovery,
  fixed telemetry, and bounded provider-owned evaluation receive reviewed
  baseline authority. Dynamic evaluation paths use
  one direct-child identifier template without retaining the observed
  identifier. MCP transport is classified only at a trusted exact endpoint:
  initialization and capability enumeration are baseline methods, while
  `tools/call` remains reviewable by exact tool name and every other action by
  exact JSON-RPC method. Request arguments, resource URIs, responses, downloads,
  file transfer, acquisition, self-update, and unmatched traffic receive no
  baseline authority. These grants are Context-wide effects, not executable
  identity; exact Deny remains terminal.
- `builtin/offline` defaults every method to terminal Deny and makes no effect
  review-eligible. `builtin/reviewed-exact` defaults every method to Exact
  Review. `builtin/get-only-reviewed` defaults to Deny with an exact GET Exact
  Review override. `builtin/public-get-reviewed` defaults to Exact Review with
  an exact GET Allow override. The three strict presets grant no immediate
  authority. GET receives no intrinsic safe or read-only classification, and
  exact Deny remains terminal over method Allow.
- Tobari-owned ordinary learned permission identity binds Context, project,
  scheme, host, port, method, and raw path. Query, headers, and bodies are not
  learned dimensions; GraphQL adds only operation type and root field, while
  MCP adds only JSON-RPC method and, for `tools/call`, exact tool name.
- Permission candidates, learned rules, exact denies, audits, and brokered
  handles retain Context and Tobari identity. `policy review` and `policy
  rules` cross all Contexts; mutations bind solely to opaque references.

### Mechanical enforcement

- Context domain and catalog tests validate stable identity, modes, current-
  default selection, effects, fixed targets, and complete output/error contracts.
- Envelope tests require source access and preset origin/revision in every
  persisted manifest/report, prove creation-only defaults and immutable
  snapshot binding, and reject missing or old state without fallback.
- Configuration tests validate the all-or-none direct/staged-editor state machine,
  terminal cancellation and explicit-empty Context rejection with zero
  mutation, binding of Apply to the Context shown across concurrent default
  changes, one atomic multi-row shell write, fixed shell and Git inventories,
  exact V1 persistence, bounded host Git
  calls with an exact child-environment allowlist, lower-precedence read-only
  projection, and exclusion of authentication and executable Git settings.
- Infrastructure tests prove exact V1 initialization, owner-only separated
  policy and broker-vault boundaries, permanent Tobari bindings, aggregate
  read-only OPA mounts, and selected agent-profile digests.
- Runtime and policy integration prove exact direct-bind access, writable
  home/tmpfs, no writable source alias, scheme-aware exact learning, and
  terminal guardrail precedence with zero candidate/DNS/Broker/upstream calls.
- Runtime tests prove the recipe build context excludes policy and broker-vault
  stores, the generated image is checked against the runtime contract, and a
  failed build leaves the previously selected image unchanged. Project runtime
  tests prove existing Workspaces reconcile to their bound Context image only
  after validation and preserve their home.
- Runtime, Gateway, OPA, Auth Broker, and integration tests prove Context secrets and learned
  permissions do not cross Context/project principals, aggregate activation is
  all-or-nothing, and forged or stale Context bindings fail closed.
- Agent-readiness validation records current-default discovery, explicit
  invocation selection, and installation-wide permission review.
- Runtime/preset compatibility tests bind both pinned agent versions to the
  exact agent-ready grant catalog, retain strict-preset zero-grant canaries,
  distinguish capability bootstrap from MCP action, exclude payload and
  acquisition authority, and prove exact Deny precedence.

## Deliberate non-goals

### Typed Workspace bootstrap is a Context recipe, not host inheritance

A Context may snapshot a closed, schema-versioned, secret-free subset of host
tool configuration for one-time projection into future Workspace homes. Each
adapter owns exact source paths, selected fields, normalization, dependencies,
semantic revision, and hostile-input tests. The first adapter accepts one AWS
IAM Identity Center profile plus its referenced SSO session from
`~/.aws/config`. One dependent `kubernetes_eks` adapter may select an exact
context from fixed `~/.kube/config`, but only when it resolves to inline CA and
commercial EKS HTTPS target data plus the reviewed `aws eks get-token` shape
whose `AWS_PROFILE` equals that AWS adapter. Tobari canonicalizes the exec
contract; it never copies its source bytes. Neither adapter reads credentials,
token caches, arbitrary helpers or executable selections, includes, arbitrary
dotfiles, or alternate paths.

Projection occurs only while a new logical Workspace is being created and is
recorded before that Workspace becomes authoritative. Context refresh changes
only the recipe for future Workspaces. Existing Workspace homes are never
synchronized or rewritten and retain their create-time revision. This grants
no network permission and transfers no authentication authority; native login
remains owned by each Workspace home.

MVP does not support multiple clusters, non-EKS clusters or generic kubeconfig,
process-level identity, a per-project
static baseline policy, non-HTTP forwarding or policy, recursive DNS, Git SSH,
provider-specific policy semantics, Git-over-HTTPS credential helpers,
manifest-defined dynamic credentials, generic refresh or signing, multiple provider accounts
per Context, approval workflows, general Kubernetes authentication or
transport, filesystem overlays, GUIs, remote execution, or multi-tenant
production use.
