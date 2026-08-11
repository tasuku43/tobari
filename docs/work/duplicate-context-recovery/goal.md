# Work Goal: Route duplicate Context recovery to the right collection

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/01_product_contract.md` executable recovery contract
- Review/delete trigger: Delete after focused tests, replay, and the required repository gate pass
- Successor: None
- Owner: Codex orchestration
- Target: Current UX review
- Related ADRs: None

## Outcome

When `context create` rejects a duplicate name, its exact recovery command leads to information that includes the duplicated Context instead of silently showing a different active Context.

## Why now

An isolated replay created `review` while `default` remained active. Repeating the create returned `context_exists` with `tobari context show`; running that exact command showed `default`, not `review`.

## Non-goals

- Do not append unchecked user input to recovery argv.
- Do not change duplicate detection, Context selection, or structured success schemas.
- Do not add a new command.

## Acceptance criteria

- [x] Runtime and catalog faults for `context_exists` agree on exact `context list` recovery.
- [x] The routed recovery command is catalog-valid and exposes the duplicated name without requiring inferred selector argv.
- [x] Recovery/error contract tests and the required repository gate pass.

## Governing documents

- Thesis: Public commands express and close user tasks.
- Product contract section: catalog-owned exact recovery commands.
- Architecture or security invariant: catalog remains the command and recovery source of truth.
- Existing ADR: None.

## Completion definition

The work is complete when acceptance criteria have evidence and this temporary packet is removed from the final tree.
