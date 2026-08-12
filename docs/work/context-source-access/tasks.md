# Work Tasks: Select direct source access per Context

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Inspect Context manifest/create flow, root mapping, mount argv, external
      Git mount, spec hashing, reconciliation, and integration coverage.
- [x] Record current direct read-write behavior and accepted risks.
- [ ] Observe supported-platform nested-bind behavior before implementation.

## Decide

- [x] Select immutable Context-owned access with `read-write` default.
- [x] Keep direct binding and reject per-entry overrides and clone semantics.
- [x] Accept exact manifest/JSON field names and user-facing wording.
      Evidence: required `source_access` uses `read-only|read-write`; text says
      `Source access: direct ...` and never labels the whole Workspace read-only.
- [x] Promote the decision through the capability-envelope ADR and ADR 0010.
      Evidence: accepted ADR 0029 revises ADR 0010 and fixes direct
      `read-only|read-write` access as immutable Context authority.

## Implement

- [x] Add failing domain tests for the closed enum and required manifest,
      summary, and report facts.
- [x] Add `--source-access` catalog input/default and output fields/fixtures.
- [x] Extend application create input/port/result validation.
- [x] Persist the exact value and include it in project desired spec/hash.
- [x] Render and inspect exact Docker read-only/read-write source mounts.
- [x] Ensure no home, generated Git projection, profile, or other mount creates a writable
      source alias.
- [x] Update context/status/list presentation and catalog source declarations.
- [x] Regenerate catalog-derived public architecture-site data after the
      integration branch has combined all V1 catalog changes. Evidence: the
      integrated architecture data and EN/JA Context pages carry
      `source_access`; `mise exec -- task check:fast` passed `sitegen --check`,
      source verification, locale parity, typecheck, and both site builds.
- [x] Update theses, product, architecture, security, threat model, harness,
      ADR, and readiness docs. Evidence: the static V1 governing-contract
      commits and readiness source-access matrix distinguish the selected live
      bind from Workspace home/tmpfs and reject clone/snapshot claims.

## Verify

- [x] Invalid access causes zero filesystem/Docker side effects.
      Evidence: application tests reject unknown access before the runtime port;
      store tests reject missing persisted access without changing owned state.
- [ ] Read-only source read succeeds and create/change/delete/Git writes fail.
      Evidence pending runtime execution: the integration journey now performs
      each operation against the selected bind and fails if any mutation
      succeeds; the local Colima engine blocked before the fixture container
      started on 2026-08-12.
- [x] Workspace home and tmpfs remain writable.
      Evidence: exact Docker argv tests keep the home bind writable and both
      tmpfs mounts present while changing only the selected source bind.
- [ ] Home-relative and external-root layouts enforce the same access.
- [ ] Same-root read-only/read-write Contexts remain separate and the read-only
      view honestly observes external changes.
      Evidence: project-state tests bind the same canonical root to distinct
      Context IDs carrying opposite access. Live cross-Context observation
      is encoded in `scripts/test-integration.sh`; the local Docker attempt was
      blocked before fixture startup and supplies no platform result.
- [x] Restart, drift, and re-entry preserve exact access.
      Evidence: access is required in the persisted manifest, included in the
      desired spec/hash, inspected from Docker Mounts, and mismatches recreate
      the work container.
- [x] Focused tests pass. Evidence: `go test ./internal/domain/tobari
      ./internal/app/contextcmd ./internal/app/tobaricmd
      ./internal/infra/dockerruntime ./internal/cli` passed on 2026-08-12.
- [ ] `task check` passes. Evidence: `mise exec -- task check:fast` passed on
      2026-08-12 on the integrated V1 branch after the final integration-script
      changes. Full check and a functioning Docker runtime remain required.
- [x] `task security` passes. Evidence: `mise exec -- task security` passed on
      the integrated V1 branch on 2026-08-12.
- [ ] `task public:check` passes. Evidence: repoguard and contractlint passed;
      the gate stopped only at the deliberate unpublished Gateway digest
      checkpoint, which requires the later approved publication lane.

## Hand off

- [ ] Acceptance criteria have evidence and the parent Context packet agrees.
- [ ] No documentation claims clone, snapshot, confidentiality, recovery, or a
      wholly read-only Workspace.
- [ ] Durable conclusions are promoted and this temporary packet is removed.
