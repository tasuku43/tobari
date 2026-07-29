# ADR 0004: Use Docker as the MVP isolation primitive

- Status: Accepted
- Date: 2026-07-29
- Deciders: Tobari maintainers
- Scope: Architecture and security
- Supersedes: None
- Superseded by: None

## Context

Tobari must run arbitrary untrusted agent processes concurrently while limiting
host filesystem access and enforcing a network topology on macOS VM-backed and
Linux hosts.

## Decision drivers

- A mature process, mount, user, capability, and network isolation interface
- Availability through Colima, Lima, Docker Desktop, and Linux Engine
- No custom container runtime or Kubernetes control plane in the MVP

## Considered options

- Docker Engine with Compose and the Docker CLI
- A host subprocess sandbox assembled separately per operating system
- Kubernetes, which adds a larger deployment and lifecycle surface

## Decision

Use Docker Engine as the isolation primitive and Compose as the reproducible
three-container topology. Keep Docker execution behind an application-owned
port so the mechanism can be replaced without changing public outcomes.

## Consequences

- Users need a working Docker Engine and bind-mount sharing.
- Docker and its Linux VM/kernel remain trusted.
- Container escape is outside the MVP guarantee.

## Mechanical enforcement

Architecture lint keeps Docker code in infrastructure. Runtime asset tests
reject privileged mode, host networking, Docker socket mounts, and added
capabilities. Docker integration proves the real topology.

## Compatibility, security, and validation

Docker resource names and ownership labels are compatibility boundaries.
`task runtime:test` validates Colima or Linux Engine behavior. Reconsider this
decision when a replacement provides equivalent mount, network, UID/GID,
health, and cleanup semantics with lower user cost.
