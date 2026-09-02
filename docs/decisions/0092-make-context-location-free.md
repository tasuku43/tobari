# ADR 0092: Make Context location-free

- Status: Accepted
- Date: 2026-08-30
- Deciders: Tobari product owner and maintainers
- Scope: Product, domain, CLI, architecture, security, state, Configurator,
  migration, harness, and public boundary
- Revises: ADR 0084, ADR 0085, ADR 0088, and ADR 0090
- Related: ADR 0086 and ADR 0091
- Revised by: ADR 0093 at current Context selection and CWD-only Workspace
  routing
- Superseded by: None

## Context

Context was modelled as a durable `(ProjectRoot, TemplateID)` binding. That
made reusable policy and authentication authority depend on the directory from
which a command happened to run. It also caused `runtime assist` to invoke the
Workspace selector and fail with `context_required`, even though editing a
managed Runtime has a complete target and mount plan independent of CWD.

The behavior exposed an ownership error: Context was carrying Workspace
location. A Context is semantic authority; a Workspace is the replaceable
execution instance that mounts one Project root.

## Decision

A Context is a location-free immutable `(ContextID, TemplateID)` binding and
owns Policy Memory. It contains no ProjectRoot, CWD, Workspace, or invocation
location. Multiple Contexts may bind the same Template; ContextID uniqueness is
the only Context uniqueness rule.

A Workspace alone binds one canonical ProjectRoot to one Context and owns the
applied entry receipt. CWD is observed only by commands whose task is Workspace
selection, status, or entry. ADR 0093 separates `context use --id`, which
changes only a location-free installation selector, from bare `tobari`, which
selects or creates the CWD-owned Workspace.

Task-scoped assistance follows target ownership:

- `runtime assist --id RUNTIME_REF` is installation-scoped. It never observes
  CWD, Workspace, or Context, executes with the installation-owned standard
  Runtime, and mounts only the target Runtime source plus an installation-owned
  per-agent Home.
- `policy assist [--context CONTEXT_REF]` uses the installation current Context
  or one exact invocation-local override, then uses that Context's exact
  Template Runtime and Policy Memory evidence. It performs no CWD or Workspace
  selection.

Context source schema is `tobari.dev/context/v2` and contains only Context ID
and Template ID. Pre-release v1 Context authority is rejected rather than
silently reinterpreted.

## Consequences

- Context create, list, show, plan, and Apply expose no Project root.
- Workspace discovery is the only source for nearest-root selection and status.
- Runtime assistance works from any directory and has no `context_required`
  fault or Workspace selector.
- Policy assistance is deterministic from one opaque Context reference and can
  be invoked from any directory.
- Runtime and policy Configurator leases are keyed by Runtime and Context
  identity respectively; aggregate pre-release Project Configurator state
  remains isolated and non-public.
- Domain validation, source parsing, catalog reference flow, negative CWD
  canaries, helper-source equality, and agent-readiness scenarios enforce the
  separation.

## Rejected alternatives

- Inferring a nearest Context from CWD keeps the ownership error and makes
  unrelated Runtime editing depend on local Workspace history.
- Keeping ProjectRoot as optional Context metadata creates two competing
  location authorities and permits later code to restore ambient selection.
- Reusing a Workspace Home for Runtime assistance couples installation source
  maintenance to a disposable execution instance and is therefore rejected.
