# Work Goal: Make Context the explicit capability envelope

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: Project theses, product contract, architecture, and security model
- Review/delete trigger: Delete after the Context decision is promoted and both implementation packets complete
- Successor: None
- Owner: Tobari maintainers
- Target: Before first public V1 release
- Related ADRs: ADR 0010, ADR 0013, ADR 0018, ADR 0026, ADR 0027, and ADR 0028
- Parent: [First public release core](../first-public-release-core/goal.md)
- Children: [Context source access](../context-source-access/goal.md),
  [policy compaction retirement](../policy-compaction-retirement/goal.md), and
  [V1 authentication narrowing](../v1-auth-narrowing/goal.md)

## Outcome

Users can understand a named Context as the host-owned capability envelope for
every Workspace permanently bound to it. Before Workspace creation, the
Context makes source authority, network guardrail origin/revision, runtime,
credential exposure, and narrow host projections explicit without collapsing
their physically separate trust boundaries.

## Why now

Context already composes runtime, policy mode, credentials, agent profile,
shell, and Git settings, but its public model does not name source access or
the origin of its initial network authority. Adding those axes independently
would turn Context into a bag of flags unless their immutability, precedence,
reporting, and Workspace binding are decided first.

## Non-goals

- Combining policy, credentials, runtime recipes, profiles, or secrets into one
  physical file, directory, mount, or process.
- Making Context names security authority; the stable Context ID remains the
  enforcement identity.
- Mutating an existing Context's source access or preset snapshot.
- Live propagation from a custom preset to existing Contexts.
- Context inheritance, templates of templates, organization policy, or remote
  distribution.
- Clone, overlay, apply-back, microVM, remote executor, configurable resources,
  or credential-mode selection in this packet.
- Implementing the child capabilities before this decision is accepted.

## Acceptance criteria

- [ ] A durable ADR defines Context as a logical host-owned capability envelope
      and revises the narrower wording in ADR 0013 without weakening physical
      store separation.
- [ ] The envelope has explicit dimensions for direct source access, snapshotted
      network policy, runtime, credential exposure/reporting, agent profile,
      shell projection, and Git projection.
- [ ] Source access and preset revision are immutable Context creation facts;
      `context use` changes only the omitted-input default, and an existing
      Workspace remains bound to its stable Context ID.
- [ ] The command contract fixes `context create --source-access
      read-only|read-write --policy-preset PRESET`, with `read-write`,
      `builtin/reviewed-exact`, and `guided` as omission defaults.
- [ ] `context list` and `context show` expose typed source access and policy
      origin/revision facts without exposing source internals, secrets, or
      inferring whole-Workspace read-only or snapshot integrity.
- [ ] The preset guardrail is an authority ceiling enforced before every
      guided baseline/learned allow and every Advanced Rego allow.
- [ ] The two child packets provide mechanical enforcement for every new
      manifest/report/CLI claim and pass their required checks.
- [ ] Durable conclusions are propagated through theses, product,
      architecture, security, harness, CLI catalog, and readiness docs.

## Governing documents

- Thesis: Context composition and one shared multi-Context cluster in
  `docs/00_theses.md`
- Product contract section: Context composition, lifecycle identity, project
  root authority, and policy learning in `docs/01_product_contract.md`
- Architecture or security invariant: stable host-issued Context/project
  principal, separate stores/projections, one controlled side-effect boundary,
  and Context/project-bound authority
- Existing ADR: ADR 0013 logical Context composition, ADR 0010 direct root,
  ADR 0026 transparent effect enforcement, ADR 0028 domain policy source

## Completion definition

This decision packet completes only after the ADR and durable documents agree,
both child packets provide executable evidence, the parent release packet
adopts the resulting contracts, required gates pass, and this temporary packet
is removed.
