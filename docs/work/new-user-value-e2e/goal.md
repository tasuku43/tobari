# Work Goal: Exercise new-user value journeys through a real pseudo-TTY

- Status: Active
- Retention: evidence
- Retention reason: Preserve human-experience E2E transcripts, friction findings, and command-surface observations until the next product or documentation slice promotes the conclusions.
- Governing contract: `docs/00_theses.md`, `docs/01_product_contract.md`, `docs/02_architecture.md`, `docs/03_security_model.md`, `docs/04_harness.md`, `docs/09_agent_readiness_validation.md`
- Review/delete trigger: Delete after every actionable finding has a successor packet or explicit deferral and the conclusions are promoted to durable contracts or a reviewed CLI decision.
- Successor: None
- Owner: Tobari product and repository maintainers
- Target: Current `main`; four new-user value journeys
- Related ADRs: None

## Outcome

The repository contains evidence from two long and two medium new-user
journeys executed through real pseudo-TTY sessions. Each journey records what a
new user sees, types, waits for, misunderstands, and uses as recovery. The
evidence distinguishes a genuine human success from a harness-only success,
identifies missing or misleading transitions, and records candidates where the
CLI should be integrated, narrowed, documented, or retired without changing
the public command surface during this investigation.

## Why now

Tobari's value is experienced at the boundary between an untrusted agent and a
trusted host: a user should enter an isolated project, reach useful work,
understand a denied operation, grant only the exact permission intended, and
leave a reusable environment. Existing contract and integration tests prove
many individual invariants, but they do not yet answer whether a first-time
human can complete the value loop through the actual terminal presentation.

The user also wants evidence about whether the current command surface is too
wide or fragmented. That question must be answered from observed journeys and
catalog/recovery impact, not from command names alone.

## Non-goals

- No production code, CLI command, catalog entry, help text, runtime image, or
  policy behavior changes in this packet.
- No command removal, rename, alias, or workflow integration is implemented
  from an anecdotal observation.
- No non-TTY, piped, JSON-only, direct-Docker, direct-OPA, source-inspection, or
  guessed-reference run counts as human journey success.
- No credentials, private URLs, local absolute paths, shell history, or raw
  confidential output is committed.
- No parallel Docker/Gateway sessions are started when they could share or
  mutate the same cluster state; scenario runs are sequenced.

## Acceptance criteria

- [ ] `scenarios.md` defines exactly two long and two medium journeys with
      preconditions, human-visible steps, value signal, success/failure
      criteria, cleanup, and allowed discovery-round-trip budget.
- [ ] Every journey is attempted through a real pseudo-TTY with a terminal
      size, human-paced keystrokes, raw control-sequence capture, screen-state
      checkpoints, timing, and exit status recorded.
- [ ] Every journey has a feedback file under `feedback/` describing the
      observed path, blockers, missing transitions, accidental complexity, and
      command integration/narrowing/retirement candidates.
- [ ] Findings distinguish product failure, environment blocker, documentation
      gap, presentation friction, and command-surface hypothesis.
- [ ] No routine successful journey relies on source inspection, provider
      notation decoding, exploratory calls, or an undeclared parser; the
      discovery-round-trip count is recorded for each scenario.
- [ ] The packet records the exact E2E commands, environment boundaries,
      cleanup result, and any unproven claim without presenting a blocked run as
      a product success.
- [ ] `task check` and `task public:check` pass for the final evidence packet;
      release/runtime/integration blockers are recorded precisely when the
      relevant profile cannot complete.

## Governing documents

- Thesis: [Project theses](../../00_theses.md), especially the safe path,
  denial-to-retry, CWD ownership, and bounded autonomy theses.
- Product contract: [Product contract](../../01_product_contract.md), workspace
  lifecycle, policy learning, exact references, and recovery commands.
- Architecture/security: [Architecture](../../02_architecture.md) and
  [Security model](../../03_security_model.md), layer ownership, trust
  boundaries, deny-by-default, and controlled Docker/network effects.
- Harness: [Harness](../../04_harness.md), pseudo-TTY policy review, integration
  profiles, public evidence, and completion gates.
- Agent readiness: [Agent Readiness Validation](../../09_agent_readiness_validation.md),
  human handoff and routine-success processing budgets.

## Completion definition

This packet is complete when all four journeys have a bounded attempted E2E or
an explicit environment blocker, each feedback file is committed, the
comparison identifies the strongest next actions without implementing them,
the required repository gates pass, and the worktree contains no sensitive
transcript artifact.
