# Work Plan: Make first Context creation atomic and actionable

- Status: Approved
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Add regression tests first, then make Context initialization and duplicate detection one lock-scoped infrastructure operation. Explicit creation of the synthetic `default` becomes the authoritative first creation while preserving legacy-store migration and active selection. Extend human recovery routing so a successful create with no cluster points to `cluster up` without starting Docker.

## Alternatives considered

### Treat the initialized default as success after `ensureContextStore`

Rejected because it would silently ignore the caller's requested image or policy mode and could report a resource the create did not actually create.

### Reserve `default` and reject explicit creation before initialization

Rejected because the public command accepts a valid Context name and the current failure would still make the synthetic/persisted transition surprising.

## Design

### Public contract

No new command or schema. `context create` remains a fixed-target create. First explicit `default` creation succeeds; duplicate creation retains `context_exists`. Human recovery uses an exact catalog path.

### Layer changes

- Domain: no vocabulary change expected.
- Application: retain existing fault mapping and mutation boundary.
- Infrastructure: perform initialization, duplicate check, creation, and active-marker setup atomically under the Context-store lock.
- CLI and catalog: render and route `cluster up` for absent-cluster create results.

### Error and cancellation behavior

Validate input before filesystem I/O. Duplicate targets fail before changing their manifest. Local filesystem failures remain non-retryable rejected outcomes with existing recovery.

### Security and public boundary

No trust-boundary, secret, destination, or dependency change.

## Implementation slices

1. Runtime regression tests for first `default` and duplicate preservation.
2. Lock-scoped infrastructure fix.
3. CLI recovery regression for absent cluster.
4. Focused and full verification plus isolated-XDG replay.

## Verification

- Unit and contract tests: Context runtime, app, and CLI packages.
- Negative side-effect tests: duplicate manifest remains unchanged.
- Structured output and recovery tests: exact `cluster up` argv routes through the catalog.
- Manual observation: isolated fresh XDG roots.
- Required profiles: `task check`.

## Rollout and rollback

Existing Context stores are unchanged. Rollback restores the prior ordering but would reintroduce failure-after-mutation for fresh explicit `default` creation.

## Documentation promotion

No durable contract change expected; tests mechanically enforce existing theses and product behavior.
