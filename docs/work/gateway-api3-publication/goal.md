# Work Goal: Publish and pin Gateway API 3

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/05_public_repository.md` and `docs/06_release.md`
- Review/delete trigger: Delete after the reviewed API-3 manifest digest is pinned and all required gates pass
- Successor: None
- Owner: Repository owner
- Target: Current GraphQL enforcement change
- Related ADRs: `docs/decisions/0022-authorize-declared-graphql-root-fields.md`

## Outcome

The reviewed Linux amd64/arm64 Gateway API-3 image is published by the
main-only GitHub workflow, its immutable multi-architecture manifest digest is
verified, and routine Tobari startup pins that digest instead of the
incompatible Gateway API-2 image.

## Why now

The current worktree requires Gateway API 3, while `versions.env` still pins
the prior API-2 manifest. The normal `tobari cluster up` therefore fails with
`gateway_image_incompatible`; only the source-built development path works.

## Non-goals

- Publishing a CLI release or Git tag
- Changing the compatible Auth Broker API-2 image pin
- Publishing an image directly from a maintainer workstation

## Acceptance criteria

- [ ] The main-only workflow publishes Gateway API 3 for Linux amd64/arm64.
- [ ] The immutable manifest and both platform members declare the reviewed source revision, API/role labels, non-root user, and entrypoint.
- [ ] `versions.env` pins the reviewed manifest digest and normal `cluster up` accepts it.
- [ ] Public and release documentation records the reviewed revision and digest without retaining moving tags as authority.
- [ ] Required repository, security, public, release, Gateway, and integration gates pass.

## Governing documents

- Thesis: 1, 2, 6, and 7
- Product contract section: Shared-cluster startup and failure recovery
- Architecture or security invariant: Fail-closed Gateway image preflight and immutable runtime selection
- Existing ADR: ADR 0022

## Completion definition

The work is complete when the API-3 image is published and independently
inspected, the immutable digest is pinned and merged, required gates pass, and
this temporary packet is removed.
