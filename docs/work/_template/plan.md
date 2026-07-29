# Work Plan: Short outcome

- Status: Proposed
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Summarize the smallest approach that satisfies the goal and governing invariants.

## Alternatives considered

### Alternative A

Explain the trade-off and why it was not chosen.

### Alternative B

Explain a meaningfully different trade-off. Remove this section if there is no credible second approach.

## Design

### Public contract

Describe the user outcome, capability-ledger status, commands,
utility/discover/act roles, produced or consumed opaque references, input
sources, output fields/types, delivery, collection coverage, prerequisites,
authentication requirement, structured failures and next actions, effects, exit
behavior, compatibility, and non-goals.

### Layer changes

- Domain:
- Application:
- Infrastructure:
- CLI and catalog:

### Data and control flow

Show how validated input reaches the controlled side-effect boundary and how results return to presentation.

### Error and cancellation behavior

State ownership, failure-before-I/O expectations, authentication versus permission behavior, retryability, retry-after, idempotency, timeout/cancellation, cleanup, stable next commands, and exit mapping.

### Security and public boundary

State assets, intent/target changes, credentials, destinations, dependencies, fixture policy, and publication review.

## Implementation slices

1. Contract and failing tests
2. Domain and application behavior
3. Infrastructure adapter
4. CLI catalog and presentation
5. Harness and documentation

Each slice should remain reviewable and leave the repository buildable where practical.

## Verification

- Unit and contract tests:
- Negative side-effect tests:
- Opaque-reference and complete-pagination tests:
- Structured output, hostile-output, and recovery tests:
- Agent-readiness scenario and discovery-round-trip count:
- Human-handoff scorecard for setup/authentication candidates:
- Manual observation:
- Required profiles:
- Generated-diff or artifact checks:

## Rollout and rollback

Describe migration, feature exposure, state compatibility, and safe rollback. Write “not applicable” with a reason only when no external state or public contract changes.

When removing or replacing a capability, also record catalog/fault/recovery
removal, dependency ownership, dormant fallbacks, and the explicit disposition
of persisted secret and non-secret state in a copied
`capability-retirement.md`.

## Documentation promotion

List conclusions that must move from this temporary plan into theses, product, architecture, security, release, or an ADR before completion.
