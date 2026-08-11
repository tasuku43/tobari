# Work Plan: Report authentication state and change truthfully

- Status: Active
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Separate Context provider configuration from task-owned Workspace activation
observations. Infrastructure returns bounded authoritative secret-free facts;
application/domain validate their Context/project/revision correlation and
compute finite activation/action/change semantics without presentation
inference. Selector rows consume the same typed provider collection and mark
configured entries with an explicit rotation warning. Logout returns either a
confirmed changed receipt or a typed no-change result.

## Alternatives considered

### Always require re-entry after any configured provider

Rejected because it is safe but permanently false after successful reconciliation.

### Infer readiness from a running container

Rejected because runtime existence does not prove the expected handle/file revision.

### Let infrastructure construct public activation and receipts

Rejected because infrastructure owns observation, not request correlation,
product state, recovery action, or presentation semantics.

### Treat absent-provider logout as successful removal

Rejected because the receipt claims revocation and impact that did not occur.

## Layer changes

- Domain: finite configuration, activation, change/no-change, scope, ordering,
  bounds, and request-correlation invariants.
- Application: validate requested Context/provider against observations and
  construct task results.
- Infrastructure: bounded secret-free provider/project/registry/Broker facts;
  authoritative Broker `changed`; no public semantic state selection.
- CLI/catalog: configured selector markers/warning, state/result rendering,
  schema 4/help/fault updates, neutral cancellation through shared foundation.

## Data and control flow

```text
bounded Context/provider + project/registry/Broker observations
  -> validate requested Context/project/revision authority
  -> finite provider, activation, exact-action, and change result
  -> schema 4 status/receipt/selector presentation

logout -> Broker changed=false -> no-change result, no activation claims
       -> Broker changed=true  -> confirmed receipt + advisory activation observation
```

## Error and cancellation behavior

Locked/unavailable Broker yields uncertainty, not absence. Selector cancellation
occurs before driver/vault I/O. Unknown mutation outcomes remain non-retryable
and reconcile through status. No-op is a confirmed non-changing result, not an
error and not a claim that a mutation occurred.

## Security and public boundary

Only non-secret revision and ownership facts cross infrastructure. Raw handles,
tokens, vault paths, provider homes, and modified Workspace auth files remain
outside results and fixtures. No provider call or remote revocation is added.

## Implementation slices

1. Authoritative state inventory and frozen typed fixtures
2. Domain/application activation and no-change semantics
3. Infrastructure projection observation and no-op classification
4. Selector/status/receipt/catalog presentation
5. Integration, no-secret checks, docs, and readiness evidence

## Verification

- Domain/app: every provider/activation/change state and invalid scope/revision.
- Infra: zero/one/many Workspaces, current/stale/missing projection, locked broker,
  absent/present logout, observation failure, and binding-call budget.
- Negative side effects: cancel/unknown provider/no-op make zero driver/write calls.
- CLI/schema: selector warning, status, receipts, neutral cancel, hostile labels.
- Pinned corpus: 10 semantic cases, answer key, exact action, zero external
  processing, hash/size/case ledger, and secret-free canaries.
- Required profiles: `task check`, `task security`, `task authbroker:test`, and
  relevant integration/auth readiness scenarios.

## Documentation promotion

- Product distinction between provider configuration and Workspace activation.
- Architecture observation/semantic layer ownership.
- Security no-op/uncertainty and no-secret boundaries.
- Harness truth-table, no-change, selector warning, and integration proof.
- Authentication and agent-readiness journeys.
