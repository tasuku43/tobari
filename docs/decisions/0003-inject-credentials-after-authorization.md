# ADR 0003: Inject credentials after authorization

- Status: Superseded
- Date: 2026-07-29
- Deciders: Tobari maintainers
- Scope: Security
- Supersedes: None
- Superseded by: [ADR 0009: Select tool-native passthrough by default](0009-defer-gateway-managed-credential-injection.md)

## Context

Agents need authenticated HTTP access, but a secret placed in Tobari can be read
by every untrusted process and sent to any reachable allowed destination.

## Decision

The original managed-authentication design kept secrets in Gateway-only files.
Tobari requested a non-secret profile name, OPA allowed and selected it, and
Gateway independently validated the normalized host and project binding before
reading the secret and injecting one bearer or fixed header immediately before
forwarding.

## Why it is historical

This decision remains the retained `managed` Gateway adapter, but it is no
longer the default authentication path. [ADR 0009](0009-defer-gateway-managed-credential-injection.md)
selected tool-native passthrough for the supported product, so this record must
not be read as permission to expose host credentials or provider login flows to
Tobari.

## Mechanical enforcement retained

Gateway tests cover host binding, removal, and redaction. Integration tests use
synthetic canaries to prove injection, cross-host denial, Tobari absence, and
secret-free logs. The retained adapter remains infrastructure-owned and is
selected only through trusted configuration.
