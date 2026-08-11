# Work Tasks: Closed Codex and Claude authentication plans

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand and decide

- [x] Read theses and product, architecture, security, harness, publication, release, authentication, external-contract, and readiness documents.
- [x] Accept ADR 0025 for provider clients, acquisition, refresh, projection, pins, API revisions, and evidence.
- [x] Confirm the capability remains inside the closed built-in provider boundary.

## Implement

- [x] Add strict OpenAI and Anthropic provider manifests.
- [x] Add bounded Codex authorization-code and Claude setup-token host drivers.
- [x] Add OpenAI Broker refresh and static Claude credential handling.
- [x] Add exact Gateway supplemental-header projection and hostile-header replacement.
- [x] Update runtime recipes, compatibility labels, embedded source snapshots, and contract checks.
- [x] Update ordinary Markdown contracts and keep GitHub Pages untouched.

## Verify

- [x] Focused auth/runtime Go tests pass.
- [x] `task security` passes.
- [x] `task authbroker:test` passes: source snapshot, image contract, and 101 tests.
- [x] `task gateway:test` passes: 82 tests.
- [x] Implementation Go and static checks pass: focused tests, `go test -race -count=1 ./internal/...`, `go vet ./...`, and `go mod tidy -diff`.
- [x] `task integration:test` passes after replaying the complete four-container suite.
- [ ] `task check` passes; current known deferral is the untouched GitHub Pages JSON-schema table.
- [x] `git diff --check` passes and no generated Python cache is retained.

## Release evidence

- [ ] Record live pinned Codex browser authorization and refresh behavior.
- [ ] Record live pinned Claude setup-token acquisition and API use.
- [ ] Build, review, and pin immutable Gateway API 4 and Auth Broker API 3 images.
- [ ] Run `task public:check` and `task release:check` for publication or release.

## Hand off

- [ ] Commit the coherent implementation without GitHub Pages changes.
- [ ] Reconcile the deferred Pages schema table and pass `task check` in its owning change.
- [ ] Remove this temporary packet after all completion evidence is promoted.
