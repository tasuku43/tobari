# Security Model

This document is the durable Tobari security contract. The detailed threat
catalog and operational limits are in [Threat Model](THREAT_MODEL.md).

## Objective

Untrusted Realm processes may freely execute and may modify the explicitly
mounted root, but they cannot access other host files, Docker control, managed
credentials, OPA administration, or direct Internet egress through the
supported configuration. Every supported HTTP/HTTPS request is normalized,
authorized by OPA, and enforced by Gateway before forwarding.

## Trust boundaries

Trusted components are the host OS, Docker Engine or its Linux VM, Tobari CLI,
Gateway, OPA, Rego policy, and host credential storage. Realm, every process in
it, coding agents, generated code, downloaded packages, request data, and
upstream responses are untrusted.

```text
host root (rw) ---> Realm --explicit proxy--> Gateway --OPA decision--> upstream
                       |                         |
                       | no route               +-- credentials (read-only)
                       +--X OPA/control
```

Realm joins only an internal realm network. OPA joins only an internal control
network. Gateway has separate interfaces for proxy, control, and egress.

## Assets

- Host files outside the selected root.
- Docker Engine and socket.
- Host authentication material and Gateway-managed credential files.
- OPA policy, decision API, and Gateway management surface.
- Denial of direct Internet connectivity.
- Integrity of request normalization, policy decisions, and audit records.

Files under the read-write root are explicitly not protected from Realm. Realm
processes can change or delete the entire mounted root. Docker or kernel
compromise, VM/container escape, allowed-destination exfiltration, permitted
credential authority, non-HTTP protocols, covert channels, same-Realm process
interference, and malware detection are outside the MVP guarantee.

## Resource and process boundary

Runtime specs prohibit privileged mode, host networking, the Docker socket,
SSH agent mounts, host home mounts, and added Linux capabilities. Realm uses a
non-root work user mapped to the invoking UID/GID where Docker supports it.
Only the selected root and the named home volume are mounted writable.

Gateway image directories are assigned to the invoking host UID/GID at build
time, and the service starts directly as that non-root identity. It opens no
root entrypoint and receives no added capability. This preserves owner-only host
credential permissions across native Linux and Docker Desktop.

All resources carry `io.tobari.owner=default`. Destructive lifecycle code
selects exact stored names and confirms the ownership label before removal.
`down` preserves the persistent home unless `--purge` is explicit.

## HTTP authorization boundary

Gateway constructs OPA input version `v1` from the exact buffered request that
will be forwarded. It includes realm/session metadata, scheme, normalized host
and port, method, path and path segments, multi-valued query, redacted headers,
bounded body metadata, and an optional requested credential profile.

Secret header values are absent from both OPA input and logs. JSON is decoded
only when the complete body fits the inspection limit. Oversized and non-JSON
bodies expose size, type, truncation, and SHA-256 metadata only. Gateway never
logs raw request or response bodies.

OPA timeout, connection failure, non-2xx status, malformed JSON, missing
fields, unknown decision values, and Gateway exceptions all deny. Plain HTTP
to non-local destinations is denied by the sample policy.

## Credentials

MVP uses static bearer or fixed-header secrets supplied through owner-only host
files. Secret files are mounted read-only into Gateway only. Configuration
contains a profile type, exact allowed hosts, and a container secret path; it
never contains the secret value.

Gateway removes Realm-provided `Authorization`, `Proxy-Authorization`,
`X-API-Key`, and configured managed-secret headers. Cookie and Set-Cookie
values may remain part of the authorized application flow but are excluded
from OPA input and Tobari audit logs. A managed header is added only after an
allow decision names a configured profile whose host binding exactly matches
the normalized host. The value is never returned to Realm, OPA, CLI output,
errors, or audit logs.

OAuth, refresh tokens, provider SDKs, OS keychains, request signing, and
process-level identity are not used. There is no application-layer
authentication session because Tobari is not calling a provider API on behalf
of the CLI; Gateway performs host-bound post-authorization injection inside the
trusted infrastructure boundary.

## Mutation policy

Lifecycle mutations target one catalog-declared `tool_local` singleton.
`up` may create/reconcile only exact Tobari resources; `down` may remove only
them. Both use complete intent and impact declarations before Docker execution.
No human approval is required for ordinary reconciliation. `--purge` is an
explicit destructive input and affects only the Realm home volume.

## External calls

Gateway applies finite OPA and upstream timeouts and performs one upstream
attempt. It does not retry requests because arbitrary HTTP methods and bodies
may be unsafe or non-idempotent. Redirect handling remains in the proxy flow and
each resulting request is independently authorized.

## Logging

Audit JSON includes timestamp, request ID, realm, host, method, path, decision,
reason, selected credential profile name, upstream status, and duration. A
profile name is non-secret metadata; secret values and raw bodies are excluded.
CLI `logs` reads only a bounded component-log window and does not add
unredacted diagnostics. Policy authors use deny records on the trusted host;
Tobari never converts observed traffic into an allow rule automatically.

## Enforcement

| Claim | Enforcement |
|---|---|
| No direct Realm egress | Internal network topology and Docker integration test |
| Realm cannot access OPA | Separate internal networks and integration test |
| OPA outage denies | Gateway unit and integration tests |
| Secrets stay outside Realm | Mount-spec tests and integration canaries |
| Secret headers and bodies stay out of logs | Gateway unit tests and log scans |
| Only owned Docker resources are removed | Label validation and fake-runner tests |
| Root path is the only host write scope | Mount-spec and path-containment tests |
| Unknown effects fail closed | Domain and catalog validation |
| Denials support safe policy learning | Structured audit assertions and integration log scan |

## Supply chain and publication

Container images and CI actions are pinned to immutable versions or digests
recorded in source. Third-party licenses are reviewed. Tests use synthetic
credentials and `example.com` identities only. Publication still requires
`task security` and `task public:check`; neither replaces a human history and
confidentiality review.
