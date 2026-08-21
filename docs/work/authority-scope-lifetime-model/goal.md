# Work Goal: Explain every authority by scope, lifetime, owner, and precedence

- Status: Accepted
- Retention: temporary
- Retention reason: None
- Governing contract: Project theses, product contract, architecture, security model, policy ADRs, and harness
- Review/delete trigger: Delete after the authority model is promoted, mechanically verified, and the change completes
- Successor: None
- Owner: Tobari maintainers
- Target: Before progressive-disclosure presentation and service-exposure design
- Related ADRs: ADR 0028, ADR 0029, ADR 0039, ADR 0049, ADR 0051, ADR 0058, ADR 0059, and ADR 0066

## Outcome

Routine users understand authority through three layers—Context Access,
remembered Workspace decisions, and this-session host access—without learning
baseline, overlay, snapshot, principal, epoch, or grant internals. Contributors
and operators have one authoritative inventory that states every source's
scope, lifetime, owner, and precedence and maps those claims to executable
Gateway, OPA, domain, and lifecycle checks.

## Why now

The implementation intentionally separates Context ceilings, trusted baseline
data, independently revised native readiness, Context/Workspace learned rules,
Advanced Rego, and attachment-scoped Host Loopback grants. Their reasons and
tests are distributed across several documents and ADRs. Upcoming onboarding,
status, Permission Inbox, and host-to-Workspace service exposure work need one
model that reduces user-facing concepts without merging distinct authority or
lifetime boundaries.

## Non-goals

- Changing OPA/Gateway evaluation order, authority scope, or policy identity.
- Making Context destination or method ceilings mutable.
- Turning native readiness into a selectable public profile or stored learned
  permission.
- Converting Attachment Grants into durable learned rules or generic temporary
  network authority.
- Changing Permission Inbox commands, Apply behavior, automatic retry, or
  host-only mutation ownership.
- Designing `tobari expose`; host-to-Workspace service exposure is the opposite
  direction and must receive a separate authority contract.
- Changing ordinary CLI presentation; progressive disclosure belongs to a
  later packet.

## Acceptance criteria

- [ ] Product documentation teaches only Context Access, remembered Workspace
      decisions, and this-session Host Loopback access in the routine path.
- [ ] Architecture and security contain one reviewed authority inventory with
      Scope, Lifetime, Owner, and Precedence for Context ceilings, trusted
      baseline denies/grants, native readiness, remembered Allow/exact Deny,
      Advanced Rego, Attachment Grants, and default deny.
- [ ] The ordinary external-HTTP evaluation order is explicit: principal and
      Context validity, terminal destination/method decision, one combined
      exact-deny tier containing trusted baseline Deny and remembered exact
      Deny, Context-policy positive authority, then learned Allow or Advanced
      Rego only when unresolved, followed by fail-closed/default review
      eligibility.
- [ ] Host Loopback is documented and tested as a separate closed branch bound
      to an active principal-owned route and Attachment Epoch. Attachment Deny,
      Attachment Allow, and exact attachment review are its complete policy
      order; ordinary Context destination/method ceilings, durable exact Deny,
      baseline/native authority, remembered Allow, and Advanced Rego do not
      enter or override that branch.
- [ ] Native readiness is explained as installed-binary compatibility authority
      admitted only inside the immutable Context ceilings, not as proof that a
      Context alone freezes the complete active aggregate revision.
- [ ] Existing tests are mapped to every precedence edge and lifetime end;
      missing terminal-deny, exact-Deny, stale/mismatched attachment, or
      default-deny canaries are added without creating a second evaluator.
- [ ] Public machine contracts retain exact scope, lifetime, owner/trust
      framing, and authority kind without inferring them from display labels.
- [ ] No command path, state schema, reference kind, external destination,
      credential route, or effect changes.
- [ ] `task check` and `task security` pass.

## Governing documents

- Thesis: North Star, Context policy learning, shared enforcement, Host
  Loopback, and Context consequences in `docs/00_theses.md`
- Product contract section: progressive policy learning, Context configuration,
  policy candidates/rules/review, and Host Loopback vocabulary
- Architecture or security invariant: one Gateway/OPA enforcement point,
  terminal ordinary-HTTP Context ceilings, combined exact-Deny precedence,
  project-principal binding, independently closed attachment-scoped route/grant
  identity, and fail-closed default
- Existing ADR: ADR 0049 attachment leases, ADR 0051/0058 native readiness,
  ADR 0059 complete method decisions, and ADR 0066 Context-owned policy

## Completion definition

The work is complete when the three-layer public explanation and complete
technical inventory are promoted, every precedence/lifetime claim has
executable evidence, no adjacent authority is broadened, required gates pass,
and this temporary packet is removed.
