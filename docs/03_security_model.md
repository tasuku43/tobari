# Security Model

This document is the durable Tobari security contract. The detailed threat
catalog and operational limits are in [Threat Model](THREAT_MODEL.md).

## Objective

Untrusted Tobari processes may freely execute and may modify their explicitly
mounted root, but they cannot access other host files, another Tobari, Docker
control, host authentication state, OPA administration, or direct Internet
egress through the supported configuration. Tool-owned credentials may exist
inside one Tobari's exact home by explicit user action. Every supported
HTTP/HTTPS request is
normalized, authorized by OPA, and enforced by the shared Gateway before
forwarding.

## Trust boundaries

Trusted components are the host OS, Docker Engine or its Linux VM, Tobari CLI,
Gateway, OPA, Rego policy, and reserved host credential storage used only by
the managed adapter. Tobari, every process in
it, coding agents, generated code, downloaded packages, request data, and
upstream responses are untrusted.

```text
host root A (rw) ---> Tobari A --+
                                  +--explicit proxy--> Gateway --OPA--> upstream
host root B (rw) ---> Tobari B --+                       |
                no cross-route                           +-- redacted policy data
                                                          +-- optional managed adapter inputs (ro)
```

Each Tobari joins only its dedicated internal network. OPA joins only an
internal control network. Gateway has separate interfaces for every Tobari
proxy network, control, and egress.

## Assets

- Host files outside the selected root.
- Docker Engine and socket.
- Tool-owned authentication state inside a Tobari home, plus the reserved host
  authentication material used by the managed adapter.
- OPA policy, decision API, and Gateway management surface.
- Denial of direct Internet connectivity.
- Integrity of request normalization, policy decisions, and audit records.

Files under a read-write root are explicitly not protected from its Tobari.
Processes can change or delete that entire mounted root. Docker or kernel
compromise, VM/container escape, allowed-destination exfiltration, the
installation-wide baseline policy, non-HTTP protocols, covert channels,
same-Tobari process interference, and malware detection are outside the MVP
guarantee. Mutable learned permissions are bound to the host-issued project
principal described below. A tool credential is intentionally available to all
processes in the same Tobari and may be sent to any policy-allowed destination.

## Resource and process boundary

Runtime specs prohibit privileged mode, host networking, the Docker socket,
SSH agent mounts, host home mounts, and added Linux capabilities. Each
project work container also receives fixed CPU, total memory-plus-swap,
PID-count, and container-log bounds through the infrastructure-owned Docker
specification;
the desired resource contract is part of the spec hash, so drift or an older
unbounded container is recreated before reuse. These bounds do not provide a
quota for the explicitly mounted project root or shape network bandwidth.
The shared Gateway and OPA services use fixed JSON-file log rotation of 10 MiB
per file and three files, plus fixed CPU, memory-plus-swap, and PID ceilings.
Those ceilings bound shared-service exhaustion but do not provide per-project
fairness inside one shared Gateway or OPA.
Tobari uses a non-root work user mapped to the invoking UID/GID where Docker
supports it.
Only the selected root and that Tobari's exact XDG-owned home directory are
mounted writable. The project-root resolver rejects filesystem root, the user's
home and its ancestors, and paths overlapping XDG configuration, state, or
shared-profile management directories, Docker sockets, and Docker management
paths. Policy source repositories remain allowed; the active host policy,
principal registry, and reserved managed credential files are separate trusted
assets and are never selected as project roots. Publishing or applying a source
to active policy is an explicit trusted-host operation; entering that source
repository never changes policy.

A custom Tobari image is untrusted input and receives no additional authority.
The CWD-owned runtime accepts only a locally available image that asserts
runtime API `1` and preserves the built-in image user, entrypoint, and
`io.tobari.runtime-lifetime-command=sleep infinity` capability needed for the
fixed Workspace lifetime command, then independently fixes the numeric runtime
user, read-only root, capabilities, security options,
mounts, network, proxy environment, and health check. Compatibility metadata is
not a signature or provenance claim. Tobari does not grant the image's `CMD`
lifecycle authority: the infrastructure supplies the long-lived command and
runs user commands through child exec sessions. Missing or incompatible image
metadata is rejected before project home, network, or container mutation. Users
remain responsible for image contents and should prefer immutable digest
references.

The official base runtime and derived agent images do not change this boundary.
The base main channel is published by a protected main-branch workflow;
derived agent variants are separate later slices in the same runtime package.
A published Claude or Codex image is a convenience rootfs and tool bundle, not
a source of host mounts, host credentials, capabilities, network routes, or lifecycle
policy, and registry provenance does not replace local runtime compatibility
validation.

The current Claude and Codex image slices are build-only and contain no
credentials or agent configuration. Their workflows verify the versioned
release packages against the checked-in per-architecture checksums and have no
package-write permission; public publication remains gated on third-party
redistribution and license review.

The optional toolbox recipe downloads version-pinned GitHub CLI, AWS CLI,
kubectl, and TWG artifacts only during an explicit trusted-host build.
Checksums verify GitHub, Kubernetes, and Atlassian artifacts; the AWS package is
verified with the reviewed AWS CLI signing key and exact fingerprint. The
result contains no credentials, copies no host CLI configuration, inherits the
same untrusted custom-image treatment, and receives no Docker socket or direct
egress.

The XDG `config.json` image default is a strict, bounded, owner-only regular
file. Bounded project metadata may select only a literal compatible image.
Malformed defaults fail before Docker resource creation.

A Dev Container file is untrusted project data. Tobari reads only configured
metadata contained by the selected root, with a fixed size limit and no variable or
environment substitution. Only a literal compatible image is consumed.
Properties that could request builds, Compose services, Features, commands,
mounts, environment, users, privileges, capabilities, or ports fail before
Docker mutation.

Project runtime readiness is an explicit healthcheck boundary. Enter waits for
healthy rather than treating a running or healthcheck-less container as ready;
unhealthy, exited, and timeout outcomes remain distinct diagnostics. Desired
runtime drift is detected from an ownership-scoped spec hash and recreates only
the work container, preserving the logical project home and root record.

Gateway image directories are assigned to the invoking host UID/GID at build
time, and the service starts directly as that non-root identity. It opens no
root entrypoint and receives no added capability. This preserves owner-only host
credential permissions across native Linux and Docker Desktop.

All resources carry `io.tobari.owner=default`; per-Tobari resources also carry
the exact stable Tobari ID and a resource role. Destructive lifecycle code
resolves the nearest canonical CWD root, selects exact stored names, and
confirms owner, ID, and role labels before removal. `delete` removes that
instance's persistent XDG home and records after the session-attachment guard;
`--force` explicitly overrides an attached-session warning.
Shared cluster removal is rejected while any Tobari record remains.

XDG policy is mounted read-only into OPA. Host-side edits are reflected by the
bind mount. OPA watch reloads them where Docker-host events propagate;
`policy apply` first tests the bind and then recreates only the exact
owner-labeled OPA component for deterministic activation. OPA receives no
authority to rewrite trusted policy.

## HTTP authorization boundary

Gateway constructs OPA input version `v1` from the exact buffered request that
will be forwarded. It includes the host-issued project principal plus
cluster/session metadata, scheme, normalized host and port, method, path and
path segments, multi-valued query, redacted headers, bounded body metadata, and
an adapter-dependent optional requested credential profile. The project
principal is derived from the local Gateway interface address and an owner-only
host registry; the session field is caller metadata only.

Secret header values are absent from both OPA input and logs. JSON is decoded
only when the complete body fits the inspection limit. Oversized and non-JSON
bodies expose size, type, truncation, and SHA-256 metadata only. Gateway never
logs raw request or response bodies. If mitmproxy reports that the body was not
captured, Gateway marks it unavailable and denies before OPA; it is not treated
as an explicit empty body.
The Gateway process also enforces an 8 MiB mitmproxy request/response body cap
before the addon hook, so an oversized body cannot be forwarded before policy.

OPA timeout, connection failure, non-2xx status, malformed JSON, missing
fields, unknown decision values, and Gateway exceptions all deny. Plain HTTP
to non-local destinations is denied by the initialized policy. The initialized
policy also requires an explicit port for each supported scheme; learned rules
retain the observed project/host/port/method/path and cannot be used on another project, port, or
scheme. The initialized Rego policy also requires a zero-length, non-truncated
metadata body for routine authorization and learning; an exact learned rule
cannot turn a non-empty body into an allow. A body-dependent exception must be
authored and activated by the trusted host as body-aware Rego. Immediately
before an upstream connection, Gateway resolves the
hostname, rejects non-global addresses for dotted hostnames, and pins the
connection to the selected resolved address. Single-label private service
names remain an explicit policy-controlled local exception for the Docker
integration shape.

## Credentials

The default `passthrough` adapter uses tool-owned authentication state created
below the selected Tobari's `HOME=/var/lib/tobari`. It redacts client
authentication and cookie values from OPA input and audit, preserves them until
policy allow, then forwards them upstream. It strips proxy and Tobari control
headers and never reads managed credential files.

The retained `managed` adapter uses static bearer or fixed-header secrets
supplied through owner-only host files. Secret files are mounted read-only into
Gateway only. Configuration contains a profile type, exact allowed hosts,
explicit project IDs, and a container secret path; it never contains the secret
value. The host-owned `principals.json` registry binds each project ID to one
exact Gateway network and is mounted read-only into Gateway.

In passthrough mode, `Authorization`, `X-API-Key`, cookies, and other client
authentication are forwarded only after allow; `Proxy-Authorization`, the
profile selector, and Tobari session control headers are removed. In managed
mode, Gateway removes Tobari-provided `Authorization`, `Proxy-Authorization`,
`X-API-Key`, and configured managed-secret headers. Cookie and Set-Cookie
values may remain part of the authorized application flow but are excluded
from OPA input and Tobari audit logs. A managed header is added only after an
allow decision names a configured profile whose project and host bindings
exactly match the established principal and normalized host. Gateway checks
the project binding before OPA and repeats it before reading the secret. The
value is never returned to Tobari, OPA, CLI output, errors, or audit logs.

OAuth, refresh tokens, provider SDKs, OS keychains, request signing, and
process-level identity are not used. The optional `session` value remains
caller metadata, not authentication. Gateway performs project- and host-bound
post-authorization handling inside the trusted infrastructure boundary. The
initialized host policy is an installation-wide baseline; the learned-rule and
managed-secret namespaces are project-bound. A missing,
malformed, ambiguous, or stale principal registry entry denies before OPA and
upstream I/O.

## Mutation policy

Shared lifecycle mutations target one catalog-declared `tool_local` cluster.
The root command uses the catalog-declared current-directory fixed target to
read and present containing Workspace candidates, then creates or reconciles
one logical Tobari only after an explicit choice and a fresh locked check. An
exact current-root record is reused directly; a nested root requires the
explicit create-here choice. The command never creates shared resources.
Each canonical root is unique: repeated or concurrent explicit creation is
serialized by the state lock and only one logical record can be committed.
Entering a session and returning from the child shell are not lifecycle-ending
mutations. `exit` only detaches the session; `delete` resolves the same target
and is the only routine lifecycle-ending mutation. Ordinary delete observes
active exec sessions and rejects when one is attached; `--force` is the
explicit override.
Neither operation accepts an ID, name, or arbitrary root selector.
All mutations use complete intent and impact declarations before Docker
execution. Ordinary runtime reconciliation needs no human approval;
ordinary deletion requires no attached session, while `--force` overrides that
guard; both affect only the selected XDG home and exact owned resources. Shared
CA purge remains separate
and only follows an empty instance repository.
`policy apply` is an access-changing write to the same fixed cluster target. It
does not edit policy, but it can change outbound authority by activating the
trusted host's current tested files. Gateway remains fail-closed while OPA is
recreated.
`policy allow` and `policy compact` are access-changing writes bound to opaque
candidate references. Discovery never mutates. An allow reference identifies
one retained validated denial that OPA marked exact-rule learnable; a
compaction reference identifies one current exact source-rule set. Scheme,
cluster, credential-binding, unavailable-body, and body-inspection failures
never become permission candidates.
The mutation rejects stale or ambiguous references, unsafe policy files,
malformed learned data, failed preflight tests, and unrecognized compaction
shapes before the atomic policy write.

Learned rules never broaden a project, host, port, or method beyond the
explicitly approved evidence. Exact approvals may override an older deny rule
only for their exact project/host/port/method/path. Prefix compaction requires three exact
sources, keeps host, port, and method fixed, requires a multi-segment directory
boundary, rejects percent
encoding, backslashes, empty segments, and dot segments, retains positive
examples, and tests its matcher against an adjacent outside-prefix canary.
Host wildcards, method wildcards, user-supplied pattern text, and automatic
compaction are unsupported.

## External calls

Gateway applies finite OPA and upstream timeouts and performs one upstream
attempt. It does not retry requests because arbitrary HTTP methods and bodies
may be unsafe or non-idempotent. Redirect handling remains in the proxy flow and
each resulting request is independently authorized.

## Logging

Audit JSON includes timestamp, request ID, cluster, project ID, host, port,
method, path, decision, reason, selected credential profile name, upstream
status, and duration. A
profile name is non-secret metadata; secret values and raw bodies are excluded.
CLI `cluster logs` reads only a bounded component-log window and does not add
unredacted diagnostics. `cluster denials` projects only validated deny records
and preserves only non-secret credential-profile names. Read-only policy candidate commands
derive opaque proposals from that evidence. Observation alone never changes
authority; only an explicit reference-bound mutation can write a learned rule.

## Enforcement

| Claim | Enforcement |
|---|---|
| No direct Tobari egress | Per-Tobari internal topology and Docker integration test |
| Tobari cannot access OPA or peers | Separate internal networks and integration test |
| OPA outage denies | Gateway unit and integration tests |
| Host-managed secrets stay outside Tobari; tool-owned state stays in its home | Mount-spec tests and integration canaries |
| Secret headers and bodies stay out of logs | Gateway unit tests and log scans |
| Only owned Docker resources are removed | Label validation and fake-runner tests |
| Attached sessions are not removed accidentally | Exact work-container Exec ID observation, guard-before-delete tests, and explicit force-override tests |
| Each root and XDG home are its Tobari's only host write scopes | Mount-spec and path-containment tests |
| Ambiguous CWD selection cannot mutate before a valid choice | Typed candidate snapshot, locked stale-choice revalidation, and zero-call cancellation tests |
| One Tobari cannot consume unbounded CPU, memory, PIDs, or container logs | Fixed create-argv and spec-hash tests plus runtime HostConfig assertions |
| A custom image cannot expand its runtime specification | Compatibility inspection, fixed create-argv tests, and integration test |
| Optional toolbox artifacts retain reviewed identity | Pinned versions, vendor checksum or signature verification, and explicit build validation |
| Dev Container metadata cannot become a second runtime boundary | Contained bounded parser, unsupported-property tests, and fixed runtime adapter |
| OPA cannot rewrite host policy | Read-only mount-spec test |
| Tested host policy activates across Docker hosts | Fixed-target OPA recreation test and integration scenario |
| CWD lifecycle actions use exact Tobari identity | Canonical-root, state, and label-validation tests |
| One canonical root has one Workspace | Root-index hash naming, locked exact-root checks, domain duplicate-index validation, and concurrent explicit-creation tests |
| Session exit cannot delete a Workspace | Child exit-status tests, host-stderr summary tests, and logical-state preservation after entry |
| Gateway cannot accept a caller-selected project principal | Owner-only atomic principal registry, local-interface derivation, malformed/unknown denial tests, and two-project integration |
| Managed credentials cannot cross project principals | Explicit profile project bindings, pre-OPA Gateway rejection, repeated injection check, and two-project integration |
| Unknown effects fail closed | Domain and catalog validation |
| Denials support safe policy learning | Typed project/host/port/method/path denial validation, secret canaries, and integration projection |
| Learned permissions stay explicit and project-bound | Opaque candidate round trips, exact project/host/port/method/path domain tests, Rego cross-project canaries, preflight-before-atomic-write tests, and Docker integration |
| Compaction preserves declared boundaries | Three-source same-host/port/method grouping invariant, retained positive examples, outside-prefix canary, stale-reference rejection, and OPA tests |
| Gateway body buffering is bounded | Fixed mitmproxy body-size asset test and over-limit integration request |

## Supply chain and publication

Container bases distributed by Tobari and CI actions are pinned to immutable
versions or digests recorded in source. User-selected local images are neither
distributed nor trusted by Tobari; their selector is persisted as user
configuration. Third-party licenses are reviewed. Tests use synthetic
credentials and `example.com` identities only. Publication still requires
`task security` and `task public:check`; neither replaces a human history and
confidentiality review.
