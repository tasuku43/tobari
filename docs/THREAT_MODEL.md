# Tobari Threat Model

## Scope

This model covers the local MVP: one shared Gateway and OPA cluster, multiple
CWD-owned Tobari, one selected read-write root and home per Tobari, a default
tool-native passthrough adapter, and a retained optional static credential
injection adapter.

## Trust classification

Trusted:

- host OS and Docker Engine or its Linux VM;
- Tobari CLI and embedded runtime assets;
- Gateway, OPA, Rego policy, and host credential storage used by the managed
  adapter.

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
| Tool-owned authentication | Another Tobari reads or reuses it | Per-Tobari home/network | Exact home bind, dedicated network, recovery/deletion tests |
| Managed secrets | Agent reads or exfiltrates them | Gateway mount | Secret files mount read-only only into Gateway; passthrough never loads them |
| Authorization integrity | Body bytes reach upstream before route authorization | Gateway header hook | Principal, credential binding, and body-free OPA decision complete before request streaming is enabled |
| Authority binding | An approved path is replayed on another port, scheme, or unsafe resolved address | OPA/Gateway connection boundary | Fixed scheme-port allowlist, learned-rule port/scheme matching, address classification, and address pinning |
| Body payload exfiltration | An allowed route carries a different or sensitive body | Gateway/OPA policy boundary | Deliberate route-level grant: review names project/host/port/method/path and authorizes any body at that route; body data is never retained as evidence |
| Gateway buffering | One Tobari uses a large body to exhaust shared Gateway memory | mitmproxy transport boundary | Allowed bodies stream instead of buffering in full; known `Content-Length` above 8 MiB retains the transport rejection and integration canary |
| Audit confidentiality | Token or body leaks | Logger | Metadata-only schema and secret-canary tests |
| Shared runtime capacity | One Tobari exhausts work-container or shared-service CPU, memory, PIDs, or logs | Docker resource contract | Fixed work-container limits plus fixed Gateway/OPA CPU/memory/PID ceilings, JSON log rotation, and runtime inspection |
| Project authority | One Tobari claims another project's learned policy or managed credential scope | Shared Gateway/OPA principal boundary | Host-owned atomic principal registry maps each exact Gateway project-network address to one UUIDv7 project ID; Gateway derives the principal from the local interface, and project-bound credentials/rules reject mismatches before upstream I/O |
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
propagate. The reference-bound `policy allow`, `policy deny`, and `policy compact`
paths test a private complete policy copy and recreate only the owned OPA
component for deterministic activation; they write only the
CLI-owned learned-rule member after validating owner-only regular files,
testing a private complete policy copy, and checking that the discovered source
state is unchanged.

### Forged authorization

Tobari supplies authorization, cookies, API keys, or another configured secret
header. Gateway excludes secret values from OPA and audit output. The default
passthrough adapter forwards client authentication only after allow and strips
proxy/Tobari control headers. The managed adapter strips managed headers before
forwarding; application cookies may pass only after allow. Only an OPA-selected
profile bound to the established project and normalized host can add a managed
value.

### Project identity and credential scope

A process can supply another-looking `x-tobari-session` value or request a
known credential profile name. The session and profile name are untrusted
inputs; passthrough ignores the profile selector. Gateway instead observes the
local address on which the proxy
connection arrived and looks it up in the host-owned, owner-only principal
registry. Docker network ownership and the exact project ID are checked before
the registry binding is written. An unknown, duplicate, malformed, or stale
binding denies before OPA and upstream I/O.

Credential profiles list the project IDs they serve, and Gateway checks that
binding before OPA and again immediately before reading the secret. Learned
denials, candidate IDs, rules, and compaction groups include the project ID, so
an approval for project A does not match project B even when host, port, method,
and path are identical. The initialized host policy remains an explicit
installation-wide baseline; this change does not claim that baseline is a
per-project allowlist. Direct network routes and secret-file mounts remain
separate boundaries. Network recreation and deletion reconcile the registry;
until reconciliation succeeds, requests fail closed.

### Policy outage or ambiguity

OPA times out, stops, returns malformed JSON, or returns an incomplete
decision. Gateway denies and does not contact upstream. Invalid watched policy
does not create an allow or deny decision, and exact policy actions refuse it
before OPA recreation. Learned-rule evaluation also validates the rule shape before
matching it. A failed learned-rule or compaction preflight leaves `data.json`
unchanged; a concurrent source change makes the opaque proposal stale.
OPA also classifies exact-rule learnability only after scheme, fixed port,
cluster, and applicable credential binding pass. Gateway records
that boolean, and candidate discovery
excludes false values instead of offering a permission that cannot resolve the
denial.

### Body-independent route authority

Tobari deliberately does not claim that policy inspected, understood, or bound
the body. Gateway authorizes the project, authority, method, and path from the
request headers before enabling upstream body streaming. An exact approval is
therefore permission to send any body value at that exact route, including a
later value that differs from the denied observation. Review and audit never
retain body content, so a reviewer must evaluate that route-level grant rather
than infer payload-level restriction. Large unknown-length bodies stream
without a total-byte cap; known `Content-Length` above 8 MiB retains the
transport rejection.

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
candidate, and a baseline or exact deny is terminal rather than reviewable. A
learned rule cannot cross that port or scheme, and an exact deny wins over a
learned allow for the same project-bound request.

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
- A tool-owned or injected credential can perform any operation allowed by both
  Rego and its upstream authority; same-Tobari processes intentionally share the
  tool-owned home.
- The initialized host policy is an installation-wide baseline rather than a
  per-project allowlist. Project-bound learned permissions and managed
  credentials are separated, but mutually untrusted tenants still require a
  stronger execution and resource boundary than this local shared cluster.
- Container, VM, kernel, Docker Engine, Gateway, OPA, or host compromise is
  outside this boundary.
- Certificate-pinned applications may fail because Tobari does not bypass
  pinning.
- HTTP/3/QUIC, SSH, raw TCP, UDP, DNS policy, covert channels, malware
  detection, and same-Tobari process isolation are not provided.
- Unix file mode checks do not establish equivalent Windows ACL guarantees.

## Reconsideration triggers

Revisit the architecture before adding transparent proxying, non-HTTP
protocols, multiple clusters or tenants, a per-project static policy baseline,
remote execution, dynamic credentials,
provider-specific semantics, or filesystem overlays. A demonstrated route from
one Tobari to another, OPA, or an external destination without Gateway is a
release-blocking security defect.
