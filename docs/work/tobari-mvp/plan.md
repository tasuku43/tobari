# Work Plan: Deliver the Tobari MVP

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Keep the inherited four-layer CLI and catalog, replace sample capabilities with
Tobari lifecycle tasks, and embed a pinned Docker runtime using mitmproxy plus a
Python addon and OPA Rego. Use the Docker CLI behind a narrow runner port.

## Alternatives considered

### Go forward proxy

A custom Go proxy would reduce Python but would require implementing and
maintaining TLS interception, HTTP/2 translation, certificate generation, and
proxy correctness. mitmproxy already owns those mechanisms.

### Transparent proxy

Transparent capture would reduce client configuration but requires routing and
platform-specific privileges. The MVP chooses explicit proxy variables plus a
network that makes bypass fail.

## Public contract

The catalog exposes `up`, `status`, `shell`, `exec`, `logs`, `down`, and
`doctor`. Lifecycle acts target the one fixed local realm. `exec` passes command
argv unchanged after `--`; no service operation is inferred.

## Layer changes

- Domain: validated config, state, path mapping, and runtime resource facts.
- Application: lifecycle, inspection, execution, logs, and diagnostics.
- Infrastructure: Docker CLI, local state, embedded assets, and host checks.
- CLI: catalog declarations, handlers, stable rendering, and exit propagation.

## Data and control flow

CLI parses once, application validates desired state, infrastructure invokes
bounded Docker commands, and CLI renders task-owned results. Inside the runtime,
mitmproxy normalizes each flow, asks OPA, conditionally injects a credential,
forwards once, and writes redacted JSON audit output.

## Error and cancellation

Validation and policy rejection make zero external calls. OPA and Gateway
failures deny. Docker operations use caller context. Unknown lifecycle mutation
outcomes are non-retryable and recover through `status`.

## Implementation slices

1. Concrete contracts, work packet, and ADRs.
2. HTTP allow/deny Gateway and Rego tests.
3. HTTPS interception, CA bootstrap, and network topology.
4. Go lifecycle/application/catalog implementation.
5. Credential injection, full integration, CI, README, and packaging.

## Verification

Go unit/contract tests, Python Gateway tests, `opa test`, Docker integration,
Quick Start replay, `task check`, `task security`, and `task public:check`.

## Rollout and rollback

MVP has no migration from prior Tobari state. `down` removes transient owned
resources; `down --purge` is the explicit persistent-home cleanup. Rollback is
source checkout followed by `down` and `up`; configuration compatibility before
v1.0 is release-noted.

## Documentation promotion

Promote topology, credential, lifecycle, and accepted-risk conclusions into
numbered docs, ADRs, README, and harness claims before deleting this packet.
