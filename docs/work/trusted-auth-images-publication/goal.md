# Work Goal: Publish and pin the trusted authentication images

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: docs/00_theses.md, docs/01_product_contract.md, docs/02_architecture.md, docs/03_security_model.md, docs/04_harness.md, docs/05_public_repository.md, docs/06_release.md, docs/07_authentication.md, docs/08_external_api_contracts.md, docs/09_agent_readiness_validation.md
- Review/delete trigger: Delete after both reviewed multi-architecture image digests are pinned and all required gates pass
- Successor: docs/06_release.md
- Owner: Tobari maintainers
- Target: Current main Auth Broker and Gateway source revision
- Related ADRs: docs/decisions/0017-gateway-source-and-image-boundary.md, docs/decisions/0019-shared-locked-auth-broker.md

## Outcome

The current Auth Broker and Gateway implementations are published as public
Linux amd64/arm64 OCI indexes, and routine Tobari startup selects each image by
its reviewed immutable manifest digest rather than a bootstrap marker or stale
Gateway build.

## Why now

The Auth Broker capability is implemented and verified locally but official
startup deliberately fails while `AUTH_BROKER_IMAGE=unpublished`. The same
change modifies Gateway's credential path, so promoting only the broker would
leave routine startup using an older Gateway image.

## Non-goals

- Do not create a CLI release or SemVer tag.
- Do not add mutable image tags to the routine startup path.
- Do not change the image trust boundary, runtime API labels, or architecture support.
- Do not claim provenance, SBOM attestation, or upstream credential revocation.

## Acceptance criteria

- [ ] The implementation revision is committed and pushed to `main` only after the required pre-publication gates pass.
- [ ] Main workflows publish public Auth Broker and Gateway OCI indexes for Linux amd64 and arm64.
- [ ] The workflow-reported immutable digests are independently inspected and pinned in `versions.env`.
- [ ] Bootstrap-only product, release, test, and recovery text is promoted to the published state.
- [ ] Official startup and synthetic integration use the pinned images successfully.
- [ ] `task check`, `task security`, `task public:check`, and `task release:check` pass on the final tree.

## Governing documents

- Thesis: real credentials remain outside Workspaces and trusted runtime images are immutable authorities.
- Product contract section: shared cluster image preflight and Auth Broker bootstrap behavior.
- Architecture or security invariant: post-policy project-bound resolution through one trusted Gateway and one locked Auth Broker.
- Existing ADR: 0017 and 0019.

## Completion definition

Both public indexes are anonymously readable, their exact platform manifests
and image contracts are verified, immutable references are committed, required
profiles pass, and this temporary packet plus the superseded Gateway packet are
removed.
