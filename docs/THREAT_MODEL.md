# Tobari Threat Model

## Scope

This model covers the local MVP: one shared Gateway and OPA cluster, multiple
named Tobari, one selected read-write root and home per Tobari, and optional
static credential injection.

## Trust classification

Trusted:

- host OS and Docker Engine or its Linux VM;
- Tobari CLI and embedded runtime assets;
- Gateway, OPA, Rego policy, and host credential storage.

Untrusted:

- every named Tobari and its selected root;
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
| Audit confidentiality | Token or body leaks | Logger | Metadata-only schema and secret-canary tests |
| Other Docker resources | Cleanup removes unrelated state | Labels/state | Exact names plus owner and opaque-ID verification |

## Credible abuse cases

### Proxy bypass

A Tobari process ignores proxy variables and connects directly. Its internal
network has no external route, so the connection fails. Empty `NO_PROXY` values
do not weaken this network control.

### Cross-Tobari access

A process resolves another named container. The two containers have different
internal networks; only Gateway joins both. Neither container receives a peer
route or shared root.

### Direct policy access

A Tobari process resolves OPA. OPA is absent from its network. Gateway exposes
the proxy port on the Tobari interface and no management port.

### Policy editing

A user may intentionally attach a Tobari to the XDG `policy/` directory. That
Tobari can then modify every policy file because the selected root is an
explicit read-write scope. Users must not attach the parent configuration
directory because it also contains credential files. OPA itself sees policy
read-only and watches host changes where Docker-host events propagate. The
fixed-target `policy apply` path tests the host files and recreates only the
owned OPA component for deterministic activation. The reference-bound
`policy allow` and `policy compact` paths write only the CLI-owned learned-rule
member after validating owner-only regular files, testing a private complete
policy copy, and checking that the discovered source state is unchanged.

### Forged authorization

Tobari supplies authorization, cookies, API keys, or another configured secret
header. Gateway excludes secret values from OPA and audit output. It strips
managed headers before forwarding; application cookies may pass only after
allow. Only an OPA-selected, exact-host-bound profile can add a managed value.

### Policy outage or ambiguity

OPA times out, stops, returns malformed JSON, or returns an incomplete
decision. Gateway denies and does not contact upstream. Invalid watched policy
does not create an allow decision, and `policy apply` refuses it before OPA
recreation. Learned-rule evaluation also validates the rule shape before
matching it. A failed learned-rule or compaction preflight leaves `data.json`
unchanged; a concurrent source change makes the opaque proposal stale.
OPA also classifies exact-rule learnability only after scheme, cluster, and
credential binding pass. Gateway records that boolean, and candidate discovery
excludes false values instead of offering a permission that cannot resolve the
denial.

### Request/body mismatch

An attacker streams or changes content while policy evaluates it. Gateway
forwards the same bytes it inspected. Inspection is bounded; an oversized body
is marked truncated and not decoded. The MVP does not claim a total request-body
memory limit.

### Host path escape

The user supplies a root or cwd containing symlinks or `..`. The CLI resolves
the canonical host path before state or Docker operations and requires cwd
containment. This does not protect files below an intentionally selected root.

### Cleanup confusion

An attacker creates similarly named Docker resources. Tobari uses exact stored
names and verifies the installation owner label. Per-Tobari cleanup also
verifies the exact opaque ID; prefixes, names, and display order are not
authority.

## Explicitly accepted risks

- A Tobari can modify or delete its full mounted root.
- A permitted destination can receive any data that Tobari can read.
- An injected credential can perform any operation allowed by both Rego and its
  upstream authority.
- Container, VM, kernel, Docker Engine, Gateway, OPA, or host compromise is
  outside this boundary.
- Certificate-pinned applications may fail because Tobari does not bypass
  pinning.
- HTTP/3/QUIC, SSH, raw TCP, UDP, DNS policy, covert channels, malware
  detection, and same-Tobari process isolation are not provided.
- Unix file mode checks do not establish equivalent Windows ACL guarantees.

## Reconsideration triggers

Revisit the architecture before adding transparent proxying, non-HTTP
protocols, multiple clusters or tenants, remote execution, dynamic credentials,
provider-specific semantics, or filesystem overlays. A demonstrated route from
one Tobari to another, OPA, or an external destination without Gateway is a
release-blocking security defect.
