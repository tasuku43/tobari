# Work Goal: Make the Context Boundary explicit

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

Users can understand a named Context as one stable host-owned work mode for
every Workspace permanently bound to it. Before Workspace creation, the
Context makes its immutable source/network Boundary explicit while separately
reporting the exact mutable Runtime binding, narrow session defaults, future-
Workspace creation defaults, and authentication ownership without collapsing
their physically separate trust boundaries.

## Why now

Context already composes runtime, policy mode, credentials, agent profile,
shell, and Git settings. ADR 0029 named its initial source/network authority,
and ADR 0071 later corrected the packet's whole-Context immutability language:
Runtime and narrow defaults have intentional mutable lifecycles, while the
creation-time Boundary, precedence, reporting, and Workspace binding remain
fixed.

## Non-goals

- Combining policy, credentials, runtime recipes, profiles, or secrets into one
  physical file, directory, mount, or process.
- Making Context names security authority; the stable Context ID remains the
  enforcement identity.
- Mutating an existing Context's source access, policy snapshot, method or
  destination ceilings, policy mode, or native-readiness participation choice.
- Live propagation from policy source to an existing Context Boundary.
- Context inheritance, templates of templates, organization policy, or remote
  distribution.
- Clone, overlay, apply-back, microVM, remote executor, configurable resources,
  or credential-mode selection in this packet.
- Implementing the child capabilities before this decision is accepted.

## Acceptance criteria

- [x] Durable ADRs define Context as a stable logical host-owned work mode whose
      creation-time Boundary is the immutable capability envelope, without
      weakening physical store separation. Evidence: ADR 0029 plus ADR 0071.
- [x] The work mode separates direct source/network Boundary, exact mutable
      Runtime binding, authentication ownership, agent profile, shell/Git
      session defaults, and future-Workspace bootstrap creation defaults.
      Evidence: ADR 0071 and current catalog contracts.
- [x] Source access and policy snapshot/revision are immutable Context Boundary
      facts;
      `context use` changes only the omitted-input default, and an existing
      Workspace remains bound to its stable Context ID. Evidence: manifest,
      catalog, and Workspace-key tests; supported-platform live bind evidence
      remains in the child packet.
- [x] The command contract fixes creation-time `--source-access`, `--mode`, and
      `--native-readiness` Boundary inputs with the reviewed defaults, while
      Runtime and bootstrap retain their separate initial values and later
      mutation paths. Evidence: current catalog and ADRs 0066/0067/0071.
- [x] `context list` and `context show` expose typed source access, policy
      revision/method facts, Runtime binding, and defaults without exposing
      source internals, secrets, or inferring whole-Workspace read-only or
      snapshot integrity. Evidence: schema-1 output contract tests.
- [x] The Context policy ceiling is enforced before every
      guided baseline/learned allow and every Advanced Rego allow.
- [ ] The two child packets provide mechanical enforcement for every new
      manifest/report/CLI claim and pass their required checks.
- [x] Durable lifecycle conclusions are propagated through theses, product,
      architecture, security, harness, CLI catalog, and readiness docs.
      Evidence: ADR 0071 implementation change; supported-platform child
      evidence remains separately unfinished.

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
