# Work Goal: Ship the first coherent development prerelease

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/00_theses.md`, `docs/03_security_model.md`, and `docs/06_release.md`
- Review/delete trigger: Delete after durable conclusions are promoted and the change completes
- Successor: None
- Owner: Release owner
- Target: `v0.1.0-dev.1`
- Related ADRs: None

## Outcome

Tobari has a reusable standard versus experimental capability profile, AWS
authentication is available only in the experimental repository build, and one
protected prerelease workflow publishes the exact Gateway, Auth Broker, and CLI
release subjects from one reviewed source revision while the agent-ready base
is built locally from the released CLI's pinned embedded recipe.

## Why now

The first development prerelease must be the first intentional GHCR
publication. Existing builds expose AWS authentication equally, and the
release/public boundary must not redistribute the bundled agent binaries.

## Non-goals

- Stable Homebrew publication.
- Runtime selection through an environment variable or user-enabled feature flag.
- Removing AWS CLI from the agent-ready base runtime.
- Claiming that inactive experimental implementation bytes are absent from every image.

## Acceptance criteria

- [x] `task build` selects local components and the standard capability profile.
- [x] `task build:dev` selects local components and the experimental profile, including AWS authentication.
- [x] A standard or release binary does not advertise, project, or invoke AWS authentication.
- [x] `version` exposes the compiled capability profile.
- [ ] `v0.1.0-dev.1` passes release classification and creates a GitHub prerelease without a Homebrew mutation.
- [x] One generated, uncommitted component lock binds Gateway and Auth Broker immutable digests to the same revision before CLI packaging.
- [x] A missing built-in agent-ready base is built locally from the CLI's pinned embedded recipe and is never pulled from or pushed to GHCR.
- [x] Only the protected Release workflow can publish Tobari GHCR packages.
- [x] Required implementation, security, public, and release gates pass after the local-base redesign.

## Governing documents

- Thesis: brokered provider plans and release authority
- Product contract section: build identity, authentication, and distribution
- Architecture or security invariant: compiled closed provider plans and generated release authority
- Existing ADR: None

## Completion definition

The work is complete when the durable contracts and executable gates agree,
the required profiles pass, existing GHCR packages have been deliberately
removed and verified absent, and this temporary packet is removed.
