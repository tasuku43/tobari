# Work Plan: Keep declared reads observational on first use

- Status: Implemented; verification deferred
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Split infrastructure store APIs into observation and ensure/mutate operations.
Read use cases call observation ports that can return synthetic, persisted,
legacy, or unsafe state without committing a default or migration. Authorized
create/write use cases alone call ensure/migrate under existing atomic locks.
A catalog-derived canary runs every public read against a fresh XDG fixture.

## Data and control flow

```text
EffectRead -> Observe store -> synthetic/persisted/legacy/unsafe typed result
EffectCreate/Write -> validate target/intent -> Ensure or migrate atomically
```

## Error and cancellation behavior

Unsafe or corrupt existing state fails closed with read-only recovery. Missing
state is typed when absence is valid. Legacy reads do not claim migration
success; later mutation revalidates before committing. Reads may complete only
bounded cleanup described by a pre-existing validated mutation journal. That
exception may create the project recovery lock solely to serialize cleanup;
fresh and ordinary reads create no lock, and no read creates the journal.

## Remaining verification

1. Obtain authority to update the English and Japanese architecture-site
   JSON-schema tables.
2. Update only those stale public schema entries.
3. Run `task check` and confirm PASS.
4. Remove this temporary packet in the same completion handoff.
