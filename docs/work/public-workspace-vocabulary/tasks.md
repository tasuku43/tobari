# Work Tasks: Separate the Tobari product from the Workspace resource

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [ ] Re-read the governing theses, product, architecture, security, harness,
      relevant ADRs, and add-capability Skill at implementation time.
- [ ] Inventory human, agent, structured, persisted, protocol, Docker-label,
      and source-code uses of Tobari, Workspace, Project, root, ID, and
      principal.
- [ ] Classify every public machine field by semantic identity and record the
      evidence in `context.md`.
- [ ] Freeze the finite typed fixtures and presentation-independent answer keys
      in `presentation-evidence.md`.
- [x] Confirm the public outcome and non-goals. Evidence: product-owner
      approval in the main design session on 2026-08-21.

## Decide

- [x] Use Tobari for the product/CLI/ownership and Workspace for the isolated
      resource. Evidence: product-owner approval on 2026-08-21.
- [x] Retain Project for the selected host source directory. Evidence:
      product-owner approval on 2026-08-21.
- [x] Reject a blind internal whole-repository rename. Evidence: the accepted
      plan requires a field-by-field public audit.
- [ ] Decide each public machine field disposition and any schema/version or
      development-state compatibility consequence.
- [ ] Decide whether a new/revised ADR is required after the machine-contract
      audit.
- [ ] Select the exact vocabulary-lint scope and allowed product-ownership
      phrases.

## Implement

- [ ] Add failing semantic presentation and vocabulary-regression tests.
- [ ] Promote the vocabulary through theses, product, architecture, security,
      harness, and any required ADR.
- [ ] Update catalog/help, renderers, faults, recovery guidance, README, and
      generated documentation.
- [ ] Apply only the public machine-field changes accepted by the audit.
- [ ] Change internal identifiers only when required to preserve a clear
      enforced invariant.
- [ ] Add deterministic vocabulary enforcement.
- [ ] Keep `../context-work-mode-contract/` separate from this implementation.

## Verify

- [ ] Focused tests pass. Evidence:
- [ ] Same-fixture before/after presentation remains semantically eligible.
      Evidence:
- [ ] Agent help and structured results retain exact next argv, canonical
      references, delivery, coverage, and zero external reconstruction.
      Evidence:
- [ ] `task check` passes. Evidence:
- [ ] `task security` passes when required by an accepted security/protocol
      change. Evidence:
- [ ] `task public:check` passes when required by a publishable schema change.
      Evidence:
- [ ] Generated diff and repository status are understood. Evidence:

## Hand off

- [ ] Acceptance criteria have evidence.
- [ ] Durable decisions are promoted out of the packet.
- [ ] Temporary fixtures or diagnostics not required by the harness are
      removed.
- [ ] The packet is removed in the same completion commit.
- [ ] The implementation is one concern-specific commit on main.
