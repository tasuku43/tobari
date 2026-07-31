# Work Plan: Align CWD-owned lifecycle boundaries

- Status: Active
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

First separate the root use case from shared cluster mutation and add protected
root validation. Then make explicit cluster reconciliation derive project
networks and counts from indexed CWD state. Add a small durable mutation journal
and latest-state locking semantics before tightening runtime readiness, desired
spec drift, and partial-delete behavior. Remove legacy authority only after the
new paths have equivalent tests.

## Alternatives considered

### Alternative A

Keep `ClusterUp` in the root command and document it as convenience behavior.
Rejected because it violates the requested lifecycle boundary and permits
project state creation after an implicit shared mutation.

### Alternative B

Replace the existing state model wholesale in one change. Rejected because it
would make cluster/policy regressions difficult to isolate; use narrow slices
with compatibility checks instead.

## Design

### Public contract

`tobari` is an interactive project-runtime action with a fixed CWD target. It
requires `cluster status` to report configured and ready, and points recovery to
exact cluster commands. Explicit cluster commands retain their current roles;
their project count and network reconciliation use indexed project state.

### Layer changes

- Domain: protected-root and project mutation/journal invariants.
- Application: read-only cluster prerequisite ordering and current-state project resolution.
- Infrastructure: cluster/project repositories, journal, Docker readiness/spec inspection, and exact cleanup.
- CLI/catalog: stable prerequisite and recovery faults plus updated outcomes/help.

### Error and cancellation behavior

No project state or Docker project resource is created before cluster readiness
and root protection checks. Interrupted create/delete leaves a journal that the
next project operation reconciles. Runtime drift is non-destructive to logical
state and recreates only the work container. Partial deletion treats missing
resources as completed and continues exact cleanup.

## Verification

- Unit tests for protected roots, journal phases, latest-state concurrency, spec hash, and readiness.
- Application tests proving no cluster mutation and no project mutation before prerequisites.
- Integration tests for explicit cluster reconcile, unconfigured/unhealthy root entry, drift/recovery, and partial delete.
- Required profiles: `task check`, `task security`, `task public:check`, `task integration:test`.

## Rollout and rollback

Legacy named state is not auto-migrated. The public root command changes from
implicit cluster setup to explicit prerequisite checking; rollback requires the
previous binary and does not delete new logical state.

## Documentation promotion

Promote the cluster/project boundary, protected-root trust boundary, and
durable mutation recovery conclusions into the product, architecture, security,
and harness documents before deleting this packet.
