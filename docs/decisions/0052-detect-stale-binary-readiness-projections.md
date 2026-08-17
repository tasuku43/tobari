# ADR 0052: Detect stale binary-readiness projections

- Status: Accepted
- Date: 2026-08-17
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, policy, runtime, authentication, and harness
- Revises: ADR 0051
- Revised by: ADR 0058
- Superseded by: None

## Context

ADR 0051 moved native readiness from immutable Context snapshots into the
trusted binary. A binary containing `twg_ready` was installed while the running
aggregate still predated that bundle. TWG login was correctly denied, but
`cluster status` incorrectly reported the stored projection as valid because it
checked only file shape and Context membership. The binary-owned update was
therefore invisible until the user happened to rerun `cluster up`.

Current selection was also declared separately from the bundle history. Adding
a family required editing parallel structures, increasing the chance that
replacement history or current selection would be incomplete.

## Decision

One compile-time native-readiness family catalog owns each stable review ID,
its selected current version, and its append-only version history. Current
projection and historical stripping are derived from that catalog. Contract
tests require unique family IDs and versions and exactly one selected version
per family.

Aggregate construction and read-only integrity inspection use one deterministic
revision calculation over the ordered Context manifest, effective policy data
including the binary overlay, and evaluator source. Integrity is valid only
when the current desired revision equals the persisted active revision and the
stored aggregate document declares that exact revision and schema.

`cluster status` remains observational and performs no repair. Root Workspace
entry requires running components plus valid policy, Gateway, and principal
projections. A mismatch fails closed with `cluster up` as the exact recovery.
Only explicit `cluster up` validates and activates the replacement aggregate.

## Consequences

- A binary readiness update requires no Context recreation or snapshot rewrite.
- The existing cluster must be reconciled once with `cluster up`; stale state is
  now visible and cannot be used for new root entry.
- Adding a native client family has one catalog location for current and
  historical exact effects. Tool-specific host interaction validators remain
  explicit security boundaries rather than generic catalog data.
- A user-selected exact Deny remains terminal after reconciliation.

## Mechanical enforcement

- Domain tests validate catalog uniqueness, one-current-version selection,
  exact effects, provider-neighbor exclusions, and historical replacement.
- Infrastructure tests prove aggregate construction and inspection share the
  content identity and reject a revision not desired by the current binary.
- Application tests prove a running cluster with an invalid projection cannot
  enter or mutate a Workspace and returns `cluster up` recovery.
- Full, security, and public gates cover the durable contract.

## Compatibility and security

No public command, schema, Context state, or snapshot migration is added. The
change tightens readiness: a previously accepted but stale projection is now
reported invalid and rejected by root entry. Status remains read-only and no
runtime input can select, widen, or extend the catalog.

## Revision by ADR 0058

ADR 0058 separates pinned client version from the readiness contract revision.
The family catalog now selects one positive contract revision and retains
append-only contract history; the aggregate content-revision and stale-state
behavior in this record are unchanged.
