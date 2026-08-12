# Work Plan: Prepare first public V1 release artifacts

## Chosen approach

Prepare deterministic artifact creation and validation early with synthetic
inputs, but defer authoritative image metadata until integrated sources stop
changing. Separate preparation from publication so local checks cannot push,
tag, publish, create a Release, or update a tap implicitly.

## Artifact flow

1. Finalize canonical Gateway/Auth Broker source snapshots after A-D integrate.
2. Build/test multi-architecture image layouts locally and define the CI digest
   handoff plus SBOM/provenance outputs.
3. Package CLI archives, checksums, SBOMs, provenance statements, and a rendered
   formula through create-only local commands.
4. Run synthetic integrity/install audits and all gates that do not require
   published image retrieval.
5. Stop and obtain explicit approval.
6. After approval, publish images, inspect indexes, pin real digests in a new
   reviewed commit, run all gates, then tag/push and create the GitHub Release
   and Homebrew update synchronously.

## Security and ownership

Workflows use least privileges, immutable action/tool versions, explicit
artifact subjects, and no credential exposure to untrusted build steps.
Checksums establish integrity, SBOMs describe contents, and provenance binds
subjects to the reviewed workflow/source; documentation must not conflate
those properties with signatures or reproducible builds.

## Verification

Run release lint, archive/package tests, SBOM/provenance schema and subject
checks, formula render/audit against synthetic local assets, action lint, source
snapshot equality, generated-site checks, and full repository gates. Any
unpublished-image failure before approval is an explicit remaining publication
dependency, not an allowed green result.

## Rollback

Before external publication, rollback is ordinary source revert. After an
immutable image or Release exists, never overwrite it: correct with a new
version and preserve the audit trail.
