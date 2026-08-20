# Presentation Evidence: Separate the Tobari product from the Workspace resource

## Frozen semantic corpus

- Typed fixture path: To be selected during the Understand phase
- Fixture SHA-256: Pending
- Presentation-independent answer-key path: To be selected during the Understand phase
- Answer-key SHA-256: Pending
- Declared task and each applicable target, parent, or scope dimension:
  lifecycle current-directory target, selected Context, Workspace inventory,
  and applicable policy-recovery scope
- Interpretation-relevant absent, empty, zero, false, unresolved, and bounded cases:
  absent Workspace, synthetic default Context, existing detached Workspace,
  empty inventory, and bounded pending-permission state
- Canonical references and exact next argv: Preserve all fixture-owned values
  and exact recovery argv unchanged

## Semantic eligibility

- [ ] The answer and exact next argv are available from one task invocation.
- [ ] Routine success requires zero undeclared joins, parsers,
      provider-notation interpretation, source inspection, or exploratory
      calls.
- [ ] Every canonical reference remains complete and byte-preserving.
- [ ] Exact scope remains available for an empty scoped collection.
- [ ] Every interpretation-relevant state distinction is retained.
- [ ] Names, adjacency, order, prose, and indentation create no unsupported
      identity or relationship.
- [ ] Recovery answers obey the executable next-command grammar.

## Reproducible comparison

| Evidence | Before | After |
|---|---:|---:|
| Golden path | Pending | Pending |
| Golden SHA-256 | Pending | Pending |
| UTF-8 bytes | Pending | Pending |
| Tokens | Pending | Pending |
| Task invocations | Pending | Pending |
| External reconstruction steps | Pending | Pending |

- Golden generator or command: Pending
- Tokenizer name and exact version: Pending if token evidence is material
- Tokenizer configuration: Pending
- Platform/runtime facts that affect the measurement: Pending
- Invalidation rule: Invalidate when the fixture, answer key, affected
  renderer, schema, or selected tokenizer changes

## Experiment outcome

- Outcome: Pending
- Eligible candidates: Pending
- Failed or invalidated candidates and reasons: Pending
- Raw evidence retained at: Pending
- Documented gates not implemented by the scorer: Semantic schema and recovery
  checks remain executable repository tests

## Product compatibility decision

- Decision owner: Tobari maintainers
- Selected presentation: Tobari as product; Workspace as isolated resource
- Compatibility rationale: Accepted product-owner decision on 2026-08-21
- Schema/version impact: Pending the public machine-field audit
- Rollout and rollback: Pending the audit; prose-only changes are source-revertible
- Relationship to the experiment outcome: The product vocabulary decision is
  accepted; the fixture determines semantic eligibility, not product naming

## Security and execution boundary

- [ ] Fixtures and evidence are synthetic and public-safe.
- [ ] Artifact paths are repository-rooted regular files reached without
      symbolic links.
- [ ] Any subprocess is purpose-bound, finite, bounded, private, and
      secret-free.
- [ ] Network and external tools remain disabled.
- [ ] Static fixture/schema checks remain deterministic and no live-model gate
      is introduced.
