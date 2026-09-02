# Work Goal: Create the v0.1.0-dev.21 development tag

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/05_public_repository.md` and `docs/06_release.md`
- Review/delete trigger: Delete after the prepared annotated tag is pushed and verified
- Successor: None
- Owner: Release owner
- Target: `v0.1.0-dev.21`
- Related ADRs: ADR 0093

## Outcome

The remote repository contains one annotated `v0.1.0-dev.21` tag bound to the
exact reviewed main revision that makes current Context selection independent
of CWD. The tag is created only after successful exact-revision CI and the
non-publishing Release preparation.

## Why now

The user requested the next development tag after completing and verifying the
current-Context and Workspace/CWD ownership change.

## Non-goals

- Publishing a GitHub Release.
- Stable or Homebrew publication.
- Publishing an OCI image or helper binary.

## Acceptance criteria

- [ ] The release revision contains the reviewed current-Context change and no unexplained local diff.
- [ ] Local full, security, public, release, and isolated cold runtime gates pass.
- [ ] The exact main-push CI run succeeds.
- [ ] Release preparation succeeds before the tag exists.
- [ ] One annotated `v0.1.0-dev.21` tag identifies the prepared revision locally and remotely.

## Completion definition

The remote tag binding is verified, no GitHub Release is published, and this
temporary packet is removed in the post-tag cleanup commit.
