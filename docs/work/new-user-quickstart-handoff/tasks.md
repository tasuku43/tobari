# Work Tasks: Make the first-use host handoff executable

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [ ] Read governing thesis/product/architecture/security/harness/public/
      release/readiness documents.
- [ ] Inspect the current README, catalog-backed help, denial output, runtime
      docs, and cleanup guidance.
- [ ] Read the completed Quick Start packet and the new-user comparison.

## Decide

- [ ] Record the final executable recovery wording after policy packet review.
- [ ] Record the runtime prerequisite/reuse wording after runtime packet review.
- [ ] Decide where host/Workspace boundary and Docker VM sharing guidance belong.

## Implement

- [ ] Update only the smallest public docs/help surface required by the
      decisions; do not add stale or guessed commands.
- [ ] Include synthetic denial/review/re-entry/runtime/cleanup examples.
- [ ] Preserve public-boundary language and remove private/machine-specific
      evidence.

## Verify

- [ ] Run exact help/catalog review for documented commands.
- [ ] Replay the complete first-use journey through a real 120x40 PTY.
- [ ] Run `git diff --check`, `task check`, `task public:check`, and relevant
      release/integration profiles.

## Hand off

- [ ] Commit the scoped docs/help changes and report the SHA.
- [ ] Update the coordinator and new-user comparison with final links.
- [ ] Mark complete only after the dependent policy/runtime contracts are
      actually verified.
