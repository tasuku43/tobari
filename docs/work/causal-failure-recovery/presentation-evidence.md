# Presentation Evidence: Causal failure diagnosis and safe recovery

This record freezes the interpretation requirements for failure output. The
examples are design candidates from verified current paths, not implementation
evidence. Final before/after goldens must be generated from the same typed
fixtures.

## Frozen semantic corpus

- Typed fixture path: planned
  `internal/cli/testdata/causal_failure_recovery_fixture.json`
- Fixture SHA-256: record after fixture creation
- Presentation-independent answer-key path: planned
  `internal/cli/testdata/causal_failure_recovery_answer.json`
- Answer-key SHA-256: record after fixture creation
- Declared tasks and dimensions: first-use `tobari`, `cluster up`, `cluster
  status`, `context create`, current-directory Workspace ensure/attach, and
  direct child execution; fixed command targets remain Catalog-owned; no new
  opaque references.
- Interpretation-relevant cases: read versus mutation; before-action zero
  change; action attempted with unknown result; proved partial state; confirmed
  mutation followed by verification/presentation failure; attachment failure;
  nonzero child; absent Engine; unsupported/unparseable version; hostile Docker
  detail; empty next actions where no recovery is valid.
- Canonical references and exact next argv: `tobari doctor`, `tobari cluster
  status`, `tobari status`, and `tobari context list` as selected by the exact
  command Catalog; no backend command or reconstructed argument-bearing action.

## Proposed common frame

The exact styling remains subject to the same-fixture comparison. Semantically,
human text adds two rows to the existing stable frame:

```text
✗ Command failed
Message     <safe task-owned explanation>
Kind        <kind>
Code        <code>
Phase       <closed phase>
Changes     <closed change_state plus safe explanation>
Retryable   <yes|no>
Retry after <duration|none|unknown>
Next        tobari <exact catalog path>
            <catalog-owned reason>
```

JSON exposes the same `phase` and `change_state` values. The human explanation
after `Changes` must not introduce a stronger state than the enum.

## Scenario comparisons

### 1. Docker unavailable during first use

Current semantic result:

```text
✓ Context created: default
✗ Command failed
Message   Cluster startup did not complete; inspect status before retrying
Code      cluster_start_failed
Next      tobari cluster status
```

The prior success line proves a retained Context, but the fault itself does not
encode it. It also does not distinguish generic Docker readiness from later
startup.

Accepted semantic result:

```text
✗ Command failed
Message   Docker Engine readiness could not be established through the selected Docker context.
Kind      unavailable
Code      docker_engine_unavailable
Phase     precondition
Changes   none
Next      tobari doctor
          Inspect the generic Docker prerequisites before starting a Workspace.
```

No Context or Docker mutation precedes this result. The message does not say an
Engine or named backend is stopped.

### 2. Cluster startup failed after action was attempted

Current semantic result:

```text
✗ Command failed
Message   Cluster startup did not complete; inspect status before retrying
Code      cluster_start_failed
Retryable no
Next      tobari cluster status
```

Accepted semantic result when no exact subset can be proved:

```text
✗ Command failed
Message   Shared-service startup was attempted, but its resulting state is unknown.
Kind      unavailable
Code      cluster_start_failed
Phase     mutation
Changes   unknown
Next      tobari cluster status
          Inspect shared-service state before another startup.
```

A separate fixture may use `partial` only when a typed receipt identifies a
confirmed proper subset. "Possible partial" alone remains `unknown`.

### 3. Workspace logical creation confirmed, runtime reconciliation failed

Current semantic result:

```text
✗ Command failed
Message   inspect the selected project before retrying
Code      runtime_reconcile_failed
Next      tobari status
```

Accepted semantic result when the receipt proves retained logical state/home:

```text
✗ Command failed
Message   The Workspace was recorded, but its runtime did not become ready.
Kind      unavailable
Code      runtime_reconcile_failed
Phase     mutation
Changes   partial
Next      tobari status
          Inspect the retained Workspace before another reconcile.
```

If the receipt is absent, the same causal path must use `unknown` and omit the
retention claim.

### 4. Workspace attachment failed after ensure completed

Current semantic result:

```text
✗ Command failed
Message   Workspace session could not be started
Code      enter_failed
Next      tobari status
```

Accepted semantic result:

```text
✗ Command failed
Message   The Workspace is ready, but this terminal session did not start.
Kind      internal
Code      enter_failed
Phase     attachment
Changes   confirmed
Next      tobari status
          Inspect the reusable Workspace before entering again.
```

The final clause is eligible only after injected cleanup tests prove no
attachment-owned route, grant, controller, or live child survives.

### 5. Context creation storage failure

Current semantic result:

```text
✗ Command failed
Message   Context could not be created
Code      context_create_failed
Next      tobari context list
```

Accepted conservative semantic result:

```text
✗ Command failed
Message   Context creation did not return a classified storage outcome.
Kind      rejected
Code      context_create_failed
Phase     mutation
Changes   unknown
Next      tobari context list
          Inspect the Context collection before creating again.
```

Validation, duplicate-name, and Runtime-selection failures have separate codes
and `none`; an invalid report after a confirmed create receipt is
`verification/confirmed`.

### 6. Direct child exits 37

Current and accepted semantic result:

```text
Workspace session closed.
Workspace remains available.

Resume: tobari
```

The host process exits 37. It emits no `Command failed`, `kind`, `code`,
`phase`, `change_state`, or recovery object. Tests must use a child status that
collides with a Tobari-classified exit as well as 37 to prove that dispatch
context, not the number alone, owns interpretation.

## Semantic eligibility

- [ ] Causal identity, phase, change state, and exact next argv are available
      from the same failure invocation.
- [ ] Human and JSON projections answer the same questions without parsing
      progress lines, message wording, row order, or indentation.
- [ ] `none`, `partial`, and `confirmed` appear only with typed proof; possibility
      or missing evidence remains `unknown`.
- [ ] Docker context/provider names, socket paths, stderr, and process state do
      not become causal or recovery authority.
- [ ] Read failures retain `not_applicable`; mutation failures never use it.
- [ ] Direct child status remains distinguishable from Tobari fault exit classes
      without an exploratory call.
- [ ] Recovery obeys the exact Catalog grammar and partial/confirmed/unknown
      mutation recovery is read-only.
- [ ] Hostile same-name, adjacent success, quoted error, raw path, version
      suffix, provider-name, and numeric-exit canaries create no unsupported
      inference.

## Reproducible comparison

| Evidence | Before | After |
|---|---:|---:|
| Golden path | planned `causal_failure_before.txt` | planned `causal_failure_after.txt` |
| Golden SHA-256 | record after generation | record after generation |
| UTF-8 bytes | record after generation | record after generation |
| Tokens | optional; record only with pinned tokenizer | optional; record only with pinned tokenizer |
| Task invocations | 1 per scenario | 1 per scenario |
| External reconstruction steps | 1 or more for mutation state/cause | 0 |

- Golden generator or command: focused CLI renderer test using the single typed
  fixture and answer key.
- Tokenizer name and exact version: not selected; byte and semantic evidence is
  sufficient unless implementation review requests token measurement.
- Platform/runtime facts: color disabled, English locale, fixed terminal width,
  synthetic provider-neutral observations, no live secrets or paths.
- Invalidation rule: regenerate both candidates and hashes when fixture, answer
  key, fault schema, Catalog declaration, renderer, or tokenizer changes.

## Experiment outcome

- Outcome: `inconclusive` until typed fixtures and goldens exist
- Eligible candidates: the common frame and conservative state wording above,
  subject to exact same-fixture rendering.
- Failed or invalidated candidates and reasons: provider-specific startup advice
  is ineligible; prose-only change claims are ineligible; possible-partial shown
  as `partial` is ineligible; wrapping child nonzero is ineligible.
- Raw evidence retained at: this temporary packet and planned synthetic CLI
  testdata only.
- Documented gates not implemented by the scorer: mutation receipt validity,
  zero side effects, provider non-authority, cleanup, and Catalog agreement are
  deterministic test duties.

## Product compatibility decision

- Decision owner: Tobari maintainer
- Selected presentation: one structured fault frame with causal identity,
  phase, change state, retry facts, and exact next Tobari action; direct child
  outcome remains outside it.
- Compatibility rationale: it makes existing safety facts explicit without
  adding a command, backend concept, or automatic action.
- Schema/version impact: structured error schema must advance; success schemas
  and persisted state remain unchanged.
- Rollout and rollback: one release-boundary schema update; rollback removes the
  fields/preflight and requires no state migration.
- Relationship to the experiment outcome: product semantics are accepted;
  exact layout remains pending deterministic evidence.

## Security and execution boundary

- [ ] Fixtures and evidence are synthetic and public-safe.
- [ ] Artifact paths are repository-rooted regular files reached without
      symbolic links.
- [ ] Any subprocess uses fixed generic Docker argv, finite timeout, bounded
      output, controlled environment, and no inherited secrets.
- [x] No network destination, provider tool, or live model is required.
- [x] Live-model evaluation remains outside `task check`; static schema and
      fixture checks are deterministic.
