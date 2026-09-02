# ADR 0093: Select current Context without CWD

- Status: Accepted
- Date: 2026-09-02
- Deciders: Tobari product owner and maintainers
- Scope: Product, domain, CLI, architecture, security, state, assistance,
  research authentication, harness, and public boundary
- Revises: ADR 0084, ADR 0085, and ADR 0092
- Related: ADR 0090 and ADR 0091
- Superseded by: None

## Context

ADR 0092 removed Project root from Context, but routine Context-aware commands
still required an explicit reference and `context enter` still combined two
different outcomes: selecting semantic Context authority and entering a
CWD-owned Workspace. Bare entry and status also used the installation default
Template as a route from CWD to Context, which made the location-free model
hard to reason about.

The product owner clarified the ownership rule: CWD may select a Workspace;
it must never select the installation current Context. Users also need one
durable Context choice for routine commands, with an explicit command-local
override when deliberate.

## Decision drivers

- Context selection must remain useful across Projects and directories.
- Workspace remains the sole owner of canonical Project-root location.
- Routine Context-aware commands should not require repeated reference
  plumbing after an explicit selection.
- An override must not silently rewrite durable installation state.

## Considered options

### Infer Context from CWD

This preserves the ownership error: multiple Workspaces and nested roots can
change semantic authority merely because the process moved directories.

### Require an explicit Context reference on every command

This is deterministic, but leaves no useful meaning for a durable current
selection and adds repeated discovery to routine success.

### Persist one location-free current Context

This gives a stable default without assigning location to Context. Commands
that need another Context can consume one opaque override for that invocation.

## Decision

The coherent final-authority collection owns one optional `CurrentContextID`.
`context use --id CONTEXT_REF` validates and atomically selects an existing
Context; it reads no CWD and performs no Workspace or Docker operation.
`context enter` is removed without an alias.

Context-aware commands resolve an optional explicit `--context` first and
otherwise resolve `CurrentContextID`. The override is invocation-local and
does not mutate the selector. Missing current selection fails before task side
effects with `current_context_required` and recovery through `context list`
and `context use`.

Bare `tobari` remains the Workspace entry surface. It selects an existing
Workspace from canonical CWD and uses that Workspace's permanent Context
binding, regardless of the installation current Context. At a root without a
Workspace, it uses the current Context to create the CWD-owned Workspace.
Fresh first use selects the first Context it activates. `status` remains a
non-creating CWD-to-Workspace observation and never rewrites current Context.

## Consequences

### Positive

- `context use` has one precise, location-free outcome.
- Policy assistance and research authentication can use the stable current
  Context while retaining an exact opaque override.
- Changing current Context cannot retarget an existing Workspace.
- CWD participates only in Workspace selection, status, and entry.

### Negative

- Existing development authority without a current selection needs
  `context use` before a Context-aware omission or new-Workspace entry.
- A current Context that already owns its one Workspace cannot create another
  Workspace under the current one-Workspace-per-Context model.
- Catalog target validation must represent an optional current-Context
  fallback without treating omission as name or CWD rediscovery.

### Risks and mitigations

- A stale selector could point outside the collection. Collection validation
  rejects it, the generation digest binds it, and current Context deletion is
  blocked until another Context is selected.
- A handler could accidentally resolve Context from CWD. Application ports for
  current resolution have no root or Workspace input, and negative tests keep
  CWD out of the path.

## Mechanical enforcement

- `WorkspaceAuthorityCollection.CurrentContextID` validation, clone, digest,
  generation-manifest round trips, and lifecycle-locked mutation.
- Separate current-Context read and selection application ports.
- Catalog validation for optional typed current-Context fallback plus exact
  override reference kinds.
- Public path, reference-graph, recovery-graph, assistance, research-auth,
  first-entry, helper-source equality, and repository contract tests.

## Compatibility and migration

This is a pre-V1 command cutover. `context enter` becomes unknown and is not
retained as a compatibility alias. The optional selector field is readable as
absent in existing development generations. The first newly activated Context
is selected automatically; otherwise users select explicitly with
`context use`. JSON for Context selection uses the existing `result` schema 1
envelope.

## Security and public-boundary impact

The new asset is one secret-free Context ID in owner-only final authority.
Selection is a local write with no external destination, credential access,
process, dependency, or Docker effect. Research authentication keeps its
existing credential boundaries; only target resolution changes. No private
identifier, secret, or external content is added.

## Validation

- `task check`
- `task security`
- Focused collection/store/application/catalog tests for selection,
  omission, override precedence, deletion protection, and zero CWD input.
- Agent-readiness policy-assistance and research-auth scenarios with current
  selection and explicit override.

## Reconsideration signals

Reconsider if users need one Context to own multiple Workspaces, if a scoped
per-terminal selection is required, or if installation-wide selection creates
measurable cross-session surprise that cannot be resolved through explicit
status and override presentation.
