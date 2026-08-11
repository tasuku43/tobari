# Work Tasks: Interactive auth login provider selection

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read governing theses, product, architecture, security, harness, authentication, external API, and readiness documents.
- [x] Reproduce current required-provider behavior.
- [x] Record verified facts and unknowns in `context.md`.
- [x] Confirm the public outcome and non-goals in `goal.md`.

## Decide

- [x] Compare approaches and record the selected design.
- [x] Confirm `RoleAct`, fixed target, unchanged effect/output, and no opaque-reference flow.
- [x] Confirm the existing `authentication.broker` public capability remains the owner.
- [x] Confirm there is no new trust boundary, dependency, or external call.

## Implement

- [x] Add failing catalog, parser, selector, and zero-mutation tests.
- [x] Implement optional provider input and interactive selection.
- [x] Update durable documentation.

## Verify

- [x] Focused tests pass. Evidence: `go test ./internal/cli ./internal/app/authcmd`; `go test -race ./internal/cli ./internal/app/authcmd`; `go test ./...`; `go test -race ./...`; `go vet ./...`; `task contracts:check`; `task security`; `go mod tidy -diff`; `git diff --check`.
- [ ] `task check` passes. Evidence: after rebasing onto `origin/main`, the gate passes repository and architecture checks, then stops because the separately maintained English and Japanese GitHub Pages JSON-schema table markers are missing or invalid. This packet does not modify `docs/architecture-site/**`.
- [x] Agent-readiness path has zero undeclared external processing. Evidence: catalog parser/help tests cover explicit and omitted forms; omitted selection uses one typed local `auth status` read and the existing login use case with no external parser.
- [x] Generated diff and repository status are understood. Evidence: site generation check passed before the unrelated locale-parity failure; unrelated localization and site-refresh paths remain untouched.

## Hand off

- [ ] Acceptance criteria have evidence.
- [ ] Durable decisions were promoted out of the work packet.
- [ ] Temporary packet is removed.
- [ ] Handoff summary explains outcome, compatibility, and checks.
