# Project Theses

These theses govern Tobari. When implementation pressure conflicts with them,
revise the thesis and its enforcement explicitly instead of adding a bypass.

## North Star

**A developer can run an untrusted coding agent with broad freedom inside one
long-lived Docker realm while every HTTP and HTTPS effect that crosses the realm
boundary is denied by default and authorized as a normalized request by OPA.**

The primary users are developers who run Claude Code, Codex, shells, tests, and
other arbitrary programs against a shared source root. Success means the same
realm can host concurrent processes, cannot directly reach the Internet, never
receives managed credentials, and can be created, inspected, entered, and
removed through one predictable CLI.

The first testable slice is one local mock upstream reached through the
Gateway: an allowed request succeeds, a denied request does not reach upstream,
direct egress fails, and an OPA outage fails closed.

The core product loop is progressive policy learning: work freely in Realm,
observe a denied boundary effect as secret-free evidence, refine the host-side
policy, test it, and retry. A useful denial shortens that loop without silently
expanding authority.

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

Realm has exactly one internal Docker network path: the Gateway proxy
interface. Gateway alone joins realm, control, and egress networks; OPA joins
only control.

### Consequences

- Proxy bypass has no route to an external upstream.
- Realm cannot reach OPA or Gateway management interfaces.
- Gateway or OPA failure denies outbound traffic.
- Host networking, privileged mode, Docker socket mounts, and added
  capabilities are forbidden.

### Mechanical enforcement

- The runtime adapter constructs labeled internal realm and control networks and
  a separate egress network.
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

## Thesis 4: One root owns one long-lived realm

MVP manages a single shared realm for one explicitly selected read-write root.
Repository-level and process-level isolation would imply safety boundaries that
the MVP does not provide.

### Consequences

- `tobari up --root` persists one canonical root and rejects conflicting roots
  while the realm exists.
- `shell` and `exec` join the existing container, filesystem, home volume, and
  network namespace.
- Every process in Realm may modify or delete every file below the mounted root.
- Multiple realms, overlays, clone modes, and change approval are non-goals.

### Mechanical enforcement

- State and Docker labels identify the one installation-owned realm.
- Path tests require `--cwd` to be inside the configured root.
- Lifecycle integration tests execute concurrent sessions and repeated
  `up`/`down` cycles without growing owned resources.

## Thesis 5: Lifecycle changes are bounded and ownership-labeled

Tobari manages only resources with its exact installation labels. Reads and
mutations are explicit catalog operations, and destructive cleanup never uses a
name prefix or broad Docker query as authority.

### Consequences

- `up` and `down` are idempotent within the declared state.
- `down` removes owned containers and transient networks but preserves the
  persistent Realm home volume unless `--purge` is explicit.
- Docker CLI is behind an infrastructure port so another engine can replace it
  later without changing application outcomes.
- `status`, `logs`, and `doctor` never repair state implicitly.

### Mechanical enforcement

- Domain resource specifications carry a fixed ownership label.
- Application tests prove validation precedes Docker calls and cleanup selects
  exact resources.
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
- `tobari logs --component gateway` is the first diagnostic step and `status`
  reports the editable host-side policy directory.
- Audit evidence never includes credential values, cookies, raw bodies, or raw
  response data.
- MVP does not automatically turn observed traffic into permission. Policy
  edits remain an explicit trusted-host action and must pass `opa test` before
  `up` reconciles the runtime.

### Mechanical enforcement

- Gateway tests assert both useful denial dimensions and absence of secret/body
  canaries.
- Integration tests deny a known request, retrieve its audit record through the
  CLI, and then exercise an allowed rule.
- README makes the observe-edit-test-retry loop the primary operating workflow.

## Deliberate non-goals

MVP does not support multiple realms, repository/process isolation, transparent
proxying, non-HTTP protocols, Git SSH, provider-specific semantic adapters,
OAuth refresh, SigV4, Keychain integration, approval workflows, Kubernetes,
filesystem overlays, GUIs, remote execution, or multi-tenant production use.
