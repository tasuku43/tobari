# Work Plan: CWD-owned Tobari lifecycle

- Status: Active
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Replace the named-instance catalog and schema with a CWD-rooted logical
instance repository. The application resolves the nearest indexed canonical
root, creates a stable ID and XDG home when absent, then asks the runtime to
ensure and enter the corresponding container. Runtime observations are
diagnostic only; persistent logical state defines existence.

## Alternatives considered

### Keep named instances with a CWD convenience wrapper

Rejected because names, root flags, user-visible IDs, and old lifecycle verbs
would remain a second public model and violate the requested outcome.

### Infer logical existence from Docker labels

Rejected because Docker prune or a missing container would erase a user's
logical environment and home identity.

## Design

### Public contract

`tobari` is a fixed-target create operation that requires a TTY and creates or
reconciles the current directory's logical environment before entering it.
`status` is a CWD-scoped read, `list` is a discovery read over local state, and
`delete [--force]` is a CWD-scoped destructive write. The old named commands
are rejected with an explicit migration message. `list` may show the internal
ID as diagnostic output but no action consumes it.

### Layer changes

- Domain: logical instance, stable ID, canonical path containment, runtime diagnostic state, and XDG record validation.
- Application: resolve/create, status/list, ensure/enter, and delete use cases over a narrow state-and-runtime port.
- Infrastructure: atomic XDG records, exclusive mutation lock, owned Docker resource reconciliation, and terminal proxying.
- CLI and catalog: root invocation and CWD-local commands with catalog-declared effects and destructive confirmation.

### Error and cancellation behavior

Root and state validation occurs before Docker mutation. An unavailable or
ambiguous runtime never deletes logical state. A confirmed delete can be
continued after partial runtime cleanup. Root invocation without a terminal
returns a typed non-TTY failure and does not create state.

### Security and public boundary

Per-instance homes become owned XDG directories bind-mounted writable; shared
profile content is XDG data mounted read-only. The selected canonical root is
the only project writable bind. All per-instance resources retain exact owner,
ID, and role labels.

## Implementation slices

1. Retire named public contract and establish the CWD lifecycle model.
2. Add canonical root resolution, stable identity, XDG records, locking, and atomic persistence.
3. Implement ensure-and-enter and CWD-local status/list/delete.
4. Add agent-profile mounts and finish migration diagnostics, tests, and docs.

## Verification

- Focused domain, application, CLI, and recording-runner tests for every slice.
- Docker integration for create/reuse/recovery/delete where the engine is available.
- `task check`, `task security`, and `task public:check` after the final slice.
- Agent readiness replay: root help, exact help, then CWD task execution without ID reconstruction.

## Rollout and rollback

This is an intentional breaking change. Schema-2 named state is neither
guessed nor altered; the old binary remains the explicit cleanup path. The
new runtime stores versioned independent root and instance records.

## Documentation promotion

Update theses 4 and 5, product contract, architecture, security model, and
harness claims with CWD identity, XDG home ownership, and deletion semantics.
