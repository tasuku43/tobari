# Work Tasks: Causal failure diagnosis and safe recovery

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

Use checkboxes for atomic work and add evidence after completion. This packet
tracks execution and does not override its goal, governing contracts, or ADR.

## Understand

- [x] Read governing theses, product, architecture, security, harness, and
      agent-readiness sections.
- [x] Audit current fault domain, execution invoker, CLI normalization/rendering,
      Catalog recovery validation, and mutation output boundary.
- [x] Inventory root first use, Context create, cluster up/status, Workspace
      resolve/create/reconcile/attach, Doctor, Runtime build, and direct child
      exit paths in `context.md`.
- [x] Verify current generic Docker Doctor argv and the unenforced public
      Docker Engine 24+ claim.
- [x] Record provider-management pressure as thesis evidence rather than a
      backend-specific workaround.
- [ ] Replay any required live generic Docker version compatibility observation
      without managing or identifying its provider. Evidence:

## Decide

- [x] Keep existing `kind` plus `code` as causal identity; reject a parallel
      cause taxonomy.
- [x] Select closed `phase` and `change_state` facts and their proof rules.
- [x] Keep direct child exit outside the structured Tobari fault contract.
- [x] Keep Doctor's complete graph separate while sharing typed generic
      observation primitives.
- [x] Select bounded first-use and cluster-up preflight placement; preserve
      standalone Context creation and avoid duplicate existing-entry preflight.
- [x] Reject Docker-provider detection, management, side effects, and recovery
      commands.
- [ ] Resolve Docker Engine 24+ as a tested minimum or revise the public claim;
      record exact evidence and decision in the ADR.
- [ ] Exhaustively classify existing mutation error declarations by phase and
      change state before production edits.
- [ ] Approve the ADR and structured error schema version/compatibility plan.

## Implement: contract and global enforcement

- [ ] Add the accepted ADR and propagate its durable consequences through
      theses/product/architecture/security/harness as required.
- [ ] Add closed domain types and validation for phase and change state.
- [ ] Extend detached structured faults without exposing causes or weakening
      `errors.Is`/`errors.As` behavior inside trusted layers.
- [ ] Extend `CommandError` and Catalog validation so phase/change state are
      required, runtime agreement is checked, and partial/confirmed/unknown
      mutation recovery is read-only.
- [ ] Assign every global and command fault declaration explicitly; add
      missing-state and disagreement negative tests.
- [ ] Advance and document the structured error schema; update fallback output,
      agent help, human rendering, JSON validation, and golden snapshots.
- [ ] Prove pre-action cancellation is `none`, raw post-action cancellation is
      `unknown`, and confirmed output failure is `confirmed`.

## Implement: generic readiness

- [ ] Add a provider-neutral domain readiness observation with a closed check
      inventory and no provider/backend field.
- [ ] Add the smallest application port/profile selection for first-use and
      cluster-up prerequisites; do not call or parse Doctor presentation.
- [ ] Reuse/refactor Doctor observations without changing its complete graph,
      blocked semantics, or read-only behavior.
- [ ] Implement fixed read-only CLI/context/Engine/Compose observations with
      finite timeout, bounded output, controlled environment, and cancellation.
- [ ] Enforce the decided generic Engine compatibility rule or remove the
      unsupported 24+ claim everywhere.
- [ ] Add recording-runner canaries proving no Docker mutation and no Colima,
      Lima, Docker Desktop, Rancher Desktop, generic app opener, process
      manager, socket probing, or provider inference.
- [ ] Add hostile/oversized/control/credential-like Docker-output tests and
      prove raw cause text is absent from structured and human faults.

## Implement: lifecycle classification

- [ ] Run first-use generic preflight after review and before Context create;
      prove failure performs zero Context/cluster/Workspace/Docker mutation.
- [ ] Keep standalone `context create` Docker-free and prove redirected/JSON
      compatibility.
- [ ] Run cluster-up prerequisites before the invoker; classify action and
      verification receipts without claiming partial change from possibility.
- [ ] Audit and type every cluster structured action fault according to its
      actual mutation point; retain `unknown` when evidence is insufficient.
- [ ] Return Context-create receipts that distinguish validation/no-change,
      unknown storage failure, and confirmed result-validation failure.
- [ ] Return Workspace receipts that retain logical instance/home creation
      evidence across runtime failure without leaking IDs or paths into faults.
- [ ] Classify attachment failure after confirmed Workspace ensure and prove
      attachment-local route/grant/controller cleanup before claiming no active
      attachment.
- [ ] Preserve exact direct child status, child stdout ownership, host stderr
      closure guidance, terminal restoration, and attachment cleanup for shell
      and direct sessions.
- [ ] Apply the model to at least one non-lifecycle read and one non-lifecycle
      mutation to prove it is cross-command rather than a root special case.

## Implement: presentation and interpretation

- [ ] Create typed fixtures and answer keys for Docker unavailable, cluster
      unknown/partial, Workspace create/reconcile, Workspace attach, Context
      create, Doctor read failure, and direct child nonzero.
- [ ] Generate before/after human goldens from the same semantic fixtures and
      complete `presentation-evidence.md` hashes/measurements.
- [ ] Assert human and JSON equivalence for causal identity, phase,
      change-state, retryability, and exact next actions.
- [ ] Add negative-inference canaries for provider names, context names, socket
      paths, same exit numbers, adjacent progress lines, and retained resources.
- [ ] Ensure no wording says "none", "partial", "confirmed", "stopped", or
      "ready" without the corresponding typed proof.
- [ ] Compare Doctor's result-first terminal failure framing and either retain
      it with evidence or adopt a non-duplicating terminator without changing
      machine facts.

## Verify

- [ ] Focused domain/application/CLI/infrastructure tests pass. Evidence:
- [ ] First-use and lifecycle agent-readiness scenarios pass with zero recovery
      discovery calls. Evidence:
- [ ] Generic Docker integration/version-boundary observation passes when
      required. Evidence:
- [ ] `task check` passes. Evidence:
- [ ] `task security` passes. Evidence:
- [ ] `task public:check` passes when public/generated docs change. Evidence:
- [ ] `task release:check` passes if the structured schema ships in a release.
      Evidence:
- [ ] Routine-success external-processing count remains zero; failure recovery
      uses only declared exact commands. Evidence:
- [ ] Generated Catalog/help/site diffs and repository status are understood.
      Evidence:

## Hand off

- [ ] Acceptance criteria have evidence and no false cause/change claim remains
      in the audited public paths.
- [ ] Durable decisions and claimed Docker requirements are promoted and
      mechanically enforced.
- [ ] No provider-specific dependency, command, process, state, or documentation
      was added.
- [ ] Temporary diagnostics, live logs, and private paths are absent.
- [ ] Goal status becomes `Complete` only after all required evidence and gates.
- [ ] Remove this temporary packet in the completion commit.
- [ ] Handoff explains the user outcome, schema compatibility, checks, and any
      deliberately retained `unknown` state.
