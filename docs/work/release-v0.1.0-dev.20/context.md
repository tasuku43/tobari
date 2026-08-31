# Work Context: Publish v0.1.0-dev.20

## Current behavior

- The latest existing development tag is `v0.1.0-dev.19`.
- Local, remote, and GitHub Release checks found no existing `v0.1.0-dev.20` binding.
- The release classifier reports `stable=false` for `v0.1.0-dev.20`.
- The current `gh` session has repository and workflow authority; no credential value is recorded here.
- The release workflow requires a successful exact-revision main-push CI run, then separate prepare and publish dispatches.

## Relevant structure

- Entry point: `.github/workflows/release.yml`
- Domain rule: `internal/domain/buildidentity`
- Infrastructure boundary: release workflow and repository-owned artifact tools
- CLI catalog or presentation: `internal/cli`
- Existing tests and harness checks: `scripts/check.sh`, `scripts/lint-release.sh`, and runtime release tasks

## Constraints

- The tag is created only after preparation succeeds.
- A tag push alone never publishes.
- The prerelease has no Homebrew or OCI mutation.
- All monkey inputs use isolated state, fake references, or parser-failing cases and avoid undeclared external effects.

## Unknowns

- [ ] Exact reviewed source revision and main-push CI run ID.
- [ ] Successful preparation and publish run IDs.

## Reproduction or observation

```sh
task check
task security
task public:check
task release:check
TOBARI_INTEGRATION_DOCKER_CONTEXT=<isolated-context> task runtime:release
```

## Final local evidence

- Final monkey pre/post tree fingerprint, before recording this evidence:
  `5d23c1fa6327a015fc35e32464d4e395fb8064daf61f522b843dfdd530e7c62a`.
  The fingerprint includes tracked diffs, status, and untracked file content.
- The final release-surface binaries were rebuilt from that tree with pinned Go
  1.26.6. A catalog-derived black-box matrix exercised all 64 exact commands
  (58 main, four exposure-helper, and two permission-helper commands) from the
  repository and a never-used directory. Human help, scoped agent help,
  unknown flags, 4 KiB positional input, and terminal-control/Unicode-format
  input produced 640/640 passing executions. Every selected failure was one
  bounded schema-2 JSON stderr document with empty stdout; no panic, hang,
  raw escape, `undeclared_fault_contract`, or
  `unclassified_mutation_outcome` occurred. The tree fingerprint was unchanged
  after the matrix.
- Runtime and Policy assistance regressions exercise repository, ancestor, and
  unrelated current directories. Runtime assistance consumes only its Runtime
  reference; Policy assistance consumes only its explicit Context reference;
  neither performs an ambient Workspace or Context selection.
- With pinned Go 1.26.6 and explicit Docker context
  `colima-tobari-release-final-cold`, `task check`, `task security`,
  `task public:check`, and `task release:check` all passed. The release gate
  ended with `lint-release: OK`; site checks built 85 pages and 140 assets, and
  Playwright passed 44/44 tests.
- The dedicated Colima profile was deleted and recreated before the runtime
  gate. Its initial inventory was zero containers, zero images, zero volumes,
  and only Docker's `bridge`, `host`, and `none` networks. From that state,
  `task runtime:release` passed, including policy and Gateway tests plus the
  release-binary cold first-use, re-entry, descendant selection, and
  stopped-ancestor nested-Workspace scenario.
- Manual final-tree review found no dependency manifest, workflow, license, or
  third-party notice change; no new external import or publish destination; and
  no credential, confidential URL, private organization identifier, personal
  path, binary payload, or unreviewed trademark asset in the current diff.
  `repoguard` hygiene/security/public, source and release-surface guards,
  runtime/helper/Gateway/Auth Broker snapshots, module verification, npm audit,
  archive reproduction, SPDX, provenance, and exact inventory checks passed.
  Provider names remain descriptive interoperability vocabulary only.

## Security and public-boundary notes

- Assets and side effects involved: one annotated tag, one GitHub prerelease, and the exact declared release assets.
- Credentials or confidential data involved: GitHub workflow authority only; no token or provider credential is a fixture or artifact.
- New dependencies, destinations, files, processes, or generated content: none beyond repository-owned release artifacts and GitHub Actions.
- Publication and licensing concerns: archive inventory, notices, pinned agent artifacts, and absence of helper/OCI publication require manual review.
- Expected prerelease inventory: five platform CLI archives plus checksums,
  SPDX SBOM, and provenance metadata; no Formula, helper, research, Auth Broker,
  or OCI asset.
- GitHub reports zero open secret-scanning alerts; secret scanning and push
  protection are enabled for the public repository.
- Publishable-history review found ten historical local-path lines across
  `a2f87c2`, `e731a905`, `eb7d9fc`, `b6c5564`, `402144a`, and `b7a6f55`.
  Every commit is already reachable from the published annotated
  `v0.1.0-dev.19` tag at `2f29cd93a0919c597427c58b5b96bdd714dfb9a5`
  and public `main`. The lines contain only this public repository/worktree
  names, local branch and commit evidence, and one Go toolchain path; review
  found no credential, private organization or domain, customer or account,
  or other confidential data. The release owner accepts this as a pre-existing
  non-secret portability defect that `v0.1.0-dev.20` does not widen; the current
  tree, diff, and artifacts must remain path-clean under `repoguard`, and the
  immutable public history is not rewritten.
- Local `refs/codex/*` checkpoints are excluded from publication: `origin`
  exposes no such ref, release pushes remain explicit `main` and tag pushes,
  and mirror pushes are prohibited.

## Glossary

- Preparation: non-publishing workflow run that assembles and verifies the exact asset set.
- Publication: protected workflow run that consumes one successful preparation without rebuilding assets.
