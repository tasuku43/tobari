# Presentation Evidence: Observational first-use reads

## Frozen semantic corpus

- Typed fixture path: To be created under `internal/cli/testdata`
- Fixture SHA-256: Pending
- Presentation-independent answer-key path: To be created beside the fixture
- Answer-key SHA-256: Pending
- Tasks/dimensions: Context collection/item, auth status, lifecycle status/list;
  requested Context/root scope and persisted-versus-synthetic identity
- Cases: fresh installation, explicit absent, known empty, legacy observed but
  uncommitted, unsafe/corrupt, existing default, concurrent reads
- Exact next argv: `context create`, `cluster up`, `context list`, `auth status`,
  or exact scoped help as declared by the owning command

## Semantic eligibility

- [ ] One read distinguishes persisted state from a synthetic display default.
- [ ] No source or filesystem inspection is required to interpret absence.
- [ ] Empty collections retain exact local scope.
- [ ] Absent, empty, legacy, unsafe, and existing remain distinct.
- [ ] A synthetic display name cannot create a stable identity or authority.
- [ ] Next actions do not claim initialization occurred.

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
- Tokenizer name/version/configuration: Pending
- Platform facts: private fresh/read-only XDG trees and fixed synthetic state
- Invalidation rule: store-state type, fixture, answer key, schema, or renderer change

## Experiment outcome

- Outcome: inconclusive
- Eligible candidates: Pending
- Failed/invalidated candidates: Pending
- Raw evidence retained at: task-owned directory outside repository
- Unscored gates: zero durable writes and zero external mutations

## Product compatibility decision

- Decision owner: Tobari maintainers
- Selected presentation: Pending
- Compatibility rationale: Pending
- Schema/version impact: coordinated with `machine-output-contract-closure`
- Rollout and rollback: Pending
- Relationship to experiment outcome: Pending

## Security and execution boundary

- [ ] Synthetic public-safe fixtures only.
- [ ] Regular symlink-free evidence paths.
- [ ] Purpose-bound fake runners with finite output/time.
- [ ] No network or live-model canonical gate.
