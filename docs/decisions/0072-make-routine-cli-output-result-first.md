# ADR 0072: Make routine CLI output result-first

- Status: Accepted
- Date: 2026-08-21
- Deciders: Tobari maintainers
- Scope: Product, CLI, architecture, Context, Workspace status, documentation,
  and harness
- Revises: The routine Context list/show presentation in ADR 0060
- Related: ADR 0051, ADR 0058, ADR 0062, ADR 0066, ADR 0067, ADR 0068, and ADR 0071
- Revised by: None
- Superseded by: None

## Context

Routine Context and Workspace reads were accurate but led with stable IDs,
paths, images, revisions, agent profile, and stored policy sources. Users had to
translate those implementation facts into the questions they were actually
asking: which work mode is current, what Access is effective, which Runtime is
selected, whether action is needed, and what to do next.

ADR 0060 moved `context list` from a tab-separated row to vertical cards and
added a concise `context show`, but its filesystem-first list and technical
summary still made diagnostic sources compete with outcomes. The established
`--details`, complete JSON, and specialist commands already provide disclosure
layers without a global beginner/expert preference.

## Decision

Default human output is result-first and task-specific:

- `context list` shows each work mode's name/current marker, typed effective
  Access, exact Runtime selection, and an action marker only when needed.
- `context show` groups typed effective Access, Tools, Workspace defaults,
  Workspace-owned login, exact Details, and exact Next. `--details` retains the
  complete sectioned diagnostic, and schema-1 JSON remains unchanged.
- A synthetic default says recommended defaults are not saved while structured
  state retains the `synthetic_default` and null/absent distinctions.
- `status` leads with Workspace result, root, Context, exact Runtime plus health,
  session, any required action, and exact Next. Healthy IDs, home paths,
  revisions, and unchanged bootstrap state remain in complete JSON.
- Root human help calls the backing control plane Shared services; exact
  `cluster` commands remain the specialist interface.

Domain owns finite routine summaries for Access, readiness after actual method
and destination ceilings, Runtime/action state, Workspace defaults, and
lifecycle attention. Renderers do not derive those meanings from labels,
ordering, or field proximity. Internal presentation metadata may supplement a
typed result only with facts resolved by the same application read and must not
change established JSON.

No `status --details`, global display mode, pending-permission aggregation,
exposure aggregation, onboarding change, authority change, or schema change is
added by this decision.

## Consequences

- Routine output answers one user task without discarding complete diagnostics.
- Context Runtime and routine-client readiness are explicit typed facts rather
  than presentation inference.
- Some human text snapshots change, while structured contracts and exact next
  argv remain stable.
- Workspace status has no separate human details command; JSON is its complete
  diagnostic surface until a separately accepted typed contract justifies one.

## Mechanical enforcement

- Same-fixture before/after Context list and Context show goldens preserve task,
  scope, state, exact Runtime, exact Next, and zero reconstruction.
- Negative-inference tests reject readiness-from-position, action-from-label,
  and invented Workspace/login claims.
- Schema tests prove human-only exact Runtime selection does not enter Context
  list or Workspace status JSON.
- Lifecycle fixtures require exact Runtime plus health and reject healthy
  Workspace ID/home exposure in routine text.
- Catalog/help, README/site, product, architecture, harness, and agent-readiness
  wording remain checked by the repository gates.

## Security and public-boundary impact

This changes trusted-host presentation only. It adds no authority, input,
external I/O, mutation, credential path, service, route, or schema. Existing
terminal projection and untrusted-text rules continue to apply.
