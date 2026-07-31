# Work Goal: Align CWD-owned lifecycle boundaries

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/00_theses.md`, `docs/01_product_contract.md`, `docs/02_architecture.md`, `docs/03_security_model.md`, `docs/04_harness.md`
- Review/delete trigger: Delete after durable conclusions are promoted and the change completes
- Successor: None
- Owner: Repository maintainer
- Target: Review feedback integration
- Related ADRs: None

## Outcome

The CWD-owned `tobari` command manages only the selected project runtime and
requires an explicitly configured, ready shared cluster. Protected host
management paths cannot become a read-write project root. Explicit cluster
operations reconcile all registered CWD-owned projects, while durable project
and cluster mutations recover from interruption, concurrency, runtime drift,
and partial deletion without reviving the old named-Tobari authority.

## Why now

The review identified a mismatch between the implemented CWD lifecycle and its
intended trust and ownership boundaries. The highest-risk mismatch is that a
bare root command currently creates and repairs shared Gateway and OPA state.

## Non-goals

- Provider-specific API adapters, OAuth, or new external integrations
- Automatic migration of legacy named-Tobari state
- Changing the supported HTTP/HTTPS policy model
- Removing explicit cluster and policy commands that remain part of the product

## Acceptance criteria

- [ ] Bare `tobari` performs no shared-cluster mutation and creates no project state when the cluster is unconfigured or not ready.
- [ ] Protected management roots and filesystem root/home-wide roots are rejected while ordinary policy-source repositories remain usable.
- [ ] Explicit cluster reconcile/status use indexed `ProjectInstance` records, not legacy named state, and reconnect Gateway to every registered project network.
- [ ] Interrupted project create/delete and concurrent enter operations converge safely; runtime readiness and spec drift are enforced.
- [ ] Partial deletion is idempotent and legacy named lifecycle code is no longer an authority.
- [ ] Required unit, integration, security, public, and full repository gates pass.

## Governing documents

- Thesis: `docs/00_theses.md`
- Product contract: `docs/01_product_contract.md`
- Architecture: `docs/02_architecture.md`
- Security: `docs/03_security_model.md`
- Harness: `docs/04_harness.md`

## Completion definition

The work is complete when every acceptance criterion has current evidence,
durable contract changes are promoted, required profiles pass, and this
temporary packet is removed from the final tree.
