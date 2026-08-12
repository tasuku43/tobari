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
- [x] Remove managed injection and its store, selector, Gateway path, mounts,
      status, tests, and assets. Evidence: the Gateway has no managed adapter
      or selector, the shared Compose contract mounts no credential directory,
      and the managed-file doctor check and runtime environment were removed.
      Follow-up retirement audit also removed Context and aggregate
      `credentials.json`/`credentials/` state, hash/copy paths, and public
      schema fields; GraphQL endpoint classification now uses the strict
      secret-free `gateway.json` projection and `TOBARI_GATEWAY_CONFIG`.
- [x] Remove AWS, Datadog, OpenAI, Anthropic, Chatwork, companion, refresh,
      signing, host drivers, exact-version contracts, state readers, and
      unowned dependencies/assets. Evidence: canonical Auth Broker and Gateway
      sources retain only static `primary_secret` header resolution; retired
      provider, refresh, signing, companion modules, dispatcher operations,
      Docker image content, tests, and embedded snapshots are absent.
- [ ] Clarify brokered versus Workspace-owned authentication in all outputs.
- [x] Synchronize canonical and embedded sources only after implementation
      stabilizes. Evidence: the repository sync scripts generated both embedded
      snapshots and both source-equality checks passed.

## Verify and integrate

- [x] Re-prove static handle binding, deny-before-resolution, one exact
      replacement, rotation, revocation, no fallback, and secret-free output.
      Evidence: 25 Auth Broker tests and 26 Gateway/GraphQL/DNS tests passed in
      pinned mitmproxy containers; negative cases cover retired operations,
      terminal denial, invalid-handle no-fallback, rotation, and revocation.
      A dedicated Gateway negative test proves the retired
      `x-tobari-credential-profile` selector fails before broker, OPA, fallback,
      or upstream processing.
- [x] Run focused Go tests. Evidence: `go test ./internal/... ./cmd/tobari`
      passed on 2026-08-12 after the Go vertical deletion.
- [x] Review dependency, generated, and image-content diffs. Evidence: canonical
      and embedded source equality plus the Auth Broker image contract passed;
      retired runtime modules account for the dependency reduction.
- [x] Review affected machine-output fixture authorities. Evidence: the managed
      credential check, projection name, and companion state were removed;
      `.harness/schemas.json` records the reviewed replacement byte count and
      SHA-256 values.
- [ ] Record `task check`, `task security`, and `task public:check` evidence.
- [ ] Commit only this packet, integrate, and repeat verification.
