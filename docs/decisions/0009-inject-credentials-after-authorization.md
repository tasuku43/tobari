# ADR 0009: Inject credentials after authorization

- Status: Accepted
- Date: 2026-07-29
- Deciders: Tobari maintainers
- Scope: Security
- Supersedes: None
- Superseded by: None

## Context

Agents need authenticated HTTP access, but a secret placed in Tobari can be read
by every untrusted process and sent to any reachable allowed destination.

## Decision drivers

- Managed secret values must stay outside Tobari
- Policy must approve request shape before credential use
- A credential must be bound to exact normalized hosts

## Considered options

- Gateway-only static secret files injected after allow
- Environment variables or files mounted into Tobari
- Provider OAuth and signing implementations

## Decision

Tobari requests a non-secret profile name. OPA must allow and select it. Gateway
then validates an independent configured host binding, reads the owner-only
secret file, removes Tobari-supplied authorization, and injects one bearer or
fixed header immediately before forwarding.

## Consequences

- Static token rotation is a host file operation.
- OAuth refresh, Keychain, GitHub App tokens, and SigV4 are excluded.
- A permitted request can exercise the credential's full provider authority.

## Mechanical enforcement

Gateway unit tests cover host binding, removal, and redaction. Integration uses
a canary digest to prove injection, cross-host denial, Tobari absence, and
secret-free logs.

## Compatibility, security, and validation

Credential configuration `v1`, profile types, and host matching are security
boundaries. `task runtime:test` validates them. Reconsider dynamic credentials
only with a separate lifecycle and storage threat analysis.
