# Work Plan: Restore the regular Gateway-image integration path

- Status: Proposed
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)

## Investigation

1. Compare the pinned Gateway digest and image labels/API contract with the
   current Gateway source and the image built by `--gateway-source`.
2. Inspect the release workflow and its artifact timing to determine whether
   the digest is stale, mis-selected, or locally cached incorrectly.
3. Reproduce on the supported Docker architecture without modifying policy
   semantics.

## Fix options

- Refresh the published image and reviewed digest through the release boundary
  if the artifact is stale.
- Repair image selection or validation if the digest resolves to the wrong
  architecture/content.
- If the failure is only an undocumented local prerequisite, add a narrow
  preflight diagnostic and documentation while keeping the default path
  pinned-image-only.

## Verification

- Run the default and explicit-source integration scenarios and compare the
  Gateway/OPA decision evidence.
- Run `task check`, `task security`, `task public:check`, and the relevant release
  gate after any release-boundary change.
- Remove this packet only after the regular path is green or the governing
  harness contract explicitly records the external prerequisite.
