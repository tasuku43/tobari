# Work Goal: Separate the Tobari product from the Workspace resource

- Status: Accepted
- Retention: temporary
- Retention reason: None
- Governing contract: Project theses, product contract, architecture, security model, and CLI catalog
- Review/delete trigger: Delete after the vocabulary is promoted, mechanically enforced, and the change completes
- Successor: None
- Owner: Tobari maintainers
- Target: Before the remaining pre-public UX capabilities are implemented
- Related ADRs: ADR 0013, ADR 0018, and ADR 0029

## Outcome

Users encounter **Tobari** as the product, CLI, and ownership adjective, and
**Workspace** as the isolated resource they create, enter, reuse, inspect, and
delete. Project refers to the selected host source directory, while public
machine identifiers distinguish project-root facts from Workspace identity.
Routine use no longer requires interpreting “a Tobari” and “Workspace” as two
names for the same resource.

## Why now

The product contract calls Workspace the human-facing name for a
directory-bound Tobari, while theses, faults, help, and technical documents
still use “logical Tobari,” “create a Tobari,” Tobari IDs, and Workspace
interchangeably. The accepted usability feedback identifies this duplicate
noun as avoidable conceptual load. Upcoming onboarding, status, Context, and
service-exposure work would otherwise reproduce the ambiguity.

## Non-goals

- Renaming the `tobari` executable or stable command paths.
- Changing Workspace selection, identity, lifecycle, isolation, or authority.
- Replacing the public Context or Runtime concepts.
- Blindly renaming every Go symbol, persisted field, Docker label, Gateway/OPA
  field, or audit key that contains `project` or `tobari`.
- Redefining Context mutability; that belongs to
  `../context-work-mode-contract/`.
- Changing `Tobari-owned` when it accurately names product ownership.

## Acceptance criteria

- [ ] Theses and product vocabulary define Tobari as the product and Workspace
      as the isolated resource selected by canonical project root plus Context
      ID.
- [ ] Human-facing commands, help, faults, recovery guidance, README, and
      ordinary documentation use Workspace for resource lifecycle actions and
      retain Project only for the host source directory.
- [ ] Public agent-help and structured-output fields are audited individually:
      project-root facts, Workspace identities, and product ownership use
      semantically correct names, with any retained compatibility name
      explicitly justified.
- [ ] Internal protocol, persistence, and source identifiers change only when
      their current name would misstate an enforced invariant; no cosmetic
      whole-repository rename is performed.
- [ ] A deterministic vocabulary check prevents ordinary public presentation
      from reintroducing “logical Tobari,” “create/delete/enter a Tobari,” or
      equivalent duplicate resource language while allowing accurate
      product-ownership phrases.
- [ ] Frozen typed fixtures and answer keys prove that terminology changes do
      not alter task identity, exact next argv, canonical references, result
      completeness, or recovery.
- [ ] No command behavior, effect, target binding, security boundary, persisted
      ownership, or exit status changes.
- [ ] `task check` passes; run additional profiles only if the implementation
      changes a machine/security/publication boundary identified by the audit.

## Governing documents

- Thesis: North Star and the shared-services/Workspace lifecycle theses in
  `docs/00_theses.md`
- Product contract section: Vocabulary, public commands, lifecycle, status,
  and policy evidence in `docs/01_product_contract.md`
- Architecture or security invariant: canonical root plus stable Context ID
  selects one logical Workspace; stable principals and stores remain exact
- Existing ADR: ADR 0013 logical Context composition, ADR 0018 shared
  enforcement routing, and ADR 0029 Context boundary composition

## Completion definition

The work is complete when the accepted vocabulary is promoted through every
public surface, public machine identifiers have an evidence-backed disposition,
the semantic presentation fixture remains valid, vocabulary enforcement and
required gates pass, and this temporary packet is removed.
