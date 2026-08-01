# Tobari Threat Model

## Scope

This model covers the local MVP: one shared Gateway and OPA cluster, multiple
CWD-owned Tobari, one selected read-write root and home per Tobari, and
optional static credential injection.

## Trust classification

Trusted:

- host OS and Docker Engine or its Linux VM;
- Tobari CLI and embedded runtime assets;
- Gateway, OPA, Rego policy, and host credential storage.

Untrusted:

- every CWD-owned Tobari and its selected root;
- coding agents, shells, generated or downloaded code, packages, and processes;
- request content and upstream responses.

## Assets and boundaries

| Asset | Threat | Boundary | Enforcement |
|---|---|---|---|
| Host files outside a selected root | Tobari reads or modifies them | Docker mounts | Only that canonical root and exact home are writable |
| Another Tobari | A process crosses isolation spaces | Docker networks | One dedicated internal network per Tobari |
| Docker Engine | Tobari controls host containers | Unix socket/process | Socket and host process interfaces are not mounted |
| Direct Internet access | Agent bypasses policy | Docker network | Each Tobari network is internal and has only Gateway |
| OPA policy/admin API | Agent changes or queries policy | Control network | OPA joins only control; Tobari do not |
| Host policy integrity | OPA rewrites trusted policy | Bind mount | XDG policy is read-only inside OPA |
| Managed secrets | Agent reads or exfiltrates them | Gateway mount | Secret files mount read-only only into Gateway |
| Authorization integrity | Policy sees a different request | Gateway buffer | One normalized buffered request is inspected and forwarded |
| Authority binding | An approved path is replayed on another port, scheme, or unsafe resolved address | OPA/Gateway connection boundary | Fixed scheme-port allowlist, learned-rule port/scheme matching, address classification, and address pinning |
| Body-dependent authority | A host-only approval authorizes a different request body or unknown body | Gateway/OPA policy boundary | Explicit-empty default, unavailable/non-empty denials are not learnable, trusted-host body-aware exception only |
| Gateway buffering | One Tobari sends an oversized body to exhaust shared Gateway memory or reach upstream before policy | mitmproxy transport boundary | Fixed 8 MiB request/response cap before the addon hook; over-limit integration canary |
| Audit confidentiality | Token or body leaks | Logger | Metadata-only schema and secret-canary tests |
| Shared runtime capacity | One Tobari exhausts work-container or shared-service CPU, memory, PIDs, or logs | Docker resource contract | Fixed work-container limits plus fixed Gateway/OPA CPU/memory/PID ceilings, JSON log rotation, and runtime inspection |
| Project authority | One Tobari claims another project's policy or credential scope | Shared Gateway/OPA principal boundary | Not provided in the MVP: session is caller metadata and the stable ID is not a trusted Gateway principal; shared policy/credential scope is an explicit deferred risk |
| Other Docker resources | Cleanup removes unrelated state | Labels/state | Exact names plus owner and opaque-ID verification |

## Credible abuse cases

### Proxy bypass

A Tobari process ignores proxy variables and connects directly. Its internal
network has no external route, so the connection fails. Empty `NO_PROXY` values
do not weaken this network control.

### Cross-Tobari access

A process resolves another CWD-owned container. The two containers have different
internal networks; only Gateway joins both. Neither container receives a peer
route or shared root.

### Direct policy access

A Tobari process resolves OPA. OPA is absent from its network. Gateway exposes
the proxy port on the Tobari interface and no management port.

### Policy editing

A process in a CWD-owned Tobari may intentionally modify the selected project
root, but the shared XDG `policy/` directory is not mounted into it. OPA itself
sees policy read-only and watches host changes where Docker-host events
propagate. The fixed-target `policy apply` path tests the host files and
recreates only the owned OPA component for deterministic activation. The
reference-bound `policy allow` and `policy compact` paths write only the
CLI-owned learned-rule member after validating owner-only regular files,
testing a private complete policy copy, and checking that the discovered source
state is unchanged.

### Forged authorization

Tobari supplies authorization, cookies, API keys, or another configured secret
header. Gateway excludes secret values from OPA and audit output. It strips
managed headers before forwarding; application cookies may pass only after
allow. Only an OPA-selected, exact-host-bound profile can add a managed value.

### Project identity and credential scope

A process can supply another-looking `x-tobari-session` value or request a
known credential profile name. The session is caller metadata and the shared
OPA/credential namespace is not bound to the source project, so the Gateway
does not claim to distinguish one CWD-owned Tobari from another for policy or
managed-secret authority. Dedicated Docker networks still prevent direct
cross-project routes and work containers still cannot read Gateway secret
files, but those facts do not provide per-project egress or credential
separation. Treat this as a release-blocking design gap for multi-tenant use;
adding identity requires a reviewed trust-boundary decision rather than a
header convention.

### Policy outage or ambiguity

OPA times out, stops, returns malformed JSON, or returns an incomplete
decision. Gateway denies and does not contact upstream. Invalid watched policy
does not create an allow decision, and `policy apply` refuses it before OPA
recreation. Learned-rule evaluation also validates the rule shape before
matching it. A failed learned-rule or compaction preflight leaves `data.json`
unchanged; a concurrent source change makes the opaque proposal stale.
OPA also classifies exact-rule learnability only after scheme, fixed port,
empty-body, cluster, and credential binding pass. Gateway records that boolean,
and candidate discovery
excludes false values instead of offering a permission that cannot resolve the
denial.

### Request/body mismatch

An attacker streams or changes content while policy evaluates it. Gateway
forwards the same captured bytes it inspected. If mitmproxy reports that the
body was unavailable, Gateway denies before OPA rather than treating it as an
empty body. The initialized policy also denies non-empty or truncated bodies
before ordinary host-only authorization and does not make those denials
learnable. Inspection is bounded; an oversized body is marked truncated and
not decoded. A trusted host may author a body-aware Rego exception for a
captured body, but observation cannot create one. The fixed 8 MiB transport cap
limits one buffered body; the MVP does not claim a total concurrent
request-body memory limit.

### Shared service exhaustion

A project can generate many policy decisions or TLS connections through the
shared Gateway and OPA. Both services have fixed CPU, memory-plus-swap, PID,
and Docker-log bounds, so this cannot consume the entire Docker VM or disk
without hitting a service cap. The shared ceilings do not establish fairness
between projects; a noisy project can still degrade other projects within the
shared allocation. Per-project identity, rate, and bandwidth controls require
separate design.

### Authority mismatch

An attacker reuses an approved host, path, and method on another port, scheme,
or DNS result. Gateway supplies the normalized port to OPA; the initialized
policy allows only its explicit scheme-port set, and learned rules bind the
observed port and scheme. Immediately before connecting, Gateway rejects
non-global results for dotted hostnames and replaces the hostname with the
selected resolved address, preventing a later DNS answer from changing the
connection target. A non-matching port is denied without becoming an approval
candidate, and a learned rule cannot cross that port or scheme.

### Host path escape

The user supplies a root or cwd containing symlinks or `..`. The CLI resolves
the canonical host path before state or Docker operations and requires cwd
containment. This does not protect files below an intentionally selected root.

### Cleanup confusion

An attacker creates similarly named Docker resources. Tobari uses exact stored
names and verifies the installation owner label. Per-Tobari cleanup also
verifies the exact opaque ID; prefixes, names, and display order are not
authority.

### Shared resource exhaustion

A process in a Tobari can intentionally fork, allocate memory, burn CPU, or
write logs. The work container has fixed CPU, total memory-plus-swap, PID-count, and
container-log bounds, and a changed or legacy spec is recreated before reuse.
The selected read-write root remains deliberately unquotaed, and network
bandwidth is not shaped; exhausting host disk through that root or consuming
authorized network capacity remains outside this control.

## Explicitly accepted risks

- A Tobari can modify or delete its full mounted root.
- A Tobari can consume free disk below its explicitly selected root and
  authorized network throughput; MVP resource controls do not provide disk
  quotas or bandwidth shaping.
- A permitted destination can receive any data that Tobari can read.
- An injected credential can perform any operation allowed by both Rego and its
  upstream authority.
- The shared local MVP does not authenticate a project principal to Gateway;
  all projects in one cluster share the policy and credential namespace. It is
  not suitable for mutually untrusted tenants until that boundary is designed
  and enforced.
- Container, VM, kernel, Docker Engine, Gateway, OPA, or host compromise is
  outside this boundary.
- Certificate-pinned applications may fail because Tobari does not bypass
  pinning.
- HTTP/3/QUIC, SSH, raw TCP, UDP, DNS policy, covert channels, malware
  detection, and same-Tobari process isolation are not provided.
- Unix file mode checks do not establish equivalent Windows ACL guarantees.

## Reconsideration triggers

Revisit the architecture before adding transparent proxying, non-HTTP
protocols, multiple clusters or tenants, project-bound identity or credentials,
remote execution, dynamic credentials,
provider-specific semantics, or filesystem overlays. A demonstrated route from
one Tobari to another, OPA, or an external destination without Gateway is a
release-blocking security defect.
