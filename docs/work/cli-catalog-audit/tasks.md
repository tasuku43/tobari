# Work Tasks: CLI catalog audit

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read governing theses, product, architecture, and security sections.
      Evidence: `docs/00_theses.md` through `docs/04_harness.md`, plus
      `docs/09_agent_readiness_validation.md` and the capability-retirement
      template.
- [x] Reproduce or observe current behavior. Evidence: source inventory and
      clean-binary E2E transcript in `e2e-transcript.md`.
- [x] Record verified facts and unknowns in `context.md`.
- [x] Record repeated decisions, friction, and potential thesis workarounds as
      evidence. Evidence: CWD lifecycle versus stale named definitions and
      policy projection overlap in `context.md`.
- [x] Confirm the public outcome and non-goals in `goal.md`.

## Decide

- [x] Compare credible approaches and record the selected design.
- [x] Identify public-contract and compatibility impact.
- [x] Classify utility/discover/act roles and opaque reference flow.
- [x] Classify each capability as public or keep its existing status; record
      candidates as delete/integrate/maintain/hold.
- [x] Identify effects, targets, assets, and trust-boundary changes.
- [x] Decide output delivery, collection coverage, retry, and recovery behavior
      from the catalog declarations.
- [x] Create or update an ADR for a durable trade-off. Evidence: not needed;
      this audit introduced no durable contract change. Revisit if a retirement
      is promoted.
- [x] Revise no thesis; record evidence and downstream impact instead.
- [x] Obtain required design approval. Evidence: user explicitly assigned the
      first-wave CLI catalog audit with E2E completion as the evaluation rule.

## Implement

- [x] Add failing contract or negative-path tests. Existing retired-command and
      whole-catalog tests are sufficient; no catalog defect was found that
      required a new test.
- [x] Implement domain invariants. Not applicable to an inventory audit.
- [x] Implement application use case and owned ports. Not applicable.
- [x] Implement bounded infrastructure adapters. Not applicable.
- [x] Register a command in `cli.Catalog` and update presentation. No new
      public command is justified by the audit.
- [x] Update the capability ledger or schema manifest. No public status change
      was justified by the audit.
- [x] Add producer/consumer graph and exact opaque-ID round-trip evidence.
- [x] Add structured output/error, cancellation, hostile-output, and
      zero-downstream-call observations in proportion to audit risk.
- [x] Add or update harness enforcement. No real catalog defect was found;
      integration isolation is an explicit follow-up finding.
- [x] Update durable documentation. Forward stale README evidence; do not edit
      another packet in this scope.
- [x] For a removed/replaced capability, prove retirement with the copied
      capability-retirement checklist. Required before any public retirement;
      no public path is being removed in this packet.

## Verify

- [x] Focused tests pass. Evidence: `GOCACHE=<task-local> go test ./internal/cli
      ./internal/app/tobaricmd` passed.
- [x] `task check` passes. Evidence: `GOCACHE=<task-local> task check:fast` and
      `GOCACHE=<task-local> task check` both passed with exit 0.
- [x] `task security` passes when required. Evidence: not required because no
      dependency/security boundary changes.
- [x] `task public:check` passes when required. Evidence: public repoguard and
      contractlint passed with exit 0.
- [x] Runtime-only behavior was observed on the required platform. Evidence:
      the isolated argv matrix and the real Docker integration runner result
      are recorded in `e2e-transcript.md`; the positive shared-resource flow is
      explicitly held after `cluster_start_failed`.
- [x] Relevant agent-readiness scenario met its discovery-round-trip budget.
      Evidence: root index, four namespace scopes, all 25 exact scopes, and
      representative argv/recovery paths were exercised; routine processing
      count is zero.
- [x] Routine-success external-processing count is zero for every supported
      outcome exercised by this audit. Evidence: direct argv and JSON parsing
      only; no provider parser or exploratory call.
- [x] Setup/authentication candidates have no new human-handoff scorecard need.
- [x] Generated diff and repository status are understood. Evidence: only this
      child packet and any explicitly proven dead-code cleanup may change.

## Hand off

- [x] Acceptance criteria have evidence in the catalog matrix and E2E
      transcript.
- [x] Goal status is `Complete` after the applicable goal and task checkboxes
      were completed. Evidence: the catalog E2E and required gates are
      complete, and scoped commit
      `a007059863ec3c5f80441c9ac86bbc643ca1d5ac` is merged into current `main`.
- [x] Durable decisions were promoted out of the work packet. Evidence: no
      durable decision was introduced; retirement and read-effect changes are
      explicit follow-up candidates.
- [x] Temporary diagnostics and sensitive artifacts were removed. Evidence:
      the audit-owned integration fixture was moved outside the repository;
      only synthetic packet text is retained by this packet. Other untracked
      paths shown by status are outside this packet and were not touched.
- [x] Follow-up work is explicit and does not block this audit: legacy source
      retirement, Context read-effect decision, README correction, and isolated
      Docker integration harness.
- [x] Handoff summary explains classifications, E2E results, checks, and risks
      in this packet and its transcript.
