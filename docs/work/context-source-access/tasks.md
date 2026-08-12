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
- [ ] Accept exact manifest/JSON field names and user-facing wording.
- [ ] Promote the decision through the capability-envelope ADR and ADR 0010.

## Implement

- [ ] Add failing domain tests for the closed enum and required manifest,
      summary, and report facts.
- [ ] Add `--source-access` catalog input/default and output fields/fixtures.
- [ ] Extend application create input/port/result validation.
- [ ] Persist the exact value and include it in project desired spec/hash.
- [ ] Render and inspect exact Docker read-only/read-write source mounts.
- [ ] Ensure no home, generated Git projection, profile, or other mount creates a writable
      source alias.
- [ ] Update context/status/list presentation and generated catalog data.
- [ ] Update theses, product, architecture, security, threat model, harness,
      ADR, and readiness docs.

## Verify

- [ ] Invalid access causes zero filesystem/Docker side effects.
- [ ] Read-only source read succeeds and create/change/delete/Git writes fail.
- [ ] Workspace home and tmpfs remain writable.
- [ ] Home-relative and external-root layouts enforce the same access.
- [ ] Same-root read-only/read-write Contexts remain separate and the read-only
      view honestly observes external changes.
- [ ] Restart, drift, and re-entry preserve exact access.
- [ ] Focused tests pass. Evidence:
- [ ] `task check` passes. Evidence:
- [ ] `task security` passes. Evidence:
- [ ] `task public:check` passes. Evidence:

## Hand off

- [ ] Acceptance criteria have evidence and the parent Context packet agrees.
- [ ] No documentation claims clone, snapshot, confidentiality, recovery, or a
      wholly read-only Workspace.
- [ ] Durable conclusions are promoted and this temporary packet is removed.
