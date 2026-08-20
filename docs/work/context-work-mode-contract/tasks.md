# Work Tasks: Define Context as a stable reusable work mode

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [ ] Re-read governing docs, ADR 0029, ADR 0062, ADR 0067, related active work
      packets, and the add-capability Skill at implementation time.
- [ ] Inventory every whole-Context immutability claim and every public/catalog
      field that needs classification.
- [ ] Verify the exact code/test activation point for Runtime, shell, Git, and
      bootstrap changes.
- [ ] Audit existing mechanical checks for Boundary immutability and no silent
      Workspace-home rewrite.
- [x] Confirm the public outcome and non-goals. Evidence: product-owner
      approval in the main design session on 2026-08-21.

## Decide

- [x] Define Context as a stable reusable work mode. Evidence: product-owner
      approval on 2026-08-21.
- [x] Keep the creation-time Boundary immutable and avoid a new Boundary
      resource. Evidence: product-owner approval on 2026-08-21.
- [x] Keep one exact mutable Runtime binding on Context and reject a
      per-Workspace override for V1. Evidence: product-owner approval and ADR
      0067.
- [x] Classify shell/Git as session defaults and bootstrap as a creation
      default, subject to exact code verification. Evidence: accepted design
      on 2026-08-21.
- [x] Set repeated same-Boundary/different-Runtime self-use demand as the
      reconsideration trigger. Evidence: accepted design on 2026-08-21.
- [ ] Select and accept the revising ADR form after inspecting repository ADR
      conventions and current active packet disposition.

## Implement

- [ ] Add or revise the durable ADR before mechanism/local wording changes.
- [ ] Revise Thesis 9 and propagate the definition through product,
      architecture, security, harness, catalog, and readiness docs.
- [ ] Update inaccurate machine-field descriptions without changing schemas.
- [ ] Add missing Boundary immutability and activation-timing enforcement.
- [ ] Reconcile `../context-capability-envelope/` and related child packet
      status without discarding unfinished evidence.
- [ ] Do not implement onboarding, Context derivation, Runtime guide, or
      progressive-disclosure UI in this packet.

## Verify

- [ ] Focused Context/Runtime/config/bootstrap/catalog tests pass. Evidence:
- [ ] No catalog mutation can change creation-time Boundary facts. Evidence:
- [ ] Runtime adoption, shell/Git late binding, and bootstrap future-only timing
      agree with durable claims. Evidence:
- [ ] Existing Workspace Context identity and home remain unchanged. Evidence:
- [ ] `task check` passes. Evidence:
- [ ] `task security` passes when required by changed security enforcement.
      Evidence:
- [ ] Generated diff and repository status are understood. Evidence:

## Hand off

- [ ] Acceptance criteria have evidence.
- [ ] Related active packets contain no conflicting definition.
- [ ] Durable decisions are promoted out of the packet.
- [ ] Temporary diagnostics are removed.
- [ ] The packet is removed in the same completion commit.
- [ ] The implementation is one concern-specific commit on main.
