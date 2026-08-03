# Work Goal: Audit the public CLI catalog

- Status: Active
- Retention: temporary
- Retention reason: Hold catalog inventory and E2E evidence until the coordinator promotes durable command-retirement or documentation decisions.
- Governing contract: `cli.Catalog` is the sole public command and invocation source of truth; `task check` is the implementation gate.
- Review/delete trigger: Delete after the classifications and E2E evidence are promoted to the coordinator or a successor packet.
- Successor: None
- Owner: CLI catalog audit
- Target: First-wave Tobari issue triage
- Related ADRs: `docs/decisions/0012-own-workspace-container-lifetime.md`, `docs/decisions/0015-defer-devcontainer-integration.md`

Execution note: the audit content and E2E are complete, but its scoped commit
is blocked by the repository's paused rebase and Git metadata permissions.

## Outcome

Produce a complete, catalog-derived inventory of every public Tobari command and
its implementation reachability, role, effect, help, recovery, and opaque
reference flow. Classify each command as maintain, integrate, delete, or hold
with evidence. Confirm the classifications through a clean build, root and
scoped human/agent help, and representative real argv executions that observe
reachability, output, faults/recovery, and side-effect boundaries. Remove only
disjoint dead CLI definitions when the contract and E2E evidence make the
removal unambiguous.

## Why now

The repository contains a CWD-owned lifecycle contract alongside legacy named
lifecycle implementations that are no longer catalog-registered. Policy
discovery, human review, and compatibility projections also have overlapping
surfaces. A catalog-first E2E audit is needed before documentation or command
retirement work can safely proceed.

## Non-goals

- Do not edit `docs/work/tobari-improvement-triage/*` or another work packet.
- Do not redesign the CWD-owned lifecycle, policy-learning model, Context model,
  or runtime-image workflow.
- Do not remove persisted state, migrate user data, or change a public command
  contract without a separate reviewed retirement decision.
- Do not treat unit or static inventory as completion without executable E2E
  evidence.
- Do not add a second command registry or a provider-specific adapter.

## Acceptance criteria

- [x] Every command returned by `cli.DefaultCatalog()` is mapped to role, effect,
      capability, inputs, output/delivery/coverage, prerequisites, stable
      faults/recovery, reference producers/consumers, and handler reachability.
- [x] Root and scoped human help plus root and scoped agent help are generated
      from a clean build and recorded in the packet.
- [x] Representative user argv for every retained command and each deletion or
      integration candidate is executed; reachability, output, fault/recovery,
      and side-effect boundaries are recorded.
- [x] Candidates are classified as delete, integrate, maintain, or hold with
      evidence and explicit compatibility/security reasoning.
- [x] Any production deletion is a disjoint dead-definition cleanup with
      catalog, negative-path, and E2E evidence; otherwise it remains a follow-up
      candidate.
- [x] `task check` passes; `task public:check` passes when packet changes affect
      the public repository boundary.
- [x] E2E transcript/fixtures and gate outputs contain no secrets or private
      identifiers.

## Governing documents

- Thesis: `docs/00_theses.md`, especially Theses 0, 5, 7, and 8.
- Product contract: public command table, output/exit contract, and compatibility.
- Architecture: four-layer direction and catalog source-of-truth sections.
- Security model: controlled side effects, opaque references, and fail-closed
  recovery.
- Harness: catalog contracts, agent-readiness scenarios, and completion gates.
- Existing retirement template: `docs/work/_template/capability-retirement.md`.

## Completion definition

The work is complete when the catalog inventory, classifications, E2E transcript,
and required gates have evidence, any safe dead-code cleanup is verified, no
other packet was changed, and the temporary packet is ready for coordinator
handoff or removal after its conclusions are promoted.
