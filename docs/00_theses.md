# Project Theses

These theses govern Tobari. When implementation pressure conflicts with them,
revise the thesis and its enforcement explicitly instead of adding a bypass.

## North Star

**A developer can attach named, long-lived Docker isolation spaces to the
directories where work already happens while every HTTP and HTTPS effect that
crosses each boundary is denied by default and authorized as a normalized
request by one shared OPA-backed Gateway.**

The primary users are developers who run Claude Code, Codex, shells, tests, and
other arbitrary programs against selected source roots. Success means each
named Tobari can host concurrent processes, cannot reach the Internet or another
Tobari directly, never receives managed credentials, and can be attached,
inspected, entered, and removed independently through one predictable CLI. One
installation-local cluster shares Gateway, OPA, policy, credentials, and CA
state without sharing Tobari roots or home volumes.

The first testable slice is one local mock upstream reached through the
Gateway: an allowed request succeeds, a denied request does not reach upstream,
direct egress fails, and an OPA outage fails closed.

The core product loop is progressive policy learning: work freely in Tobari,
observe a denied boundary effect as secret-free evidence, refine the host-side
XDG policy, test it, and retry. OPA watches that read-only host policy bind, so
valid edits take effect without restarting active Tobari. A useful denial
shortens that loop without silently expanding authority.

## Thesis 1: Authorize effects at the isolation boundary

Tobari authorizes the HTTP request that actually attempts to leave the realm.
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

## Thesis 3: Secrets enter only after authorization

Realm never receives a Gateway-managed secret. OPA may select a credential
profile only after allowing a request, and Gateway injects it only when the
profile is configured for the normalized destination host.

### Consequences

- Tokens are supplied through host files mounted read-only into Gateway.
- CLI arguments, Realm environment, Realm files, OPA input, and audit logs
  never contain token values.
- Realm-supplied authorization and other configured secret headers are removed
  before policy evaluation and upstream forwarding.
- The MVP supports only static bearer or fixed-header injection; OAuth,
  refresh, signing, and OS keychains are excluded.

### Mechanical enforcement

- Credential configuration validation binds every profile to explicit hosts.
- Gateway tests use canary secrets and scan decisions and audit logs.
- Integration tests prove injection at an allowed host and non-disclosure to
  Realm or another host.

## Thesis 4: One shared cluster hosts multiple named Tobari

MVP manages one installation-local enforcement cluster and multiple named
Tobari. Each Tobari binds exactly one explicitly selected read-write root, one
dedicated internal network, and one persistent home volume.

### Consequences

- `tobari cluster up` starts shared policy enforcement without mounting a work
  root.
- `tobari attach --name --root` persists a unique human-readable name and
  canonical root, and `list` emits an opaque ID for later actions.
- `shell`, `exec`, `logs`, and `detach` consume that opaque ID unchanged rather
  than rediscovering by name.
- Every process in a Tobari may modify or delete every file below that Tobari's
  mounted root.
- Multiple clusters, overlays, clone modes, and change approval are non-goals.

### Mechanical enforcement

- State and Docker labels identify the one installation-owned cluster and each
  exact named Tobari resource.
- Path tests require `--cwd` to be inside the selected Tobari root.
- Reference tests pass `list` IDs byte-for-byte into actions.
- Lifecycle integration tests create multiple roots, prove network separation,
  execute concurrent sessions, and repeat attach/detach without growing owned
  resources.

## Thesis 5: Lifecycle changes are bounded and ownership-labeled

Tobari manages only resources with its exact installation labels. Reads and
mutations are explicit catalog operations, and destructive cleanup never uses a
name prefix or broad Docker query as authority.

### Consequences

- Cluster and attach reconciliation are idempotent within declared state.
- `detach` removes one exact container and network but preserves that Tobari's
  persistent home volume unless `--purge` is explicit.
- `cluster down` refuses to remove shared enforcement while any Tobari remains;
  `--purge` affects only shared CA state after the cluster is empty.
- Docker CLI is behind an infrastructure port so another engine can replace it
  later without changing application outcomes.
- `cluster status`, `list`, `logs`, and `doctor` never repair state implicitly.

### Mechanical enforcement

- Domain resource specifications carry a fixed ownership label.
- Application tests prove validation and reference binding precede Docker calls
  and cleanup selects exact resources.
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
  method, path, decision, and reason.
- `tobari cluster logs --component gateway` is the first diagnostic step and
  `cluster status` reports the editable host-side policy directory.
- OPA watches the policy directory mounted read-only from XDG; the trusted host
  remains the only policy writer.
- Audit evidence never includes credential values, cookies, raw bodies, or raw
  response data.
- MVP does not automatically turn observed traffic into permission. Policy
  edits remain an explicit trusted-host action and must pass `opa test` before
  `up` reconciles the runtime.

### Mechanical enforcement

- Gateway tests assert both useful denial dimensions and absence of secret/body
  canaries.
- Integration tests deny a known request, retrieve its audit record through the
  CLI, edit policy on the host, and then exercise an allowed rule without
  restarting the cluster.
- README makes the observe-edit-test-retry loop the primary operating workflow.

## Deliberate non-goals

MVP does not support multiple clusters, process-level identity, transparent
proxying, non-HTTP protocols, Git SSH, provider-specific semantic adapters,
OAuth refresh, SigV4, Keychain integration, approval workflows, Kubernetes,
filesystem overlays, GUIs, remote execution, or multi-tenant production use.
