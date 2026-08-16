# Work Tasks: Capability profiles and first prerelease

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand and decide

- [x] Read governing documents and the add-capability Skill.
- [x] Verify prerelease classification and current workflow branches.
- [x] Enumerate existing public Tobari GHCR packages without mutation.
- [x] Select reusable standard/experimental profiles.
- [x] Replace the release lock with local-only embedded Gateway and base images.

## Implement

- [x] Add profile vocabulary and build identity.
- [x] Filter provider projection, catalog, and drivers.
- [x] Add standard and experimental negative/positive tests.
- [x] Remove the component lock and add embedded local Gateway/base ensuring.
- [x] Remove every Tobari-owned OCI publication path from Release.
- [x] Update durable governing documentation and release wording.

## Verify and hand off

- [x] Focused standard and experimental tests pass.
- [x] `task check` passes after the local-base redesign.
- [x] `task security` passes after the local-base redesign.
- [x] `task public:check` passes after the local-base redesign.
- [x] `task release:check` passes after the local-base redesign.
- [x] Runtime checks pass after the local-base redesign.
- [x] Existing GHCR packages are deleted and anonymous reads confirm absence.
- [x] Exact `v0.1.0-dev.1` dry-run and publish commands are recorded.
- [x] Advance the public documentation source snapshot after the implementation commit and regenerate its evidence.
- [ ] Temporary packet is removed after durable promotion and completion.
