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
      Docker image content, tests, and embedded snapshots are absent. The
      static-only Broker now joins only the internal control network; Compose
      and reconciliation no longer give it provider egress, while Gateway
      alone retains the upstream egress interface.
- [x] Clarify brokered versus Workspace-owned authentication in all outputs.
      Evidence: root/scoped help, README, authentication/security contracts,
      threat model, readiness journey, and EN/JA architecture pages distinguish
      Workspace-owned tool state from optional static Broker handles and never
      describe tool-owned credentials as host-managed or outside the Workspace.
      Final catalog review also replaced the retired “managed-credential store”
      outcome with the retained brokered-authentication state.
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
      or upstream processing. Integration follow-up aligned the Compose timeout
      with the bounded static Broker, mounted the required synthetic Gateway
      configuration in the focused harness, updated two retired API-shaped
      Gateway tests, and standardized provider status on `configured`. The
      pinned suites now pass 25 Auth Broker and 27 Gateway/GraphQL/DNS tests;
      the Docker journey reaches Context import/status and project-bound handle
      issuance before the separately scoped learned-policy scheme defect.
- [x] Run focused Go tests. Evidence: `go test ./internal/... ./cmd/tobari`
      passed on 2026-08-12 after the Go vertical deletion; after removing
      dormant Broker egress, `go test ./internal/infra/dockerruntime
      ./internal/infra/runtimeassets` passed on the integration branch.
- [x] Review dependency, generated, and image-content diffs. Evidence: canonical
      and embedded source equality plus the Auth Broker image contract passed;
      retired runtime modules account for the dependency reduction. The local
      toolbox no longer fetches, installs, checks, or notices `pup`/`cwk`, whose
      only repository ownership was the retired Datadog/Chatwork broker flows;
      its retained tools are generic Workspace tools, not broker adapters.
- [x] Review affected machine-output fixture authorities. Evidence: the managed
      credential check, projection name, and companion state were removed;
      `.harness/schemas.json` records the reviewed replacement byte count and
      SHA-256 values.
- [x] Record `task check`, `task security`, and `task public:check` evidence.
      Evidence: `mise exec -- task check` and `mise exec -- task security`
      passed after full auth/policy/site integration on 2026-08-12; the full
      gate includes race tests and Playwright 40/40.
      `task public:check` passed repoguard and contractlint and stopped only at
      the deliberate unpublished Gateway digest checkpoint. The separate live
      synthetic integration subsequently passed on the recovered local Colima
      engine, including static import, deny-before-resolution, exact approval,
      rotation, logout, and re-entry.
- [x] Commit only this packet, integrate, and repeat verification. Evidence:
      the auth retirement was split into public/domain, canonical runtime,
      dormant managed-state, toolbox, and Broker-egress commits; each was
      reviewed and focused-tested before integration. Fast/security and source
      equality checks passed on the integration branch.
