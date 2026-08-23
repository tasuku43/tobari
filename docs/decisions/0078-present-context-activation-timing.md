# ADR 0078: Present Context activation timing

- Status: Accepted
- Date: 2026-08-22
- Deciders: Tobari maintainers
- Scope: Product, CLI, architecture, Context, documentation, and harness
- Revises: ADR 0072
- Related: ADR 0071
- Revised by: None
- Superseded by: ADR 0079

## Context

ADR 0071 defines Boundary, Runtime binding, and Workspace defaults as one
Context with distinct activation lifecycles. ADR 0072 made `context show`
result-first, but its Access, Tools, and Workspace-default groupings left those
lifecycles implicit. In particular, one Bootstrap row could be read as current
Workspace state even though the recipe applies only when a future Workspace
home is created.

## Decision

Default and detailed human Context reports group the same typed result as:

1. Boundary, fixed for the Context;
2. Runtime binding, adopted on next Workspace entry;
3. Workspace defaults, with later-entry/session and new-Workspace-home-only
   timing shown as subgroups rather than new resources; and
4. Login ownership, separate from Context defaults and credential-state
   inference.

Human text presents the create-only bootstrap projection as AWS and Kubernetes
EKS setup for new Workspace homes. Stable command paths and schema-1
machine fields retain `bootstrap`. JSON values and activation behavior do not
change.

## Consequences

- A Context read answers when each selected value takes effect.
- Workspace defaults remain one product concept despite their two activation
  timings.
- Human output grows slightly and no longer uses Access/Tools as its primary
  lifecycle headings.
- Login ownership remains visible without inspecting tool credential files.

## Mechanical enforcement

- One typed fixture and answer key drive summary and detailed goldens.
- Required-fact checks pin all lifecycle headings and setup components.
- Negative-inference canaries reject active-Runtime, current-Workspace
  bootstrap, login-success, and indentation-derived authority claims.
- Exact schema tests keep JSON identical and catalog descriptions retain each
  field's timing.

## Security and public-boundary impact

This is trusted-host presentation only. It adds no authority, I/O, credential
inspection, state, dependency, or external text.
