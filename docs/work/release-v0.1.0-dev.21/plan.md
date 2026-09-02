# Work Plan: Create the v0.1.0-dev.21 development tag

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Close all local release gates on the reviewed tree, commit it to main, push the
exact revision, wait for its main-push CI, run the non-publishing Release
preparation, then create and push one annotated tag. Verify the remote binding
and leave the GitHub Release unpublished.

## Verification

- `task check`
- `task security`
- `task public:check`
- `task release:check`
- `TOBARI_INTEGRATION_DOCKER_CONTEXT=<cold-context> task runtime:release`
- exact main-push CI and Release preparation run inspection
- local and remote annotated-tag object inspection

## Rollout and rollback

Any blocker stops before tagging. Once pushed, the tag is immutable under the
release contract; a defect requires a new version rather than moving the tag.
