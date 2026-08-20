# ADR 0060: Complete Context management through one create wizard and guarded deletion

- Status: Accepted
- Date: 2026-08-18

## Context

Context creation exposed deterministic flags but no human completion path. A
bare `context create` failed because `--name` was required, even though source
access and the complete method-policy model are choices users need to see
together. Rich Context list facts were packed into one tab-separated row and
collapsed at ordinary terminal widths. Context creation also had no symmetric
safe deletion outcome.

## Decision

An argument-free `context create` is a terminal-only wizard. It collects a
validated Context name, direct source access, and one complete HTTP method
policy: an extension-method default plus exact standard-method decisions, each
`allow`, `exact_review`, or `deny`. It performs one create mutation only after
the full selection. Any explicit command input selects the existing
deterministic direct mode and requires `--name`; redirected or JSON
argument-free use fails before mutation.

The selected preset remains the destination and baseline source. A wizard
method policy replaces only its method policy. Positive baseline rules made
unreachable by method Deny are removed before normalization; destination
ceilings, exact Denies, and native-readiness filtering remain terminal. The
composed normalized snapshot and digest are immutable Context authority.

Human `context list` uses vertical cards with filesystem and method-policy rows;
schema-1 JSON is unchanged.

Human `context show` uses an outcome-first summary for the selected Context's
current state, boundary, runtime, authentication, and continuation. The
optional boolean `--details` expands the same single read into sectioned host
diagnostics. Schema-1 JSON is complete and byte-identical for either flag
value.

`context delete --name NAME` is a destructive write to the fixed Context
catalog. It rejects the foundational `default` Context, the current Context,
and any Context with a durable Workspace binding. It has no force option and no
implicit replacement selection. Success removes only the exact owner-only
Context and Context-ID authentication stores, preserves project roots and
shared runtime images, and reports whether shared policy needs reconciliation.

## Consequences

- Humans can create a complete capability envelope without memorizing flags.
- Automation retains a prompt-free direct path.
- A method-policy choice cannot leave positive baseline authority above a
  terminal method Deny.
- Context lifecycle cleanup is explicit and cannot orphan a Workspace binding.
- Context collections are longer vertically but remain readable as method
  overrides grow.
- Context inspection keeps revisions and host stores available without making
  them compete with the ordinary readiness decision.

## Verification

Tests cover line and raw-terminal wizard selection, invalid names,
cancellation/non-terminal rejection before mutation, composed snapshot
normalization, unchanged JSON, vertical text cards, catalog mutation contracts,
protected/current/Workspace guards, exact store removal, and repository gates.
Context-show presentation tests use one typed fixture and answer key for the
concise and detailed goldens, exact inactive-Context continuation, complete
method overrides, detail-only diagnostics, and identical JSON.
