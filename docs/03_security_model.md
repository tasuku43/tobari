# Security Model

This document is the durable Tobari security contract. The detailed threat
catalog and operational limits are in [Threat Model](THREAT_MODEL.md).

## Objective

Untrusted Tobari processes may freely execute and may modify their explicitly
mounted root, but they cannot access other host files, another Tobari, Docker
control, managed credentials, OPA administration, or direct Internet egress
through the supported configuration. Every supported HTTP/HTTPS request is
normalized, authorized by OPA, and enforced by the shared Gateway before
forwarding.

## Trust boundaries

Trusted components are the host OS, Docker Engine or its Linux VM, Tobari CLI,
Gateway, OPA, Rego policy, and host credential storage. Tobari, every process in
it, coding agents, generated code, downloaded packages, request data, and
upstream responses are untrusted.

```text
host root A (rw) ---> Tobari A --+
                                  +--explicit proxy--> Gateway --OPA--> upstream
host root B (rw) ---> Tobari B --+                       |
                no cross-route                           +-- credentials (ro)
```

Each Tobari joins only its dedicated internal network. OPA joins only an
internal control network. Gateway has separate interfaces for every Tobari
proxy network, control, and egress.

## Assets

- Host files outside the selected root.
- Docker Engine and socket.
- Host authentication material and Gateway-managed credential files.
- OPA policy, decision API, and Gateway management surface.
- Denial of direct Internet connectivity.
- Integrity of request normalization, policy decisions, and audit records.

Files under a read-write root are explicitly not protected from its Tobari.
Processes can change or delete that entire mounted root. Docker or kernel
compromise, VM/container escape, allowed-destination exfiltration, permitted
credential authority, non-HTTP protocols, covert channels, same-Tobari process
interference, and malware detection are outside the MVP guarantee.

## Resource and process boundary

Runtime specs prohibit privileged mode, host networking, the Docker socket,
SSH agent mounts, host home mounts, and added Linux capabilities. Tobari uses a
non-root work user mapped to the invoking UID/GID where Docker supports it.
Only the selected root and that Tobari's named home volume are mounted writable.

Gateway image directories are assigned to the invoking host UID/GID at build
time, and the service starts directly as that non-root identity. It opens no
root entrypoint and receives no added capability. This preserves owner-only host
credential permissions across native Linux and Docker Desktop.

All resources carry `io.tobari.owner=default`; per-Tobari resources also carry
the exact opaque Tobari ID. Destructive lifecycle code selects exact stored
names and confirms both labels before removal. `detach` preserves the
persistent home unless `--purge` is explicit. Shared cluster removal is rejected
while any Tobari remains.

XDG policy is mounted read-only into OPA. Host-side edits are reflected by the
bind mount and OPA watch; OPA receives no authority to rewrite trusted policy.

## HTTP authorization boundary

Gateway constructs OPA input version `v1` from the exact buffered request that
will be forwarded. It includes cluster/session metadata, scheme, normalized host
and port, method, path and path segments, multi-valued query, redacted headers,
bounded body metadata, and an optional requested credential profile.

Secret header values are absent from both OPA input and logs. JSON is decoded
only when the complete body fits the inspection limit. Oversized and non-JSON
bodies expose size, type, truncation, and SHA-256 metadata only. Gateway never
logs raw request or response bodies.

OPA timeout, connection failure, non-2xx status, malformed JSON, missing
fields, unknown decision values, and Gateway exceptions all deny. Plain HTTP
to non-local destinations is denied by the initialized policy.

## Credentials

MVP uses static bearer or fixed-header secrets supplied through owner-only host
files. Secret files are mounted read-only into Gateway only. Configuration
contains a profile type, exact allowed hosts, and a container secret path; it
never contains the secret value.

Gateway removes Tobari-provided `Authorization`, `Proxy-Authorization`,
`X-API-Key`, and configured managed-secret headers. Cookie and Set-Cookie
values may remain part of the authorized application flow but are excluded
from OPA input and Tobari audit logs. A managed header is added only after an
allow decision names a configured profile whose host binding exactly matches
the normalized host. The value is never returned to Tobari, OPA, CLI output,
errors, or audit logs.

OAuth, refresh tokens, provider SDKs, OS keychains, request signing, and
process-level identity are not used. There is no application-layer
authentication session because Tobari is not calling a provider API on behalf
of the CLI; Gateway performs host-bound post-authorization injection inside the
trusted infrastructure boundary.

## Mutation policy

Shared lifecycle mutations target one catalog-declared `tool_local` cluster.
`attach` creates within that exact cluster. Individual `detach` consumes one
opaque `tobari_id` produced by `list`; it does not select by name or Docker
discovery. All mutations use complete intent and impact declarations before
Docker execution. No human approval is required for ordinary reconciliation.
`--purge` is an explicit destructive input and affects only the exact selected
home or, after all Tobari are detached, shared CA volumes.

## External calls

Gateway applies finite OPA and upstream timeouts and performs one upstream
attempt. It does not retry requests because arbitrary HTTP methods and bodies
may be unsafe or non-idempotent. Redirect handling remains in the proxy flow and
each resulting request is independently authorized.

## Logging

Audit JSON includes timestamp, request ID, cluster, host, method, path, decision,
reason, selected credential profile name, upstream status, and duration. A
profile name is non-secret metadata; secret values and raw bodies are excluded.
CLI `logs` reads only a bounded component-log window and does not add
unredacted diagnostics. Policy authors use deny records on the trusted host;
Tobari never converts observed traffic into an allow rule automatically.

## Enforcement

| Claim | Enforcement |
|---|---|
| No direct Tobari egress | Per-Tobari internal topology and Docker integration test |
| Tobari cannot access OPA or peers | Separate internal networks and integration test |
| OPA outage denies | Gateway unit and integration tests |
| Secrets stay outside Tobari | Mount-spec tests and integration canaries |
| Secret headers and bodies stay out of logs | Gateway unit tests and log scans |
| Only owned Docker resources are removed | Label validation and fake-runner tests |
| Each root is its Tobari's only host write scope | Mount-spec and path-containment tests |
| OPA cannot rewrite host policy | Read-only mount-spec test |
| Actions use exact Tobari identity | Reference round-trip and label-validation tests |
| Unknown effects fail closed | Domain and catalog validation |
| Denials support safe policy learning | Structured audit assertions and integration log scan |

## Supply chain and publication

Container images and CI actions are pinned to immutable versions or digests
recorded in source. Third-party licenses are reviewed. Tests use synthetic
credentials and `example.com` identities only. Publication still requires
`task security` and `task public:check`; neither replaces a human history and
confidentiality review.
