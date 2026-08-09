# Work Tasks: Host-driven brokered CLI authentication

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read governing theses, product, architecture, security, harness,
      authentication, external API, and readiness documents.
- [x] Observe the base runtime, optional toolbox, provider schema, vault,
      Gateway, Auth Broker, and login helpers.
- [x] Confirm the user's correction that public/base images must not become the
      provider tool catalog.
- [x] Verify Docker host-service and AWS CLI refresh/export contracts.
- [x] Inspect TWG's refresh/export surface and record why general typed refresh
      is not yet claimed.

## Decide

- [x] Select a resident host companion and post-policy Broker call.
- [x] Select an encrypted reverse `docker exec` channel rather than a
      cross-kernel UDS or host listener.
- [x] Keep arbitrary executable selection out of provider manifests.
- [x] Finalize protocol schemas, state bounds, session rotation, and driver
      executable identity.
- [x] Rewrite ADR 0020 and propagate its consequences through durable docs.

## Implement

- [x] Remove new tools and their contracts from the published base runtime.
- [x] Add cwk and pup to the optional local toolbox.
- [x] Add companion lifecycle, reverse bridge, health, and status reporting.
- [x] Add strict host-driver request/response contracts and fake driver tests.
- [x] Move GitHub login to the trusted-host fixed driver and remove provider
      CLIs from the Auth Broker image.
- [x] Move AWS login execution to the trusted host.
- [x] Store only encrypted opaque AWS driver state and automate refresh through
      host AWS CLI credential export.
- [x] Refactor Broker refresh locking and stale-result rejection.
- [x] Complete AWS Gateway/SigV4 negative and synthetic end-to-end tests,
  including exact post-policy request snapshot mutation canaries.
- [x] Sync canonical Auth Broker/Gateway sources into embedded snapshots.
- [x] Update catalog faults, capability ledger, docs, and readiness fixture.

## Verify

- [x] Focused Go and Python tests pass. Evidence: Auth Broker 71/71; Gateway
      55/55; targeted Go unit, race, vet, Darwin PTY, and Linux arm64 PTY race
      suites pass. The Linux cancellation suite passed 20 repetitions in a
      network-disabled disposable container.
- [x] Local toolbox build/version smoke passes. Evidence: local image built
      with Git 2.39.5, GitHub CLI 2.96.0, AWS CLI 2.36.11, kubectl 1.36.3,
      TWG 1.1.1, cwk 0.2.4, and pup 1.10.5.
- [x] `task check` passes. Evidence: full profile passed twice after the final
      security hardening.
- [x] `task security` passes. Evidence: gosec, govulncheck, security repoguard,
      module verification, and site-source verification pass; govulncheck
      reports no vulnerabilities.
- [x] `task public:check` passes. Evidence: public repoguard, generated-site
      source checks, static build, and distribution verification pass.
- [x] `task release:check` passes. Evidence: release tests and `lint-release`
      pass.
- [x] Agent-readiness and zero external-processing checks pass. Evidence:
      catalog/readiness contracts and the synthetic Gateway/Broker/fake-
      companion AWS flow pass. The real Docker integration profile passes with
      API v2, deny-before-host-execution, one post-allow host refresh, SigV4
      signing, one upstream attempt, and complete test-resource cleanup.
- [x] Generated diff and unrelated worktree changes are understood. Evidence:
      `git diff --check` passes; the pre-existing Context configuration, Git
      identity, and architecture-site work remain preserved and unstaged.

## Hand off

- [ ] Compatible Gateway/Auth Broker API-v2 images are published, independently
      inspected, and pinned by the explicitly authorized release action.
- [ ] Durable conclusions are promoted and this temporary packet is removed.
- [x] Handoff material explains UX, refresh boundaries, checks, compatibility,
      and known provider limits in the durable documentation; final delivery
      reports the remaining live-integration and publication boundaries.
