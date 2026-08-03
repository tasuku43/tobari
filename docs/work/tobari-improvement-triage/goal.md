# Work Goal: Triage Tobari product and maintenance backlog

- Status: Active
- Retention: temporary
- Retention reason: Coordinate the reported product-loop, integration, documentation, command-surface, and work-packet hygiene issues until each has a bounded successor or an explicit deferral.
- Governing contract: docs/00_theses.md, docs/01_product_contract.md, docs/02_architecture.md, docs/03_security_model.md, docs/04_harness.md, docs/05_public_repository.md, docs/06_release.md, docs/09_agent_readiness_validation.md
- Review/delete trigger: Delete after every issue has a bounded successor packet or an explicit deferred decision, and the packet-lifecycle audit has been completed.
- Successor: None
- Owner: Tobari maintainers
- Target: Product-loop and repository-maintenance issue triage
- Related ADRs: None

## Outcome

Tobari's current improvement backlog has one reviewed register covering the
policy-review failure, the deferred auth-broker branch, agent integrations and
runtime-image workflows, the README/architecture publication work, the public
CLI surface audit, and completed work-packet cleanup. Each item has a state,
priority order, dependency, owner role, next action, and completion evidence;
implementation work is routed into bounded successor packets rather than being
left as an implicit promise in this coordinator packet.

## Why now

The reported denial-to-review path appears to expose a pending permission in
machine-readable output while the interactive command ends as canceled without
an understandable review screen. That directly challenges Tobari's central
adoption loop. At the same time, the auth-broker work is intentionally paused,
runtime customization is being finalized in a dirty worktree, and the next
documentation and command-surface decisions depend on a clear product boundary.
Several old or empty work-packet paths also make it difficult to tell what is
active, complete, deferred, or merely stale.

## Non-goals

- Do not implement the policy-review fix, agent integrations, documentation site, or command retirement in this coordinator packet.
- Do not merge, rebase, restore, or otherwise alter `codex/auth-broker`.
- Do not create or mutate external GitHub issues, pull requests, branches, or publications as part of triage.
- Do not absorb, rewrite, or clean unrelated changes already present in the worktree.
- Do not delete a non-empty work packet or promote an unverified conclusion merely to make the inventory look complete.

## Acceptance criteria

- [x] The issue register in `context.md` covers all six tracked topics with state, proposed priority, owner role, dependency, next action, and completion evidence. Evidence: the current-main register records the first-wave and second-wave handoffs plus the remaining runtime, publication, image, and coordinator follow-ups.
- [x] The policy-review report has a deterministic PTY reproduction, a bounded successor implementation packet, and separate acceptance criteria for interactive, redirected, and JSON behavior. Evidence: `policy-review-tty` contains the original fix evidence plus the delayed-confirmation/redraw follow-up from the new-user comparison.
- [x] The auth-broker branch is explicitly recorded as deferred and detached from `main`, with a condition for resumption and no accidental reintroduction into the current product path. Evidence: `auth-broker-deferral` is docs-only on `main`, while `codex/auth-broker` is not an ancestor and its implementation paths are absent from `main`.
- [x] The agent-plugin/runtime-skill question has a discovery outcome that preserves Tobari's generic boundary, avoids provider-specific auth shortcuts, and chooses skill, plugin, MCP, or explicit deferral deliberately. Evidence: `agent-integration-discovery` is Complete/evidence with the skill-first conclusion and scoped commit `8e1adfa`.
- [x] README, Quick Start, runtime-image customization, architecture presentation, and any GitHub Pages publication are split into a public-boundary-ready successor with a runnable denial-to-review-to-retry journey. Evidence: `quickstart-runtime-docs` and `architecture-publication` are Complete/evidence with commits `d98e086` and `001c4a7`; external Pages activation remains an explicit owner-side follow-up.
- [x] The CLI audit derives its candidate list from `cli.Catalog`, records compatibility and capability-retirement impact, and does not remove a command without evidence and a recovery decision. Evidence: `cli-catalog-audit` is Complete/evidence with scoped commit `a007059` and no safe removal candidate.
- [x] Existing work packets are classified as active, complete, superseded, deferred, or stale; durable conclusions are promoted before cleanup, and empty stale directories are removed only after their disposition is recorded. Evidence: the current-main lifecycle refresh in `work-packet-retirement` commit `1cc400e` reports no deletion candidate and preserves all active/deferred evidence.
- [x] The relevant child packets name their agent-readiness scenario, discovery-round-trip budget, security/public/release gates, and exact handoff conditions. Evidence: the policy, runtime lifecycle, Quick Start handoff, and PTY evidence successor packets each define these constraints and handoff checklists; implementation remains pending in their own packets.

## Governing documents

- Thesis: [Project theses](../../00_theses.md), especially Theses 0, 7, 8, and 9
- Product contract: [Product contract](../../01_product_contract.md), especially progressive policy learning, command roles, and runtime customization
- Architecture and security: [Architecture](../../02_architecture.md) and [Security model](../../03_security_model.md)
- Harness and readiness: [Harness](../../04_harness.md) and [Agent readiness validation](../../09_agent_readiness_validation.md)
- Public and release: [Public Repository](../../05_public_repository.md) and [Release Model](../../06_release.md)

## Completion definition

This coordinator packet is complete when every issue is either handed to a
bounded successor packet with an explicit dependency and owner role or is
recorded as deliberately deferred with a resumption condition; active packets
are not mislabeled as complete; durable decisions are promoted; stale empty
directories are handled; and the required repository checks for this
coordination change pass. The coordinator packet is then removed as temporary
work history, not retained as a second permanent roadmap.
