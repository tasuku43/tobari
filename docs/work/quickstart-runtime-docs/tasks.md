# Work Tasks: Quick Start runtime documentation

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read `AGENTS.md`, `docs/00_theses.md`, `docs/01_product_contract.md`,
      `docs/02_architecture.md`, `docs/03_security_model.md`,
      `docs/04_harness.md`, `docs/05_public_repository.md`,
      `docs/06_release.md`, and `docs/09_agent_readiness_validation.md` before
      editing. Evidence: all required documents were read before the patch.
- [x] Inspect the current README, catalog-backed public paths, runtime recipe
      template, and integration/runtime harness. Evidence: existing commands
      and the first-wave integration packet were read; no production file was
      changed.
- [x] Record the current journey, constraints, unknowns, and public-boundary
      facts in `context.md`.
- [x] Confirm the outcome and non-goals in `goal.md`.

## Decide

- [x] Select a single README journey over existing commands. Evidence:
      `plan.md` chooses CWD entry, explicit cluster startup, host policy
      recovery, and active-Context runtime build.
- [x] Preserve discover/act separation and opaque-reference round trips.
      Evidence: review/candidates remain read-only and allow/deny consume one
      unchanged `pcy_...` value.
- [x] Preserve the host/agent trust boundary and explicit runtime-build
      authority. Evidence: no agent-side policy, Docker, Context, or credential
      action is documented.
- [x] Compare the split README and direct-Docker alternatives. Evidence:
      `plan.md` records both rejected alternatives.

## Implement

- [x] Create `docs/work/quickstart-runtime-docs/` from the work-packet template
      with only the requested packet files.
- [x] Rewrite the README Quick Start with prerequisites, explicit `cluster up`,
      denied in-Tobari curl, host review/allow, same-request retry, runtime
      recipe edit, explicit build, and final entry.
- [x] State exact recovery commands for startup, TTY, denial, runtime recipe,
      build, attached session, and cleanup failures.
- [x] Remove the stale retired `tobari exec` example from the public journey.
- [x] Keep examples synthetic and public-safe; add no command, flag, image
      authority, dependency, or implicit pull.

## Verify

- [x] Focused help/catalog review passes. Evidence: root agent help exited 0
      with schema 8/index/25 entries; scoped policy/runtime help exited 0.
- [x] The documented journey is replayed through the repository integration or
      bounded Docker E2E. Evidence: the real aggregate profiles reached the
      existing PTY stop; the separate bounded Docker runtime replay completed.
- [x] Runtime customization evidence includes the active Context Dockerfile
      edit, explicit build, promotion, and entry—or records the exact stop.
      Evidence: `tree` was built and printed `tree v2.1.0` inside the Workspace;
      external XDG bind visibility and aggregate PTY stops are recorded.
- [x] `git diff --check` passes. Evidence: clean verification checkout returned
      exit 0.
- [x] `task check` passes. Evidence: clean verification checkout returned exit 0.
- [x] `task public:check` passes. Evidence: clean verification checkout returned
      exit 0 with public repoguard and contractlint OK.
- [x] Relevant `task integration:test` and `task runtime:test` checks pass or
      their exact environment blockers are recorded without claiming success.
      Evidence: both stopped with task exit 130 at the existing PTY review
      helper; policy/Gateway/base runtime checks passed separately.
- [x] Final diff contains only the six allowed paths and no first-wave packet
      changes. Evidence: final explicit staging and pre-commit path review.

## Hand off

- [x] Acceptance criteria in `goal.md` have evidence.
- [x] The packet records the final E2E/gate outputs and any blocker.
- [x] `main` remains the current branch and no history rewrite/reset/force was
      used.
- [x] One intentional scoped commit contains exactly the allowed paths.
- [x] Final handoff reports changed paths, E2E/gate output, blockers, and the
      commit SHA.
