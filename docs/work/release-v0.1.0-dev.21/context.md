# Work Context: Create the v0.1.0-dev.21 development tag

## Verified facts

- The latest existing development tag is `v0.1.0-dev.20`.
- No local tag, remote tag, or GitHub Release named `v0.1.0-dev.21` exists.
- Existing tags are annotated and use the message `Tobari <tag>`.
- The working tree contains the reviewed current-Context implementation and documentation.
- The current branch is `main`; its pre-release HEAD is one commit ahead of `origin/main`.
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
- Final diff review finds no dependency manifest, workflow, license, notice,
  binary, credential, private identifier, or local absolute path change.
