# Work Goal: Make runtime preparation and reuse deterministic

- Status: Draft
- Retention: temporary
- Retention reason: Resolve runtime lifecycle and shell-entry findings from the blind new-user E2E packet.
- Governing contract: `docs/00_theses.md`, `docs/01_product_contract.md`, `docs/02_architecture.md`, `docs/03_security_model.md`, `docs/04_harness.md`, `docs/09_agent_readiness_validation.md`
- Review/delete trigger: Delete after the runtime lifecycle decision, E2E evidence, and durable contract/docs promotion are complete.
- Successor: None
- Owner: Runtime and CLI maintainers
- Target: `runtime init`, `runtime build`, Workspace registration/re-entry, and interactive shell entry
- Related ADRs: None

## Outcome

A new user can prepare or customize a runtime without creating an unrecoverable
Workspace registration, can tell exactly when a build affects a Workspace, and
can re-enter a resulting Workspace through a stable interactive shell.

## Why now

Parent and blind Long-01 E2E runs found that entering before a runtime is ready
can leave an instance requiring deletion/re-registration, while a later
`runtime build` does not refresh an existing Workspace. Blind runs also exposed
an `I have no name!` shell prompt that may be a runtime identity defect. These
facts affect the first-use value signal and must be decided before documenting
the runtime path further.

## Non-goals

- Do not add an implicit network pull or project-local runtime authority.
- Do not silently replace a running Workspace image.
- Do not add a shell selector, unrestricted executor, or provider adapter.
- Do not change the detached `codex/auth-broker` branch.
- Do not rewrite the public Quick Start until the runtime contract is decided.

## Acceptance criteria

- [x] The packet records and implements a reviewed precondition for
      runtime-not-ready registration that prevents a broken Workspace.
- [x] The packet records the explicit new-Workspace-only image-build contract;
      existing Workspaces are not silently refreshed.
- [x] The interactive shell identity/prompt is reproduced in a clean PTY and
      explained by the public-safe runtime identity contract; no host username
      or path is committed.
- [x] A fresh disposable project completes the chosen runtime path through a
      real 120x40 PTY, with runtime build/re-entry evidence; cleanup remains
      host-owned and is replayed before packet closure.
- [ ] Existing Context ownership, fixed Workspace lifetime, credentials, and
      network boundaries remain unchanged unless governing documents are
      updated in the same change.
- [ ] `task runtime:test`, `task check`, `task security`, and
      `task public:check` pass, or exact external blockers are recorded.

## Governing documents

- Thesis: [Project theses](../../00_theses.md)
- Product contract: [Product contract](../../01_product_contract.md)
- Architecture: [Architecture](../../02_architecture.md)
- Security: [Security model](../../03_security_model.md)
- Harness/readiness: [Harness](../../04_harness.md) and [Agent Readiness Validation](../../09_agent_readiness_validation.md)

## Completion definition

The runtime lifecycle decision is implemented or explicitly documented,
human-facing runtime behavior has a fresh E2E proof, the shell identity finding
has a disposition, required gates pass, and the durable consequences are
promoted before this temporary packet is removed.

## Evaluation and handoff

- Agent-readiness scenario: runtime customization and Workspace re-entry from
  `docs/09_agent_readiness_validation.md`, exercised through a fresh
  disposable project and a real 120x40 PTY.
- Discovery budget: the implementation replay records each README/help lookup
  and exact recovery lookup; the human journey targets the existing two
  lifecycle lookups (bootstrap and cleanup), with extra lookups recorded as
  friction rather than hidden recovery.
- Required gates: `task runtime:test`, `task check`, `task security`,
  `task public:check`, and the relevant integration/readiness profile.
- Handoff: the implementing agent commits the scoped change, reports the SHA,
  records the E2E and external-blocker result, and updates the dependent
  Quick Start packet without changing unrelated runtime or auth work.
