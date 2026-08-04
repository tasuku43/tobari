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
authorized as a normalized request by one shared OPA-backed Gateway.

The primary users are developers who run Claude Code, Codex, shells, tests, and
other arbitrary programs against project roots. Success has two inseparable
parts: each Tobari can host concurrent processes, cannot reach the Internet or
another Tobari directly, never receives host-managed credentials, and is
selected from the canonical current directory rather than a user-managed name,
root flag, or container identifier; and the user can reach that boundary
without becoming a Docker or policy operator. A user may deliberately create
tool-owned authentication state in that Tobari's own persistent home; this is
not host credential inheritance. One installation-local cluster shares
Gateway, OPA, an installation-wide baseline policy, and CA state without
sharing Tobari roots or homes. Host-issued project principals bind mutable
learned permissions to the exact current-directory network that originated a
request; they do not select credentials.

The first testable slice is one local mock upstream reached through the
Gateway: an allowed request succeeds, a denied request does not reach upstream,
direct egress fails, and an OPA outage fails closed.

The core product loop is progressive policy learning: work freely in Tobari,
observe a denied boundary effect as secret-free evidence, receive a fixed
host-side review cue, review the pending exact permission, approve the minimum
rule, and retry. The normal path does not require writing OPA or Rego by hand:
interactive `policy review` presents a Permission Inbox and delegates one
explicitly confirmed candidate to `policy allow` or `policy deny`; its
non-interactive and machine-readable path remains read-only. Exact actions
test and activate their private policy copy without restarting active Tobari.
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
  review-to-decision flow; it must not be paired with a second JSON review.
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
  customizable through the deny-review-allow-retry loop without direct egress.

## Thesis 1: Authorize effects at the isolation boundary

Tobari authorizes the HTTP request that actually attempts to leave a Tobari.
It does not infer intent from a command name, process name, shell text, or agent
brand.

### Consequences

- Any process that honors the explicit proxy receives the same enforcement.
- Gateway sends generic HTTP attributes to OPA rather than service-specific
  operations.
- The initialized policy denies non-empty request bodies unless a trusted host
  authors a body-aware Rego rule; body-bearing observations never become
  host-only approval candidates.
- A body that is unavailable because it was not captured is denied before OPA;
  it is never treated as an explicit empty body or a learning candidate.
- Provider-specific adapters are not part of the MVP. GitHub, AWS, Claude, and
  other tools use their own native flows through the generic boundary.
- HTTP methods are evidence supplied to policy, not a CLI effect classifier.

### Mechanical enforcement

- Gateway unit tests fix the OPA input schema and secret-header redaction.
- Rego tests exercise host, port, method, path, scheme, body-empty, and
  project-principal boundaries.
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

## Thesis 3: Authentication handling is pluggable and tool-owned by default

Tobari does not inherit host authentication material. A user may run a tool's
normal login or configuration flow inside a Tobari, and the tool may write its
credential state below that Tobari's exact persistent home. Gateway credential
handling is selected through an explicit adapter boundary. The default
`passthrough` adapter does not load or inject managed profiles; it continues to
authorize the generic HTTP/HTTPS effect and forwards tool/client authentication
only after allow. The existing `managed` profile-injection adapter remains
available for a later trusted runtime switch.

### Consequences

- Host home, host CLI configuration, keychains, SSH agents, and credential
  environment variables are never mounted or copied into Tobari.
- Tool-owned credential state is available to every process in the same
  Tobari by design, survives runtime-container recreation, and is removed by
  the explicit Tobari delete operation.
- Client authentication and cookie values are redacted from OPA input, Gateway
  audit, denial projections, and CLI output. After policy allow, the selected
  adapter forwards or injects authentication; proxy and Tobari control headers
  are not forwarded upstream.
- Gateway-managed profile selection is not the default. Its existing static
  injection adapter remains reserved and keeps its host/project/host binding
  checks. Refresh, signing, and OS-keychain integration remain outside the
  product boundary. A tool may implement its own native OAuth, SigV4, or
  keychain-compatible flow inside the home without Tobari interpreting it.

### Mechanical enforcement

- Runtime mount tests reject host-home mounts and verify the selected default
  adapter. Gateway tests use canary secrets to prove redaction and client-header
  forwarding only after allow, while managed adapter tests retain binding and
  injection coverage.
- Integration tests prove one Tobari's tool-owned state persists through runtime
  recovery, is unavailable to another Tobari, and is removed by exact delete.

## Thesis 4: One shared cluster hosts multiple CWD-owned Tobari

MVP manages one installation-local enforcement cluster and multiple logical
Tobari. The shared cluster is one host trust domain with a host-issued
project-principal boundary: a stable Tobari ID is not trusted merely because
it appears in caller data, but the host binds it to the exact Gateway network
interface that received the request. The initialized policy is an
installation-wide baseline; learned permissions and managed profiles are
project-bound and cannot be selected by another project. A Tobari is selected
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
  selected compatible runtime image, profile, XDG home, and diagnostic runtime
  identifiers. Container or network loss never changes logical existence.
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
- A canonical root is a unique Workspace key. Repeated or concurrent explicit
  creation at the same canonical root must yield one logical record and a
  typed already-exists outcome for losing callers.
- The active Context is the only runtime-image authority for new Workspaces;
  project metadata does not silently override the execution boundary.
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
- Multiple clusters, overlays, clone modes, and change approval are non-goals.
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
  override the execution boundary before Docker mutation.

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
  `--purge` affects only shared CA state after the cluster is empty.
- Docker CLI is behind an infrastructure port so another engine can replace it
  later without changing application outcomes.
- The project runtime spec hash includes the fixed resource contract, so an old
  or drifted container is recreated before reuse.
- The project runtime spec hash includes the fixed Workspace lifetime command,
  and image compatibility is rejected before project runtime resources are
  mutated.
- Shared Gateway and OPA services use the same fixed JSON log rotation bounds;
  a project cannot fill their host-side Docker logs without a cap.
- Shared Gateway and OPA services also carry fixed CPU, memory-plus-swap, and
  PID bounds; those limits protect the Docker VM but do not promise per-project
  fairness inside the shared service.
- `status`, `list`, and `doctor` never reconcile Docker or create/delete
  runtime resources. They may perform bounded journal cleanup before selecting
  logical state so an interrupted multi-file write cannot remain authoritative.
  The root command is the single deliberate ensure-and-enter runtime
  reconciliation path.

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

Unknown configuration, malformed OPA responses, timeouts, body inspection
overflow, and unclassified Gateway errors do not authorize traffic.

### Consequences

- Default policy denies.
- OPA and upstream calls have finite timeouts and body inspection has a fixed
  maximum; mitmproxy rejects request and response bodies above the fixed 8 MiB
  transport cap before the Gateway addon can forward them.
- JSON bodies are inspected only when fully captured within the limit;
  non-JSON bodies expose metadata only.
- Audit logs contain metadata and hashes, never raw bodies or secret values.

### Mechanical enforcement

- Gateway unit tests cover timeout, invalid decision, unavailable bodies,
  truncation, and body-type cases. Runtime-asset tests keep the transport body
  cap present, and integration sends an over-limit body to prove it stops at
  Gateway.
- Rego tests start from deny and add explicit allow rules.
- Structured logs are scanned for secret and body canaries.

## Thesis 7: Claims must be executable

Documentation is complete only when the same boundary is covered by a type,
test, lint, policy test, or integration scenario.

### Consequences

- `cli.Catalog` remains the public command source of truth.
- The four-layer dependency direction remains in force.
- `task check` is the implementation completion gate; security and public
  changes also run their named profiles.
- Docker integration is a separate explicit profile because it requires a
  working Engine, but CI runs it on supported Linux runners.

### Mechanical enforcement

- `tools/archlint`, catalog contract tests, Go unit tests, Gateway tests, OPA
  tests, and Docker integration tests cover distinct boundaries.
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
- Interactive `policy review` is the ordinary human Permission Inbox over the
  retained queue: selection, detail inspection, and explicit confirmation can
  delegate exactly one opaque candidate to `policy allow` or `policy deny`.
  Redirected and
  machine-readable `policy review` remains read-only. `policy candidates`
  remains the machine discovery surface, and `policy tail` remains a
  compatibility projection. All three turn only learnable retained denials
  into unique exact host/port/method/path proposals with stable opaque IDs.
- `tobari cluster denials` remains the lower-level diagnostic step: it projects
  validated denial records, reports the editable host-side policy directory,
  and points to the Permission Inbox. Raw component logs remain available.
- `policy allow --id` and `policy deny --id` are the explicit trusted-host
  actions that consume one candidate ID unchanged. Allow preflights and
  atomically records one exact learned rule; deny records one exact
  project-bound terminal rule. Both activate through the same portable OPA
  boundary, and an exact deny wins over a learned allow for the same request.
- Repeated exact learned rules may produce a `policy compactions` proposal only
  for a fixed host, port, and method beneath a sufficiently specific path prefix.
  `policy compact --id` is a separate explicit action whose positive examples
  and boundary canaries must pass before the source rules are replaced.
- OPA watches the policy directory mounted read-only from XDG. Exact allow,
  deny, and compaction actions test a private complete policy copy, atomically
  update CLI-owned data, and activate only the exact owned OPA component. The
  trusted host remains the only policy writer.
- Audit evidence never includes credential values, cookies, raw bodies, or raw
  response data.
- Tobari never changes permission from observation alone. Every learned rule or
  compaction remains an explicit opaque-reference-bound trusted-host action;
  interactive review is only a confirmation UI for that action and it must
  pass `opa test`; finite examples and canaries detect declared
  regressions but do not prove safety for every unknown future request.

### Mechanical enforcement

- Gateway tests assert both useful denial dimensions and absence of secret/body
  canaries.
- Integration tests deny a known request, retrieve its typed audit record
  through the CLI, assert the structured agent navigation and host-only session
  summary, review exact candidates through the human queue, allow one and deny
  one, exercise the allowed rule, compact repeated exact rules, and retain a
  denied boundary without restarting any Tobari.
- README makes the observe-review-decide-retry loop the primary operating
  workflow, keeps routine permission growth free of hand-authored OPA/Rego,
  and keeps tested host editing as the advanced escape hatch.

## Thesis 9: One logical Context composes the execution boundary

Users should choose one understandable execution setup, not assemble an agent
profile, runtime image, policy directory, and credential configuration from
unrelated paths.
Tobari therefore presents a named Context as the logical bundle for an agent's
configuration, network policy, and credential references. The Context manifest
is a host-owned composition record; it does not collapse the physical trust
boundaries between read-only agent data, OPA policy, and Gateway-only secret
stores. The active Context is selected by a trusted host operation and cannot be
selected by an agent inside a Tobari.

The first shared-cluster slice has one active Context for the installation.
Per-Workspace Context routing is deferred until the policy-routing and
project-principal consequences are explicit. Tool-native authentication state
remains below each Workspace home and is not a Context secret. The default
passthrough adapter remains the universal path for tools that own their login
flow; a Context may reference managed credential metadata without making a
profile name an authority.

### Consequences

- `context list`, `context show`, `context create`, and `context use` are the
  host-facing composition surface. Existing `policy` commands operate on the
  active Context's policy store.
- `runtime init` and `runtime build` are the host-facing runtime customization
  surface. The active Context owns one fixed `runtime/Dockerfile`; build
  validates the resulting image and promotes it into that Context without
  requiring a second image-selection command.
- Context creation initializes separate owner-only policy and credential
  stores, references a read-only agent profile, and records the compatible
  Tobari runtime image. It never accepts a secret value in an argument,
  environment variable, or manifest.
- A Context switch is an explicit configuration mutation. It does not silently
  restart Docker; a running cluster using another Context fails closed until
  the user runs the declared reconciliation action.
- The ordinary guided mode keeps deny/review/allow exact permission growth as
  the default. Advanced mode keeps trusted-host Rego and tests available for
  policy that cannot be expressed as an exact learned rule.
- A future per-Workspace Context model must add explicit Gateway/OPA routing,
  learned-rule scope, credential binding, and migration decisions; it cannot be
  introduced by adding a field to project state alone.

### Mechanical enforcement

- Context domain and catalog tests validate the manifest, modes, active
  selection, effects, fixed targets, and complete output/error contracts.
- Infrastructure tests prove legacy default migration, owner-only separate
  stores, active-context state, read-only OPA mounts, and selected agent-profile
  digests.
- Runtime tests prove the recipe build context excludes policy and credential
  stores, the generated image is checked against the runtime contract, and a
  failed build leaves the previously selected image unchanged.
- Runtime tests prove Context secrets are never mounted into a Workspace and
  that active-context drift blocks access until explicit reconciliation.
- Agent-readiness validation records Context discovery and selection as part of
  the bounded-autonomy setup path.

## Deliberate non-goals

MVP does not support multiple clusters, process-level identity, a per-project
static baseline policy, transparent proxying, non-HTTP protocols, Git
SSH, provider-specific semantic adapters,
Gateway-managed OAuth refresh, Gateway-managed SigV4, Gateway-managed Keychain
integration, approval workflows, Kubernetes,
filesystem overlays, GUIs, remote execution, or multi-tenant production use.
