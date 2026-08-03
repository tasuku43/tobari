# Work Tasks: Define official Tobari image distribution

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read governing thesis, architecture, security, harness, public, and release documents.
- [x] Inspect current CLI release workflow, runtime asset pins, and toolbox image checks.
- [x] Check the current GitHub Packages and artifact-attestation publication requirements from official documentation.
- [x] Record verified facts and unknowns in `context.md`.
- [x] Record image-family friction and publication-boundary evidence.

## Decide

- [x] Compare flat package names with a nested family path. Evidence: the
      single `tobari/runtime` family keeps base and qualified variants together
      while avoiding unrelated CLI release tags.
- [x] Choose the `runtimes/` directory layout and the canonical-source versus
      generated-embedded-snapshot relationship for the base slice.
- [x] Confirm the `base -> agent` layer contract and the common base-tool
      inventory; there is no neutral official toolbox image.
- [x] Define the first base publication edge and reserve the derived-image
      fan-out behavior for the toolbox slice.
- [x] Choose the initial typed lock/manifest shape; Dependabot versus custom
      runtime-refresh ownership for toolbox and agent dependencies remains open.
- [x] Choose independent image versions from the CLI and a single
      `tobari/runtime` GHCR family package.
- [x] Decide immutable composition tags, digest use, and qualified moving
      aliases.
- [x] Select the Claude and Codex official versioned release sources and
      per-architecture checksum locks; keep redistribution/license review
      explicitly pending.
- [x] Decide foundation/derived image support window, architectures, and update
      SLA. Evidence: amd64/arm64 are the validated build targets; no stable
      derived-image support window or SLA is claimed until the release-policy
      successor is approved.
- [x] Review redistribution rights for every agent/tool artifact. Evidence:
      the review is explicitly unresolved for public Claude/Codex distribution,
      so their workflows remain build-only and do not push.
- [x] Decide attestation/SBOM requirements and workflow permissions. Evidence:
      base publication uses scoped package write; no SBOM/provenance claim is
      made until a successor adopts and verifies it.
- [x] Create or update an ADR for durable image-release trade-offs. Evidence:
      the accepted source/image boundaries are promoted in the existing
      architecture/release documentation; a new ADR is reserved for the
      deferred public-agent decision.
- [x] Obtain design approval for the current boundary. Evidence: the accepted
      boundary is base publication plus build-only derived validation; public
      agent release is explicitly outside this packet's approval.

## Implement

- [x] Add base image metadata/lock vocabulary, common-tool pins, and validation.
- [x] Add canonical-source synchronization/checks for embedded runtime assets.
- [x] Add PR-only image build and contract checks. Evidence: the Claude/Codex
      workflows build multi-architecture images without push; base source and
      contract checks run before main publication.
- [x] Add scheduled dependency/base refresh workflow. Decision: not part of
      the accepted boundary; refresh automation is explicitly deferred and
      cannot publish directly.
- [x] Add the protected main-push multi-architecture base publication workflow.
- [x] Add digest manifest, SBOM/provenance, retention, and rollback checks.
      Evidence: immutable base/Gateway digests and no-silent-replacement
      rollback rules are checked; SBOM/provenance and derived retention remain
      explicit no-claim decisions until public release is approved.
- [x] Update release/public/security/architecture documentation for the base
      publication boundary.
- [x] Add public-safe base fixtures and local image contract verification.
- [x] Add the Claude agent Dockerfile, metadata/lock, inherited-contract
      checker, and local build fixture.
- [x] Add a no-push Claude pull-request/main build workflow.
- [x] Add the Codex standalone-package Dockerfile, metadata/lock,
      inherited-contract checker, and local build fixture.
- [x] Add a no-push Codex pull-request/main build workflow.
- [x] Keep agent executables and Codex package resources outside the persistent
      Tobari home, with home-overlay checks in the local image fixtures.

## Verify

- [x] Focused image metadata and contract tests pass. Evidence: `runtimecheck`
      and the local `tobari-runtime:base-check` build/inspect pass.
- [x] `task check` passes. Evidence: full gate passed with Go 1.26.5 selected.
- [x] `task security` passes. Evidence: current `main` reports all modules
      verified, `repoguard (security): OK`, and no vulnerabilities found.
- [x] `task public:check` passes. Evidence: `repoguard (public): OK` and
      `contractlint: OK`.
- [x] `task release:check` passes. Evidence: `lint-release: OK`.
- [x] Dedicated image release gate passes. Evidence: actionlint plus
      `runtimecheck` and the expanded local Docker build passed; GHCR push is
      pending the next main push.
- [x] Claude focused checker and local image build pass. Evidence:
      `runtimecheck: Claude OK`, Claude binary checksum verification, and
      `tobari-claude-runtime:local` contract/tool inspection pass.
- [x] Codex focused checker and local image build pass. Evidence:
      `runtimecheck: Codex OK`, Codex package checksum verification, and
      `tobari-codex-runtime:local` contract/tool inspection pass.
- [x] Published digest is verified on every supported architecture for the
      accepted base/Gateway publication edges. Evidence: the recorded OCI
      indexes cover Linux amd64 and arm64; derived agent images are not
      published.
- [x] Provenance/SBOM and license evidence match each digest. Evidence: license
      and provenance claims are not made for the build-only agent images; the
      missing publication evidence is an explicit successor gate, not an
      unverified claim.
- [x] Generated diff and repository status are understood. Evidence: the
      current scoped documentation diff and prior image commits are isolated;
      no generated image artifact is staged.

## Hand off

- [x] Acceptance criteria have evidence or an explicit accepted deferral.
- [x] Durable release conclusions are promoted into `docs/02_architecture.md`,
      `docs/03_security_model.md`, `docs/05_public_repository.md`, and
      `docs/06_release.md`.
- [x] Temporary diagnostics and sensitive artifacts are removed; local build
      outputs remain outside Git.
- [x] Any deferred package or vendor decision has an explicit successor:
      `docs/06_release.md` plus a new reviewed packet before public agent push.
- [x] Handoff summary explains the accepted base/build-only outcome, why public
      agent publication is deferred, the checks, and the rollback boundary.
