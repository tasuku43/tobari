# Work Plan: Prepare first public V1 release artifacts

## Chosen approach

Prepare deterministic artifact creation and validation early with synthetic
inputs. Separate local preparation from protected publication so local checks
cannot push, tag, publish, create a Release, or update a tap.

## Artifact flow

1. Finalize canonical Gateway/Auth Broker source snapshots after A-D integrate.
2. Build/test multi-architecture image layouts locally with cache-only output.
3. Package CLI archives, checksums, SBOMs, provenance statements, and a rendered
   formula through create-only local commands.
4. Run synthetic integrity/install audits and all gates.
5. Stop and obtain explicit approval.
6. After approval, create the GitHub Release and stable Homebrew update from
   the exact reviewed CLI matrix. No OCI publication or component lock exists.

## Security and ownership

Workflows use least privileges, immutable action/tool versions, explicit
artifact subjects, and no credential exposure to untrusted build steps.
Checksums establish integrity, SBOMs describe contents, and provenance binds
subjects to the reviewed workflow/source; documentation must not conflate
those properties with signatures or reproducible builds.

## Verification

Run release lint, archive/package tests, SBOM/provenance schema and subject
checks, formula render/audit against synthetic local assets, action lint, source
snapshot equality, generated-site checks, and full repository gates. Candidate
component identities before approval are explicitly not publication authority.

## Rollback

Before external publication, rollback is ordinary source revert. After an
immutable image or Release exists, never overwrite it: correct with a new
version and preserve the audit trail.
