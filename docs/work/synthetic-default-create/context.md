# Work Context: Make first Context creation atomic and actionable

## Current behavior

- Fresh reads expose a synthetic `default` without persisting authority.
- `Runtime.CreateContext` currently calls `ensureContextStore` before checking the requested name. That initializer persists `default`, so an explicit first create of `default` subsequently returns `ErrContextExists`.
- The isolated-XDG replay therefore observed a failing command that had already changed state.
- A successful create with no configured cluster reports `not_applicable`; human rendering only emits a next command for `requires_reconcile`.

## Relevant structure

- Entry point: `internal/cli/context.go`
- Application use case: `internal/app/contextcmd/service.go`
- Infrastructure boundary: `internal/infra/dockerruntime/context_store.go`
- Existing tests: Context service, runtime store, CLI rendering, catalog recovery routing.

## Constraints

- Synthetic reads remain observational and authority-free.
- The first authorized mutation may initialize or migrate the Context store.
- Creates remain fixed-target `EffectCreate` mutations with no Docker side effect.
- Existing persisted and legacy stores remain compatible.

## External facts

None.

## Unknowns

- [x] The smallest ordering validates the requested manifest first, then performs duplicate detection, default-store initialization or ordinary initialization, creation, and active-marker resolution under the existing Context-store lock. Explicit fresh `default` initialization reuses the same legacy migration helper with the caller's manifest.
- [x] The typed absent-cluster result remains `not_applicable`; only human recovery presentation maps create-time `not_applicable` and `requires_reconcile` to exact `tobari cluster up` argv.

## Verified result

- Fresh explicit `default` creation now owns the first persisted manifest and retains the caller's image and policy mode.
- Duplicate detection occurs before store initialization while holding the Context-store lock; the existing manifest and owned tree remain byte-for-byte unchanged.
- Runtime tests observe zero Docker runner calls for both the successful first create and rejected duplicate.
- Existing legacy migration, owner-only path, and atomic manifest-write coverage passes unchanged.

## Thesis evidence

- User friction observed: a command reports failure after persisting the requested resource.
- Current thesis resolution: mutation outcome must be truthful, and routine recovery must be executable.
- Expected impact: infrastructure ordering and CLI recovery tests; no thesis revision expected.

## Reproduction or observation

```sh
XDG_CONFIG_HOME=<empty-temp>/config XDG_DATA_HOME=<empty-temp>/data \
  go run ./cmd/tobari context create --name default
```

Observed: nonzero `context_exists`, followed by persisted Context files under the isolated configuration root.

## Security and public-boundary notes

- Side effects are owner-only local Context files.
- No credentials, external calls, dependencies, or confidential fixtures are involved.
- Create is non-retryable when the target pre-exists; a confirmed first create must not be reported as failure.

## Glossary

- Synthetic default: an observational fallback shown when no Context catalog has persisted authority.
