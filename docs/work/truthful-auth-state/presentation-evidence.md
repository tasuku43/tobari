# Presentation Evidence: Truthful authentication state

## Frozen semantic corpus

- Typed fixture path: `internal/cli/testdata/truthful-auth-state-fixture.json`
- Fixture SHA-256: `121c173f118213ceb1bc7546504851659dc50dae89c47be93d2b36448e57ca66`
- Fixture bytes: 13,095
- Presentation-independent answer-key path:
  `internal/cli/testdata/truthful-auth-state-answer-key.json`
- Answer-key SHA-256: `6434c5947a3c5ca9b2aa3d1de86c0c548c215e080f4d63ffcc1b6d2ab9fd7cc3`
- Answer-key bytes: 2,718
- Task/dimensions: one exact Context; provider configuration and Broker
  availability; zero/one eligible Workspace; logical project ID, canonical
  root, Context ID, registry revision/digest, and Broker binding observation;
  confirmed changed/no-change mutation outcome
- Ten cases: current, missing, stale, configured with zero Workspaces,
  enumeration unavailable, Broker locked with exhaustive enumeration, mixed
  unresolved, confirmed changed login, confirmed changed logout, and no-op logout
- Exact Workspace action: canonical working directory plus argv
  `tobari --context <validated-context>` only for a fully known missing/stale row
- Exact recovery is catalog-owned `auth status`, `context list`, or exact auth
  help; unsupported login does not point to `auth import`.

## Semantic eligibility

- [x] Provider configuration and Workspace activation are separately answerable.
- [x] No source, vault, handle, or external parser is required.
- [x] Empty provider/Workspace collections retain exact Context scope.
- [x] Current/stale/missing/unavailable/no-change remain distinct.
- [x] Same account labels or provider order create no identity inference.
- [x] Re-entry appears only with authoritative stale or missing projection evidence.
- [x] No-op receipt cannot imply revocation or file removal.
- [x] Mixed unavailable and re-entry evidence is unresolved and supplies no action.
- [x] Routine-success external processing count is zero for every answer-key case.

## Reproducible comparison

| Evidence | Before | After |
|---|---:|---:|
| Golden semantic corpus | none | 10 typed cases |
| Golden SHA-256 | none | pinned fixture and answer key |
| UTF-8 bytes | not applicable | 13,095 + 2,718 |
| Presentation candidates | one misleading behavior | one contract-derived behavior |
| Task invocations for one result | 1 | 1 |
| External reconstruction steps | not declared | 0 |

- Golden verifier: `go test ./internal/cli -run
  TestTruthfulAuthStateTypedCorpusClosesInterpretationBoundaries`
- Tokenizer: not used; candidate selection was not scored by token count.
- Platform facts: repository-authored synthetic broker/runtime state and
  explicit terminal modes; no network or live provider state.
- Invalidation rule: domain state, fixture, answer key, schema, selector,
  catalog, renderer, bound, or authority-correlation rule changes.

## Presentation-specific typed evidence

- `TestAuthLoginSelectorMarksConfiguredProviderAndWarnsBeforeRotation` pins the
  configured marker and rotation/revocation warning before mutation.
- Selector cancellation tests pin shared neutral exit 11 and zero login calls.
- Status CLI tests pin no action for current projection and exact canonical cwd
  plus Context argv for stale projection.
- Logout CLI tests pin distinct `changed` and `no_change` headings and forbid
  removal, revocation, and re-entry claims in the no-op result.
- Catalog tests pin schema 4, finite enums, nested Workspace shape, and exact
  supported recovery commands.

## Experiment outcome

- Outcome: selected by semantic contract, not by subjective visual scoring.
- Eligible candidates: one; facts and actions are mechanically derived from
  typed state after authority correlation.
- Rejected prior behavior: configured-implies-re-entry, running-process-
  implies-current, and absent-logout-implies-removal.
- Raw evidence retained at: the pinned repository corpus; no secret or live
  provider transcript is retained.
- Unscored gates: none. Secret absence and exact authority correlation are
  executable tests rather than presentation scores.

## Product compatibility decision

- Decision owner: Tobari maintainers
- Selected presentation: separate provider configuration, aggregate coverage,
  per-Workspace projection rows, exact conditional action, and changed/no-change receipt
- Compatibility rationale: schema 3 could not truthfully express the new
  activation and mutation distinctions without consumers inferring semantics.
- Schema/version impact: public auth result/status envelope remains `auth` and
  advances explicitly to schema 4.
- Rollout and rollback: pre-v1 release notes identify schema 4; rollback changes
  presentation only and does not rewrite credential, registry, or handle state.
- Relationship to experiment outcome: the only eligible presentation preserves
  all task-owned dimensions and has zero routine external reconstruction.

## Security and execution boundary

- [x] Synthetic no-secret fixtures only.
- [x] Regular symlink-free artifact paths.
- [x] Purpose-bound bounded subprocesses if any.
- [x] No network or live-model canonical gate.
