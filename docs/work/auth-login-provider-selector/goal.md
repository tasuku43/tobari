# Work Goal: Interactive auth login provider selection

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/01_product_contract.md`
- Review/delete trigger: Delete after durable conclusions are promoted and the change completes
- Successor: None
- Owner: Maintainers
- Target: Current change
- Related ADRs: None

## Outcome

`auth login` accepts the provider through optional `--provider`. When it
is omitted on an interactive terminal, Tobari presents the installed reviewed
login providers for the selected Context, accepts one explicit choice, and
then runs the same bounded trusted-host login flow as an explicitly supplied
provider.

## Why now

The current command rejects an omitted provider before the interactive login
flow starts, forcing a human to discover and type a provider ID even though
Tobari already owns the installed-provider inventory.

## Non-goals

- Change `auth import` or `auth logout` provider requirements.
- Infer a provider from ambient host state, configured credentials, or labels.
- Add provider helpers, provider API behavior, credential handling, or network destinations.
- Change auth output schema or mutation target/effect.

## Acceptance criteria

- [x] `auth login [--provider PROVIDER]` documents the optional provider flag.
- [x] Omission on a TTY presents only installed reviewed login providers and passes the selected provider unchanged to the existing login use case.
- [x] The omitted-provider review remains bound to the Context returned by its status snapshot across concurrent default-Context changes.
- [x] Explicit `--provider` invocations remain deterministic before the provider-native interactive flow, and the former positional form is rejected.
- [x] Redirected omission, cancellation, invalid/empty selection state, and `--method` without a provider fail before provider login mutation.
- [x] No credential, handle, vault, root-key, provider executable, or external destination boundary changes.
- [ ] Focused tests and `task check` pass. Focused and full implementation Go/race tests plus `task security` pass; the full gate is blocked by the separately maintained GitHub Pages JSON-schema table markers.

## Governing documents

- Thesis: `docs/00_theses.md`, Thesis 0, Thesis 3, Thesis 7, and Thesis 9
- Product contract section: `docs/01_product_contract.md`, public commands and authentication input contract
- Architecture or security invariant: `docs/02_architecture.md` command catalog and Context composition; `docs/03_security_model.md` mutation policy
- Existing ADR: None required; this changes presentation and argv optionality without a trust-boundary decision

## Completion definition

The work is complete when the catalog, interaction, tests, and durable
documentation agree; `task check` passes; and this temporary packet is removed.
