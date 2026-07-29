# ADR 0005: Use an explicit intercepting proxy

- Status: Accepted
- Date: 2026-07-29
- Deciders: Tobari maintainers
- Scope: Product, architecture, and security
- Supersedes: None
- Superseded by: None

## Context

Tobari must authorize decrypted HTTP semantics, including HTTPS requests,
without inferring intent from commands. Transparent interception would require
platform-specific routing and elevated network capabilities.

## Decision drivers

- L7 access to host, method, path, headers, and body metadata
- The same behavior for curl, Git, Python, `gh`, and coding agents
- No `CAP_NET_ADMIN`, host firewall changes, or Docker Desktop-specific feature

## Considered options

- Explicit `HTTP_PROXY` and `HTTPS_PROXY` with mitmproxy
- Transparent proxying with packet redirection
- CONNECT-only allowlists, which cannot authorize decrypted request paths

## Decision

Realm clients use `http://gateway:8080` for both proxy variables. HTTPS
uses CONNECT, client-to-Gateway TLS under the Tobari CA, and a separate
Gateway-to-upstream TLS session. Realm has no direct route, so ignoring the
proxy fails rather than bypassing policy.

## Consequences

- Clients must honor proxy variables and trust the Tobari CA.
- Certificate-pinned clients are unsupported.
- Raw TCP, UDP, QUIC, and Git SSH are outside the MVP.

## Mechanical enforcement

Compose network tests prove Realm joins only the internal realm network.
Integration tests prove HTTPS without `-k`, direct-egress failure, and Gateway
failure denial.

## Compatibility, security, and validation

The proxy endpoint and CA volume are local compatibility boundaries.
`task integration:test` validates CONNECT interception. Reconsider transparent
mode only if it preserves least privilege and cross-platform behavior.
