# Work Context: Route duplicate Context recovery to the right collection

## Current behavior

- `context create --name review` returns `context_exists` when `review` is present.
- Both `internal/app/contextcmd/service.go` and `internal/cli/runtime_catalog.go` declare bare `context show` as recovery.
- Bare `context show` resolves the active Context, so it can display `default` while the rejected target was `review`.
- `context list` is an exact catalog command that includes all persisted names and the active marker.

## Relevant structure

- Application fault: `internal/app/contextcmd/service.go`
- Catalog error contract: `internal/cli/runtime_catalog.go`
- Recovery consistency/routing tests: `internal/cli` tests.

## Constraints

- Recovery commands are exact catalog paths; they cannot interpolate unchecked request argv.
- The fault remains rejected, non-retryable `context_exists`.
- No external I/O or secret-bearing state is involved.

## External facts

None.

## Unknowns

- [x] The application service fake proves the runtime fault contract, while the
  CLI catalog test proves the matching declaration, typed route, and a
  non-active duplicate in the recovered list.

## Thesis evidence

- Friction: executing the advertised recovery inspects the wrong Context.
- Existing thesis resolves it: use exact catalog recovery that truthfully closes the task.
- No thesis revision is expected.

## Reproduction or observation

```sh
tobari context create --name review
tobari context create --name review
tobari context show
```

Observed after `default` remained active: the suggested final command displayed `default`.

## Security and public-boundary notes

- Read-only recovery only; no new files, destinations, credentials, or dependencies.

## Glossary

- Exact recovery: a catalog path that is executable without synthesizing unchecked argv.
