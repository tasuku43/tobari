# ADR 0077: Classify causal failure recovery

- Status: Accepted
- Date: 2026-08-22

## Context

Tobari already publishes stable fault `kind` and command-specific `code`, strips
private adapter causes, and replaces runtime recovery prose with exact
Catalog-owned actions. Those facts identify what failed, but they do not say
where the failure was established or what the owning layer proved about the
requested change. Users and agents could therefore confuse a precondition
failure with an uncertain post-action result and repeat a mutation unsafely.

First-use root entry also created a durable Context before discovering whether
the generic Docker CLI, selected Engine and Context, and Compose v2 were ready
for the promised Workspace outcome. The published support floor already says
Docker Engine 24 or newer, but the routine path did not enforce it.

## Decision

`kind` plus command-specific `code` remain the sole causal identity. Structured
error schema 2 adds two orthogonal closed facts:

- `phase`: `precondition`, `observation`, `mutation`, `verification`,
  `attachment`, or `presentation`;
- `change_state`: `not_applicable`, `none`, `partial`, `confirmed`, or
  `unknown`.

The layer that owns the relevant boundary may publish only the strongest state
it proves. Pre-action failure is `none`; ordinary read failure is
`not_applicable`; an unclassified post-action result is `unknown`; a confirmed
mutation whose result write fails remains `confirmed`. Possibility alone never
becomes `partial`. Catalog declarations own these fields and exact recovery.
A runtime classification disagreement becomes an undeclared-fault contract
failure. A mutation fault with `partial`, `confirmed`, or `unknown` state may
point only to a Catalog-validated read before another mutation is chosen.

The application owns one closed `workspace_start` readiness profile containing
only generic Docker CLI, Engine version, selected Docker Context, and Compose v2
observations. Infrastructure executes fixed read-only Docker argv with a
five-second timeout and 4096-byte output bound. First use runs the profile after
human review and before Context creation. Direct `cluster up` runs the same
profile before its mutation invoker; a typed context receipt avoids repeating
it inside the composed first-use flow. Standalone `context create` remains
Docker-independent.

Docker Engine major version 24 is the supported V1 floor and is enforced from
the structured Engine-version observation. Tobari neither detects nor manages
the provider behind Docker and never emits provider-specific recovery. A direct
foreground child's nonzero exit remains the child's exact status and is not a
Tobari structured fault.

## Consequences

- Structured-error consumers must accept schema 2 and the two required fields.
- Human and JSON errors expose the same causal identity, phase, change state,
  retry facts, and exact next action without exposing upstream prose.
- Lifecycle code retains `unknown` whenever no receipt proves zero, partial, or
  confirmed change; presentation cannot strengthen that claim.
- First-use Docker failure performs zero Context, cluster, Workspace, network,
  or Docker mutation. Recovery begins with `doctor` and remains provider-neutral.
- Tests must cover the closed taxonomy, Catalog/runtime agreement, read-only
  reconciliation, Engine 23/24 boundaries, fixed readiness argv, first-use
  zero-mutation behavior, confirmed-output failure, and direct-child status.
