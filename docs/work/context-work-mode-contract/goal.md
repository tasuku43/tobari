# Work Goal: Define Context as a stable reusable work mode

- Status: Accepted
- Retention: temporary
- Retention reason: None
- Governing contract: Project theses, product contract, architecture, security model, Context ADRs, and harness
- Review/delete trigger: Delete after the revised Context decision is promoted, enforced, and the change completes
- Successor: None
- Owner: Tobari maintainers
- Target: Before first-run, Context derivation, and guided Runtime UX changes
- Related ADRs: ADR 0013, ADR 0029, ADR 0062, ADR 0066, ADR 0067, and ADR 0070
- Related active work: `../context-capability-envelope/` and `../context-source-access/`

## Outcome

Users and contributors understand Context as a stable, reusable work mode whose
creation-time Boundary is immutable while its exact Runtime binding and narrow
Workspace defaults change only through explicit reviewed mutations with
declared activation timing. Existing Workspaces remain bound to the stable
Context ID, and mutable Context settings never silently retarget a Workspace or
rewrite its persistent home.

## Why now

Current architecture calls the entire Context an immutable capability envelope,
but supported Context mutations change Runtime, shell presentation, Git
identity fallback, and future-Workspace bootstrap. The implementation already
distinguishes their lifecycles; the public concept does not. Upcoming one-screen
onboarding, `context create --from`, and guided Runtime work need an exact
definition before they present or copy Context settings.

## Non-goals

- Making source access or the Context-owned network Boundary mutable.
- Moving Runtime binding to each Workspace or adding Workspace Runtime
  overrides.
- Adding a separately identified Boundary resource, command family, or store.
- Changing the Workspace key or allowing Context mutation to retarget an
  existing Workspace.
- Rewriting or synchronizing existing Workspace homes after Context changes.
- Implementing first-run summary, Context derivation, Runtime customization, or
  progressive-disclosure presentation in this packet.
- Reopening standard Workspace-owned authentication or the experimental Broker
  boundary.

## Acceptance criteria

- [ ] Theses define Context as a stable host-owned work mode and name its
      immutable Boundary, mutable Runtime binding, and mutable Workspace
      defaults without introducing three new public resources.
- [ ] ADR 0029's whole-envelope immutability wording is revised while preserving
      immutable source access, policy snapshot/ceilings, stable Context ID, and
      separate physical trust boundaries.
- [ ] The durable decision keeps one exact Runtime binding on Context, records
      why no Workspace override exists, and names repeated self-use demand for
      same-Boundary/different-Runtime projects as the reconsideration trigger.
- [ ] Runtime, shell, Git fallback, and bootstrap contracts state their distinct
      activation timing and prove that existing Workspace homes are not
      silently rewritten.
- [ ] Public catalog/help and technical documentation no longer claim that the
      entire Context is immutable; machine fields accurately distinguish
      immutable selections from mutable bindings/defaults.
- [ ] Existing Context mutation commands remain the only controlled paths for
      Runtime and defaults; no mutation can alter creation-time Boundary facts.
- [ ] The active `context-capability-envelope` work packet and related ADRs are
      reconciled so the repository retains no conflicting active plan or
      durable definition.
- [ ] No state schema, command path, effect, target binding, authentication
      ownership, or authority precedence changes.
- [ ] `task check` passes; `task security` runs if the implementation changes a
      security claim or enforcement test beyond terminology/contract accuracy.

## Governing documents

- Thesis: Thesis 9 and Context/Runtime/Workspace lifecycle consequences in
  `docs/00_theses.md`
- Product contract section: Context composition, configuration contract,
  Runtime selection, shell/Git projections, bootstrap, and root entry in
  `docs/01_product_contract.md`
- Architecture or security invariant: stable Context ID and Workspace binding,
  immutable source/network authority, separate stores, controlled mutations,
  and explicit activation timing
- Existing ADR: ADR 0029 immutable capability envelope, ADR 0062 typed
  bootstrap once, ADR 0067 shared versioned Runtime binding

## Completion definition

The work is complete when the revised definition is promoted through theses,
ADRs, product, architecture, security, harness, catalog, and executable tests;
related active packets no longer conflict; required gates pass; and this
temporary packet is removed.
