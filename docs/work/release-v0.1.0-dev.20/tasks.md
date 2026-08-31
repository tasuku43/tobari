# Work Tasks: Publish v0.1.0-dev.20

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand and test

- [x] Read governing release and public-repository contracts.
- [x] Confirm tag classification and absence of a conflicting local/remote release.
- [x] Enumerate every public command and helper-program command.
- [x] Complete the final fixed-revision monkey reruns with zero remaining defect.

## Fix and verify

- [x] Add regression tests for discovered completion, fault-classification, helper-help, JSON-stream, and bounded-argv defects.
- [x] Pass `task check`, `task security`, `task public:check`, and `task release:check` on the final tree.
- [x] Pass the isolated `task runtime:release` aggregate on the final tree.
- [x] Complete manual source, history, dependency, license, confidentiality, trademark, and generated-artifact review.

## Prepare and publish

- [ ] Commit and push the exact reviewed revision to `main`.
- [ ] Verify the exact successful main-push CI run.
- [ ] Run and verify non-publishing preparation; record its run ID.
- [ ] Create and push the annotated `v0.1.0-dev.20` tag.
- [ ] Publish from the exact preparation run and verify prerelease status and inventory.
- [ ] Remove this temporary packet in the cleanup commit.
