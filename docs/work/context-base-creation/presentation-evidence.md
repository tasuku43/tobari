# Presentation Evidence: Base-first Context creation

This record will compare the current Context-create review with the accepted
Base-first flow from one presentation-independent corpus.

## Frozen semantic corpus

- Typed fixture path: To be created under `internal/cli/testdata/`.
- Fixture SHA-256: Pending.
- Presentation-independent answer-key path: To be created under
  `internal/cli/testdata/`.
- Answer-key SHA-256: Pending.
- Declared task and each applicable target, parent, or scope dimension: one
  fixed Context-catalog create; Base is a draft source and not a mutation
  target, parent, or persisted scope.
- Interpretation-relevant absent, empty, zero, false, unresolved, and bounded
  cases: zero Contexts, current versus non-current Base, recommended Base,
  absent bootstrap, empty learned/Workspace/Attachment state, unchanged versus
  edited draft, declined reset, and unavailable Runtime.
- Canonical references and exact next argv: no opaque reference is introduced;
  exact next argv includes `tobari context create --base <context-name>` and the
  post-create `tobari --context <new-name>` continuation.

## Semantic eligibility

- [ ] The answer and exact next argv are available from one task invocation.
- [ ] Routine success requires zero undeclared joins, parsers, source inspection,
      or exploratory calls.
- [ ] The complete fixed mutation target remains explicit and no Base lineage is
      inferred from presentation.
- [ ] Absent bootstrap and all excluded lower-lifetime state remain distinct.
- [ ] Same-name, current-marker, adjacency, ordering, prose, and indentation
      create no unsupported Base or inheritance relationship.
- [ ] Recovery answers obey the executable next-command grammar.

## Reproducible comparison

| Evidence | Before | After |
|---|---:|---:|
| Golden path | Pending | Pending |
| Golden SHA-256 | Pending | Pending |
| UTF-8 bytes | Pending | Pending |
| Task invocations | Pending | Pending |
| External reconstruction steps | Pending | Pending |

- Golden generator or command: Pending.
- Platform/runtime facts that affect the measurement: terminal raw and bounded
  line modes are evaluated separately from one typed fixture.
- Invalidation rule: any fixture, answer key, renderer, catalog input, or
  Context-create semantic change invalidates the comparison.

## Experiment outcome

- Outcome: `inconclusive` until implementation evidence exists.
- Eligible candidates: current flow and accepted Base-first flow, pending
  semantic fixtures.
- Failed or invalidated candidates and reasons: Base-inside-Customize is
  product-rejected because it mixes abstraction levels; current-only selection
  is product-rejected because it overloads `context use`.
- Raw evidence retained at: this temporary packet and repository testdata until
  completion promotion/removal.
- Documented gates not implemented by the scorer: catalog validation, mutation
  isolation, storage atomicity, and lower-lifetime authority exclusion remain
  deterministic tests rather than presentation scores.

## Product compatibility decision

- Decision owner: Product owner.
- Selected presentation: dedicated Base chooser before name/settings when
  persisted Contexts exist; current Context initially selected; `--base` skips
  the chooser; zero-Context first use remains direct.
- Compatibility rationale: one additional step appears only when a meaningful
  Base choice exists and preserves the current complete review/customize model.
- Schema/version impact: no stored lineage or machine-output change intended;
  exact audit pending.
- Rollout and rollback: ordinary standalone Contexts remain schema-compatible;
  removing the chooser restores the prior presentation.
- Relationship to the experiment outcome: direct product decision precedes the
  deterministic same-fixture evidence.

## Security and execution boundary

- [ ] Fixtures and evidence are synthetic and public-safe.
- [ ] Artifact paths are repository-rooted regular files reached without links.
- [ ] No subprocess, network, credential, or external tool is required for the
      presentation corpus.
- [ ] Static fixture/schema checks remain deterministic and gate completion.
