# Work Context: Separate the Tobari product from the Workspace resource

## Current behavior

- `docs/01_product_contract.md` defines Workspace as the human-facing name for
  one directory-bound Tobari and says it is not a separate resource.
- The same contract, theses, architecture, security model, CLI catalog, faults,
  and tests also contain “one Tobari,” “logical Tobari,” “enter a Tobari,” and
  “Create a Tobari” for Workspace lifecycle operations.
- The product already uses Workspace in onboarding, status, list, Runtime, and
  attachment descriptions, so this is a conflicting vocabulary rather than a
  missing concept.
- One Workspace is selected by `(canonical project root, stable Context ID)`;
  product state and Docker labels also carry internal IDs and project-bound
  principals.
- Machine-readable policy and status surfaces contain a mix of `root`, `id`,
  `project_id`, Context identity, and product-owned component fields. Their
  semantic meanings must be inspected before any rename.

## Relevant structure

- Entry point: root `tobari`, `status`, `list`, `delete`, Context readers, and
  policy review/recovery commands
- Domain rule: Workspace/project identity and principal types under
  `internal/domain/tobari`
- Application use case: project lifecycle and policy results under
  `internal/app/tobaricmd`
- Infrastructure boundary: root index, instance state, Docker labels, Gateway
  and OPA projection under `internal/infra/dockerruntime`
- CLI catalog or presentation: `internal/cli/runtime_catalog.go`, lifecycle and
  policy renderers, help, faults, README, and numbered docs
- Existing tests and harness checks: catalog/help fixtures, lifecycle and
  policy JSON contracts, architecture generation, contractlint, and repository
  public-boundary checks

## Constraints

- Stable command paths and machine identifiers are language-neutral public
  contracts and cannot be changed by an unreviewed prose replacement.
- Opaque identity and principal values must remain byte-preserving even if a
  public field label changes.
- Project remains a useful noun for the host source directory; Workspace is the
  Context-bound isolated resource using that directory.
- `Tobari-owned` remains correct when ownership belongs to the product rather
  than to one Workspace.
- Public documentation remains English under the repository locale contract;
  generated English/Japanese architecture-site parity must be preserved when
  affected.
- Existing user changes and persisted development state must not be silently
  migrated or reinterpreted.

## External facts

No external source is required. The product-owner decision is based on the
repository's current public contract and the accepted 2026-08-21 conceptual
design feedback.

## Unknowns

- [ ] Inventory every public `project_id`, generic `id`, principal label, and
      JSON schema field; classify it as project-root identity, Workspace
      identity, Context identity, product component identity, or internal-only.
- [ ] Determine whether any public field rename requires a pre-public schema
      version change or whether presentation can project a Workspace term while
      internal transport remains unchanged.
- [ ] Identify the smallest deterministic vocabulary lint that rejects
      duplicate resource language without rejecting legitimate product-name or
      ownership uses.
- [ ] Select the finite semantic fixtures whose before/after presentations
      cover lifecycle, status/list, Context, and policy recovery vocabulary.

## Thesis evidence

- Repeated design decision or point of agent confusion: Workspace is declared
  to be only a human alias, yet both names remain active throughout governing
  and executable public surfaces.
- User outcome or friction observed in the minimal slice: understanding the
  product requires distinguishing product Tobari, a logical Tobari, Workspace,
  project, root, and project principal even when several identify one resource.
- Code workaround or exception being considered: local copy edits would leave
  machine help, faults, and generated contracts inconsistent.
- Current thesis that resolves it, or proposed thesis revision: keep the
  existing canonical-root plus Context identity and make Workspace its sole
  public resource name.
- Downstream product, architecture, security, Skill, catalog, and harness
  impact: vocabulary/glossary, catalog summaries/outcomes/faults, structured
  field audit, presentation fixtures, generated docs, and vocabulary checks.

## Reproduction or observation

```sh
rg -n 'logical Tobari|one Tobari|Create a Tobari|enter.*Tobari|Workspace' \
  docs/00_theses.md docs/01_product_contract.md docs/02_architecture.md \
  docs/03_security_model.md internal/cli
```

Observed 2026-08-21: the governing documents explicitly define Workspace as a
human-facing alias while continuing to use both nouns for lifecycle and
identity claims.

## Security and public-boundary notes

- Assets and side effects involved: public prose, command metadata, structured
  result schemas, recovery guidance, and potentially internal principal labels
- Credentials or confidential data involved: none
- New dependencies, destinations, files, processes, or generated content: no
  runtime dependency or destination; deterministic fixture/golden files may be
  added
- External schema provenance, publication rights, and drift evidence: all
  affected schemas are Tobari-owned
- Output delivery, collection coverage, pagination, timeout, retry,
  idempotency, and cancellation facts: unchanged; the audit must prove the
  vocabulary edit does not change them
- Publication and licensing concerns: synthetic fixtures only; no external
  copied content

## Glossary

- **Tobari:** the product, executable, and ownership adjective.
- **Project:** the selected host source directory and its contents.
- **Workspace:** the reusable isolated resource selected by canonical project
  root plus stable Context ID.
- **Workspace identity:** the stable identity of that Context-bound resource,
  regardless of an existing internal transport field name.
- **Tobari-owned:** owned and controlled by the product rather than by a
  Workspace or untrusted process.
