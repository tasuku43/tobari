# Work Tasks: Publish and pin Gateway API 3

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read governing theses and publication/release contracts.
- [x] Reproduce and identify the API-2 pin versus API-3 CLI mismatch.
- [x] Confirm the source-built API-3 development cluster is healthy.

## Verify source change

- [ ] Run focused Gateway and policy tests.
- [ ] Run `task check`.
- [ ] Run `task security`.
- [ ] Run `task public:check`.
- [ ] Run `task release:check`.
- [ ] Run Auth Broker and integration tests required by the release contract.

## Publish

- [ ] Commit the complete API-3 feature on a publication branch.
- [ ] Push and open a reviewed pull request.
- [ ] Merge to `main` and wait for the main-only Gateway workflow.
- [ ] Record the immutable source revision and manifest digest.
- [ ] Inspect both supported platform members and image metadata.

## Pin

- [ ] Update `versions.env` to the reviewed API-3 manifest digest.
- [ ] Update durable public/release evidence.
- [ ] Verify normal production-resolver startup.
- [ ] Merge the pin update.

## Hand off

- [ ] Confirm all acceptance criteria.
- [ ] Remove this temporary packet.
- [ ] Summarize publication, pin, checks, and remaining release caveats.
