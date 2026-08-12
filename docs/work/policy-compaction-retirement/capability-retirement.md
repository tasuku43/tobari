# Capability Retirement: Learned policy compaction

## Decision

`policy.learning` remains public but narrows to exact candidates, explicit
review, and exact learned rules. The `policy compactions`/`policy compact`
surface, `policy-compaction` reference kind, prefix learned-rule authority, and
all supporting state and dependencies are excluded from first public V1.

## Required negative evidence

- [x] Removed command paths and aliases are unknown and absent from help.
- [x] No producer or consumer for `policy-compaction` remains.
- [x] Prefix policy state fails closed rather than loading, converting, or
      falling back to exact behavior.
- [x] OPA cannot authorize by learned path prefix.
- [ ] No dormant configuration selector, recovery path, generated entry,
      dependency, or fixture retains the capability.

## State handling

Unpublished development Contexts containing compaction or prefix-rule state
are not migrated. Operators delete and recreate them under ADR 0027. No old
reader may log, decode, or reinterpret the retired state during migration.
