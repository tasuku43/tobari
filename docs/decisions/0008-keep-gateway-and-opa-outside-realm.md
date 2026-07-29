# ADR 0008: Keep Gateway and OPA outside Realm

- Status: Accepted
- Date: 2026-07-29
- Deciders: Tobari maintainers
- Scope: Architecture and security
- Supersedes: None
- Superseded by: None

## Context

Realm, including root inside it, is untrusted. Putting the enforcement point,
decision point, policy, or credentials in Realm would let the subject modify
the mechanism that constrains it.

## Decision drivers

- Realm compromise must not rewrite policy or read managed credentials
- Gateway alone needs external egress
- OPA administration must be unreachable from Realm

## Considered options

- Separate Gateway and OPA containers on distinct networks
- Run Gateway or OPA inside Realm
- Run trusted components directly on the host

## Decision

Gateway and OPA are separate trusted containers. Gateway joins realm, control,
and egress networks. OPA joins only control. Realm joins only realm and can
reach only Gateway's proxy listener.

## Consequences

- Three containers and three networks must be managed consistently.
- Gateway or OPA outage intentionally stops supported egress.
- Docker Engine remains part of the trusted computing base.

## Mechanical enforcement

Embedded Compose tests assert membership and hardening. Integration tests stop
Gateway and OPA independently and prove fail closed and direct OPA isolation.

## Compatibility, security, and validation

The topology is a security contract, not an implementation detail.
`task integration:test` is the acceptance test. Reconsider consolidation only
if an equally strong privilege and network boundary is demonstrated.
