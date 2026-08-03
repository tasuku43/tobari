# Work Goal: Agent integration discovery

- Status: Complete
- Retention: evidence
- Retention reason: Preserve the dated, environment-specific E2E transcript and surface-selection evidence until the follow-up skill slice is reviewed and its conclusions are either promoted or replaced by a repeatable proof.
- Governing contract: `docs/00_theses.md` Thesis 8 and Thesis 9; `docs/09_agent_readiness_validation.md`
- Review/delete trigger: Delete after the follow-up skill packet records the decision and this E2E evidence is linked from a durable contract or replaced by a repeatable harness result.
- Successor: None
- Owner: Agent integration discovery
- Target: First-wave Tobari issue triage, 2026-08-03
- Related ADRs: None

## Outcome

Tobari does not need a new authority-bearing plugin or MCP server at this
stage. The existing catalog-owned CLI already closes the local agent journey:
an in-Tobari request is denied, the agent receives fixed secret-free host
navigation, the host reviews an exact opaque candidate, one explicit allow or
deny action activates policy, and the request is retried. The next integration
slice should therefore be a small standalone, cross-agent skill that teaches
Codex and Claude Code to use those existing commands. A separate runtime-build
skill may follow the same pattern. Plugin packaging and MCP remain deferred
until repeated skill use or a real external-service requirement demonstrates
that distribution or live tools add value.

## Why now

Policy authoring and Context runtime customization are the two places where an
agent user can otherwise fall back to host execution or manual Docker/OPA
operation. The repository already contains both workflows and an executable
integration harness, so the surface choice can be tested before adding another
command registry, authentication layer, or executor.

## Non-goals

- No production Go, CLI catalog, Gateway, OPA, runtime image, or existing agent image changes.
- No generic authentication broker, OAuth/PAT/keychain integration, or credential forwarding surface.
- No arbitrary shell, Docker, filesystem, network, or policy executor exposed as an agent tool.
- No MCP server, published plugin, marketplace entry, or claim of endorsement by OpenAI or Anthropic.
- No change to the public policy, Context, runtime, opaque-reference, or recovery contracts.

## Acceptance criteria

- [x] The decision is explicit: create the agent integration as a skill-first slice; do not add a plugin or MCP server yet.
- [x] An existing agent-equivalent CLI workflow is executed end to end, including input, Tobari enforcement, expected result, explicit recovery, retry, and boundary canaries; transcript and processing stages are recorded in `context.md`.
- [x] The proposed Codex/Claude Code boundary preserves catalog ownership, discover/act separation, opaque-reference round trips, host-only policy authority, fixed Context/runtime targets, and zero credential exposure.
- [x] The custom runtime image workflow is evaluated separately from policy recovery and remains a host-side explicit build action.
- [x] No production code, existing CLI, coordinator packet, or other work packet is changed; only this child packet is produced.
- [x] Required verification passes: the real agent-equivalent integration workflow reaches `integration: OK`, and repository check/public gates pass after this packet is written; the unrelated current-worktree TTY helper issue is recorded separately.

## Governing documents

- Thesis: [Project theses](../../00_theses.md), especially the North Star, Thesis 8, and Thesis 9.
- Product contract section: [Product Contract](../../01_product_contract.md), public command table, input/output contract, Context/runtime contract, and side-effect boundary.
- Architecture or security invariant: [Architecture](../../02_architecture.md) command catalog and controlled Docker boundary; [Security Model](../../03_security_model.md) trust boundaries, credential scope, and mutation policy.
- Harness: [Harness](../../04_harness.md), adoption check, integration profile, and zero undeclared external-processing rule.
- Agent readiness: [Agent Readiness Validation](../../09_agent_readiness_validation.md), required policy-learning and runtime-customization scenarios.

## Completion definition

This discovery work is complete because the surface decision has evidence from
the real integration profile, the no-new-authority boundary is explicit, no
durable contract requires revision, and follow-up implementation is bounded to
standalone skill slices. The packet is retained temporarily as evidence until
the follow-up decision is promoted or the evidence is replaced by a repeatable
harness result.
