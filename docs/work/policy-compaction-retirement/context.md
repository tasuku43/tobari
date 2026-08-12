# Work Context: Retire learned policy compaction before V1

## Verified current facts

- `policy compactions` discovers proposed path-prefix replacements and
  `policy compact` applies one opaque `policy-compaction` reference.
- Compaction vocabulary spans domain rules and reports, application ports,
  CLI catalog/output, Context policy-data state, OPA matching, integration
  fixtures, documentation, and generated architecture data.
- `PolicyMatchPrefix` remains loadable policy authority even if public command
  routing alone is removed.
- GraphQL learned rules are currently ineligible for compaction, but ordinary
  exact HTTP rules can still widen to a prefix.
- There is no public release or retained denial-volume evidence justifying
  learned authority widening.

## Constraints

- Remove policy-prefix authority without confusing unrelated string prefixes,
  path validation, identifier prefixes, or UI labels.
- Keep candidate discovery and action separation. Exact candidate identifiers
  remain opaque and unchanged between discovery and allow/deny.
- Do not leave compatibility readers for unpublished development state; ADR
  0027 requires explicit state recreation.
- Advanced owner Rego remains executable policy, but the later immutable preset
  guardrail will be authoritative above it.

## Evidence to capture

- Catalog/reference/fault/recovery inventory before and after deletion.
- Policy-data and OPA negative fixtures rejecting prefix state.
- Dependency and generated-data diff proving no unowned compaction surface.
- Focused and repository gate output attached to `tasks.md`.
