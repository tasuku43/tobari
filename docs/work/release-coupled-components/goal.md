# Work Goal: Release-coupled component images

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/00_theses.md`, `docs/01_product_contract.md`, `docs/06_release.md`
- Review/delete trigger: Delete after durable conclusions are promoted and the change completes
- Successor: None
- Owner: Tobari maintainers
- Target: First public release
- Related ADRs: 0017, 0032

## Outcome

One release invocation builds Gateway and Auth Broker from the selected source
revision, produces one validated component lock, injects that lock into every
CLI archive, and publishes the mutually bound artifacts without a digest-pin
commit. Repository development uses content-addressed local component images.

## Why now

The existing image-first flow requires publishing two images, copying their
digests into source, committing that generated authority, and rebuilding the
CLI. That loop makes source/image compatibility a manual release operation and
makes the ordinary repository build unusable while pins are unpublished.

## Non-goals

- Publishing the base runtime or third-party agent images in this unit.
- Removing immutable GHCR images from the installed-user path.
- Weakening component image metadata, architecture, or runtime preflight.

## Acceptance criteria

- [ ] A strict schema-1 component lock binds source revision, both immutable image references, APIs, and supported platforms.
- [ ] Release packaging requires the lock and injects its exact authorities into every CLI executable.
- [ ] Release CI creates the lock from the same requested revision before CLI packaging, with no source pin commit.
- [ ] Repository development builds and selects content-addressed local images and reuses unchanged images.
- [ ] Public startup still requires immutable digest references and performs existing image contract checks.
- [ ] `task check`, security, release, and public profiles pass.

## Governing documents

- Thesis: bounded autonomy, executable claims, immutable runtime authority
- Product contract section: runtime image selection and `cluster up`
- Architecture or security invariant: embedded runtime assets and controlled Docker boundary
- Existing ADR: 0017

## Completion definition

The work is complete when the new lock and local resolver contracts are tested,
the release workflow consumes them, durable documents agree, all required gates
pass, and this temporary packet is removed.
