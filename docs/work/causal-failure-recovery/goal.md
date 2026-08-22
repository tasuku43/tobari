# Work Goal: Causal failure diagnosis and safe recovery

- Status: Accepted
- Retention: temporary
- Retention reason: None
- Governing contract: Theses 0 and 5; public structured-error, lifecycle, Doctor, and exit contracts
- Review/delete trigger: Delete after the failure model, presentation, and recovery contracts are promoted and the capability passes its required gates
- Successor: None
- Owner: Tobari maintainer
- Target: Pre-publication self-use quality
- Related ADRs: A new failure-state ADR is required before implementation; ADRs 0012, 0068, and 0069 provide adjacent evidence

## Outcome

When a normal Tobari command fails, the user and an agent can tell from one
bounded, secret-free result what causal class Tobari established, which command
phase failed, whether that command made no change, a known partial change, a
confirmed change, or an unknown change, and which exact safe Tobari command to
run next. This applies coherently to first use, shared-service lifecycle,
Workspace resolution/creation/reconciliation/attachment, standalone Context
creation, direct child execution, and representative read and mutation
commands.

Tobari remains agnostic to the host's Docker provider. It observes only the
generic Docker CLI, selected Docker context, Engine, and Compose contracts. It
does not identify, start, stop, configure, repair, or recommend commands for
Colima, Lima, Docker Desktop, Rancher Desktop, or another backend.

## Why now

The current fault envelope reliably strips private adapter causes and supplies
catalog-owned exact recovery, but it does not carry command phase or mutation
state. Several composed lifecycle paths therefore rely on prose such as
"could not be created", "did not complete", or "may need reconciliation" even
when the implementation knows more, and sometimes when it cannot safely know
that claim. First use can also persist a Context before discovering that the
generic Docker prerequisites for the promised Workspace outcome are
unavailable. The public installation guide claims Docker Engine 24 or newer,
while current readiness checks only print the server version and do not enforce
that minimum.

## Non-goals

- Detect, name, manage, start, stop, install, upgrade, or repair a Docker
  provider or VM backend.
- Parse provider-specific daemon errors, Docker context names, socket paths, or
  desktop application state to guess a remedy.
- Expose raw adapter errors, Docker stderr, paths, URLs, environment, or process
  output in the structured fault contract.
- Turn every command into an eager full-Doctor run or duplicate Doctor's
  complete dependency graph in routine execution.
- Automatically retry, reconcile, roll back, delete, or repair after a failure.
- Treat a direct child's nonzero exit as a Tobari infrastructure fault.
- Add a public setup/backend-management command or broaden Docker authority.
- Promise that Tobari can prove a partial or zero mutation when an adapter
  supplies no such evidence.

## Acceptance criteria

- [ ] The structured error contract exposes a closed command phase and closed
      change state in human and JSON output; `kind` plus command-specific
      `code` remain the causal identity, and no second competing cause registry
      is introduced.
- [ ] Change state distinguishes `not_applicable`, `none`, `partial`,
      `confirmed`, and `unknown`. A precondition failure before the mutation
      boundary is `none`; an unclassified post-action result is `unknown`; a
      confirmed mutation whose final output fails remains `confirmed`.
- [ ] Catalog declarations own phase, change state, and exact recovery just as
      they own kind/retryability. Runtime faults that disagree fail closed as a
      contract fault, and reconciliation after `partial`, `confirmed`, or
      `unknown` points only to a catalog-validated read.
- [ ] First-use root entry performs only the smallest generic read-only Docker
      prerequisite profile needed for the promised Start-Workspace outcome,
      after review and before Context creation. Failure performs zero Context,
      cluster, Workspace, or Docker mutation.
- [ ] Standalone `context create` remains Docker-independent, and existing
      entry/status paths do not repeat the preflight when their own bounded
      observation already establishes the prerequisite.
- [ ] Cluster startup, Context creation, Workspace logical creation/runtime
      reconciliation, and attachment return typed state only when the owning
      layer can prove it. Possible partial state without proof is `unknown`,
      never `partial` or `none`.
- [ ] Docker readiness uses only bounded, fixed, read-only generic Docker CLI
      observations. Tests prove zero provider process lookup, inference,
      command execution, application opening, or Docker mutation.
- [ ] The published Docker Engine 24-or-newer claim is either mechanically
      enforced by the generic readiness contract or revised everywhere in the
      same change based on recorded compatibility evidence; claimed and
      enforced requirements agree.
- [ ] Docker unavailable, possible-partial cluster startup, Workspace
      create/reconcile failure, Workspace attachment failure, Context creation
      failure, and direct child nonzero exit have frozen same-fixture human and
      JSON evidence with no unsupported cause or mutation claim.
- [ ] A direct foreground child nonzero exit still returns its exact status,
      closes attachment-owned capabilities, prints normal host-side session
      closure guidance, and emits no Tobari structured fault.
- [ ] Diagnostics are bounded, valid UTF-8, visibly projected, secret-free, and
      independent from raw adapter prose. Human and JSON projections contain
      the same causal, phase, change-state, retry, and recovery facts.
- [ ] Recovery requires zero undeclared parsing, provider notation, source
      inspection, or exploratory calls: the initial failure contains the exact
      safe Tobari action, and any necessary read-only reconciliation provides
      the subsequent exact action.
- [ ] Focused tests, the relevant agent-readiness scenarios, `task check`, and
      `task security` pass. `task public:check` also passes if public install or
      generated architecture-site content changes.

## Governing documents

- Thesis: Thesis 0's one actionable continuation and advanced-only Docker
  details; Thesis 5's exact owned lifecycle, observational reads, and explicit
  reconciliation
- Product contract section: public command ledger; guided first entry; output
  and exit contract; side effects and lifecycle recovery
- Architecture or security invariant: four-layer ownership, Docker abstraction,
  structured outcome/cancellation rules, cause stripping, and exact read-only
  recovery after uncertain mutation
- Existing ADR: ADR 0012 for pre-mutation compatibility, ADR 0068 for child
  terminal/status ownership, and ADR 0069 for bounded Docker build diagnostics

## Completion definition

The work is complete when the accepted taxonomy and first-use preflight are
promoted into an ADR and governing contracts, every acceptance criterion has
test evidence, required profiles pass, no provider-specific authority or raw
cause reaches the public boundary, and this temporary packet is removed from
the final tree.
