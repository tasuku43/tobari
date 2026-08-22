# Work Tasks: Align trusted-host review commands

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read governing theses, product, architecture, security, harness, and
      agent-readiness sections.
- [x] Reproduce the current root selector, Permission Inbox, and catalog
      namespace behavior.
- [x] Record the command/namespace collision constraint and current behavior.
- [x] Record repeated review-layer confusion as thesis evidence.
- [x] Confirm the accepted public outcome and non-goals.

## Decide

- [x] Compare review-first, subject-first, and inbox-first approaches.
- [x] Accept a pure `review` namespace with `permissions` and `services`
      children.
- [x] Preserve existing leaf roles, effects, target binding, references,
      delivery, coverage, and authority semantics.
- [x] Classify the old root selector and `policy review` path for pre-public
      replacement without aliases.
- [x] Confirm no domain, adapter, credential, external-I/O, or trust-boundary
      change is intended.
- [ ] Audit whether ADR revision or a new ADR best records the stable hierarchy.
- [x] Obtain explicit user design approval.

## Implement

- [ ] Add failing catalog, namespace, dispatch, help, and retirement tests.
- [ ] Move the Permission Inbox catalog path and exact recovery strings to
      `review permissions`.
- [ ] Replace the unified selector with the direct `review services` leaf and
      remove selector-only code.
- [ ] Update TTY and redirected presentation fixtures without changing
      semantic result fields.
- [ ] Update cluster denial guidance and every catalog-owned next action.
- [ ] Update capability ledger and generated catalog/schema artifacts where
      exact command identity is recorded.
- [ ] Update theses, product, architecture, security, harness, readiness,
      README, architecture site, and affected ADRs.
- [ ] Prove old paths have no alias, hidden dispatch, dormant fallback, or
      generated-document occurrence claiming they remain public.

## Verify

- [ ] Focused tests pass. Evidence:
- [ ] Bare `tobari review` and both exact human/scoped-agent help forms match the
      accepted hierarchy. Evidence:
- [ ] Redirected leaf commands remain read-only. Evidence:
- [ ] Existing Permission Inbox TTY and Service review interaction corpora pass.
      Evidence:
- [ ] Retired-path negative tests pass. Evidence:
- [ ] Relevant agent-readiness replay meets the discovery budget and zero
      external-processing requirement. Evidence:
- [ ] `task check` passes. Evidence:
- [ ] `task public:check` passes. Evidence:
- [ ] `task security` passes if required by the final claim/boundary diff.
      Evidence:
- [ ] Generated diff, temporary artifacts, and repository status are understood.
      Evidence:

## Hand off

- [ ] Acceptance criteria have evidence.
- [ ] Durable decisions are promoted out of this packet.
- [ ] This temporary packet and generated caches are removed.
- [ ] One atomic implementation commit explains why the review hierarchy was
      aligned.
- [ ] Handoff reports behavior, retirement, gates, and residual risks.
