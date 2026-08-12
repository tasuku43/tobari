# Work Tasks: Narrow brokered authentication for first public V1

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)
- Context: [context.md](context.md)
- Retirement record: [capability-retirement.md](capability-retirement.md)

## Understand and decide

- [x] Inventory providers, plans, catalog, state, managed adapter, companion,
      drivers, assets, dependencies, tests, and generated surfaces. Evidence:
      read-only inventory on 2026-08-12 recorded in `context.md`.
- [ ] Accept and propagate the superseding exact V1 auth decision.

## Implement

- [ ] Add negative catalog/provider/plan/state/selector/fallback tests.
- [ ] Narrow public auth catalog and application/domain results to GitHub plus
      strict owner static import/status/logout.
- [ ] Remove managed injection and its store, selector, Gateway path, mounts,
      status, tests, and assets.
- [ ] Remove AWS, Datadog, OpenAI, Anthropic, Chatwork, companion, refresh,
      signing, host drivers, exact-version contracts, state readers, and
      unowned dependencies/assets.
- [ ] Clarify brokered versus Workspace-owned authentication in all outputs.
- [ ] Synchronize canonical and embedded sources only after implementation
      stabilizes.

## Verify and integrate

- [ ] Re-prove static handle binding, deny-before-resolution, one exact
      replacement, rotation, revocation, no fallback, and secret-free output.
- [ ] Run focused Go, Gateway, Auth Broker, runtime, and snapshot tests.
- [ ] Review dependency, generated, and image-content diffs.
- [ ] Record `task check`, `task security`, and `task public:check` evidence.
- [ ] Commit only this packet, integrate, and repeat verification.
