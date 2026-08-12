# Work Tasks: Narrow brokered authentication for first public V1

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)
- Context: [context.md](context.md)
- Retirement record: [capability-retirement.md](capability-retirement.md)

## Understand and decide

- [x] Inventory providers, plans, catalog, state, managed adapter, companion,
      drivers, assets, dependencies, tests, and generated surfaces. Evidence:
      read-only inventory on 2026-08-12 recorded in `context.md`.
- [x] Accept and propagate the superseding exact V1 auth decision. Evidence:
      ADR 0030 and the revised authentication thesis define the retained static
      broker core and complete retirement scope before deletion.

## Implement

- [x] Add negative catalog/provider/plan/selector tests. Evidence: catalog
      parsing rejects omitted providers, retired providers, and `--method`;
      provider parsing rejects retired credential kinds and signing fields;
      broker control and host-runtime tests reject retired providers, driver
      metadata, and companion operations before acquisition.
- [x] Narrow public auth catalog and application/domain results to GitHub plus
      strict owner static import/status/logout. Evidence: `auth login` requires
      `--provider=github`, only the GitHub manifest remains built in, and the
      provider domain accepts only `primary_secret` header-binding plans.
- [ ] Remove managed injection and its store, selector, Gateway path, mounts,
      status, tests, and assets.
- [ ] Remove AWS, Datadog, OpenAI, Anthropic, Chatwork, companion, refresh,
      signing, host drivers, exact-version contracts, state readers, and
      unowned dependencies/assets. Partial evidence: Go provider built-ins,
      dynamic/signing domain shapes, non-GitHub host drivers, companion package,
      private command path, lifecycle/status/doctor fields, and browser targets
      are removed. Canonical/embedded Python broker and Gateway dynamic,
      signing, managed, and companion surfaces remain for the source-finalizing
      follow-up and are intentionally not claimed complete by this commit.
- [ ] Clarify brokered versus Workspace-owned authentication in all outputs.
- [ ] Synchronize canonical and embedded sources only after implementation
      stabilizes.

## Verify and integrate

- [ ] Re-prove static handle binding, deny-before-resolution, one exact
      replacement, rotation, revocation, no fallback, and secret-free output.
- [x] Run focused Go tests. Evidence: `go test ./internal/... ./cmd/tobari`
      passed on 2026-08-12 after the Go vertical deletion.
- [ ] Review dependency, generated, and image-content diffs.
- [ ] Record `task check`, `task security`, and `task public:check` evidence.
- [ ] Commit only this packet, integrate, and repeat verification.
