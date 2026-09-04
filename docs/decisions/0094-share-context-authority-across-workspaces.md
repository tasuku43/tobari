# ADR 0094: Share Context authority across sibling Workspaces

- Status: Accepted
- Date: 2026-09-04
- Deciders: Tobari product owner and maintainers
- Scope: Product, domain, CLI, architecture, security, state, policy learning,
  authentication, runtime projection, recovery, harness, and public boundary
- Revises: ADR 0044, ADR 0081, ADR 0084, ADR 0085, ADR 0092, and ADR 0093
- Related: ADR 0090 and ADR 0091
- Superseded by: None

## Context

Location-free Context selection made semantic authority independent from CWD,
but retained a one-Workspace-per-Context restriction. That restriction forced
users to duplicate Contexts merely to use the same Template, Policy Memory,
Home, and native authentication state at another Project root. It also made
Workspace and directory provenance accidental dimensions of learned-policy
identity.

The product owner clarified that one Context is reusable across any number of
Workspaces and that its Home and authentication state are shared by all of
them.

## Decision

One Context owns one immutable Template binding, one Policy Memory, and one
complete managed Context Home containing tool-native authentication state. A
Context may own arbitrarily many Workspaces. Each Workspace owns one stable
Workspace ID, one canonical ProjectRoot, its AppliedEntry, runtime resources,
network, and Gateway principal.

The uniqueness key is `(ContextID, canonical ProjectRoot)`. The same root may
also host Workspaces owned by different Contexts. At a root without an exact
Workspace, create-here uses the installation current Context even when it
already owns sibling Workspaces; it does not create a Context implicitly.

Every sibling mounts the exact same Context Home and inherits the same
create-once Home defaults. Creating a later sibling never re-runs bootstrap
against that Home. Same-Context Workspace processes are mutually trusted for
Home and native authentication contents; Tobari does not promise
application-level file locking between concurrently running client tools.
The Context record retains the exact Home path and creation-default digest even
while it owns zero Workspaces. Entry plans bind an explicit first-Home
initialization fact so crash replay may repeat the initial idempotent bootstrap,
while sibling creation, ordinary re-entry, and post-deletion recreation cannot.

Persistent learned policy identity and compatibility are Context-scoped. A
candidate is identified by ContextID plus normalized request effect and its
protocol coordinates. WorkspaceID, ProjectRoot, CWD, and directory ancestry
are observation provenance only. Gateway routing still uses one distinct
principal per Workspace, and one Context policy body is projected to every
active sibling principal.

Workspace deletion removes only the exact Workspace record, runtime, network,
and principal. It preserves sibling Workspaces, Context Home, authentication,
and Policy Memory. Context deletion remains blocked until every owned
Workspace, attachment, Configurator operation, and research credential is
absent, then retires Context-lifetime authority.

Entry recovery is bound to the exact Context, canonical ProjectRoot, and
WorkspaceID. A pending entry for one sibling cannot be resumed from another
root.

## Consequences

Context list/show expose complete Workspace membership rather than a singular
Workspace or AppliedEntry. Exact Workspace list/status retain Workspace-level
entry facts. Template change plans carry every affected Workspace and count
running Workspaces rather than Contexts.

The outer JSON schema is version 2 for `template plan` and `context list`,
`context show`, and `context apply`. Policy candidate/review envelopes remain
version 3 because their field shape and accepted opaque-reference contract are
unchanged: retained legacy and current IDs are semantic aliases and are
collapsed and consumed together.

Existing single-Workspace collections are a valid subset of the plural model.
New policy candidate and path-template identities use the Context-only form;
legacy candidate IDs remain valid only as retained upgrade/recovery evidence.
Legacy singular active policy projections are treated as stale reconstruction
evidence and are replaced through the normal locked aggregate reconciliation.

Sibling sessions may coexist. Operations that replace aggregate Gateway or OPA
state remain installation-exclusive and can report a retryable busy result
while any Workspace session is attached.

## Mechanical enforcement

- Collection validation for WorkspaceID and `(ContextID, ProjectRoot)`
  uniqueness, shared Context Home, shared create-once defaults, and retained
  zero-Workspace Context Home authority.
- Decision-bound first-Home initialization and no-rebootstrap sibling tests.
- Exact Workspace selection helpers and root-bound entry recovery.
- Plural Context policy principals and exact-principal retirement.
- Context-plus-effect candidate and reviewed-template identity tests with
  cross-Context negative canaries.
- Complete plural Context and Template-plan output contracts.
- Cold first-use and predecessor upgrade scenarios, including predecessor-side
  final-Workspace deletion followed by candidate recovery of the retained
  zero-Workspace Context Home authority.

## Security and public-boundary impact

Sharing a Context Home intentionally expands native-state visibility from one
Workspace to every Workspace in that Context, but never across Contexts. No
host credential is inherited. Workspace principal identity continues to bind
source and routing, while persistent policy matching cannot be narrowed or
widened by directory labels.

## Validation

- `task check`
- `task security`
- `task public:check`
- `task first-use:test` with an explicit isolated Docker context
- `task upgrade:test` with an explicit isolated Docker context

## Reconsideration signals

Reconsider if same-Context Home sharing causes measurable corruption that
requires Context-wide application locking, or if live principal updates must
complete while sibling interactive sessions remain attached.
