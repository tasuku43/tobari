# Work Context: Prepare first public V1 release artifacts

## Verified current facts

- `.github/workflows/release.yml` is now manual and separates create-only
  preparation from the `release-publication` approval environment. It builds
  five CLI archives plus exact checksum/SPDX/provenance/Formula subjects. A
  stable protected publication propagates the exact released Formula through
  a pull request to `tasuku43/homebrew-tap`.
- Component workflows accept one exact revision and validate both supported
  Linux architectures with cache-only output. No image workflow can publish.
- `versions.env` contains reviewed source inputs only; release packaging has no
  owned image identity or component lock.
- `scripts/package-release.sh`, `tools/archivepack`, formula rendering/audit,
  and release lint already provide a deterministic base.
- Repository-owned component SBOM/provenance evidence and CLI archive-level
  SPDX/unsigned in-toto-SLSA metadata are implemented and linted. They are not
  described as signatures, dependency SBOMs for archive contents, or
  reproducibility proofs.
- The generated architecture site is pinned to integrated committed source
  `ab158bde6f0d6ba9e5b3c99aebd5e4ed07b510c6`; catalog and component metadata
  were regenerated and verified from that commit, including exact denied
  scheme fields and the retained brokered-authentication wording.

## Constraints

- Official embedded image source/metadata is finalized only after auth/policy
  integration. Tobari publishes no owned image.
- Pushes can trigger Pages; no release push is allowed before the approval checkpoint.
- Release tooling and dependencies require license, integrity, permission, and
  public-boundary review.
- Secrets, private URLs, personal identifiers, and live provider responses
  cannot enter artifacts or provenance.

## Existing local evidence

- `./scripts/lint-release.sh` passes at the baseline revision.
- `task public:check` and `task release:check` reject component locks and every
  owned-image publication path.
- Architecture-site generation and `generate:check` pass against the final
  integrated committed source snapshot.

## Early preparation evidence (2026-08-12)

- `tools/releaseartifacts` uses only the Go standard library and the existing
  repository metadata/tag validators. No module or external generator was
  added. The workflow retains immutable checkout/setup-go/upload/download
  Action revisions and uses the reviewed GitHub App token Action for the
  Formula pull-request boundary.
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
  asset; only after Release creation does the protected stable path open a
  Formula-only tap pull request with a repository-scoped GitHub App token.
- `./scripts/lint-release.sh` passed. Its synthetic dry run sets
  `GOPROXY=off GOSUMDB=off`, independently reproduces five archives and all
  three metadata files, verifies create-only collisions, renders/audits the
  Formula, and accepts only the exact final regular-file inventory.
- The focused release-artifact/archive/version Go tests and pinned `actionlint`
  v1.7.7 passed. After integration, `task public:check` passes repoguard and
  contractlint; component authority is generated only in release assembly. No
  stale catalog/schema/site blocker remains.
- Canonical Gateway/Auth Broker source snapshots and generated architecture
  output are finalized. The live synthetic integration also passes after
  exercising explicit review/allow/retry, preset guardrails, source snapshot
  immutability, and retained static Broker lifecycle. Real component
  metadata/SBOM/provenance tied to an immutable published digest, final public
  and release gate completion, and every external publication operation remain
  intentionally deferred until approval; that evidence cannot exist before
  the manual image publication step.
