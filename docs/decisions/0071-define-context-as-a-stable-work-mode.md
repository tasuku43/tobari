# ADR 0071: Define Context as a stable reusable work mode

- Status: Accepted
- Date: 2026-08-21
- Deciders: Tobari maintainers
- Scope: Product, CLI, architecture, security, Context, Runtime, harness, and
  public boundary
- Revises: ADR 0013 and ADR 0029
- Related: ADR 0062, ADR 0066, ADR 0067, and ADR 0070
- Revised by: None
- Superseded by: None

## Context

ADR 0029 described the whole Context as an immutable capability envelope. That
framing correctly made direct source access and the Context-owned network
policy creation-time authority, but it no longer describes the accepted
Context lifecycle. ADR 0067 added an explicit mutation that replaces one
Context's exact Runtime binding. `config shell` and `config git` replace narrow
Context-owned defaults that are resolved for later child sessions, and typed
bootstrap commands replace a recipe used only when a future Workspace is
created.

Treating each operation as an exception would leave users unable to tell which
changes require a new Context and which deliberately preserve its identity and
bound Workspaces. Moving Runtime selection to each Workspace would instead add
a routine selector and permit Workspaces in one Context to diverge.

## Decision

Context is one stable, host-owned, reusable work mode. Its stable ID remains
the enforcement identity and the permanent half of each `(canonical project
root, Context ID)` Workspace key. Its name remains only a human selector.

A Context contains the following lifecycle classes without making them new
public resources:

1. **Boundary:** creation-time source and network authority. Direct
   `source_access`, policy mode, the normalized Context-owned policy snapshot
   and revision, complete destination and method ceilings, and the
   enabled/disabled native-readiness participation choice cannot be mutated on
   an existing Context. Enabled readiness admits only the installed trusted
   binary's current reviewed compatibility overlay inside those ceilings; it
   does not freeze that independently revisioned overlay.
2. **Runtime binding:** one exact ready Runtime ID and semantic revision. Only
   the explicit `context runtime set` mutation may replace it. Bound Workspaces
   adopt the new image on their next entry through ordinary reconciliation,
   while their Context binding, logical identity, and persistent home remain.
3. **Workspace defaults:** narrow Context-owned inputs with explicit activation
   timing. Shell presentation and Git identity fallback are session defaults
   resolved on later Workspace entry or child-session creation without
   rewriting the Workspace home. Typed bootstrap is a creation default applied
   once only to future Workspace homes; existing homes retain their create-time
   bytes and revision.

The fixed read-only agent-profile reference remains a trusted structural input,
not a user-selectable mutation. Standard authentication remains created and
owned by each Workspace home. Experimental broker state remains physically
separate and keyed by stable Context ID. Neither authentication model becomes a
mutable Boundary field.

Context creation owns all Boundary defaults and the initial Runtime/default
values. After creation, only catalog-owned Runtime, shell, Git, and bootstrap
mutations may change their respective Context components. `context use` changes
only the omitted-input default. No Context mutation retargets an existing
Workspace, changes its stable Context ID, widens its Boundary, or silently
rewrites its persistent home.

## Consequences

### Positive

- Users can reuse one named work mode while deliberately updating its tools or
  narrow defaults.
- The security statement remains exact: changing source or network authority
  still requires a new Context and therefore a distinct stable identity.
- Runtime rollout and default activation timing are visible without adding a
  Context version, Workspace Runtime override, or second source of truth.

### Negative

- A Context report contains facts with different lifetimes, so help and human
  presentation must name their activation timing instead of implying one
  uniform immutable object.
- Projects that need the same Boundary but different Runtimes still require
  different Contexts in V1.

### Reconsideration signal

Reconsider a per-Workspace Runtime binding only after repeated self-use evidence
shows that same-Boundary, different-Runtime projects are a routine need. A
single preference or onboarding simplification is insufficient because the
change would add per-Workspace selection and divergence.

## Mechanical enforcement

- Exact-V1 manifest and policy tests require stable IDs, direct source access,
  policy snapshot/revision, complete method policy, and native-readiness choice;
  current readers expose no mutation for those Boundary facts.
- Catalog tests reject Boundary inputs on every existing-Context write and pin
  the only mutable component targets to Context Runtime binding, shell session
  defaults, Git session defaults, and future-Workspace bootstrap.
- Runtime reconciliation tests prove explicit upgrade/rollback takes effect on
  next entry while Workspace identity and home remain.
- Shell and Git tests prove entry-time resolution through narrow environment
  and read-only Git projections rather than Workspace-home mutation.
- Bootstrap tests prove refresh/removal affects only future Workspace creation
  and preserves existing home bytes.

## Compatibility and migration

No persisted schema, command path, effect, target binding, output field, or
compatibility reader changes. ADR 0070 remains the only enumerated predecessor
migration. Existing Context IDs, Workspace bindings, homes, policy, Runtime
bindings, defaults, and authentication state retain their current meaning.

## Security and public-boundary impact

This decision narrows an inaccurate immutability claim; it does not widen
authority. Source and network Boundary facts remain immutable and terminal.
The installed compatibility overlay cannot exceed them. No destination,
credential route, process, dependency, arbitrary configuration input, or
per-Workspace authority is added.

## Validation

```sh
task check
task security
task public:check
```
