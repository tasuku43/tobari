# Work Tasks: Make the first-use host handoff executable

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read governing thesis/product/architecture/security/harness/public/
      release/readiness documents.
- [x] Inspect the current README, catalog-backed help, denial output, runtime
      docs, and cleanup guidance.
- [x] Read the completed Quick Start packet and the new-user comparison.

## Decide

- [x] Record the final executable recovery wording after policy packet review:
      `tobari` is the host re-entry path after exact allow; no `retry` command.
- [x] Record the runtime prerequisite/reuse wording after runtime packet review:
      preflight before new registration; `runtime build` affects new
      Workspaces only and existing Workspaces retain their image.
- [x] Put host/Workspace ownership in the Quick Start handoff and Docker VM
      sharing in prerequisites plus bind-mount troubleshooting.

## Implement

- [x] Update only the smallest public docs/help surface required by the
      decisions; do not add stale or guessed commands.
- [x] Include synthetic denial/review/re-entry/runtime/cleanup examples.
- [x] Preserve public-boundary language and remove private/machine-specific
      evidence.

## Verify

- [x] Run exact help/catalog review for documented commands. Evidence: root
      help plus scoped policy/runtime/context/delete/cluster selectors all
      exited 0; the catalog contains no retry path.
- [x] Replay the complete first-use journey through a real 120x40 PTY as far
      as the environment permits. The focused policy PTY and dependent runtime
      lifecycle transcript pass; the aggregate runtime replay reaches the
      policy review child and is recorded as an exit-130 external blocker.
- [x] Run `git diff --check`, `task check`, `task public:check`, and relevant
      release/integration profiles. `task check`, `task public:check`,
      `task security`, and `task runtime:base:check` pass; `runtime:test`
      exits 130 at the known PTY input handoff and `release:check` exits 1 at
      the existing ShellCheck warnings in `scripts/test-integration.sh:238`.

## Hand off

- [x] Commit the scoped docs/help changes and report the SHA.
- [x] Keep the coordinator and new-user comparison unchanged; their evidence
      is consumed here without widening this packet's public scope.
- [x] Mark complete after the dependent policy/runtime contracts were verified;
      the known aggregate policy-review PTY handoff remains recorded as an
      external blocker rather than being hidden.
