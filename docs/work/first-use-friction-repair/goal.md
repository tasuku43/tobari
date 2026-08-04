# Work Goal: First-use friction repair

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: docs/00_theses.md, docs/01_product_contract.md, docs/02_architecture.md, docs/03_security_model.md, docs/04_harness.md
- Review/delete trigger: Delete after durable conclusions are promoted and the change completes
- Successor: None
- Owner: Maintainer
- Target: Current change
- Related ADRs: None

## Outcome

A first-time Tobari user can follow the README and CLI next actions through build, cluster startup, Workspace entry, denial review, runtime customization, and cleanup without needing source inspection or policy knowledge. Known broken or ambiguous first-use states produce actionable diagnostics, and the README quick-start denial remains learnable.

## Why now

An initial user-journey replay found that the README quick-start denial was not learnable in a clean XDG state, stale local policy data could make every policy command fail while `doctor` reported policy as passing, the Gateway host-review hint was easy to run in the Workspace, runtime builds did not warn about existing Workspaces, and build/install expectations were ambiguous.

## Non-goals

- Do not broaden learned permissions beyond exact project, host, port, method, and path rules.
- Do not make the root `tobari` command start or repair the shared cluster implicitly.
- Do not add a provider-specific API adapter, OAuth flow, or host credential inheritance.
- Do not change the fixed discover/action split for policy mutations.

## Acceptance criteria

- [ ] The README quick-start `PUT https://example.com/quickstart` denial is learnable in a clean first-use Context and appears in `policy review`. Source-Gateway evidence passes; ordinary pinned-image evidence remains blocked on `docs/work/gateway-image-refresh-e2e`.
- [x] Policy-data corruption or incompatible stale data is surfaced by `doctor` with the same actionable next step shown by policy commands.
- [x] A learnable Gateway denial tells an agent to leave the Workspace before running the host-side review command.
- [x] `runtime build` human output warns when existing Workspaces keep their stored image and names the cleanup/recreate path.
- [x] The source-build/PATH installation path is unambiguous after `task build`.
- [x] Existing security boundaries remain intact: non-learnable causes stay non-actionable, body-bearing or unavailable-body requests do not become learned candidates, and all policy writes remain reference-bound.
- [x] Focused tests and `task check` pass.

## Governing documents

- Thesis: Thesis 0, Thesis 1, Thesis 7, Thesis 8, Thesis 9
- Product contract section: Primary operating loop, input and path contract, output and exit contract, side effects
- Architecture or security invariant: Gateway request flow, HTTP authorization boundary, mutation policy
- Existing ADR: None

## Completion definition

The work is complete when acceptance criteria have evidence, durable decisions have been promoted to numbered documentation or tests, required profiles pass, temporary diagnostics are removed, and this temporary packet is removed from the final tree.
