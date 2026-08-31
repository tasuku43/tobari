# Work Goal: Publish v0.1.0-dev.20 from one reviewed revision

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/05_public_repository.md` and `docs/06_release.md`
- Review/delete trigger: Delete after publication is verified and the release handoff completes
- Successor: None
- Owner: Release owner
- Target: `v0.1.0-dev.20`
- Related ADRs: ADR 0082

## Outcome

GitHub contains one immutable `v0.1.0-dev.20` prerelease assembled from the
exact reviewed `main` revision and its successful main-push CI run. The release
contains only the declared CLI archive and metadata inventory and performs no
Homebrew or OCI publication.

## Why now

The user requested a final exhaustive monkey test over every command and a
development release only after no remaining defect is found.

## Non-goals

- Stable or Homebrew publication.
- Publication of Tobari-owned OCI images or helper binaries.
- Widening the release capability surface.

## Acceptance criteria

- [ ] Every public command and both embedded helper CLIs pass bounded hostile-input and typed-boundary monkey testing without panic, hang, malformed JSON, or undeclared fault.
- [ ] The location-free Runtime assistance workflow behaves identically across unrelated current directories.
- [ ] The exact release revision passes all local and main-push CI release gates.
- [ ] Manual public-boundary, ownership, confidentiality, dependency, license, and artifact review finds no blocker.
- [ ] Preparation succeeds before the tag exists; publish consumes that exact preparation and creates a GitHub prerelease with the exact inventory.

## Governing documents

- Thesis: task closure, semantics before presentation, and one completion gate
- Product contract section: public CLI and distribution
- Architecture or security invariant: release surface and controlled publication boundary
- Existing ADR: ADR 0082

## Completion definition

The release and inventory are independently verified, all run and revision
identities are recorded during handoff, and this temporary packet is removed in
the cleanup commit.
