# Work Plan: Publish v0.1.0-dev.20

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Run deterministic catalog-wide and bounded generated monkey matrices first,
fix every reproducible contract defect with regression tests, then close all
local release gates. Commit and push one reviewed revision to `main`, wait for
its exact CI run, prepare assets, create the tag, and publish only that prepared
inventory.

## Alternatives considered

### Publish after unit tests only

Rejected because parser, generated help, CWD, helper-program, and real-process
boundaries require black-box coverage beyond unit tests.

### Create the tag before preparation

Rejected because the release contract intentionally reserves tag creation for
the explicit approval checkpoint after successful non-publishing preparation.

## Design

### Public contract

No new command is added. Defects found by monkey testing are closed within the
existing catalog, structured fault, help, and JSON stream contracts.

### Error and cancellation behavior

All tested failures must be declared schema-2 errors with bounded public text.
Cancellation must retain no side effect and no command may panic or hang.

### Security and public boundary

Tests use temporary roots and fake references. Publication is limited to the
manual protected workflow and exact prerelease inventory.

## Implementation slices

1. Enumerate and exercise all public commands and helper programs.
2. Add regression tests and minimal contract fixes for every defect.
3. Run all local release and isolated-runtime gates.
4. Confirm exact-revision main CI and prepare assets.
5. Tag, publish, verify inventory, and remove this packet.

## Verification

- Unit and contract tests: focused regressions plus `task check`.
- Structured output, hostile-output, and recovery tests: catalog-wide main/helper monkey matrices.
- Manual observation: Runtime assistance from repository, ancestor, and unrelated directories.
- Required profiles: full, security, public, release, policy, Gateway, cold first-use, and runtime release aggregate.
- Generated-diff or artifact checks: helper snapshot, release preparation, checksum, SPDX, provenance, and final inventory verification.

## Rollout and rollback

Publication is immutable. Any blocker stops before tagging; a post-publication
defect is replaced by a new version rather than overwritten.

## Documentation promotion

No new durable release decision is expected. Any discovered policy gap must be
promoted before publication rather than retained in this packet.
