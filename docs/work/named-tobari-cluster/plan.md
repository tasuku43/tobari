# Work Plan: Named Tobari cluster

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Split shared enforcement lifecycle from named isolation lifecycle. Compose owns
only OPA, Gateway, their control/egress networks, and shared CA volumes. The
runtime adapter creates each Tobari container, internal network, and home volume
with exact labels, then joins Gateway to that network under the `gateway` alias.
OPA watches the read-only XDG policy bind.

## Alternatives considered

### One shared Tobari network

This is mechanically smaller, but every Tobari could directly reach every other
Tobari. Dedicated networks preserve the existing isolation claim.

### Read-write OPA policy mount

A bind mount reflects host changes even when the container view is read-only.
Granting OPA write access adds host mutation authority without enabling the
requested live-edit loop, so the design uses read-only plus OPA watch.

## Design

### Public contract

- `cluster up`: fixed-target create for the one local enforcement cluster.
- `cluster status`: utility read of Gateway, OPA, policy, and recent error.
- `cluster logs`: bounded shared component logs.
- `cluster down [--purge]`: fixed-target write; rejects while Tobari exist.
- `attach --name --root`: fixed-cluster create with name/root configuration inputs.
- `list`: discover command returning opaque `tobari_id`, name, root, and status.
- `shell --id`, `exec --id`, `logs --id`: reference-bound reads.
- `detach --id [--purge]`: reference-bound write.

Names are validated display identities and unique within local state. Actions do
not rediscover by name. Outputs are complete; `list` has exhaustive local
coverage. Runtime failures are non-retryable until read-only reconciliation.

### Layer changes

- Domain: cluster and named-Tobari state, references, validation, path mapping, and status.
- Application: cluster lifecycle plus attach/list/execute/log/detach use cases.
- Infrastructure: shared Compose reconciliation, per-Tobari Docker resource creation, exact ownership checks, and schema-2 state.
- CLI and catalog: namespace cluster, discovery producer, opaque-reference consumers, and renderers.

### Data and control flow

CLI parses catalog-owned inputs, application validates task and mutation intent,
and infrastructure performs exact Docker argv calls. The state file supplies
exact resource names; Docker labels are verified before removal.

### Error and cancellation behavior

Validation, reference lookup, and mutation policy run before Docker mutation.
Cluster removal refuses while configured Tobari remain. Attach is idempotent for
an exact name/root pair and rejects a conflicting name. Confirmed mutations are
rendered through the existing mutation-complete boundary.

### Security and public boundary

OPA and Gateway remain trusted. Tobari remain untrusted and receive no Docker
socket or managed secrets. Each selected root is an explicit read-write scope.
No new dependencies or external destinations are introduced.

## Implementation slices

1. Promote durable contract changes and add domain tests.
2. Implement schema-2 state and shared/per-Tobari runtime operations.
3. Register catalog reference flows and CLI presentation.
4. Rewrite integration lifecycle and user documentation.
5. Run all required gates and remove this packet.

## Verification

- Unit and contract tests: domain, application, infrastructure, CLI, catalog.
- Negative side-effect tests: invalid name/root/reference and unowned resources.
- Opaque-reference tests: `list` ID passed unchanged into act commands.
- Structured output and recovery tests: cluster status/list plus declared faults.
- Agent readiness: root help, exact help, `list`, then action; no external processing.
- Manual observation: two roots, two isolated networks, live policy edit.
- Required profiles: full, security, public, runtime.

## Rollout and rollback

This is an explicit pre-v1 breaking change. Schema-1 state is rejected with an
actionable cleanup/migration diagnostic rather than guessed. Existing Docker
resources remain ownership-labeled and can be removed by the old binary before
upgrade. A rollback must use the old binary only after the new cluster is
removed.

## Documentation promotion

Promote the cluster topology, named-Tobari vocabulary, command workflow, state
compatibility, live policy watch, and exact cleanup rules into documents 00
through 04, the threat model where affected, and README.
