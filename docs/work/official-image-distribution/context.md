# Work Context: Define official Tobari image distribution

This file records verified facts and unresolved questions. It does not turn a
desired publication model into current repository behavior.

## Current behavior

- Before this slice, `docs/06_release.md` stated that container images were
  built locally by `tobari cluster up` in the MVP and that registry publication
  was not promised. The first base-image publication slice now supersedes that
  statement for the main development channel.
- `.github/workflows/release.yml` is triggered by `v*` tags and publishes CLI
  archives and checksums. Its preflight runs full, security, release, public,
  and runtime gates; it has no package publication permission or image push.
- `internal/infra/runtimeassets/assets/versions.env` pins the Debian, OPA, and
  mitmproxy inputs by reviewed version and SHA-256 digest.
- `images/toolbox/versions.env` pins the toolbox's GitHub CLI, AWS CLI,
  kubectl, and TWG versions. `scripts/check-toolbox.sh` checks the Dockerfile's
  source shape and `scripts/build-toolbox.sh` validates the built image's
  runtime API, lifetime capability, user, and entrypoint.
- The canonical base source is now `runtimes/base`; the embedded Dockerfile,
  bootstrap, and AWS verification key are checked byte-for-byte by
  `tools/runtimecheck`. The base declares `io.tobari.runtime-api=1`,
  `io.tobari.runtime-lifetime-command=sleep infinity`, and the common Git,
  HTTP, JSON, Python, SSH, GitHub, and AWS tool set; derived images inherit the
  contract when they use `FROM tobari-runtime:local`.
- The first base version is `0.1.0`. The official GHCR package is
  `ghcr.io/<owner>/tobari/runtime`; its base development tags are `main` and
  `sha-<commit>`, while future agent tags are qualified compositions such as
  `claude.2.1.34-base.0.1.0-r1`.
- `.github/workflows/runtime-base.yml` builds and pushes
  `ghcr.io/<owner>/tobari/runtime:main` plus `sha-<commit>` on a main push,
  after the base source/snapshot check. Agent publication remains deferred to
  a later slice; there is no neutral official toolbox image.

## Relevant structure

- Image foundation: `internal/infra/runtimeassets/assets/tobari/` and the
  embedded runtime asset materializer.
- Local derived image: `images/toolbox/` plus `scripts/build-toolbox.sh` and
  `scripts/check-toolbox.sh`.
- Runtime compatibility boundary: `internal/infra/dockerruntime/runtime.go`
  and the domain image-contract constants in
  `internal/domain/tobari/tobari.go`.
- Existing release composition: `.github/workflows/release.yml`,
  `scripts/package-release.sh`, `scripts/release-archive-entries.sh`, and
  `tools/releaseversion`.
- Existing repository gates: `scripts/check.sh`, `task check`,
  `task security`, `task public:check`, and `task runtime:test`.
- Implemented first source tree: a root `runtimes/` directory containing
  `base`, its JSON metadata/lock, and a family manifest. `claude` and `codex`
  remain planned derived siblings. The future dependency graph must rebuild
  every derived image when `base` or shared build logic changes.

## Constraints

- Official images are untrusted environment/tool contents. Tobari must retain
  ownership of UID/GID, mounts, network, capabilities, resources, health, and
  lifetime regardless of image provenance.
- Runtime API compatibility and image version are different concepts. A
  compatible image can be released independently as long as it preserves the
  declared API and lifetime capability.
- Published artifacts must be reproducible from a reviewed source revision and
  must not silently replace an existing release identity.
- Pulls from public GHCR must not become a startup requirement for local
  `tobari`; the local embedded runtime and explicit local image workflow remain
  valid.
- There must be one canonical base Dockerfile. The current Go `embed.FS`
  layout makes a direct root-level embed impossible, so this slice uses a
  generated snapshot/check rather than a second independently edited source.
- Public publication requires source/license review, synthetic fixtures, least
  privilege workflow permissions, and release evidence.

## External facts

- **GitHub Docs, “Publishing Docker images,”**
  https://docs.github.com/en/actions/tutorials/publish-packages/publish-docker-images,
  checked 2026-08-01: the documented GHCR workflow uses a job-scoped
  `packages: write` permission, logs in with `GITHUB_TOKEN`, builds with
  Docker build/push actions, and can generate an image attestation from the
  pushed digest.
- **GitHub Docs, “Using artifact attestations to establish provenance for
  builds,”**
  https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations,
  checked 2026-08-01: container attestations require `id-token: write`,
  `attestations: write`, `packages: write`, and an image name plus digest; the
  same facility supports SBOM attestations.
- **GitHub Docs, “Configuring a package's access control and visibility,”**
  https://docs.github.com/en/packages/learn-github-packages/configuring-a-packages-access-control-and-visibility,
  checked 2026-08-01: a package can be public or private, and making a package
  public is irreversible back to private visibility.
- **GitHub Docs, “Introduction to GitHub Packages,”**
  https://docs.github.com/en/packages/learn-github-packages/introduction-to-github-packages,
  checked 2026-08-01: GHCR supports Docker and OCI images and exposes package
  metadata such as licensing and version history.
- **GitHub Docs, “Dependabot version updates,”**
  https://docs.github.com/en/code-security/concepts/supply-chain-security/dependabot-version-updates,
  checked 2026-08-01: Dependabot version updates are configured per package
  ecosystem and dependency-definition directory; Docker and GitHub Actions are
  supported ecosystems.
- **GitHub Docs, “Supported ecosystems and repositories,”**
  https://docs.github.com/en/code-security/reference/supply-chain-security/supported-ecosystems-and-repositories,
  checked 2026-08-01: Docker dependency updates inspect Docker image
  references and can attach source metadata when published images expose the
  OCI source label and matching source tags.

The design implication for Tobari is an inference from that supported-input
boundary: the current toolbox's arbitrary `versions.env` values and custom
release-asset checksum shell should not be assumed to be maintained by the
Docker Dependabot manager. They need either a standard package manifest or a
Tobari-owned refresh step that changes the version and integrity data as one
reviewed update.

## Unknowns

- [x] Use one nested GHCR family package (`tobari/runtime`) with variant-
      qualified agent tags.
- [x] Use independent image versions from the CLI: base `<version>` and agent
      `<agent>.<agent-version>-base.<base-version>-r<revision>`.
- [ ] Whether `stable`/`latest` aliases are needed; immutable version tags and
      digests are the safer initial contract.
- [ ] Which agent/tool installers and licenses permit redistribution inside a
      public image, especially for Claude and Codex.
- [ ] Which multi-architecture targets and base-image update SLA are supported
      for each package.
- [ ] Whether the first release should include provenance/SBOM attestation as a
      hard gate or publish only after that capability is validated in CI.
- [ ] Whether `runtime.lock.json` replaces `versions.env` immediately or a
      checked compatibility bridge is kept during migration.
- [ ] Which layer owns each neutral tool and which tools are intentionally
      excluded from the initial toolbox to keep its size bounded.
- [ ] Which agent distributions can use standard package manifests and which
      require the Tobari-owned release-asset refresh updater.
- [ ] The supported-version and deprecation window for old runtime and agent
      images, including GHCR untagged-manifest retention.
- [x] `runtimes/base` is canonical and the embedded runtime assets are a checked
      generated snapshot.
- [x] Main pushes publish a moving `main` development tag and an immutable
      commit-addressed base tag; pull requests remain no-push.

## Thesis evidence

- Repeated design decision or point of agent confusion: users want a tool-ready
  image without surrendering the Tobari runtime contract or having an agent's
  `CMD` own Workspace lifetime.
- User outcome or friction observed in the base slice: a common work runtime
  plus derived agent images makes custom-image setup less constraining than
  asking every user to install common CLIs manually.
- Code workaround or exception being considered: add GHCR publication to the
  CLI release workflow, which would couple unrelated release cadences and
  widen package-write permissions.
- Current thesis that resolves it, or proposed thesis revision: image family
  management is a separate release boundary; the base runtime owns the
  compatibility contract and common work tools, while agent variants own only
  their agent tool, dependencies, and composition tag.
- Downstream product, architecture, security, Skill, catalog, and harness
  impact: add public image documentation, release workflow and permission
  checks, image metadata/lock validation, multi-architecture integration, and
  provenance/license evidence. No CLI command or catalog entry is required.

## Reproduction or observation

```sh
sed -n '1,220p' docs/06_release.md
sed -n '1,280p' .github/workflows/release.yml
cat internal/infra/runtimeassets/assets/versions.env
cat images/toolbox/versions.env
```

Observed 2026-08-01: the first base workflow is main-push-driven and image
publication is limited to the base development channel; local image inputs are
still digest-pinned and the CLI `v*` release workflow remains separate.

## Security and public-boundary notes

- Assets and side effects involved: public OCI manifests and layers, package
  metadata, GitHub Actions tokens, build logs, attestations, SBOMs, and cached
  base/tool downloads.
- Credentials or confidential data involved: no runtime credentials should
  enter an image build; publication requires only the least package-write and
  attestation permissions on a protected release job.
- New dependencies, destinations, files, processes, or generated content:
  future BuildKit/buildx workflow, GHCR packages, image manifests, SBOM and
  provenance artifacts, and image-specific lock/metadata files.
- External schema provenance, publication rights, and drift evidence: every
  copied installer, binary, base image, and license must be identified and
  pinned before publication; vendor redistribution terms remain unresolved.
- Output delivery, collection coverage, pagination, timeout, retry,
  idempotency, and cancellation facts: release publication is create-only for
  immutable image identities; retries must detect an existing digest/tag and
  refuse replacement rather than republish a different manifest.
- Publication and licensing concerns: do not make packages public until the
  repository history, image layers, licenses, package visibility, workflow
  permissions, and attestation claims receive explicit review.

## Glossary

- **Foundation image:** the minimal Tobari runtime that carries the API label,
  bootstrap entrypoint, lifetime capability, user, and common OS tools.
- **Derived image:** an image built `FROM` the foundation and adding a reviewed
  agent or tool bundle without replacing the Tobari lifecycle contract.
- **Image release identity:** the immutable image digest plus its package,
  version tag, source revision, and metadata manifest.
- **Image release train:** the cadence and approval path for a package; it need
  not match the CLI's `v*` release train.
