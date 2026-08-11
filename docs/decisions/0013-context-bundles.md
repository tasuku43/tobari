# ADR 0013: Use a logical Context with physically separated stores

- Status: Accepted, revised by ADR 0018
- Date: 2026-08-02
- Revised: 2026-08-08
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, and harness
- Supersedes: None
- Superseded by: ADR 0018 supersedes the single-active-Context and deferred-routing parts
- Revised by: ADR 0027 removes retained pre-public migration and compatibility
  clauses and resets the current Context contract to exact V1

## Context

Users need one understandable execution setup without turning policy, agent
configuration, runtime, and secrets into one broad mount or interchangeable
authority. Tobari originally introduced Context with one active Context for the
cluster. ADR 0018 retains the composition model and replaces that temporary
selection model with permanent per-Tobari binding and shared multi-Context
enforcement.

## Decision

A named Context is stored below
`${XDG_CONFIG_HOME:-$HOME/.config}/tobari/contexts/<name>/`. Its schema-v3
manifest has a stable UUIDv7 ID, human name, agent profile, compatible runtime
image, runtime build record, and `guided` or `advanced` policy mode. Separate
owner-only children hold the runtime recipe, authoritative policy source,
managed credential metadata, and managed secrets. Tool-native authentication
remains in each Tobari's independent home.

Names are host selectors; stable IDs are persistent and enforcement identity.
Each Tobari records one Context ID. A compatibility-named `active.json` marker
means only the current/default Context for omitted invocations. `context use`
changes only that marker and never starts or reconciles Docker. Project runtime
reconciliation resolves the bound ID, not the current marker.

The installation uses one Gateway and one OPA for all Contexts. Context source
directories are not mounted wholesale. `cluster up` and policy mutations create
the purpose-limited aggregate projections and routing boundary specified by ADR
0018. Context creation never starts Docker; a configured cluster requires an
explicit `cluster up` before the new Context is loaded.

The default Context is created by compatibility migration. Legacy top-level
policy and credential source is copied once; existing homes and source files
are not deleted. Legacy project binding is accepted only from consistent,
validated single-Context evidence.

## Consequences

- Users compose runtime, agent, policy, and credential choices as one object
  without weakening their physical trust boundaries.
- Same-root Tobari may select different Contexts and still retain independent
  homes, runtime resources, network principals, and enforcement authority.
- A Context name change cannot silently change authority; no rename or Tobari
  move operation is introduced by this decision.
- Gateway sees only generated principal and credential projections. OPA sees
  only generated policy projection. Workspaces see neither secret stores nor
  policy source.

## Mechanical enforcement

- Domain tests validate stable IDs, names, modes, manifest completeness, and
  current/default reports.
- Infrastructure tests validate owner-only stores, atomic marker updates,
  legacy migration, permanent project binding, and separated projections.
- Catalog and CLI tests validate Context commands and Context-bearing human and
  JSON output.
- Multi-Context Docker integration proves same-root binding, one Gateway/OPA,
  Context-specific policy/runtime/credential behavior, and restart restoration.

## Validation

- `task check`
- `task security`
- `task public:check`
- `task integration:test`
