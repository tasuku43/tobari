# Work Plan: Route duplicate Context recovery to the right collection

- Status: Approved
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Change only the application-owned `context_exists` next action and matching catalog declaration from `context show` to `context list`. Add a negative recovery test that models a non-active duplicate and proves the exact route includes its name.

## Alternatives considered

### Append `--name <requested>` to `context show`

Rejected because public recovery is derived from exact catalog paths and must not append unchecked request argv.

### Keep `context show` and weaken the reason text

Rejected because the command still fails to expose the object needed for the next decision.

## Design

### Public contract

No command, role, effect, output schema, or fault-code change. Only the declared exact recovery path and reason change.

### Layer changes

- Domain: none.
- Application: update the next action on the duplicate fault.
- Infrastructure: none.
- CLI and catalog: update matching error metadata and regression tests.

### Error and cancellation behavior

`context_exists` remains rejected and non-retryable; `context list` is read-only and executable.

### Security and public boundary

No trust-boundary or dependency change.

## Implementation slices

1. Add failing recovery consistency/routing test.
2. Align application fault and catalog metadata.
3. Run focused tests and replay.

## Verification

- Unit and contract tests: app context service and CLI catalog/recovery.
- Manual observation: duplicate non-active name followed by the advertised list.
- Required profiles: `task check:fast` for this follow-up, after the immediately preceding full `task check` on the same implementation stack.

## Rollout and rollback

No state migration. Rollback restores the misleading recovery path.

## Documentation promotion

None; this enforces the existing exact-recovery contract.
