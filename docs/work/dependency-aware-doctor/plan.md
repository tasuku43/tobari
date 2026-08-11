# Work Plan: Make doctor diagnose prerequisites without false blame

- Status: Active
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Model doctor as a finite task-owned check graph. Each check declares only the
prerequisites needed to run. Application orchestration observes root causes
once, continues independent checks, and emits typed blocked results for
dependents. Presentation summarizes root failures and then blocked checks
without assigning speculative blame. Recovery combines a human prerequisite
step with the exact catalog command to rerun after correction.

## Alternatives considered

### Stop at the first failure

Rejected because it violates the complete-report contract and forces repeated discovery rounds.

### Run every check and retain all downstream failures

Rejected because unavailable prerequisites cannot prove semantic failure and create false diagnosis.

### Hide dependent failures only in text

Rejected because JSON/TSV and agent interpretation would remain incorrect.

## Design

### Public contract

`doctor` remains `RoleUtility`, `EffectRead`, complete delivery, and the same
capability. The report schema adds or clarifies a blocked state with a typed
prerequisite identifier. `diagnostic_failed` remains exit 10 when required root
checks fail. Human output includes one concrete prerequisite action and exact
rerun command; agent output obtains the same facts structurally.

### Layer changes

- Domain: finite check IDs, statuses, prerequisite relations, and validation.
- Application: dependency-aware orchestration and complete-result assembly.
- Infrastructure: narrow independent observation ports and explicit
  unavailable results; no speculative domain labels.
- CLI/catalog: blocked rendering, root-cause summary, schema/help/fault updates.

### Data and control flow

```text
finite check graph -> observe prerequisites once
  -> run ready independent checks
  -> mark dependents blocked with exact prerequisite
  -> validate complete report -> text/TSV/JSON
```

### Error and cancellation behavior

Cancellation produces no further calls and no partial success. A complete
typed diagnostic failure is still emitted when all scheduled observations
settle; internal collection failure remains a structured internal fault. No
check performs repair or automatic retry.

### Security and public boundary

The graph cannot add external destinations or executor authority. Diagnostics
retain existing redaction and visible projection. Root-key/vault checks never
create, unlock, or rewrite state.

## Implementation slices

1. Dependency matrix and failing complete-report fixtures
2. Domain statuses/prerequisite validation
3. Application orchestration and narrow infra outcomes
4. Text/TSV/JSON/catalog recovery
5. No-mutation canaries, docs, and agent-readiness replay

## Orchestration

- Prerequisites: `observational-read-purity` and
  `human-presentation-foundation`.
- Blocks: no core A packet; closes recovery quality.
- Parallel safety: may run after presentation with Auth only if shared doctor,
  catalog, and numbered-document edits are serialized.

## Verification

- Domain/app: DAG validation, root-cause/blocked combinations, complete ordering.
- Infrastructure: fake Docker missing/down, policy invalid, broker locked/unavailable.
- Zero side effects: empty XDG before/after, recording runner, no Docker mutation.
- Output: exact text/TSV/JSON keys, explicit blocked state, hostile diagnostics.
- Agent readiness: one doctor report identifies corrective step with zero
  exploratory calls.
- Required profiles: focused tests, `task check`, `task security`.

## Rollout and rollback

If the report schema changes, advance it explicitly and retain release notes
before v1.0. Blocked is additive semantic precision; rollback must not map it
back to false policy/auth failure.

## Documentation promotion

- Product complete-report and blocked-state semantics.
- Architecture application-owned check graph.
- Security observational guarantee.
- Harness dependency-matrix and zero-mutation checks.
