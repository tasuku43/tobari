# ADR 0010: Accept a direct read-write root mount

- Status: Accepted
- Date: 2026-07-29
- Deciders: Tobari maintainers
- Scope: Product and security
- Supersedes: None
- Superseded by: None

## Context

Coding agents need normal filesystem performance and must share changes across
concurrent shells and processes. Overlay, clone, and approval mechanisms would
substantially expand the MVP.

## Decision drivers

- One long-lived Realm for an entire selected source root
- Immediate, shared, ordinary filesystem semantics
- Honest scope rather than implied per-repository protection

## Considered options

- Direct read-write bind mount
- Copy-on-write overlay with explicit apply
- Per-repository clones or volumes

## Decision

`up --root` mounts the one canonical selected directory read-write at
`/workspace`. Realm can modify or delete every file below it. No other host
directory, host home, credential directory, SSH agent, or Docker socket is
mounted.

## Consequences

- Users must choose the root as an authority boundary.
- Tobari cannot recover or approve file changes under that root.
- Multiple processes see the same files and home immediately.

## Mechanical enforcement

Path validation rejects `--cwd` outside root. Runtime asset tests assert the
single host write mount and forbidden mounts. The threat model and README state
the accepted loss explicitly.

## Compatibility, security, and validation

The canonical root is persisted and conflicting roots are rejected.
Integration inspects Realm mounts. Reconsider overlays when users need
reversible changes more than direct shared-workspace simplicity.
