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

- [ ] Compare flat package names with a nested family path.
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
- [ ] Decide foundation/derived image support window, architectures, and update SLA.
- [ ] Review redistribution rights for every agent/tool artifact.
- [ ] Decide attestation/SBOM requirements and workflow permissions.
- [ ] Create or update an ADR for durable image-release trade-offs.
- [ ] Obtain design approval.

## Implement

- [x] Add base image metadata/lock vocabulary, common-tool pins, and validation.
- [x] Add canonical-source synchronization/checks for embedded runtime assets.
- [ ] Add PR-only image build and contract checks.
- [ ] Add scheduled dependency/base refresh workflow.
- [x] Add the protected main-push multi-architecture base publication workflow.
- [ ] Add digest manifest, SBOM/provenance, retention, and rollback checks.
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
- [ ] `task security` passes. Evidence: the gate is currently blocked by four
      pre-existing gosec findings in untouched `internal/cli` files.
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
- [ ] Published digest is verified on every supported architecture. Evidence:
- [ ] Provenance/SBOM and license evidence match each digest. Evidence:
- [ ] Generated diff and repository status are understood. Evidence:

## Hand off

- [ ] Acceptance criteria have evidence.
- [ ] Durable release conclusions are promoted out of the work packet.
- [ ] Temporary diagnostics and sensitive artifacts are removed.
- [ ] Any deferred package or vendor decision has an explicit successor.
- [ ] Handoff summary explains outcome, why, checks, and rollback.
