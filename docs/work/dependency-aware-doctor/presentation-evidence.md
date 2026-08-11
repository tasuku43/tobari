# Presentation Evidence: Dependency-aware doctor

## Frozen semantic corpus

- Typed fixture path: `internal/cli/testdata/human-presentation-foundation-fixture.json`, plus task-owned typed matrices in `internal/app/doctorcmd/service_test.go` and `internal/infra/dockerruntime/doctor_observer_test.go`
- Fixture SHA-256: `ea0c39f32078014cfb228725d013c79538eb5357814fc51a71fbec03a3078ee3`
- Presentation-independent answer-key path: `internal/cli/testdata/human-presentation-foundation-answer-key.json`, plus explicit status/blocker maps beside the typed matrices
- Answer-key SHA-256: `f2dbd3c1c819abf0da5ee05121b13178f9d2889afa9092563a4104de80e1fd32`
- Task/scope: complete doctor check inventory for one prospective root
- Cases: Docker CLI missing, Engine down, cluster absent, invalid policy,
  broker locked, independent warning, healthy, cancellation
- Exact next argv: `doctor` or `cluster up` after the named correction

## Semantic eligibility

- [x] One invocation returns the complete declared inventory.
- [x] Failed and blocked checks remain distinct.
- [x] No downstream label implies a check ran when it did not.
- [x] Empty/absent/unavailable/warning/failure distinctions survive rendering.
- [x] Recovery requires no source or raw Docker inspection.
- [x] `NO_COLOR` changes no diagnosis or Next fact.

## Reproducible comparison

| Evidence | Before | After |
|---|---:|---:|
| Golden path | Existing one-row warning | Fixed 22-row inventory with reachable root-sharing warning |
| Golden SHA-256 | Superseded | `ea0c39f32078014cfb228725d013c79538eb5357814fc51a71fbec03a3078ee3` |
| UTF-8 bytes | Not used as a selection metric | Deterministic fixture bytes pinned by SHA-256 |
| Tokens | Not used as a selection metric | Not applicable |
| Task invocations | 1 | 1 |
| External reconstruction steps | Required repeated diagnosis after false blame | 0 for supported recovery selection |

- Golden generator or command: checked-in synthetic JSON plus `go test -count=1 ./internal/cli`
- Tokenizer name/version/configuration: Not applicable; no token-count comparison selected this presentation
- Platform facts: isolated XDG, fake Docker runner, explicit terminal mode
- Invalidation rule: check graph, fixture, answer key, schema, or renderer change

## Experiment outcome

- Outcome: selected typed dependency-aware projection
- Eligible candidates: text semantic-token view, flattened TSV, recursive JSON schema 2
- Failed/invalidated candidates: stop-at-first-failure, speculative downstream failure, presentation-only suppression
- Raw evidence retained at: task-owned directory outside the repository
- Unscored gates: zero mutation and bounded external calls

## Product compatibility decision

- Decision owner: Tobari maintainers
- Selected presentation: fixed ordered rows with explicit blocked relation and task-owned recovery
- Compatibility rationale: semantic correction requires additive fields and a new blocked enum; the pre-1.0 JSON schema advances explicitly
- Schema/version impact: doctor JSON schema 1 to 2; TSV adds `BLOCKED_BY`, `RECOVERY_ACTION`, and `NEXT_COMMAND`
- Rollout and rollback: catalog and renderer advance atomically; rollback must restore schema 1 rather than mapping blocked to a false failure
- Relationship to experiment outcome: selected candidate is the only one that preserves the typed matrix without external reconstruction

## Security and execution boundary

- [x] Synthetic public-safe fixtures only.
- [x] Regular symlink-free evidence paths.
- [x] Purpose-bound fake runners with finite output/time.
- [x] No network or live-model canonical gate.
