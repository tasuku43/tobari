# Work Tasks: Publish and pin the trusted authentication images

## Prepare

- [x] Read governing documents and the `add-capability` skill.
- [x] Inspect the worktree, workflows, current image references, GitHub authentication, and package visibility.
- [x] Record the two-image publication decision and constraints.

## Publish implementation revision

- [x] Run `task check`, `task security`, and `task public:check`.
- [ ] Commit and push the complete implementation to `main`.
- [ ] Record successful Auth Broker and Gateway workflow runs.
- [ ] Verify public anonymous access and exact multi-architecture indexes.

## Promote digests

- [ ] Pin both reviewed immutable digests.
- [ ] Update published-state documentation, tests, and typed recovery behavior.
- [ ] Run official-image runtime/integration validation.
- [ ] Run `task check`, `task security`, `task public:check`, and `task release:check`.
- [ ] Push the promotion and confirm required GitHub checks.

## Close

- [ ] Promote durable evidence to governing documents.
- [ ] Remove this temporary packet and the superseded Gateway publication packet.
- [ ] Leave a concise implementation and publication handoff.
