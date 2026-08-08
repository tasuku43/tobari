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

- One long-lived Context-bound Tobari for each selected source-root/Context pair
- Immediate, shared, ordinary filesystem semantics
- Honest scope rather than implied per-repository protection

## Considered options

- Direct read-write bind mount
- Copy-on-write overlay with explicit apply
- Per-repository clones or volumes

## Decision

Each Tobari mounts one canonical selected directory read-write at its mapped
workspace path. That Tobari can modify or delete every file below it. Same-root
Tobari in different Contexts and overlapping parent/child roots are allowed and
observe the same host-file mutations. No other host
directory, host home, credential directory, SSH agent, or Docker socket is
mounted.

## Consequences

- Users must choose the root as an authority boundary.
- Tobari cannot recover or approve file changes under that root.
- Multiple processes see the same files and home immediately.
- Runtime, home, network, policy, and managed credentials remain separated, but
  overlapping host project files do not have integrity isolation. No overlay,
  automatic checkout copy, root lock, session exclusion, or warning gate is
  added.

## Mechanical enforcement

Path validation rejects `--cwd` outside the selected root. Runtime tests assert
one host write mount per Tobari and forbidden mounts. The threat model and README state
the accepted loss explicitly.

## Compatibility, security, and validation

Canonical root plus stable Context ID is unique in persisted state.
Integration inspects Tobari mounts. Reconsider overlays when users need
reversible changes more than direct shared-workspace simplicity.
