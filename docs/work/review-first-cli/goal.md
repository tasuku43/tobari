# Work Goal: Align trusted-host review commands

- Status: Accepted
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/00_theses.md`, `docs/01_product_contract.md`, and ADRs 0073 and 0074
- Review/delete trigger: Delete after durable command and interaction contracts are promoted and the change completes
- Successor: None
- Owner: Tobari maintainers
- Target: Pre-public CLI
- Related ADRs: 0073, 0074

## Outcome

A person uses one task-level `review` namespace for trusted-host decisions:
`tobari review permissions` opens the Permission Inbox and `tobari review
services` reviews Workspace service exposure requests. The lower-level
`policy` and `service` namespaces retain resource discovery and explicit
reference-bound actions without competing human review entry points.

## Why now

The first service-exposure slice added a root `tobari review` selector beside
the established `tobari policy review`. Both are trusted-host review tasks, but
their command hierarchy places one under a resource noun and the other at a
generic verb. The accepted interface exploration found that this makes the
same conceptual layer appear at two different depths and makes `review` both a
command and the desired parent namespace.

## Non-goals

- Do not change permission authority, staged Apply, watch, notification, or
  automatic-retry behavior.
- Do not change service request, Allow once, Deny, relay, exposure, stop, or
  attachment lifetime behavior.
- Do not add a same-terminal shortcut or allow Workspace code to perform
  trusted-host decisions.
- Do not rename the lower-level `policy candidates`, `policy rules`, `policy
  allow`, `policy deny`, `policy reset`, `service requests`, `service allow`,
  or `service deny` operations.
- Do not add compatibility aliases for the replaced pre-public command paths.
- Do not change machine result schemas except catalog-owned command identity
  and exact recovery command strings where those fields name the moved task.

## Acceptance criteria

- [ ] Bare `tobari review` is a catalog-derived namespace listing containing
      exactly the public child tasks `permissions` and `services`; it is not a
      separately registered command or a special selector UI.
- [ ] `tobari review permissions` preserves the complete former `policy
      review` input grammar and behavior, including read-only redirected/JSON
      output and TTY-only staged Apply, watch, and notification semantics.
- [ ] `tobari review services` directly preserves the former service branch of
      root `review`, including fresh exhaustive read-only redirected output and
      immediate attachment-local TTY Allow once or Deny.
- [ ] `tobari policy review` and the former registered root `review` path are
      absent with no alias, hidden dispatch, dormant fallback, or recovery
      reference.
- [ ] Catalog, human help, scoped agent help, dispatch, errors, recovery
      commands, denial guidance, README, product/architecture/security/harness
      contracts, site content, ADR wording, and readiness scenarios agree.
- [ ] Permission and service review remain visibly distinct in lifetime and
      application semantics even though navigation places them at one level.
- [ ] A known review task requires one scoped-help lookup and routine success
      requires zero undeclared external-processing steps.
- [ ] No domain, application, infrastructure, credential, external-I/O,
      mutation-authority, or trust-boundary behavior changes.
- [ ] Focused tests, `task check`, and `task public:check` pass. Run `task
      security` only if implementation changes a security claim or boundary
      beyond exact command navigation wording.

## Governing documents

- Thesis: task-first public CLI; host-only review; transparent Workspace
  terminal ownership
- Product contract section: public command table, permission review, and
  Workspace service exposure
- Architecture or security invariant: catalog as source of truth; separate
  trusted-host review; unchanged permission and attachment authority
- Existing ADR: 0073 and 0074

## Completion definition

The work is complete when the catalog-derived hierarchy, both leaf workflows,
negative retirement canaries, durable contracts, and readiness evidence agree;
required gates pass; temporary artifacts are removed; and this packet is
deleted from the final tree.
