# Project Theses

These theses govern Tobari. When implementation pressure conflicts with them,
revise the thesis and its enforcement explicitly instead of adding a bypass.

## North Star

**Tobari gives a coding agent an execution boundary in advance, then lets it
move freely inside that boundary. By making isolation startup, understanding a
denied operation, and granting the minimum required permission extremely easy,
it makes the safe execution path a more natural choice than running the agent
directly on the host.**

Every HTTP and HTTPS effect that crosses that boundary is denied by default and
authorized as a normalized request by one shared OPA-backed Gateway. Trusted
policy may declare an exact GraphQL endpoint whose generic L7 identity extends
the HTTP coordinates with one operation type and root field per effect.

The primary users are developers who run Claude Code, Codex, shells, tests, and
other arbitrary programs against project roots. Success has two inseparable
parts: each Tobari can host concurrent processes, cannot reach the Internet or
another Tobari directly, never receives a real host-managed credential, and is
selected from the canonical current directory rather than a user-managed name,
root flag, or container identifier; and the user can reach that boundary
without becoming a Docker or policy operator. A user may deliberately create
tool-owned authentication state in that Tobari's own persistent home; this is
not host credential inheritance. One installation-local cluster shares one
Gateway, one OPA, one locked Auth Broker, an atomic all-Context policy projection, and CA state without
sharing Tobari homes or runtime networks. Host-issued Context/project principals
bind mutable learned permissions and brokered credential handles to the exact
current-directory network that originated a request; they do not select a real
credential on their own.

The first testable slice is one local mock upstream reached through the
Gateway: an allowed request succeeds, a denied request does not reach upstream,
direct egress fails, and an OPA outage fails closed.

The core product loop is progressive policy learning: work freely in Tobari,
observe a denied boundary effect as secret-free evidence, receive a fixed
host-side review cue, keep the current Workspace and agent session running,
review the pending exact permission from a separate trusted-host terminal,
approve the minimum rule, and retry in that same session. The normal path does
not require writing OPA or Rego by hand:
interactive `policy review` presents a Permission Inbox, stages explicit exact
Allow or Deny choices from candidate detail, and applies the reviewed set once;
its non-interactive and machine-readable path remains read-only. Staging grants
no authority. Final Apply revalidates every unchanged opaque candidate, tests
one complete all-Context candidate, and hot-activates one revision without
restarting active Tobari or the shared OPA. Single-reference `policy allow` and
`policy deny` remain available to machines and recovery workflows.
Successful Apply reports the authoritative active revision and the ordered
exact decision receipts; it never asks the caller to re-enter the Workspace.
A separate `policy rules` view makes the complete current learned Allow and
exact Deny decisions visible, and its TTY flow can explicitly reset one
decision to default deny. Reset never grants or retries; it makes the retained
effect eligible for `policy review` again.
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
- A denied network effect should provide enough secret-free context and a
  concrete host-side review action to reach one exact, tested permission. The
  safe action may remain opaque-reference-bound internally, but the human
  presentation must not make reference plumbing or OPA syntax the user's
  primary task.
- Host-authored Rego, raw logs, Docker diagnostics, and provider-specific
  details remain available as advanced paths; they do not define the routine
  agent workflow.
- The human first-use route starts with the explicit shared `cluster up` and
  CWD-first `tobari` entry. Read-only `doctor`, status/list inspection, and
  opaque-ID policy actions remain recovery or machine paths rather than steps
  in the normal journey. On a TTY, `policy review` is the complete human
  review-to-decision flow, while `policy rules` is the complete human
  inventory-and-reset flow; neither requires a second JSON review.
- The current explicit `cluster up` bootstrap and separate denial discovery
  commands are compatibility surfaces under review against this thesis. They
  must not be mistaken for the product's central value.

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

- Any process that honors the explicit proxy receives the same enforcement.
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

Each Tobari has exactly one internal Docker network path: the Gateway proxy
interface. Gateway alone joins every Tobari network plus the shared control and
egress networks; OPA joins only control.

### Consequences

- Proxy bypass has no route to an external upstream.
- Tobari cannot reach OPA, Gateway management interfaces, or another Tobari.
- Gateway or OPA failure denies outbound traffic.
- Each work container receives fixed CPU, memory (including total memory plus
  swap), process-count, and
  container-log bounds so one untrusted workload cannot consume those shared
  resources without limit. Disk quota and network bandwidth remain separate
  unclaimed controls.
- Host networking, privileged mode, Docker socket mounts, and added
  capabilities are forbidden.

### Mechanical enforcement

- The runtime adapter constructs one labeled internal network per Tobari, a
  shared internal control network, and a separate egress network.
- Integration tests prove direct egress, direct OPA access, and traffic during
  Gateway or OPA failure do not succeed.
- Runtime specification tests reject privileged mode, host networking, Docker
  socket mounts, broad host-home mounts, and missing fixed resource bounds.
- Runtime integration inspects CPU, memory, PID, and log limits after creation
  and after recovery.
- Image-selection tests require a locally available runtime-API-compatible
  image before any per-Tobari resource is created; image configuration cannot
  replace the CLI-owned isolation arguments. Runtime API compatibility includes
  the bootstrap needed to execute Tobari's fixed Workspace lifetime command.

## Thesis 3: Authentication handling is pluggable and real credentials stay outside Workspaces

Tobari does not inherit host authentication material. Tool-native login inside
one Tobari home remains the universal default, and the retained static
`managed` adapter remains compatible. The supported Auth Broker route adds one
Context-owned credential acquired on the trusted host and stores it in an
authenticated encrypted vault. Each eligible Workspace receives only a
distinct random handle bound to its stable Context, project, provider,
credential revision, and exact HTTP binding. Gateway resolves the real value
from one shared locked broker only after OPA allows the ordinary HTTP effect.

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
  audit, denial projections, and CLI output. After policy allow, the selected
  adapter forwards or injects authentication; proxy and Tobari control headers
  are not forwarded upstream.
- One shared Auth Broker joins only control and egress, never a Workspace
  network. Workspaces and OPA cannot address its runtime socket; only Gateway
  mounts that socket. Host auth commands use bounded in-container control
  operations and never expose a public broker TCP API.
- The broker starts locked and retains the installation root key only in
  memory. Cluster reconciliation unlocks it with key bytes transferred through
  stdin. Context vaults are keyed by stable Context ID and bind their version
  and Context ID as authenticated data.
- Valid schema-v1 static providers remain supported and normalize into the
  schema-v2 projection. Schema v2 adds only typed, reviewed built-in credential
  plans; owner manifests still contain no secrets, executable shell, refresh
  logic, or signer and remain single-secret protected-stdin imports.
- Provider-native executables do not enter the Auth Broker image. Reviewed
  GitHub, AWS, Datadog pup, Codex, and Claude acquisition drivers execute fixed
  argv against verified host CLI identities in private temporary homes with
  sanitized environments. The
  GitHub driver is API-authentication-only, opens exactly the fixed device page
  or leaves the same manual URL, and configures no Git protocol or credential
  helper.
  AWS selection is explicit: the backward-compatible `identity-center` method
  uses the fixed device flow, while `console` uses AWS CLI 2.32-or-newer's
  cross-device local-development login. Console mode opens only the strict
  region-bound AWS authorization URL emitted by that fixed process and leaves
  the same terminal URL as fallback. Neither method reads an ambient AWS home
  or starts a callback listener.
  Datadog runs pup's fixed US1 OAuth PKCE/DCR flow in an isolated file-backed
  home; pup alone opens the generated consent URL and owns the bounded loopback
  callback. Tobari accepts only strict default-session state and deletes the
  home on every outcome.
  OpenAI runs the pinned Codex 0.146.0 native ChatGPT device flow and accepts
  only strict managed file state. Claude runs pinned Claude Code 2.1.220
  `setup-token`, captures one inference-only OAuth token without displaying it,
  and has no refresh state. Both delete their isolated homes on every outcome.
- One resident trusted-host companion uses the current Tobari executable's
  private same-binary mode. It reaches an unmounted Broker-private socket only
  through a fixed reverse `docker exec -i` stream protected by a root-key-
  derived, direction-separated authenticated session. It opens no host or
  container listener, mounts no host socket or provider home, accepts no
  repository-selected executable/argv, and is unreachable from Workspaces,
  Gateway, and OPA. Its only provider operation in this slice is the
  post-policy AWS credential export; interactive GitHub/AWS/Datadog/OpenAI/
  Anthropic login runs directly through context-bound host drivers.
- The macOS root-key provider stores one installation key in Keychain. Linux
  uses an owner-only XDG state file and makes no host-user-compromise claim.
- A recognized malformed, copied, stale, or mismatched Tobari handle fails
  closed and is never forwarded upstream. Login and logout rotate or revoke
  every associated project handle; existing sessions receive a concrete
  re-entry action because their environment cannot change retroactively.
- Brokered login does not grant network authority. OPA remains the sole
  authority for Context, project, scheme, host, port, method, and path. A
  brokered request does not inherit a broad static host/method allow; its first
  exact L7 effect remains reviewable until the host installs one exact learned
  rule.
- Refresh and signing are permitted only for finite reviewed built-in plans.
  One is a refreshable AWS CLI session plus standard SigV4. Acquisition
  is either the reviewed IAM Identity Center device flow or AWS console-based
  local-development login: Auth Broker owns
  encrypted opaque AWS CLI state, handle/revision authority, and signing; only
  after OPA allow does the companion run the fixed host AWS credential export.
  Broker rechecks the record/revision, persists any refreshed opaque state,
  and signs the unchanged bounded request. Request region is Context/tool
  configuration, not login state. The second is Datadog's fixed US1 pup OAuth
  state: after OPA allow Broker returns a still-valid bearer or performs one
  exact proxy-free, no-redirect token refresh behind a durable barrier.
  The third is Codex 0.146.0's fixed ChatGPT OAuth state: Workspace receives a
  version-pinned external-host compatibility shim containing only a handle;
  after OPA allow Broker returns a still-valid bearer or performs one exact
  proxy-free, no-redirect OpenAI refresh behind the same durable barrier, and
  Gateway supplies Broker-owned account routing. Anthropic's fixed Claude
  setup-token plan is non-renewable and resolves only its unexpired stored
  inference token after allow.
  Arbitrary OAuth, manifest-selected helpers/
  signers, general TWG refresh, SigV4a, presigning, provider-operation
  inference, and Git credential helpers remain outside the slice.

### Mechanical enforcement

- Runtime mount tests reject host-home mounts and verify the selected default
  adapter. Gateway tests use canary secrets to prove redaction, deny-before-
  resolution, exact replacement, and client-header forwarding only after
  allow, while managed adapter tests retain binding and injection coverage.
- Integration tests prove one Tobari's tool-owned state persists through runtime
  recovery, is unavailable to another Tobari, and is removed by exact delete.
  Broker tests prove encrypted Context ownership, project-specific handles,
  restart locking, rotation, revocation, and canary-free output.
- Acquisition tests fix the GitHub, AWS, pup, Codex, and Claude host executable
  identity, version, argv,
  environment, conventional non-project installation-root selection,
  control-safe visible output, checked private-home cleanup, purpose-limited
  fixed or parameterized browser target, manual fallback, cancellation, and
  absence of Broker Git/AWS CLI configuration.
- Companion tests fix same-binary private startup, reverse-exec container
  identity, authenticated frame sequencing and bounds, no-listener/no-mount
  topology, post-policy call order, bounded per-record single-flight refresh,
  durable no-replay barriers, receipt-only cancellation acknowledgments,
  blocked-peer teardown, stale result rejection, and secret-free failures.
- Narrow-projection tests fix every allowed scalar, bounded host read, private
  re-encoding target, precedence rule, and hostile source-file/key canary; no
  projection test treats identity as authentication authority.

## Thesis 4: One shared cluster hosts multiple CWD-owned Tobari

MVP manages one installation-local enforcement cluster and multiple logical
Tobari. The shared cluster contains exactly one Gateway, one OPA, and one Auth
Broker in one host trust domain with a host-issued
Context/project-principal boundary: stable Tobari and Context IDs are not
trusted merely because they appear in caller data, but the host binds both to
the exact Gateway network interface that received the request. Context policy,
learned permissions, and managed profiles cannot cross that Context/project
binding. A Tobari is selected
from the canonical current directory: an exact indexed root is reused directly;
when only ancestor roots exist, the interactive root command presents every
containing root nearest-first and an explicit create-here option. A new nested
root is never implicit. Every selected Tobari binds exactly one read-write
root, one dedicated internal network, and one persistent XDG-owned home
directory.

### Consequences

- `cluster up` explicitly creates or reconciles the shared enforcement runtime;
  `tobari` requires that configured cluster to be ready, resolves an existing
  canonical-root record or creates one at the current directory, reconciles
  only the project runtime, and enters the work container on a terminal.
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
  the Workspace. The base runtime carries the common work tools required by
  supported agents, and each published agent image adds only its agent-specific
  tool and dependencies under the same contract.
- `tobari delete` is the only routine operation that ends a logical Tobari;
  ending a shell or losing a runtime resource leaves it existing.
- Every process in a Tobari may modify or delete every file below that Tobari's
  mounted root.
- The host-owned principal registry is the only source of project authority at
  Gateway; session headers and profile names cannot select a project.
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
  recovery at multi-file boundaries, and explicit handling of corrupt or legacy
  state.
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
- Shared Gateway, OPA, and Auth Broker services use the same fixed JSON log rotation bounds;
  a project cannot fill their host-side Docker logs without a cap.
- Shared Gateway, OPA, and Auth Broker services also carry fixed CPU, memory-plus-swap, and
  PID bounds; those limits protect the Docker VM but do not promise per-project
  fairness inside the shared service.
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
  mitmproxy's streaming path rather than full-body retention. The reviewed AWS
  SigV4 plan is the sole exception: after allow it retains one complete request
  within the same 8 MiB cap only long enough to hash and sign it, then forwards
  once. A trusted declared GraphQL endpoint is the other narrow exception: it
  requires an unambiguous positive length of at most 1 MiB, retains the request
  before policy only long enough to parse generic operation/root identity, and
  forwards the original bytes once after allow. Unknown-length ordinary bodies
  remain streaming; unsupported GraphQL and AWS streaming forms fail closed.
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
  resolver channel, source-required Gateway/Auth Broker APIs, and the APIs
  selected by that resolver. Missing source metadata or an API mismatch is
  never presented as compatible.
- `task check` is the implementation completion gate; security and public
  changes also run their named profiles.
- Docker integration is a separate explicit profile because it requires a
  working Engine, but CI runs it on supported Linux runners.

### Mechanical enforcement

- `tools/archlint`, catalog contract tests, Go unit tests, Gateway tests, OPA
  tests, and Docker integration tests cover distinct boundaries.
- Standard and `tobari_dev` build fixtures prove that published pins and local
  development tags cannot cross resolver channels, and cluster preflight
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
  method, path, decision, reason, and whether an exact learned rule can resolve
  the denial without weakening orthogonal invariants.
- A learnable denial returns a fixed, secret-free host-side review command to
  the agent; a completed session also summarizes the pending queue on host
  stderr. Neither notification can mutate policy or trigger a retry.
- Interactive `policy review` is the installation-wide human Permission Inbox
  over retained queues from every Context. Selection and detail inspection may
  stage several explicit Allow-exact or Deny-exact choices, but staging grants
  no authority and cancellation discards the whole staged set. A choice is
  accepted only from its exact detail screen. One final Apply confirmation
  binds the complete typed snapshot and applies the staged set as one
  command-owned installation policy decision-set mutation. Every opaque
  candidate ID is retained unchanged and revalidated against fresh evidence.
  A manual refresh reconciles staged choices only by candidate ID: retained
  IDs keep their decision and order, stale IDs lose Apply eligibility, and a
  matching display label never transfers authority to a replacement ID.
  Its list groups by validated stable Context/project identity, presents that
  scope once per group, and leads each selectable row with the exact HTTP
  effect plus bounded observation evidence. Display labels, adjacency, and
  indentation never create policy identity.
  Redirected and
  machine-readable `policy review` remains read-only. `policy candidates`
  remains the machine discovery surface, and `policy tail` remains a
  compatibility projection. `policy rules` is the exhaustive inventory of
  current CLI-owned learned decisions; `policy reset --id` removes exactly one
  Allow or exact Deny and returns that effect to default deny. The discovery
  queue turns only learnable retained denials into unique exact
  Context/project/host/port/method/path proposals with stable opaque IDs, the latest
  retained observation time, and a retained-window observation count; the current-rule
  inventory exposes the separate rule IDs used for reset.
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
- Repeated exact learned rules may produce a `policy compactions` proposal only
  for a fixed host, port, and method beneath a sufficiently specific path prefix.
  `policy compact --id` is a separate explicit action whose positive examples
  and boundary canaries must pass before the source rules are replaced.
- OPA watches one revisioned complete bundle mounted read-only from an exact
  owner-labeled Docker-managed volume. Exact allow, deny, reset, compaction,
  and reviewed-set actions test a private complete policy copy, atomically
  update CLI-owned data, build a revision-named archive through pinned OPA,
  atomically rename it through a fixed pinned publisher, and
  report success only after the running OPA proves the expected revision is
  active. Authority-reducing or mixed changes first confirm a complete
  deny-all transition revision and restore the prior known-good revision on
  failure. The trusted host remains the only policy writer.
- Audit evidence never includes credential values, cookies, raw bodies, or raw
  response data.
- Tobari never changes permission from observation alone. Every learned rule,
  reset, or compaction remains an explicit trusted-host mutation. Machine
  actions stay opaque-reference-bound; interactive reviewed-set Apply is one
  fixed-target mutation whose typed contents are unchanged opaque references
  selected from exact detail screens. Every candidate must pass `opa test`;
  finite examples and canaries detect declared
  regressions but do not prove safety for every unknown future request.

### Mechanical enforcement

- Gateway tests assert both useful denial dimensions and absence of secret/body
  canaries.
- Integration tests deny known requests, retrieve their typed audit records
  through the CLI, assert the structured agent navigation and host-only session
  summary, stage exact Allow and Deny choices through the human queue, apply
  them with one activation, exercise the allowed rule, compact repeated exact
  rules, and retain a denied boundary without restarting any Tobari or OPA.
- README makes the observe-review-decide-retry loop the primary operating
  workflow, keeps routine permission growth free of hand-authored OPA/Rego,
  and keeps tested host editing as the advanced escape hatch.

## Thesis 9: Every Tobari belongs to one logical Context

Users should choose one understandable execution setup, not assemble an agent
profile, runtime image, policy directory, and credential configuration from
unrelated paths.
Tobari therefore presents a named Context as the logical bundle for an agent's
configuration, network policy, and credential references. The Context manifest
is a host-owned composition record; it does not collapse the physical trust
boundaries between read-only agent data, OPA policy, and Gateway-only secret
stores. Each Context has a stable opaque identity; its name is a human selector,
not authority. Each Tobari permanently records one Context identity, and the
host derives that binding for Gateway and OPA from its network principal.

The installation runs one shared Gateway, one shared OPA, and one shared locked
Auth Broker for every Context.
The current Context is only the default when a host invocation omits a Context;
changing it cannot migrate or mutate existing Tobari or shared enforcement.
Tool-native authentication state remains below each Workspace home and is not a
Context secret. A brokered credential is owned once by the stable Context and
enables every permanently bound Workspace to receive a different project-bound
handle on its next reconciliation. The default passthrough adapter remains the
universal path for tools that own their login flow; neither a provider name nor
a handle selects authority without the trusted principal and OPA allow.

### Consequences

- `context list`, `context show`, `context create`, and `context use` are the
  host-facing composition surface. `context use` changes only the omitted-
  Context default. `tobari --context NAME` chooses an invocation Context
  without changing that default.
- `config shell` and `config git` own the Context's narrow non-secret host
  projections. A complete setting group is deterministic for agents and
  scripts; wholly omitted setting flags open a terminal-only staged editor.
  Shell presents the complete fixed inventory and commits every distinct
  staged row through one atomic Apply; Git presents its complete source choice
  and uses the same stage/Apply vocabulary. Partial, redirected, and JSON
  wizard attempts fail before mutation. `config shell` retains only `PS1`, `TERM`,
  `COLORTERM`, and `NO_COLOR`; new Contexts and Contexts migrated from schemas
  1–3 inherit exported `PS1`, while schema-4 migration preserves its existing
  shell policy. An absent export retains Tobari's built-in prompt. `config git`
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
- Context creation initializes separate owner-only policy and credential
  stores, references a read-only agent profile, and records the compatible
  Tobari runtime image. It never accepts a secret value in an argument,
  environment variable, or manifest.
- Auth login/import affects one explicit or current Context and makes the
  Context-wide Workspace eligibility explicit. Login does not rewrite running
  Workspaces; their next matching entry issues or refreshes project-bound
  handles and recreates only a changed work container while preserving home.
- Context source changes become active only through an explicit `cluster up` or
  policy mutation. The host generates and validates one atomic projection of
  every Context for the shared OPA and Gateway; a failed candidate preserves
  the complete prior known-good projection.
- The ordinary guided mode keeps deny/review/allow exact permission growth as
  the default. Advanced mode keeps trusted-host Rego and tests available for
  policy that cannot be expressed as an exact learned rule. The projection
  namespaces Advanced modules and prevents them from claiming the Tobari-owned
  router or system packages.
- Permission candidates, learned rules, exact denies, compactions, audits, and
  managed credentials retain Context and Tobari identity. `policy review` and
  `policy rules` cross all Contexts; mutations bind solely to opaque references.

### Mechanical enforcement

- Context domain and catalog tests validate stable identity, modes, current-
  default selection, effects, fixed targets, and complete output/error contracts.
- Configuration tests validate the all-or-none direct/staged-editor state machine,
  terminal cancellation and explicit-empty Context rejection with zero
  mutation, binding of Apply to the Context shown across concurrent default
  changes, one atomic multi-row shell write, fixed shell and Git inventories,
  schema migration, bounded host Git
  calls with an exact child-environment allowlist, lower-precedence read-only
  projection, and exclusion of authentication and executable Git settings.
- Infrastructure tests prove legacy default migration, owner-only separate
  stores, permanent Tobari bindings, aggregate read-only OPA mounts, and
  selected agent-profile digests.
- Runtime tests prove the recipe build context excludes policy and credential
  stores, the generated image is checked against the runtime contract, and a
  failed build leaves the previously selected image unchanged. Project runtime
  tests prove existing Workspaces reconcile to their bound Context image only
  after validation and preserve their home.
- Runtime, Gateway, OPA, Auth Broker, and integration tests prove Context secrets and learned
  permissions do not cross Context/project principals, aggregate activation is
  all-or-nothing, and forged or stale Context bindings fail closed.
- Agent-readiness validation records current-default discovery, explicit
  invocation selection, and installation-wide permission review.

## Deliberate non-goals

MVP does not support multiple clusters, process-level identity, a per-project
static baseline policy, transparent proxying, non-HTTP protocols, Git SSH,
provider-specific policy semantics, Git-over-HTTPS credential helpers,
arbitrary provider OAuth or signing, SigV4a, AWS presigning or streaming,
multiple provider accounts per Context, approval workflows, general Kubernetes
authentication or transport, filesystem overlays, GUIs, remote execution, or
multi-tenant production use.
