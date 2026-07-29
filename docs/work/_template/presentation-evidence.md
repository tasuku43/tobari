# Presentation Evidence: Short outcome

Use this optional record when changing a default human or agent presentation in
a way that could alter interpretation, command selection, canonical-reference
reuse, recovery, or context cost. It complements the work packet; it is not a
live-model gate or a substitute for the public output contract.

## Frozen semantic corpus

- Typed fixture path:
- Fixture SHA-256:
- Presentation-independent answer-key path:
- Answer-key SHA-256:
- Declared task and each applicable target, parent, or scope dimension:
- Interpretation-relevant absent, empty, zero, false, unresolved, and bounded cases:
- Canonical references and exact next argv:

The answer key must be checked against the typed fixture before either the old
or new renderer is evaluated. Generate both presentations from this same input;
do not transcribe separate semantic fixtures for each candidate.

## Semantic eligibility

- [ ] The answer and exact next argv are available from one task invocation.
- [ ] Routine success requires zero undeclared joins, parsers, provider-notation
      interpretation, source inspection, or exploratory calls.
- [ ] Every canonical reference remains complete and byte-preserving.
- [ ] When the task is scoped, exact scope remains available when its collection
      is empty.
- [ ] Every interpretation-relevant absent, empty, zero, false, unresolved, and
      bounded state retains its declared distinction.
- [ ] Same-name, adjacency, order, quoted-prose, raw-notation, unknown-parent,
      out-of-window, and indentation canaries create no unsupported inference.
- [ ] Recovery answers obey the executable next-command grammar, including any
      intentional nonzero-exit scenario.

A candidate that fails semantic eligibility is ineligible regardless of byte,
token, latency, or subjective-preference results.

## Reproducible comparison

| Evidence | Before | After |
|---|---:|---:|
| Golden path |  |  |
| Golden SHA-256 |  |  |
| UTF-8 bytes |  |  |
| Tokens |  |  |
| Task invocations |  |  |
| External reconstruction steps |  |  |

- Golden generator or command:
- Tokenizer name and exact version:
- Tokenizer configuration:
- Platform/runtime facts that affect the measurement:
- Invalidation rule when fixture, answer key, renderer, or tokenizer changes:

Byte and token measurements are secondary evidence after semantic eligibility;
they are not standalone promotion thresholds.

## Experiment outcome

- Outcome: `winner`, `combination`, `inconclusive`, or `invalidated`
- Eligible candidates:
- Failed or invalidated candidates and reasons:
- Raw evidence retained at:
- Documented gates not implemented by the scorer:

Do not rewrite an inconclusive or invalidated comparison as a winning benchmark.
Retain failed runs when they explain a corrected oracle, protocol, or security
boundary.

## Product compatibility decision

- Decision owner:
- Selected presentation:
- Compatibility rationale:
- Schema/version impact:
- Rollout and rollback:
- Relationship to the experiment outcome:

The product owner may make a compatibility decision after an inconclusive
experiment, but the decision and the benchmark result remain separate records.

## Security and execution boundary

- [ ] Fixtures and evidence are synthetic and public-safe.
- [ ] Artifact paths are repository-rooted regular files reached without
      symbolic links.
- [ ] Any subprocess has a purpose-bound executable/argv, finite timeout,
      bounded output, private temporary storage, and no inherited secrets.
- [ ] Network and tools are disabled unless the accepted protocol requires and
      documents them.
- [ ] Live-model evaluation, when explicitly chosen, is separate from
      `task check`; static fixture/schema checks remain deterministic.
