# Work Context: Prepare first public V1 release artifacts

## Verified current facts

- `.github/workflows/release.yml` runs on `v*`, builds five CLI archives,
  creates checksums and a GitHub Release, then opens a formula PR; it has no
  pre-publication approval checkpoint.
- Component workflows publish Gateway/Auth Broker multi-architecture images,
  but do not produce a repository-owned digest handoff artifact.
- `versions.env` selects both images as `unpublished`; public and release gates
  fail intentionally at that contract.
- `scripts/package-release.sh`, `tools/archivepack`, formula rendering/audit,
  and release lint already provide a deterministic base.
- Repository-owned component SBOM and provenance/attestation generation is not
  implemented. Current release contract explicitly avoids those claims and
  must be revised before adding them.
- The generated architecture site is pinned to an older source snapshot even
  though its current `generate:check` passes against that declared snapshot.

## Constraints

- Official image source/digest/metadata is finalized only after auth/policy
  integration. Never invent or pin a digest that has not been published and
  inspected.
- Pushes can trigger Pages or component publication; no push is allowed before
  the approval checkpoint.
- Release tooling and dependencies require license, integrity, permission, and
  public-boundary review.
- Secrets, private URLs, personal identifiers, and live provider responses
  cannot enter artifacts or provenance.

## Existing local evidence

- `./scripts/lint-release.sh` passes at the baseline revision.
- `task public:check` and `task release:check` stop at unpublished component
  selection; checks are not weakened.
- Architecture-site generation currently passes but needs a final integrated
  source snapshot and regenerated output.
