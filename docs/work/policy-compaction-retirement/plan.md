# Work Plan: Retire learned policy compaction before V1

## Chosen approach

Delete the capability vertically. First add negative contract tests for the
public commands, reference kind, and persisted prefix shapes. Remove catalog
and application flow, then domain/state/OPA authority, then fixtures,
documentation, generated data, and unowned dependencies. Do not keep a hidden
reader or internal action for migration.

## Layer ownership

- Domain: remove compaction proposals/IDs/reports and the prefix learned-rule
  variant while retaining exact HTTP/GraphQL rule invariants.
- Application: remove discovery/action ports and use cases.
- Infrastructure: reject prefix state and remove proposal grouping,
  activation, and OPA prefix matching.
- CLI: remove commands, references, output/fault/recovery contracts, and
  catalog-derived generated entries.

## Ordering

1. Land the Context capability-envelope decision.
2. Add retirement canaries and remove public/catalog flow.
3. Remove domain, application, infrastructure, OPA, and persisted state.
4. Regenerate owned surfaces and run focused gates.
5. Integrate this commit before policy preset implementation begins.

## Risks and verification

The main risk is a dormant prefix reader or fallback surviving outside the
public CLI. Search both identifiers and semantic shapes, exercise hostile
persisted input, and inspect catalog reference flow. Run focused Go/OPA tests,
then `task check`, `task security`, and `task public:check` after integration.

## Rollout and rollback

There is no public compatibility migration. Old development Context state is
removed and recreated. Before publication rollback is a source revert plus
development-state recreation; prefix authority is not silently reinterpreted.
