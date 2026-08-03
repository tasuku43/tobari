# Work Goal: Keep the auth-broker experiment detached from `main`

- Status: Accepted
- Retention: temporary
- Retention reason: Mechanical evidence for the explicit decision to keep the auth-broker experiment out of the supported `main` product surface.
- Governing contract: `docs/00_theses.md`, `docs/01_product_contract.md`, `docs/02_architecture.md`, `docs/03_security_model.md`, `docs/04_harness.md`, `docs/07_authentication.md`, `docs/08_external_api_contracts.md`, and `docs/09_agent_readiness_validation.md`
- Review/delete trigger: Delete after the deferral is durable or a successor packet is accepted and this packet has been committed.
- Successor: None; any restart must create a new reviewed packet.
- Owner: Tobari maintainers
- Target: `codex/auth-broker`
- Execution state: Explicitly deferred; do not schedule or integrate until the maintainer resumes this packet
- Related ADRs: Branch-only proposals `codex/auth-broker:docs/decisions/0013-host-side-agent-auth-broker.md` and `codex/auth-broker:docs/decisions/0014-close-auth-broker-execution-modes.md`; neither is accepted into `main` by this packet.

## Outcome

`main` remains the supported Tobari product boundary. Its CWD-owned
isolation, Gateway policy-learning, Context, and runtime surfaces remain
available, while auth-broker implementation and auth-profile authoring remain
reachable only through the separate `codex/auth-broker` ref. A clean archive of
`main` is built and tested, its public help/Catalog is checked for absence,
and branch-only experiment paths are proven with direct Git object queries. No
authentication implementation is reintroduced or merged.

## Why now

The auth-broker branch contains a materially larger provider-facing capability
slice than the supported product. The explicit decision is to lower its
priority and keep it detached until Tobari's core value is coherent. This must
be executable because a future merge or Catalog extension could silently turn
an experiment into a public authority surface.

## Non-goals

- Reimplementing, repairing, cherry-picking, merging, rebasing, or deleting the experiment
- Updating a branch ref, checking out another ref, or absorbing existing worktree changes
- Changing `docs/work/tobari-improvement-triage/*`, production code, or durable governing documents
- Removing the retained generic Gateway credential-profile or tool-native authentication contracts
- Deciding or implementing provider-specific OAuth, SigV4, Keychain, browser, token-parser, or login integrations

## Acceptance criteria

- [x] Current refs, merge-base, divergence, changed scope, and auth-related tree reachability are recorded.
- [x] A `main`-based full build/test gate passes without checking out or incorporating the dirty worktree.
- [x] `main` Catalog, capability ledger, agent help, and `help auth` prove auth-broker is not usable from the supported surface.
- [x] Experiment paths resolve from `codex/auth-broker` but not from `main`.
- [x] Deferral reasons and future restart conditions are stated in thesis, product, architecture, security, and harness terms.
- [x] No coordinator packet, code, or branch ref is changed by this work.
- [x] The required governance-boundary E2E completes without an environment blocker.
- [x] These four evidence files are committed on `main` as a docs-only deferral
      record; the auth implementation remains branch-only.

## Governing documents

- Thesis: `docs/00_theses.md`, especially bounded autonomy, Thesis 3 (tool-owned authentication), Thesis 7 (executable claims), Thesis 9 (separated Context stores), and deliberate non-goals.
- Product: `docs/01_product_contract.md`, public command/catalog and credential boundaries.
- Architecture: `docs/02_architecture.md`, four-layer direction, Catalog ownership, and controlled side effects.
- Security: `docs/03_security_model.md`, trust boundaries, project-principal binding, secrets, and provider exclusions.
- Harness: `docs/04_harness.md`, required quality/public/security profiles and claims-to-checks discipline.
- Authentication/API/readiness: `docs/07_authentication.md`, `docs/08_external_api_contracts.md`, and `docs/09_agent_readiness_validation.md`.

## Completion definition

The governance evidence is recorded and the packet is now parked. It is not a
current-wave implementation or integration dependency. On explicit resumption,
the maintainer must revalidate the refs and branch-only reachability, rerun the
required gates, and create a new reviewed successor packet. The four evidence
files may remain as the durable deferral record; nothing from the
implementation branch may be merged or cherry-picked into the current product
line before that review. The clean main `task security` run is passing at the
time of this packet's commit.
