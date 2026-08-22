# Presentation Evidence: Runtime source Base creation

Use this record for the conditional default Base chooser. It complements the
work packet; it is not a live-model gate or a substitute for the public output
contract.

## Frozen semantic corpus

- Typed fixture path: planned `internal/cli/testdata/runtime_create_base_report.json`
- Fixture SHA-256: record after fixture creation
- Presentation-independent answer-key path: planned
  `internal/cli/testdata/runtime_create_base_answer.json`
- Answer-key SHA-256: record after fixture creation
- Declared task and each applicable target, parent, or scope dimension:
  `runtime.create`; fixed Runtime-catalog creation scope; target new name
  `mobile`; Base candidates `standard`, current editable `frontend`, and
  current editable `backend`; no opaque target or parent input.
- Interpretation-relevant absent, empty, zero, false, unresolved, and bounded
  cases: absent explicit Base; zero managed sources; one or several managed
  sources; managed source with zero revisions; managed source with revisions
  that remain irrelevant; source readiness is not inferred from history.
- Canonical references and exact next argv: no opaque references;
  `tobari runtime build --name mobile` after creation; Context selection remains
  a separate later task and is not invented by the chooser.

## Semantic eligibility

- [ ] The selected Base name and exact new Runtime name are available from one
      creation interaction without parsing proximity or order.
- [ ] Routine success requires zero undeclared joins, parsers, revision-to-mode
      reconstruction, source inspection, or exploratory calls.
- [ ] No ordinal revision is presented as an editable source Base.
- [ ] The standard-only empty managed collection remains explicit and skips a
      redundant chooser without implying a missing Runtime catalog.
- [ ] Zero revision history and one-or-more revision history do not change
      which editable source a managed Runtime name denotes.
- [ ] Same-name, adjacency, order, quoted-prose, raw-path, ready-head, and
      indentation canaries create no lineage or snapshot inference.
- [ ] Cancellation and recovery answers obey the executable next-command grammar.

## Reproducible comparison

| Evidence | Before | After |
|---|---:|---:|
| Golden path | planned `runtime_create_before.txt` | planned `runtime_create_base_summary.txt` |
| Golden SHA-256 | record after generation | record after generation |
| UTF-8 bytes | record after generation | record after generation |
| Tokens | optional; record only with pinned tokenizer | optional; record only with pinned tokenizer |
| Task invocations | 1 | 1 |
| External reconstruction steps | 1 manual source reconstruction for a nearby Runtime | 0 |

- Golden generator or command: focused CLI fixture renderer test, using the
  same typed fixture and answer key for both candidates.
- Tokenizer name and exact version: not selected; byte evidence is sufficient
  unless implementation review requests token measurement.
- Tokenizer configuration: not applicable until a tokenizer is selected.
- Platform/runtime facts that affect the measurement: color disabled; English
  locale; fixed terminal width/height; raw and bounded line-mode variants.
- Invalidation rule when fixture, answer key, renderer, or tokenizer changes:
  regenerate both candidates and hashes; never compare separately transcribed
  semantic inputs.

## Experiment outcome

- Outcome: `inconclusive` until implementation fixtures exist
- Eligible candidates: accepted direction is a dedicated Base-first chooser;
  exact selector-versus-combined-review rendering remains to be compared.
- Failed or invalidated candidates and reasons: immutable revision chooser is
  ineligible because frozen `0400` files cannot reconstruct executable modes;
  putting Base beside build settings is ineligible because it mixes the source
  initializer with peer source facts.
- Raw evidence retained at: this temporary packet and planned CLI testdata.
- Documented gates not implemented by the scorer: source-copy security,
  mutation output, atomic cleanup, and catalog binding remain test/gate duties.

## Product compatibility decision

- Decision owner: Tobari maintainer
- Selected presentation: conditional dedicated Base-first chooser, with
  `standard` initially selected and no chooser when it is the sole Base.
- Compatibility rationale: it exposes the higher-level source initializer only
  when a real choice exists while preserving redirected/JSON omission and the
  existing explicit `--name` automation path.
- Schema/version impact: no Runtime report or persisted manifest schema change.
- Rollout and rollback: additive `--base` plus conditional TTY presentation;
  removing them restores prior creation and leaves created standalone Runtimes valid.
- Relationship to the experiment outcome: product direction is accepted;
  frozen evidence must still choose the smallest eligible rendering.

## Security and execution boundary

- [ ] Fixtures and evidence are synthetic and public-safe.
- [ ] Artifact paths are repository-rooted regular files reached without symbolic links.
- [ ] Any subprocess has a purpose-bound executable/argv, finite timeout,
      bounded output, private temporary storage, and no inherited secrets.
- [x] Network and tools are disabled; this is an installation-local presentation.
- [x] Live-model evaluation is unnecessary and remains outside `task check`.
