# Work Tasks: Explain every authority by scope, lifetime, owner, and precedence

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [ ] Re-read governing docs, ADRs 0028/0029/0039/0049/0051/0058/0059/0066,
      related policy code, and the add-capability Skill at implementation time.
- [ ] Trace ordinary and Host Loopback policy branches from typed request to
      final enforcement and record exact precedence.
- [ ] Build the Scope/Lifetime/Owner/Precedence evidence matrix with one or more
      existing tests for every row and edge.
- [ ] Resolve the Advanced Rego and baseline-deny/exact-Deny ordering unknowns
      in `context.md`.
- [x] Confirm the public outcome and non-goals. Evidence: product-owner
      approval in the main design session on 2026-08-21.

## Decide

- [x] Teach Context Access, remembered Workspace decisions, and this-session
      Host Loopback access to routine users. Evidence: product-owner approval
      on 2026-08-21.
- [x] Use Scope, Lifetime, Owner, and Precedence as the required technical
      dimensions. Evidence: product-owner approval on 2026-08-21.
- [x] Keep native readiness internal/detailed and describe its result as
      routine client traffic inside Context ceilings. Evidence: accepted
      design on 2026-08-21.
- [x] Keep Host Loopback on a separate attachment-scoped branch and exclude
      future `expose` design. Evidence: accepted design on 2026-08-21.
- [x] Keep ordinary Context ceilings and durable authority out of Host Loopback,
      and keep attachment authority out of ordinary external traffic. Evidence:
      product-owner approval after the implementation conflict audit on
      2026-08-21.
- [x] Treat trusted baseline Deny and remembered exact Deny as one terminal tier
      without inventing an order between them. Evidence: aggregate evaluator
      audit accepted by the product owner on 2026-08-21.
- [x] Decide whether evidence reveals a thesis/ADR correction before adding any
      missing enforcement test. Evidence: the packet was corrected to existing
      ADR 0049 and runtime behavior; no enforcement change is authorized.

## Implement

- [ ] Add failing tests for every genuinely missing precedence or lifetime
      canary; do not duplicate covered evaluator logic.
- [ ] Promote the three-layer explanation and complete technical authority
      inventory through product, architecture, security, and harness.
- [ ] Correct detailed catalog/output descriptions only where typed scope or
      lifetime framing is incomplete.
- [ ] Add claim-to-enforcement mapping for every table row and precedence edge.
- [ ] Preserve current command, schema, reference, mutation, and relay behavior.
- [ ] Do not implement progressive-disclosure screens or `tobari expose`.

## Verify

- [ ] Focused domain/policy/Gateway/attachment/catalog tests pass. Evidence:
- [ ] Terminal destination/method decisions prove zero downstream authority and
      external I/O. Evidence:
- [ ] The combined ordinary exact-deny tier wins over every applicable ordinary
      positive source. Evidence:
- [ ] Missing, stale, mismatched, or closed attachment authority reaches no
      host target. Evidence:
- [ ] Ordinary authority cannot decide Host Loopback, and attachment authority
      cannot decide ordinary external traffic. Evidence:
- [ ] Public output retains typed destination kind, lifetime, scope, and exact
      recovery without display inference. Evidence:
- [ ] Relevant agent-readiness scenario requires zero external reconstruction.
      Evidence:
- [ ] `task check` passes. Evidence:
- [ ] `task security` passes. Evidence:
- [ ] Generated diff and repository status are understood. Evidence:

## Hand off

- [ ] Acceptance criteria have evidence.
- [ ] Durable conclusions are promoted out of the packet.
- [ ] Temporary diagnostics are removed.
- [ ] The packet is removed in the same completion commit.
- [ ] The implementation is one concern-specific commit on main.
