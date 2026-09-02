# Work Context: Create the v0.1.0-dev.21 development tag

## Verified facts

- The latest existing development tag is `v0.1.0-dev.20`.
- No local tag, remote tag, or GitHub Release named `v0.1.0-dev.21` exists.
- Existing tags are annotated and use the message `Tobari <tag>`.
- Main contains the reviewed current-Context implementation and documentation
  at `37b93f0089354e1c9b9bb853cc22722a67aa8850`; the final repair is pending a
  follow-up commit.
- Release preparation requires a successful exact-revision main-push CI run and must precede tag creation.

## Constraints

- The requested scope ends at the annotated tag; the GitHub Release publish operation is excluded.
- The tag must not point at the pre-change HEAD.
- No tag or branch history is overwritten.
- Release checks use the repository-pinned Go toolchain and an explicit non-default cold Docker context.

## Security and public-boundary notes

- External mutations are the reviewed main push, preparation artifacts, and one annotated tag push.
- No credential value, private identifier, binary artifact, or authenticated output enters the repository.
- Preparation is non-publishing and the tag push does not trigger a release.

## Local release evidence

- With repository-pinned Go 1.26.6, `task check`, `task security`,
  `task public:check`, and `task release:check` pass on the release tree.
- Public documentation builds 85 pages and 140 static assets; public and
  release-surface guards pass with no npm or Go vulnerability finding.
- A fresh non-default `colima-tobari-dev21-release-cold` profile began with no
  containers, images, or volumes. `task runtime:release` passed there,
  including policy 89/89, Gateway 140 tests, and cold release-surface first
  use. The dedicated profile was deleted afterward and the ambient Docker
  context was restored to `colima`.
- `release:check` found one ShellCheck SC1007 regression in the changed
  integration fixture; explicit empty-string initialization fixed it before
  the final gates.
- Exact-revision main CI run `33577765071` succeeded in release packaging,
  container policy/Gateway, public, security, and implementation-quality jobs,
  but its cold first-use job rejected stopped-ancestor nested creation with
  `context_in_use`. The attached current Context had been reused even though it
  already owned the parent Workspace.
- Root creation now binds the current Context only while it is unattached. If
  it is already attached, explicit create-here publishes a distinct Context
  from the default Template and leaves the installation selector unchanged.
- The repaired cleanup exposed a second selector edge: deleting an otherwise
  unreferenced current Context was rejected even though the public command
  contract listed only Workspace, attachment, credential, and Stage blockers.
  Context deletion now atomically removes that Context and clears the selector
  in one authority generation.
- A fresh non-default `colima-tobari-dev21-nested-fix-cold` profile passed the
  complete first-use scenario, including cold entry, re-entry, descendant
  selection, stopped-ancestor nested creation, and cleanup. The dedicated
  profile was deleted afterward and the ambient Docker context remained
  `colima`.
- The ambient `colima` daemon later returned a containerd metadata
  `input/output error` while `task check` pulled the pinned OPA image. Without
  restarting or deleting that unrelated daemon, a fresh isolated
  `colima-tobari-dev21-final-gates` profile passed `task check`, `task
  security`, `task public:check`, and `task release:check`. That dedicated
  profile was deleted and the ambient Docker context was restored to `colima`.
- Final diff review finds no dependency manifest, workflow, license, notice,
  binary, credential, private identifier, or local absolute path change.
