# Work Tasks: Keep the auth-broker experiment detached from `main`

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read `AGENTS.md` and `docs/00_theses.md` through `docs/04_harness.md`.
- [x] Read `docs/07_authentication.md`, `docs/08_external_api_contracts.md`, and `docs/09_agent_readiness_validation.md`.
- [x] Inspect refs, merge-base, divergence, changed paths, and auth tree reachability without checkout.
- [x] Preserve unrelated worktree changes.
- [x] Record the thesis/product/architecture/security/harness reason for deferral.

Evidence: `main=966dd08`, `codex/auth-broker=4ccc756c`, merge-base
`98a9b7d`, divergence `23 5`, and 77 changed paths with 7,153 insertions and
382 deletions.

## Decide

- [x] Classify auth-broker as deferred experiment, not a `main` capability.
- [x] Prove no `main` Catalog command, capability ID, help entry, or implementation path exists.
- [x] Prove experiment files remain addressable only by `codex/auth-broker`.
- [x] Define the future restart gate and require a new reviewed packet.
- [x] Confirm no auth implementation or ref operation is authorized here.

## Verify

- [x] Ref governance E2E passed on 2026-08-03.
- [x] Main archive `task check` passed.
- [x] Main archive `task public:check` passed.
- [x] Main archive `task build` passed.
- [x] Main help/Catalog/capability/path negative E2E passed.
- [x] `task security` was rerun on clean main and passed: `repoguard (security): OK` and `No vulnerabilities found.`
- [x] No coordinator, code, or branch ref was changed by this packet.
- [x] Stage exactly four packet files and create one docs-only main commit. Evidence: the commit contains only `docs/work/auth-broker-deferral/{goal,context,plan,tasks}.md`; the auth implementation branch is not staged.

## Hand off

- [x] Return commit SHA and final status. Evidence: docs-only commit `93e5cac`;
      packet status remains `Accepted` / explicitly deferred.
- [x] Keep the packet `Accepted`/deferred after the four-file commit; do not
      mark the unimplemented auth branch as product-complete.
- [x] Keep the security result explicit; clean main `task security` passed after the first-wave merge.
- [x] Keep future restart work behind a new reviewed successor packet.
