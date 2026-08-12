# Work Plan: Select direct source access per Context

- Status: Proposed
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)
- Parent decision: [Context capability envelope](../context-capability-envelope/plan.md)

## Chosen approach

Add one closed `ContextSourceAccess` domain enum with `read-only` and
`read-write`, persist it as a required exact-V1 Context manifest field, and
carry it unchanged into reports and project reconciliation. The runtime emits
the same one direct source bind with an added `readonly` option only for the
read-only value. No alternative source-binding implementation is introduced.

Keep `read-write` as the omission default because the ordinary coding outcome
requires edits. Use a separately named read-only Context for investigation,
review, static analysis, or other workflows that tolerate source write
failures.

## Alternatives considered

### Read-only as the global default

This minimizes write authority but makes the primary coding workflow fail in
surprising ways and pushes most users toward a broad override. Rejected for V1;
the selected access is always visible.

### Per-entry `tobari --read-only`

This permits one Context-bound Workspace to change authority between entries
or concurrent sessions and makes runtime reconciliation ambiguous. Rejected.

### Host chmod or container user permissions

Host chmod mutates user data and can affect unrelated processes; UID-based
permissions are not an explicit mount boundary. Rejected.

### Clone or overlay

These provide writable isolated results but add copy, Git, synchronization,
and apply-back semantics. Deferred separately.

## Design

### Public contract

`context create` gains:

```text
--source-access read-only|read-write
```

It is optional with declared default `read-write`. It is a creation fact, not a
mutation target. Context list/show and relevant status/list observations expose
the exact value. Text output uses “Source access” and “direct read-only” or
“direct read-write”; it never says merely “read-only Workspace.”

No command mutates this value. To change it, create another Context and use
that Context explicitly. Deleting a Context is not added by this packet.

The capability remains `context.composition`; no new capability ID or opaque
reference kind is needed. `context create` remains a fixed-target create
against the installation Context catalog.

### Layer changes

- Domain: closed enum, required manifest/summary/report facts, validation, and
  project desired-spec identity.
- Application: carry the parsed value into the existing create port and
  validate returned task identity/value.
- Infrastructure: persist/read exact V1, include value in spec hash, render
  exact Docker mount, inspect drift, and reconcile safely.
- CLI and catalog: input enum/default, output fields, text/JSON presentation,
  errors, fixtures, and scoped help.

### Data and control flow

```text
context create --source-access VALUE
        ↓ typed validation
atomic Context manifest
        ↓ stable Context ID selected at root entry
project desired spec + hash
        ↓
Docker create --mount type=bind,...[,readonly]
        ↓ inspect exact mount RW flag before exec
```

### Error and cancellation behavior

- Unknown/empty source access fails before Context state or Docker I/O.
- Missing access in a persisted manifest is unsupported V1, not inferred
  read-write.
- A mount mismatch prevents entry and uses existing reconciliation; the old
  container is not treated as satisfying the desired Context.
- Source write failures inside a read-only Workspace are ordinary tool errors.
  Tobari does not translate them into network permission candidates or offer a
  mutation recovery command.
- Context creation cancellation follows the existing fixed-target mutation
  outcome contract.

### Security and public boundary

The control reduces write authority only through the selected bind. It does
not prevent reads, allowed egress, host mutation, same-root read-write Context
mutation, or container/Engine boundary compromise. The persistent home remains
writable and may contain real tool-native credentials.

## Implementation slices

1. Contract and failing manifest/catalog tests.
2. Domain/application propagation.
3. Store/spec-hash/mount rendering and inspection.
4. Text/JSON output and durable documentation.
5. Docker integration across path layouts and supported platforms.

## Verification

- Unit and contract tests: enum, defaults, required field, output schemas.
- Negative side-effect tests: invalid input creates no Context/Docker resource.
- Runtime tests: exact argv and inspect shape for both access values.
- Integration: read/write/delete/Git operations, home/tmp writes, nested
  home-relative root, generated Git projection, same-root cross-Context changes,
  restart/re-entry, and no writable alias.
- Agent readiness: exact create help and unambiguous report interpretation.
- Required profiles: `task check`, `task security`, `task public:check`.

## Rollout and rollback

Pre-public Context manifests are recreated. New exact-V1 manifests require the
field. There is no runtime toggle or compatibility default. Before publication,
rollback requires source revert and development-state recreation.

## Documentation promotion

- Revise ADR 0010 from unconditional read-write to direct Context-selected
  access.
- Update Context/product/runtime/security/threat-model contracts.
- Add harness claims for exact mount mode, writable home separation, no alias,
  and honest limitations.
- Update Context and first-denial/readiness examples.
