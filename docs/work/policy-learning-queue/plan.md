# Plan: Low-friction policy learning and compaction

## Chosen approach

Extend the existing `policy.learning` capability with two discover/act
reference chains. Denial candidates derive from retained validated Gateway
records; compaction candidates derive from current validated learned rules.
Keep `policy tail` read-only and make both mutations pass through one
infrastructure policy-file transaction and the existing OPA activation path.

## Alternatives

- Accept host/method/path directly on the mutation: rejected because it invites
  shell quoting mistakes and bypasses exact observed evidence.
- Make tail approve interactively: rejected because one command would combine
  discovery with access-changing action.
- Compact after every approval: rejected because sparse observations would
  generalize too early and make each ordinary approval harder to reason about.
- Support arbitrary wildcard syntax: rejected because static bounded
  path-prefix matching is easier to validate and test.

## Risks and controls

- Stale Docker logs: return a typed not-found fault and rediscover current candidates.
- Over-broad prefix: require three exact sources, fixed host/method, two path
  segments before the wildcard boundary, explicit approval, and generated
  positive/boundary tests.
- Partial file write: preflight in a private temporary directory and use
  same-directory atomic rename only after tests pass.
- Activation ambiguity: use the existing mutation invoker and OPA-only
  activation; unknown post-action outcomes are non-retryable and reconcile via reads.
- Manual policy coexistence: preserve unknown JSON members and validate only
  the CLI-owned learned-rule member.

## Verification

- Domain tests for IDs, exact matching, grouping, stale IDs, and canaries.
- Application tests for reference binding and zero writes before validation.
- Infrastructure tests for bounded strict JSON, preflight ordering, atomic replacement, and activation.
- CLI catalog/output/help and opaque-reference round-trip tests.
- Docker integration for exact approval and prefix compaction.
