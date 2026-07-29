# ADR 0002: Isolate routine and specialized harness profiles

- Status: Accepted
- Date: 2026-07-20
- Deciders: Agentic CLI Foundry maintainers
- Scope: Local verification, CI, release preflight, and Codex automation
- Supersedes: None
- Superseded by: None

## Context

The `full` profile nested the complete security, public, and release profiles.
Production use in a derived repository measured about 90 seconds for a warm
`full` run, with about 75 seconds spent rebuilding the five-target release
matrix twice. The isolated implementation gate completed in about 10 seconds.
A tracked Stop hook also added one fast gate to every agent turn while
remaining sensitive to the PATH-selected local Go installation.

## Decision

- `full` is the ordinary implementation gate: `fast`, vet, race, tidy-diff,
  and Git whitespace checks.
- `security`, `release`, and `public` remain complete standalone profiles.
- Pull-request CI runs `full` and security/public boundary checks in parallel.
- Release preflight invokes all four required profiles explicitly.
- The repository installs no tracked automatic Codex Stop hook.
- CI caches may accelerate ordinary jobs; release reproducibility retains its
  own two isolated build caches.

## Consequences

Routine feedback is substantially shorter without removing architecture,
catalog, behavior, race, security, public-boundary, or release guarantees.
Maintainers must name the specialized profiles that apply to a change.

## Mechanical enforcement

`scripts/check.sh` owns isolated profiles, CI composes implementation and
boundary jobs, release workflow lint requires all four preflight invocations,
and repository guard no longer requires a Stop hook.

## Compatibility and migration

Public commands, output, configuration, state, and artifacts do not change.
Local automation becomes opt-in.

## Security and public-boundary impact

No credential, destination, dependency, or publication boundary is added.
Security and public checks remain required in pull-request CI.

## Validation

Run every named profile and `git diff --check`.

## Reconsideration signals

Supersede this ADR if ordinary CI latency, release cost, or missed-boundary
incidents show that profile composition no longer provides useful separation.
