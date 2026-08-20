# Work Plan: Separate the Tobari product from the Workspace resource

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Apply one semantic rule across the public surface: Tobari names the product and
its ownership; Workspace names the reusable isolated resource. Keep Project for
the host source directory. Audit public machine fields individually, but do
not use a user-facing vocabulary improvement to justify a cosmetic rename of
internal state, protocol, labels, or Go symbols.

## Alternatives considered

### Prose-only cleanup

Changing README and ordinary text would leave agent help, structured output,
fault recovery, and generated references with a conflicting model. Rejected
because those are also public product surfaces.

### Whole-repository rename

Replacing every project/Tobari identifier with Workspace would churn stable
state, Gateway/OPA protocols, Docker labels, tests, and audit fields without
showing that their current semantics are wrong. Rejected in favor of a
field-by-field public-contract audit.

## Design

### Public contract

No command, effect, role, input grammar, reference flow, delivery, collection
coverage, or failure behavior changes. Human and agent presentation describe
Workspace lifecycle consistently. Public structured fields use project terms
only for project-root facts and Workspace terms for Workspace identity; any
retained legacy key has an explicit schema/compatibility rationale.

### Layer changes

- Domain: only semantic type or invariant names proven misleading by the audit
- Application: task-owned result field terminology where public semantics
  currently expose a logical Workspace as a project/Tobari identity
- Infrastructure: no change unless an internal name causes a real identity or
  trust-boundary ambiguity; preserve exact stored and projected authority
- CLI and catalog: human help, summaries, outcomes, faults, recovery, renderers,
  structured field declarations, and fixtures

### Data and control flow

```text
existing typed task result
        ↓ unchanged semantic validation
public field classification
        ↓
human / agent / structured presentation using one vocabulary
        ↓
unchanged command, next argv, reference values, and side effects
```

### Error and cancellation behavior

Unchanged. Existing stable fault codes, retryability, next commands, mutation
completion, and child exit behavior remain authoritative. Only explanatory
text changes unless a separately reviewed public schema rename is necessary.

### Security and public boundary

No authority changes. A field rename must not change principal construction,
opaque bytes, policy matching, audit redaction, Context/Workspace binding, or
persisted ownership. Fixtures are synthetic and deterministic.

## Implementation slices

1. Freeze typed lifecycle/status/policy fixtures and answer keys.
2. Inventory and classify public human, agent, and structured identifiers.
3. Revise governing vocabulary and any required public schema contracts.
4. Update catalog, renderers, faults, recovery, README, and generated docs.
5. Add vocabulary and semantic-regression enforcement.

## Verification

- Unit and contract tests: catalog/help/result schemas and exact-key fixtures
- Negative side-effect tests: existing lifecycle/policy tests prove no changed
  effect; add a zero-effect canary if a projection path is refactored
- Opaque-reference and complete-pagination tests: preserve every applicable
  existing reference and delivery contract
- Structured output, hostile-output, and recovery tests: exact keys, exact next
  argv, and unchanged escaping
- Agent-readiness scenario and discovery-round-trip count: unchanged root and
  scoped-help budgets with Workspace terminology
- Human-handoff scorecard: not applicable; no setup or authentication transfer
- Manual observation: compare representative lifecycle/status/list/help output
- Required profiles: `task check`; conditionally `task security` or
  `task public:check` only when the audited implementation changes those
  boundaries
- Generated-diff or artifact checks: architecture-site and catalog generation

## Rollout and rollback

Human terminology is a pre-public contract clarification. Any structured field
rename must declare its schema/version and development-state compatibility
before implementation. Rollback is a source revert only when no persisted
schema changes; otherwise the packet must record the explicit pre-public state
disposition.

## Documentation promotion

- Define product/resource vocabulary in theses and product contract.
- Propagate Workspace identity language through architecture and security.
- Update harness claims and add deterministic vocabulary enforcement.
- Record any machine-schema compatibility decision in the relevant durable
  contract or ADR rather than only in this packet.
