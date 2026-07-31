# Project Theses

These theses govern Tobari. When implementation pressure conflicts with them,
revise the thesis and its enforcement explicitly instead of adding a bypass.

## North Star

**A developer can run `tobari` in a project directory to create or reuse its
long-lived isolated environment while every HTTP and HTTPS effect that crosses
the boundary is denied by default and authorized as a normalized request by one
shared OPA-backed Gateway.**

The primary users are developers who run Claude Code, Codex, shells, tests, and
other arbitrary programs against project roots. Success means each Tobari can
host concurrent processes, cannot reach the Internet or another Tobari directly,
never receives managed credentials, and is selected from the canonical current
directory rather than a user-managed name, root flag, or container identifier.
One installation-local cluster shares Gateway, OPA, policy, credentials, and CA
state without sharing Tobari roots or homes.

The first testable slice is one local mock upstream reached through the
Gateway: an allowed request succeeds, a denied request does not reach upstream,
direct egress fails, and an OPA outage fails closed.

The core product loop is progressive policy learning: work freely in Tobari,
observe a denied boundary effect as secret-free evidence, refine the host-side
XDG policy, test and activate it, and retry. OPA watches that read-only host
policy bind where Docker-host filesystem events propagate; `policy apply` is
the deterministic portable activation path and does not restart active Tobari.
A useful denial shortens that loop without silently expanding authority.

## Thesis 1: Authorize effects at the isolation boundary

Tobari authorizes the HTTP request that actually attempts to leave a Tobari.
It does not infer intent from a command name, process name, shell text, or agent
brand.

### Consequences

- Any process that honors the explicit proxy receives the same enforcement.
- Gateway sends generic HTTP attributes to OPA rather than service-specific
  operations.
- GitHub, AWS, and other provider adapters are not part of the MVP.
- HTTP methods are evidence supplied to policy, not a CLI effect classifier.

### Mechanical enforcement

- Gateway unit tests fix the OPA input schema and secret-header redaction.
- Rego tests exercise host, method, path, scheme, and credential bindings.
- Docker integration tests use curl and Python rather than a named coding agent.

## Thesis 2: Network topology is an enforcement mechanism

Each Tobari has exactly one internal Docker network path: the Gateway proxy
interface. Gateway alone joins every Tobari network plus the shared control and
egress networks; OPA joins only control.

### Consequences

- Proxy bypass has no route to an external upstream.
- Tobari cannot reach OPA, Gateway management interfaces, or another Tobari.
- Gateway or OPA failure denies outbound traffic.
- Host networking, privileged mode, Docker socket mounts, and added
  capabilities are forbidden.

### Mechanical enforcement

- The runtime adapter constructs one labeled internal network per Tobari, a
  shared internal control network, and a separate egress network.
- Integration tests prove direct egress, direct OPA access, and traffic during
  Gateway or OPA failure do not succeed.
- Runtime specification tests reject privileged mode, host networking, Docker
  socket mounts, and broad host-home mounts.
- Image-selection tests require a locally available runtime-API-compatible
  image before any per-Tobari resource is created; image configuration cannot
  replace the CLI-owned isolation arguments.

## Thesis 3: Secrets enter only after authorization

Tobari never receives a Gateway-managed secret. OPA may select a credential
profile only after allowing a request, and Gateway injects it only when the
profile is configured for the normalized destination host.

### Consequences

- Tokens are supplied through host files mounted read-only into Gateway.
- CLI arguments, Tobari environment, Tobari files, OPA input, and audit logs
  never contain token values.
- Tobari-supplied authorization and other configured secret headers are removed
  before policy evaluation and upstream forwarding.
- The MVP supports only static bearer or fixed-header injection; OAuth,
  refresh, signing, and OS keychains are excluded.

### Mechanical enforcement

- Credential configuration validation binds every profile to explicit hosts.
- Gateway tests use canary secrets and scan decisions and audit logs.
- Integration tests prove injection at an allowed host and non-disclosure to
  Tobari or another host.

## Thesis 4: One shared cluster hosts multiple CWD-owned Tobari

MVP manages one installation-local enforcement cluster and multiple logical
Tobari. A Tobari is selected by the nearest indexed canonical ancestor of the
current directory, binds exactly one read-write root, one dedicated internal
network, and one persistent XDG-owned home directory.

### Consequences

- `cluster up` explicitly creates or reconciles the shared enforcement runtime;
  `tobari` requires that configured cluster to be ready, resolves an existing
  canonical-root record or creates one at the current directory, reconciles
  only the project runtime, and enters the work container on a terminal.
- The logical record owns a generated stable internal ID, canonical root,
  selected compatible runtime image, profile, XDG home, and diagnostic runtime
  identifiers. Container or network loss never changes logical existence.
- `status` and `delete` resolve the same nearest canonical root. `list` shows
  local roots and diagnostics without turning an ID into a normal user input.
- An explicit in-root image-based `devcontainer.json` may select that image,
  but cannot delegate mounts, privileges, environment, lifecycle, or
  orchestration to another tool.
- `tobari delete` is the only routine operation that ends a logical Tobari;
  ending a shell or losing a runtime resource leaves it existing.
- Every process in a Tobari may modify or delete every file below that Tobari's
  mounted root.
- Multiple clusters, overlays, clone modes, and change approval are non-goals.
- A project root cannot be the filesystem root, the user's home or its
  ancestor, or any XDG configuration, state, or shared-profile management
  path. A policy source repository remains an ordinary allowed project root;
  active policy is published separately by explicit trusted-host operations.

### Mechanical enforcement

- Root-index and instance-state tests prove canonical path resolution, nearest
  ancestor selection, stable IDs, atomic state updates, and explicit handling
  of corrupt or legacy state.
- State and Docker labels identify the one installation-owned cluster and each
  exact logical Tobari resource.
- Lifecycle integration tests create multiple roots, prove network separation,
  enter and recover the same root repeatedly, and delete only the selected
  instance without growing owned resources.
- Dev Container tests accept bounded JSONC image metadata and reject every
  unsupported runtime property before Docker mutation.

## Thesis 5: Lifecycle changes are bounded and ownership-labeled

Tobari manages only resources with its exact installation labels. Reads and
mutations are explicit catalog operations, and destructive cleanup never uses a
name prefix or broad Docker query as authority.

### Consequences

- Root resolution, logical creation, runtime reconciliation, and deletion are
  idempotent within declared state. Concurrent mutations serialize through the
  XDG state lock and atomic durable records.
- `delete` removes one exact container, network, home directory, and instance
  records after an explicit destructive confirmation. It can continue after
  partial runtime cleanup and never selects by a Docker name or prefix.
- `cluster down` refuses to remove shared enforcement while any Tobari remains;
  `--purge` affects only shared CA state after the cluster is empty.
- Docker CLI is behind an infrastructure port so another engine can replace it
  later without changing application outcomes.
- `status`, `list`, and `doctor` never repair state implicitly. The root
  command is the single deliberate ensure-and-enter reconciliation path.

### Mechanical enforcement

- Domain resource specifications carry a fixed ownership label.
- Application tests prove validation and CWD-local target resolution precede
  Docker calls and cleanup selects exact resources.
- The catalog declares every read/create/write effect and mutation impact.

## Thesis 6: Fail closed with bounded evidence

Unknown configuration, malformed OPA responses, timeouts, body inspection
overflow, and unclassified Gateway errors do not authorize traffic.

### Consequences

- Default policy denies.
- OPA and upstream calls have finite timeouts and body inspection has a fixed
  maximum.
- JSON bodies are inspected only when fully captured within the limit;
  non-JSON bodies expose metadata only.
- Audit logs contain metadata and hashes, never raw bodies or secret values.

### Mechanical enforcement

- Gateway unit tests cover timeout, invalid decision, truncation, and body-type
  cases.
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
rule from the trusted host.

### Consequences

- Every HTTP denial emits bounded structured audit metadata including host,
  method, path, decision, reason, and whether an exact learned rule can resolve
  the denial without weakening orthogonal invariants.
- `tobari cluster denials` is the first diagnostic step: it projects only
  validated denial records, reports the editable host-side policy directory,
  and names the exact activation command. Raw component logs remain available.
- `policy candidates` turns only learnable retained denials into unique exact
  host/method/path proposals with stable opaque IDs. `policy tail` is the
  bounded human review of that same queue. Neither command changes authority.
- `policy allow --id` is the explicit trusted-host action that consumes one
  candidate ID unchanged, preflights and atomically records one exact learned
  rule, then activates it through the same portable OPA boundary.
- Repeated exact learned rules may produce a `policy compactions` proposal only
  for a fixed host and method beneath a sufficiently specific path prefix.
  `policy compact --id` is a separate explicit action whose positive examples
  and boundary canaries must pass before the source rules are replaced.
- OPA watches the policy directory mounted read-only from XDG; `policy apply`
  tests the same directory, recreates only exact owned OPA, and waits for
  health when Docker-host filesystem events are unavailable. The trusted host
  remains the only policy writer.
- Audit evidence never includes credential values, cookies, raw bodies, or raw
  response data.
- Tobari never changes permission from observation alone. Every learned rule or
  compaction remains an explicit opaque-reference-bound trusted-host action and
  must pass `opa test`; finite examples and canaries detect declared
  regressions but do not prove safety for every unknown future request.

### Mechanical enforcement

- Gateway tests assert both useful denial dimensions and absence of secret/body
  canaries.
- Integration tests deny a known request, retrieve its typed audit record
  through the CLI, approve the exact candidate, exercise the allowed rule,
  compact repeated exact rules, and retain a denied boundary without restarting
  any Tobari.
- README makes the observe-review-approve-retry loop the primary operating
  workflow and keeps tested host editing as the advanced escape hatch.

## Deliberate non-goals

MVP does not support multiple clusters, process-level identity, transparent
proxying, non-HTTP protocols, Git SSH, provider-specific semantic adapters,
OAuth refresh, SigV4, Keychain integration, approval workflows, Kubernetes,
filesystem overlays, GUIs, remote execution, or multi-tenant production use.
