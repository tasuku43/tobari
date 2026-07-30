# ADR 0008: Keep Gateway and OPA outside Tobari

- Status: Accepted
- Date: 2026-07-29
- Deciders: Tobari maintainers
- Scope: Architecture and security
- Supersedes: None
- Superseded by: None

## Context

Each Tobari, including its root, is untrusted. Putting the enforcement point,
decision point, policy, or credentials in Tobari would let the subject modify
the mechanism that constrains it.

## Decision drivers

- Tobari compromise must not rewrite policy or read managed credentials
- Gateway alone needs external egress
- OPA administration must be unreachable from Tobari

## Considered options

- Separate Gateway and OPA containers on distinct networks
- Run Gateway or OPA inside Tobari
- Run trusted components directly on the host

## Decision

Gateway and OPA are separate trusted containers. Gateway joins every dedicated
Tobari network plus shared control and egress networks. OPA joins only control.
Each Tobari joins only its own internal network and can reach only Gateway's
proxy listener.

## Consequences

- Shared components and a dynamic internal network per Tobari must be managed consistently.
- Gateway or OPA outage intentionally stops supported egress.
- Docker Engine remains part of the trusted computing base.

## Mechanical enforcement

Embedded Compose and runtime tests assert membership and hardening. Integration
creates two Tobari networks, stops Gateway and OPA independently, and proves
fail closed, direct OPA isolation, and cross-Tobari isolation.

## Compatibility, security, and validation

The topology is a security contract, not an implementation detail.
`task integration:test` is the acceptance test. Reconsider consolidation only
if an equally strong privilege and network boundary is demonstrated.
