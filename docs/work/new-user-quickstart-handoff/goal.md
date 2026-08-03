# Work Goal: Make the first-use host handoff executable

- Status: Draft
- Retention: temporary
- Retention reason: Promote blind new-user findings into a runnable, public-safe Quick Start and recovery handoff.
- Governing contract: `docs/00_theses.md`, `docs/01_product_contract.md`, `docs/02_architecture.md`, `docs/03_security_model.md`, `docs/04_harness.md`, `docs/05_public_repository.md`, `docs/06_release.md`, `docs/09_agent_readiness_validation.md`
- Review/delete trigger: Delete after the public journey, recovery commands, runtime prerequisites, and cleanup ownership have durable docs/tests and gates.
- Successor: None
- Owner: Documentation and CLI maintainers
- Target: README Quick Start, denial/exit handoff, runtime prerequisites, and cleanup guidance
- Related ADRs: None

## Outcome

A new user can start from a project directory, understand which actions belong
on the host versus inside the Workspace, follow only executable recovery cues
after a denial, understand runtime preparation and reuse, and clean up the
intended project/shared state without source, JSON, Docker, or command guesses.

## Why now

Blind E2E confirmed that the core loop works but exposed an unavailable
`tobari retry` message, implicit host/Workspace ownership, runtime lifecycle
preconditions, `context show` grammar friction, cleanup ownership questions,
and Docker VM sharing pitfalls for disposable projects.

## Non-goals

- Do not add a retry alias or new public command solely to match stale wording.
- Do not add an unrestricted in-Workspace Tobari control path.
- Do not decide runtime refresh semantics; consume the runtime packet's decision.
- Do not publish GitHub Pages or architecture HTML in this packet.
- Do not change policy, credentials, image authority, or security boundaries.

## Acceptance criteria

- [ ] README and exact help/recovery text name only executable catalog paths.
- [ ] The denial and Workspace-exit handoff explicitly identify the host owner
      of review/recovery without exposing secrets or private endpoints.
- [ ] Runtime prerequisites, new-Workspace effect, shell expectation, and
      cleanup ownership match the completed runtime lifecycle decision.
- [ ] `context show` grammar and Docker VM/shared-path prerequisites are
      understandable through public-safe synthetic examples.
- [ ] A fresh new-user PTY replay reaches the documented value signal and
      cleanup after the policy/runtime successor packets land.
- [ ] `task check`, `task public:check`, and relevant release/integration gates
      pass or exact blockers are recorded.

## Governing documents

- Thesis: [Project theses](../../00_theses.md)
- Product: [Product contract](../../01_product_contract.md)
- Architecture/security: [Architecture](../../02_architecture.md) and [Security model](../../03_security_model.md)
- Harness/public/release: [Harness](../../04_harness.md), [Public Repository](../../05_public_repository.md), and [Release](../../06_release.md)
- Readiness: [Agent Readiness Validation](../../09_agent_readiness_validation.md)

## Completion definition

The public first-use path contains only executable, contract-aligned guidance,
the dependent policy/runtime decisions are reflected, a real PTY replay and
public gates pass, and no private or stale transcript material remains.

## Evaluation and handoff

- Agent-readiness scenario: the denial-to-review-to-retry and runtime
  customization handoffs in `docs/09_agent_readiness_validation.md`, replayed
  through the public README/help path in a real 120x40 PTY.
- Discovery budget: preserve the new-user budgets from the source journeys:
  no more than three lookups for the long first-use path and no more than two
  for the medium bootstrap/lifecycle path; every extra lookup is recorded.
- Required gates: `task check`, `task public:check`, relevant release/runtime
  checks, and the supported integration/readiness replay where the environment
  permits it.
- Handoff: the implementing agent commits only the public docs/help changes,
  reports the SHA and exact replay result, and marks this packet complete only
  after policy/runtime dependencies are verified.
