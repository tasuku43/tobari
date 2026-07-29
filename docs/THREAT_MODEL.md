# Tobari Threat Model

## Scope

This model covers the local MVP: one host-selected read-write root, one
long-lived Realm, one mitmproxy Gateway, one OPA instance, and optional static
credential injection.

## Trust classification

Trusted:

- host OS and Docker Engine or its Linux VM;
- Tobari CLI and embedded runtime assets;
- Gateway, OPA, Rego policy, and host credential storage.

Untrusted:

- Realm, including root inside Realm;
- Claude Code, Codex, shells, generated or downloaded code, packages, and every
  Realm process;
- request content and upstream responses.

## Assets and boundaries

| Asset | Threat | Boundary | Enforcement |
|---|---|---|---|
| Host files outside root | Realm reads or modifies them | Docker mounts | Only canonical root and named home are writable mounts |
| Docker Engine | Realm controls host containers | Unix socket/process | Socket and host process interfaces are not mounted |
| Direct Internet access | Agent bypasses policy | Docker network | Realm network is internal and has only Gateway |
| OPA policy/admin API | Agent changes or queries policy | Control network | OPA joins only control; Realm joins only realm |
| Managed secrets | Agent reads or exfiltrates them | Gateway mount | Secret files mount read-only only into Gateway |
| Authorization integrity | Policy sees different request | Gateway buffer | One normalized buffered request is inspected and forwarded |
| Audit confidentiality | Token or body leaks | Logger | Metadata-only schema and secret-canary tests |
| Other Docker resources | Cleanup removes unrelated state | Labels/state | Exact names plus `io.tobari.owner=default` verification |

## Credible abuse cases

### Proxy bypass

A Realm process ignores `HTTP_PROXY` and connects directly. The internal realm
network has no external route, so the connection fails. `NO_PROXY` is empty and
does not weaken this network control.

### Direct policy access

A Realm process resolves or connects to the OPA service. OPA is absent from its
network. Gateway exposes only the proxy port on the Realm interface and binds
no management port there.

### Forged authorization

Realm supplies `Authorization`, cookies, API keys, or another configured secret
header. Gateway strips these before OPA and before upstream forwarding. Only an
OPA-selected, host-bound profile can add a managed value.

### Policy outage or ambiguity

OPA times out, stops, returns malformed JSON, or returns an incomplete
decision. Gateway denies and does not contact upstream.

### Request/body mismatch

An attacker streams or changes content while policy evaluates it. Gateway
buffers the bounded request body once and forwards those same bytes. Oversized
bodies are marked truncated and not decoded; policy decides with that explicit
metadata.

### Host path escape

The user supplies a root or cwd containing symlinks or `..`. CLI resolves the
canonical host path before state or Docker operations and requires cwd
containment. This does not protect files below an intentionally selected root.

### Cleanup confusion

An attacker creates similarly named Docker resources. Tobari uses exact stored
names and verifies the ownership label; prefixes and display order are not
authority.

## Explicitly accepted risks

- Realm can modify or delete the full mounted root.
- A permitted destination can receive any data the Realm can read.
- An injected credential can perform any operation allowed by both Rego and the
  credential's upstream authority.
- Container, VM, kernel, Docker Engine, Gateway, OPA, or host compromise is
  outside this boundary.
- Certificate-pinned applications may fail because Tobari does not bypass
  pinning.
- HTTP/3/QUIC, SSH, raw TCP, UDP, DNS policy, covert channels, malware
  detection, and same-Realm process isolation are not provided.
- Unix file mode checks do not establish equivalent Windows ACL guarantees.

## Reconsideration triggers

Revisit the architecture before adding transparent proxying, non-HTTP
protocols, multiple tenants/realms, remote execution, dynamic credentials,
provider-specific semantics, or filesystem overlays. A demonstrated route from
Realm to an external destination without Gateway is a release-blocking
security defect.
