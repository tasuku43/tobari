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

## Early preparation evidence (2026-08-12)

- `tools/releaseartifacts` uses only the Go standard library and the existing
  repository metadata/tag validators. No module, external generator, workflow
  Action, or network fetch was added. The workflow retains the previously
  reviewed immutable checkout/setup-go/upload/download Action revisions and
  removes the Formula pull-request Action.
- The local generator accepts separate stable builder/workflow and concrete
  invocation identities. It creates `checksums.txt`, `sbom.spdx.json`, and
  `provenance.intoto.jsonl` without overwrite, and exact regeneration rejects
  changed subjects, parameters, builder, invocation, symlinks, and extra
  assets. The SPDX 2.3 document is deliberately an archive-package inventory
  with `filesAnalyzed: false`, not a dependency, OCI-layer, or vulnerability
  inventory. The in-toto/SLSA document is unsigned provenance metadata, not an
  attestation or signature.
- The release workflow is now manual dispatch. Preparation and final
  publication are separate jobs; publication requires both `publish: true`
  and the protected `release-publication` environment, validates the existing
  tag/revision binding, reverifies the exact final asset inventory, and refuses
  an existing Release. Stable Formula output is audited and included as an
  asset but the workflow does not create a branch, pull request, or tap update.
- `./scripts/lint-release.sh` passed. Its synthetic dry run sets
  `GOPROXY=off GOSUMDB=off`, independently reproduces five archives and all
  three metadata files, verifies create-only collisions, renders/audits the
  Formula, and accepts only the exact final regular-file inventory.
- The focused release-artifact/archive/version Go tests and pinned `actionlint`
  v1.7.7 passed. `task release:check` stopped before lint at the
  intentional unpublished Gateway image authority. The ambient
  `task public:check` first found the expected local Node/npm mismatch; under
  `mise exec` it reached integration-owned stale compaction fixture/schema/site
  references. Those ledgers and final image pins remain integration work, not
  release-tooling exceptions.
- Gateway/Auth Broker source snapshots, image metadata/SBOM/provenance, real
  digest handoff, generated architecture output, full gates, and every external
  publication operation remain intentionally deferred until auth/policy
  integration is final.
