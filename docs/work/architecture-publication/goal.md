# Work Goal: Publish the second-wave architecture presentation

- Status: Complete
- Retention: evidence
- Retention reason: Reproducible public-site, local-E2E, and publication-gate evidence for the second-wave architecture presentation.
- Governing contract: docs/00_theses.md, docs/01_product_contract.md, docs/02_architecture.md, docs/03_security_model.md, docs/04_harness.md, docs/05_public_repository.md, docs/06_release.md, docs/09_agent_readiness_validation.md
- Review/delete trigger: Delete after the next architecture publication replaces this evidence or after one release review confirms the Pages workflow and static artifact remain current.
- Successor: None
- Owner: Tobari maintainers
- Target: Public GitHub Pages architecture presentation on the current main product line
- Related ADRs: None

## Outcome

Tobari has a clear, public-safe, English static architecture presentation at
`docs/architecture-site/`. It shows the four-layer dependency direction, the
host/agent/Gateway/OPA trust boundaries, the CWD-owned Workspace lifecycle, the
denial-to-review-to-retry policy loop, and Context runtime customization. The
presentation is linked from `docs/README.md`, and a least-scope GitHub Pages
workflow publishes only that repository-owned directory.

## Why now

The current main product line has durable theses and contracts for bounded
autonomy, a shared Gateway/OPA boundary, reusable CWD-owned Workspaces,
progressive policy learning, and Context composition. A second-wave public
presentation makes those relationships reviewable without adding another
runtime or CLI contract.

## Non-goals

- Do not change the root README, production Go code, CLI catalog, numbered contracts, auth-broker work, first-wave packets, or runtime/Gateway implementation.
- Do not add JavaScript, a CDN, remote fonts, images, credentials, private URLs, vendor endorsements, or copied external content.
- Do not deploy, push, or publish from the agent; the workflow only defines the repository-owned Pages publication path.
- Do not make the presentation a substitute for the numbered contracts or an interactive control surface.

## Acceptance criteria

- [x] The static page presents all five requested architecture topics with semantic headings, concise explanatory copy, and responsive accessible layout.
- [x] Agent discovery and interpretation remain bounded: the page is read-only, has deterministic in-page section navigation, contains no hidden action or reference input, and does not imply that display position grants permission.
- [x] The page uses only local HTML/CSS assets, has no remote runtime dependency, and the Pages workflow uploads only `docs/architecture-site`.
- [x] The local E2E validates HTML/CSS and links, serves the directory, fetches the page, checks required sections/terms, and records exact commands and results without local paths or secrets.
- [x] `task check` and `task public:check` pass in the clean-checkout verification; `task release:check` is blocked by the pre-existing ShellCheck warning recorded precisely in [e2e-transcript.md](e2e-transcript.md).
- [x] One intentional commit contains only the allowed paths and is handed off with its SHA in the final response; unrelated worktree changes remain unstaged and preserved.

## Governing documents

- Thesis: [Project theses](../../00_theses.md), especially bounded autonomy, network topology, CWD-owned Workspaces, denial as policy-development interface, and Context composition.
- Product contract section: [Product contract](../../01_product_contract.md), especially public vocabulary, Workspace lifecycle, output/recovery, and Context runtime customization.
- Architecture or security invariant: [Architecture](../../02_architecture.md) and [Security model](../../03_security_model.md), especially four-layer direction, Gateway/OPA flow, host-derived principals, and separated stores.
- Existing ADR: None; this is a presentation/publication slice and does not revise a durable decision.

## Completion definition

The page and workflow are reviewed against the numbered contracts, the bounded
local E2E and required publication/release gates have evidence, the final diff
contains only the allowed paths, and one scoped commit is created on `main`.
