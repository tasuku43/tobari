# Work Plan: Define Context as a stable reusable work mode

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Revise the top-level Context framing instead of treating supported mutations as
exceptions. Context remains one public work-mode concept and one stable
authority identity. Its Boundary section retains the immutable facts accepted
by ADR 0029; its exact Runtime binding and narrow Workspace defaults remain
explicit mutable components with their already-declared activation timing.

Promote the decision through a durable ADR that revises whole-envelope
immutability wording and relates ADR 0067. Preserve the existing physical store
and trust-boundary separation. Do not introduce a Boundary resource or a
per-Workspace Runtime selection.

## Alternatives considered

### Keep calling the whole Context immutable

Each mutable command could be described as an exception. Rejected because the
public definition would remain false and future UX would not know which values
may be copied or revised.

### Make Context security-only and move Runtime to Workspace

This would permit one Boundary with different per-project tool environments,
but adds a second routine selection and lets same-Context Workspaces diverge.
Rejected for V1. Reconsider only after repeated self-use evidence shows that
same-Boundary/different-Runtime projects are a routine need.

### Make the whole Context versioned and immutable

Every Runtime/default change could create a new Context identity. Rejected
because it would strand existing Workspace bindings, multiply work modes for
non-authority changes, and conflict with accepted Runtime/default mutation
contracts.

## Design

### Public contract

No command grammar or machine schema changes are planned. Context is described
as:

```text
Context
├─ Boundary (immutable after creation)
├─ Runtime binding (explicitly mutable; active on next entry)
└─ Workspace defaults
   ├─ session defaults (late-bound shell/Git behavior)
   └─ creation defaults (future-Workspace bootstrap only)
```

The exact field classification is derived from current typed contracts. Native
readiness is described precisely: the immutable Context selection admits the
installed binary's reviewed compatibility rules only inside the Context's
immutable ceilings; Context alone does not claim to freeze the installed
compatibility revision.

### Layer changes

- Domain: no data-model change unless a missing invariant prevents mechanical
  classification of Boundary versus mutable binding/default fields
- Application: preserve existing task-specific Context mutations and timing
- Infrastructure: no new adapter; retain exact persistence, reconciliation,
  late binding, and future-only bootstrap behavior
- CLI and catalog: correct summaries, outcomes, field descriptions, and help
  that currently imply whole-Context immutability

### Data and control flow

```text
stable Context ID
  ├─ immutable Boundary → source mount + policy ceilings
  ├─ exact Runtime binding → next-entry reconciliation, home preserved
  ├─ session defaults → next child-session resolution, no home rewrite
  └─ creation defaults → future Workspace creation snapshot only
```

### Error and cancellation behavior

Unchanged. Boundary mutation remains unavailable. Existing Runtime/config
mutations validate intent, fixed target, impact, and complete input before
their controlled side-effect boundary. Cancellation and confirmed mutation
output retain current contracts.

### Security and public boundary

The decision narrows an inaccurate immutability claim; it does not weaken any
authority. Source/network Boundary facts remain immutable and terminal.
Installed-client compatibility remains independently revisioned and bounded.
No credential, host configuration, arbitrary image, or per-Workspace authority
is introduced.

## Implementation slices

1. Inventory durable/public whole-Context immutability claims and exact field
   activation timing.
2. Add the revising ADR and update Thesis 9 before local wording changes.
3. Propagate product, architecture, security, harness, catalog, and readiness
   language.
4. Add missing mechanical Boundary-mutation and activation-timing checks.
5. Reconcile the older active Context packet and verify no conflicting plan
   remains.

## Verification

- Unit and contract tests: immutable Boundary fields, allowed Context mutation
  paths, Runtime next-entry adoption, shell/Git resolution, bootstrap
  future-only behavior
- Negative side-effect tests: attempted Boundary mutation remains impossible or
  fails before I/O; mutable setting failures do not rewrite Workspace home
- Opaque-reference and complete-pagination tests: not applicable; no new
  reference or collection contract
- Structured output, hostile-output, and recovery tests: existing schemas and
  exact recovery remain unchanged
- Agent-readiness scenario and discovery-round-trip count: scoped Context help
  explains creation-time versus mutable settings without source inspection
- Human-handoff scorecard: not applicable; no new setup/auth transfer
- Manual observation: inspect one Context before and after Runtime/default
  mutations and verify Workspace binding/home identity
- Required profiles: `task check`; `task security` if security claim
  enforcement changes
- Generated-diff or artifact checks: ADR/architecture-site/catalog generation
  as applicable

## Rollout and rollback

This is a pre-public semantic correction with no planned persisted-state or
command migration. Rollback is a source revert. If implementation discovers a
required schema change, stop and revise this plan before acting.

## Documentation promotion

- Revise Thesis 9 and whole-envelope wording.
- Add a durable ADR revising ADR 0029 and relating ADR 0067.
- Propagate exact activation timing through product, architecture, security,
  harness, catalog, and readiness docs.
- Reconcile or supersede conflicting conclusions in the active
  `context-capability-envelope` packet without losing unfinished child evidence.
