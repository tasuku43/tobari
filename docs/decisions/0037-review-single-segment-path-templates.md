# ADR 0037: Review single-segment path templates after two examples

- Status: Accepted
- Date: 2026-08-15
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, harness, compatibility, and public boundary
- Revises: ADR 0024, ADR 0028, and the exact-policy decision retained by ADR 0030

## Context

The Permission Inbox already coalesces repeated observations of one exact HTTP
effect, stages explicit decisions, survives refresh by typed identity, and
activates a reviewed set once. It still requires a new exact approval whenever
only a resource identifier changes in the URL path. Coding agents can encounter
that pattern early enough for review fatigue to undermine the bounded-autonomy
product goal.

The retired learned-policy compaction does not solve this safely. It inferred a
directory prefix after three exact rules and authorized every descendant under
that prefix. A reviewed `/path/{id}` template instead preserves every literal
path segment and varies exactly one non-empty segment.

## Decision

Permission Inbox may propose one `path_template` HTTP allow after two distinct,
compatible retained examples. Repeated observations of one exact path count as
one example. Proposal derivation is pure typed domain logic and requires equal
Context, project, scheme, host, port, method, protocol, and segment count; HTTP
is the only eligible protocol. Exactly one safe raw segment differs, at least
one non-empty literal segment remains fixed, and ambiguous overlapping
proposals are suppressed.

The placeholder is the literal syntax `{id}` and means exactly one non-empty
raw path segment. It never matches `/`, an empty segment, `.` or `..`, a
backslash, or any percent-encoded segment in the initial contract. It does not generalize query,
fragment, scheme, host, port, method, Context, project, GraphQL operation, or
GraphQL root.

One example may come from a current exact learned allow and the other from a
pending candidate. A proposal appears only when at least one source remains
pending. This preserves the experience in which a first exact approval can be
promoted when a second resource value is later denied.

Observation and proposal rendering grant no authority. From the proposal detail
screen the human explicitly stages one of:

- Allow template: replace contributing exact allow rules with one reviewed
  template allow and cover future values in that segment;
- Allow observed exact: retain existing exact allows and allow only the pending
  exact candidates;
- Deny pending exact: retain existing decisions and deny only pending exact
  candidates.

Final Apply remains one command-owned fixed-target mutation. It rebuilds every
staged review item from fresh denials and current rules, rejects changed scope,
preflights the complete all-Context bundle, and activates one revision. A
proposal ID binds the proposed authority rather than its growing evidence set,
so a compatible third example does not silently change scope or invalidate a
stage. `policy candidates`, `policy allow --id`, and `policy deny --id` remain
exact machine and recovery workflows.

Learned state admits only `exact` and `path_template`. Prefix, glob, wildcard,
regex, multiple placeholders, multiple varying segments, and compatibility
fallbacks remain invalid. Template rules remain resettable through the existing
opaque `policy-rule` inventory/action path.

## Consequences

### Positive

- Identifier churn becomes one explicit bounded decision after the second
  distinct example rather than one approval per value.
- The user sees the exact future authority before staging it and may choose the
  exact fallback without losing existing decisions.
- Agent/vendor API names do not become compiled default policy.

### Negative

- Two examples can share structure without sharing product meaning; explicit
  review and conservative ambiguity suppression remain necessary.
- The policy data and OPA evaluator gain a second learned allow variant and its
  associated state, presentation, and release evidence.
- Fixed but unrelated startup endpoints still require separate exact decisions;
  this ADR deliberately does not introduce permission packs or batch semantics.

## Mechanical enforcement

- Domain tests fix proposal identity, two-distinct-example threshold, exact one
  varying segment, cross-dimension isolation, ambiguity suppression, and every
  unsafe path rejection.
- Application tests prove fresh revalidation, zero-write stale rejection,
  replacement of contributing exact rules, and exact allow/deny fallback.
- CLI transcript tests prove template proposals are typed, detail-only actions
  are explicit, staging is inert, refresh uses review-item identity, and final
  Apply states whether future IDs are included.
- Strict state readers accept only exact and path-template variants. Aggregate
  and OPA tests prove an unseen safe segment matches while parent, child,
  sibling, empty, encoded-separator, cross-authority, cross-method,
  cross-project, and GraphQL canaries fail closed.
- The claim-to-enforcement table and agent-readiness denial-to-retry scenario
  include template review without undeclared parsing or provider knowledge.

## Compatibility and migration

No public version contains learned policy templates. Existing exact V1 policy
sources remain valid and require no migration. The new strict variant is read
only when all required fields validate; retired prefix state remains rejected
and is never reinterpreted as a template.

## Security and public-boundary impact

The change deliberately permits future unseen raw path-segment values after an
explicit trusted-host review. It adds no external endpoint, credential,
provider operation, secret field, body identity, query identity, executable,
or network client. Fixtures use synthetic hosts, paths, principals, and values.

## Validation

```sh
task check
task security
task public:check
```

## Reconsideration signals

Revisit if real Inbox evidence shows frequent ambiguity, unsafe upstream path
decoding, user confusion about future authority, or review volume dominated by
unrelated fixed routes rather than identifier churn. Do not respond by adding
prefix or multi-segment inference without a new reviewed decision.
