# Work Goal: Make first Context creation atomic and actionable

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/01_product_contract.md` Context observation and mutation rules
- Review/delete trigger: Delete after the fix, runtime replay, and `task check` complete
- Successor: None
- Owner: Codex orchestration
- Target: Current UX review
- Related ADRs: None

## Outcome

On a fresh installation, explicitly creating the `default` Context succeeds exactly once without first materializing it and then reporting failure. Creating any Context while the shared cluster is absent gives an executable continuation to start the cluster and enter a Workspace.

## Why now

A bounded isolated-XDG replay observed `context create --name default` return `context_exists` after writing the default Context store. The same replay observed a successful non-default create report `not_applicable` without a next command when no cluster existed.

## Non-goals

- Do not make observational Context reads initialize persistent state.
- Do not make `context create` start Docker or reconcile the cluster.
- Do not change JSON schemas, command paths, target binding, or Context naming rules.

## Acceptance criteria

- [x] Fresh `context create --name default` succeeds and returns a valid persisted report.
- [x] A second identical create returns `context_exists` without changing the existing manifest.
- [x] Failure paths do not materialize the requested Context before reporting that it already exists.
- [x] A successful create with an absent cluster renders exact recovery through `tobari cluster up`.
- [x] Focused tests, an isolated-XDG replay, and `task check` pass.

## Governing documents

- Thesis: Public commands close user tasks; discovery and action remain separate.
- Product contract section: Context observation, creation, and shared-cluster lifecycle.
- Architecture or security invariant: Mutation stays behind the Context runtime port and controlled filesystem boundary.
- Existing ADR: None.

## Completion definition

The work is complete when the acceptance criteria have evidence, required profiles pass, and this temporary packet is removed from the final tree.
