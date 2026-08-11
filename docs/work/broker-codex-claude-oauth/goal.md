# Work Goal: Closed Codex and Claude authentication plans

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/07_authentication.md`
- Review/delete trigger: Delete after release evidence is complete and the change passes the full repository gate
- Successor: None
- Owner: Maintainers
- Target: Current change
- Related ADRs: `docs/decisions/0025-broker-codex-claude-oauth.md`

## Outcome

Tobari can acquire and broker reviewed OpenAI Codex and Anthropic Claude
credentials through closed, provider-specific plans without exposing primary
credentials to a Workspace. Codex uses the pinned ChatGPT OAuth flow and
Broker refresh; Claude uses the pinned Claude Code setup-token flow and a
static inference token. The Gateway projects only the exact provider headers
declared by each reviewed plan after policy allows the ordinary HTTP request.

## Why now

The authentication architecture already supports project-bound opaque handles,
but the prior built-in catalog did not close the routine login and projection
journeys for Codex and Claude. ADR 0025 records the reviewed acquisition,
refresh, header, pinning, and trust-boundary decisions needed to add them
without introducing arbitrary executable adapters or provider-specific policy
operations.

## Non-goals

- Support arbitrary OAuth clients, provider endpoints, headers, or executable adapters.
- Forward host credential files or primary provider secrets into Workspaces.
- Add browser automation, PAT discovery, ambient-account selection, or silent login.
- Change GitHub Pages documentation in this worktree.
- Publish images, create a release, or claim live-provider validation from synthetic fixtures.

## Acceptance criteria

- [x] The built-in catalog contains strict OpenAI and Anthropic provider manifests backed by compiled host drivers and exact Gateway projection plans.
- [x] Codex login imports a bounded authorization-code result, stores refresh-capable Broker state, and refreshes only through the fixed reviewed transport.
- [x] Claude login imports a setup token through protected terminal handling and never declares or attempts refresh.
- [x] Brokered handles remain project-, Context-, provider-, revision-, and exact-HTTP-binding scoped; Gateway resolves them only after policy allow.
- [x] Provider headers are injected exactly as specified, with caller-supplied credential headers removed and no secret-bearing output or logs.
- [x] Canonical Broker and Gateway sources are byte-identical to their embedded runtime snapshots.
- [x] Focused Go, Broker, Gateway, security, and integration checks pass.
- [ ] The full `task check` profile passes after the separately maintained GitHub Pages schema table is synchronized.
- [ ] Live-provider manual release checks are recorded before publication or release.

## Governing documents

- Thesis: `docs/00_theses.md`, especially brokered credential and closed-plan invariants
- Product contract section: `docs/01_product_contract.md`, authentication outcomes and CLI contract
- Architecture or security invariant: `docs/02_architecture.md` and `docs/03_security_model.md`, brokered credentials and exact post-policy projection
- Existing ADR: `docs/decisions/0025-broker-codex-claude-oauth.md`

## Completion definition

The work is complete when automated evidence and required live-provider release
evidence satisfy ADR 0025, `task check` passes, durable contracts remain in the
numbered documentation and ADR, and this temporary packet is removed.
