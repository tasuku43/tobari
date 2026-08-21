# Work Tasks: Switch from an attached Workspace to trusted-host review

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read governing theses, product, architecture, security, and harness
      sections. Evidence: current `docs/00_theses.md` through
      `docs/04_harness.md` reviewed in numeric order on 2026-08-21.
- [x] Reproduce or observe current behavior. Evidence: root and policy-review
      help plus current Docker entry, PTY relay, browser channel, Permission
      Inbox, and harness contracts inspected.
- [x] Record verified facts and unknowns in `context.md`.
- [x] Record repeated decisions, friction, and the required thesis revision as
      evidence.
- [x] Confirm the public outcome and non-goals in `goal.md`.

## Decide

- [x] Compare credible approaches and record the selected design. Evidence:
      separate host terminal, Workspace helper handoff, host browser UI, and
      established terminal prefix patterns were compared; the product owner
      selected the `Ctrl+]` then `r` host prefix on 2026-08-21.
- [x] Identify public-contract and compatibility impact. Evidence: root
      attachment help and the direct-Docker terminal promise change; no
      persisted schema changes.
- [x] Classify utility/discover/act roles and opaque reference flow. Evidence:
      no new command or reference; the root attachment composes the existing
      Permission Inbox read and canonical reviewed-set Apply.
- [ ] Mark the capability public in the capability ledger when the PTY evidence
      and ADR pass; keep it unshipped before then.
- [x] Identify effects, target, assets, and trust-boundary changes. Evidence:
      host terminal input/PTY ownership changes; existing policy read/write
      boundaries remain authoritative.
- [x] Decide external API contracts. Evidence: not applicable; no external API,
      pagination, or authentication change.
- [ ] Write and accept the terminal-ownership ADR before mechanism code.
- [ ] Revise and propagate the current no-input-interception thesis before
      adding the exception.
- [x] Obtain required design approval. Evidence: product owner selected the
      same-terminal Trusted Host Review and then selected `Ctrl+]`, followed by
      `r`, after comparison with established patterns on 2026-08-21.

## Implement

- [ ] Add failing prefix grammar, catalog/help, isolation, restoration, and
      child-status tests.
- [ ] Implement pure attachment/review state invariants.
- [ ] Compose existing Permission Inbox reads and reviewed-set Apply without a
      second policy path.
- [ ] Implement the bounded PTY experiment and record go/no-go evidence.
- [ ] If the experiment passes, implement the infrastructure prefix decoder,
      mode switch, child-output handling, and exact cleanup.
- [ ] Update root catalog interactive metadata and trusted presentation.
- [ ] Update the capability ledger and any publishable schema/fixture manifest.
- [ ] Preserve existing policy opaque-reference round trips and add same-label,
      stale, and hostile-output canaries through the host surface.
- [ ] Add structured failure, cancellation, child-exit, resize, signal,
      continuous-output, raw-mode, and alternate-screen tests.
- [ ] Update durable documentation and ADR.
- [ ] Add the typed semantic fixture, answer key, exact next interaction, and
      before/after evidence in `presentation-evidence.md`.

## Verify

- [ ] Focused tests pass. Evidence:
- [ ] Relevant Docker/runtime integration passes. Evidence:
- [ ] `task check` passes. Evidence:
- [ ] `task security` passes. Evidence:
- [ ] `task public:check` passes. Evidence:
- [ ] Runtime-only behavior is observed on supported macOS and Linux terminal
      paths, including US and JIS layouts. Evidence:
- [ ] The relevant agent-readiness scenario needs one documented prefix and no
      separate terminal. Evidence:
- [ ] Routine success has zero undeclared external-processing steps. Evidence:
- [ ] Generated diff and repository status are understood. Evidence:

## Hand off

- [ ] Acceptance criteria have evidence.
- [ ] Goal status is changed to `Complete` only after all goal and task
      checkboxes are complete.
- [ ] Durable decisions are promoted out of the work packet.
- [ ] Temporary diagnostics and terminal transcripts are removed.
- [ ] The follow-on Workspace service-exposure packet explicitly depends on
      the accepted terminal switch and keeps its authority separate.
- [ ] Handoff summary explains outcome, why, checks, and remaining terminal
      risks.
