# Work Tasks: Make routine CLI output result-first and task-specific

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [ ] Re-read governing docs, relevant ADRs, dependency packets, the complete
      interface-exploration-loop Skill, and add-capability Skill at
      implementation time.
- [ ] Re-run representative current CLI commands and record bounded before
      goldens.
- [ ] Inventory every Context list/show and status semantic result variant,
      including synthetic, absent, detached, attached, and action-required.
- [ ] Define finite typed routine-client and Workspace-default summary states
      without renderer inference.
- [ ] Complete the frozen fixture and answer-key fields in
      `presentation-evidence.md`.
- [x] Confirm the public outcome and non-goals. Evidence: product-owner
      selected the combined result-first/task-specific concept on 2026-08-21.

## Decide

- [x] Use result-first ordinary output and task-specific commands. Evidence:
      product-owner approval on 2026-08-21.
- [x] Keep grouped-complete technical output on `--details`, JSON, and
      specialist commands. Evidence: product-owner approval on 2026-08-21.
- [x] Reject a global beginner/expert preference. Evidence: product-owner
      approval on 2026-08-21.
- [x] Hide agent profile, native-readiness terminology, IDs, revisions, images,
      and paths from routine human output while preserving accepted detailed
      contracts. Evidence: approved concept matrix on 2026-08-21.
- [ ] Decide exact finite summary enums and attention-worthy bootstrap/runtime
      states after typed-result audit.
- [ ] Decide human root-help grouping only if it remains catalog-derived.
- [ ] Decide whether status needs a catalog-declared `--details` input or an
      existing specialist command is sufficient.

## Implement

- [ ] Add failing typed-summary, fixture, answer-key, and negative-inference
      tests.
- [ ] Add minimal domain/application summary facts required by presentation.
- [ ] Update ordinary Context list/show and retain complete detailed/JSON views.
- [ ] Update ordinary status without adding pending/exposure aggregation.
- [ ] Update human root help and Shared-services wording only through
      catalog-derived presentation.
- [ ] Promote product, architecture, harness, README/site, fixtures, and
      generated documentation.
- [ ] Do not implement dependency packets, onboarding, status aggregation, or
      service exposure in this packet.

## Verify

- [ ] Focused domain/app/CLI/catalog/presentation tests pass. Evidence:
- [ ] Same-fixture before/after presentations remain semantically eligible.
      Evidence:
- [ ] JSON exact keys and null/absent/state distinctions remain complete.
      Evidence:
- [ ] Exact next argv and recovery remain executable and unchanged. Evidence:
- [ ] Routine success requires zero external reconstruction. Evidence:
- [ ] `task check` passes. Evidence:
- [ ] `task public:check` passes if a publishable schema changes. Evidence:
- [ ] Generated diff and repository status are understood. Evidence:

## Hand off

- [ ] Acceptance criteria have evidence.
- [ ] Durable conclusions are promoted out of the packet.
- [ ] Temporary comparison artifacts are removed or retained only as harness
      fixtures.
- [ ] The packet is removed in the same completion commit.
- [ ] The implementation is one concern-specific commit on main.
