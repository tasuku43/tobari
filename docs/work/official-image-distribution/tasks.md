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
- [x] Confirm the provisional `base -> toolbox -> agent` layer contract and
      the initial neutral-tool inventory; toolbox implementation remains next.
- [x] Define the first base publication edge and reserve the derived-image
      fan-out behavior for the toolbox slice.
- [x] Choose the initial typed lock/manifest shape; Dependabot versus custom
      runtime-refresh ownership for toolbox and agent dependencies remains open.
- [ ] Choose independent image SemVer versus CLI lockstep.
- [ ] Decide immutable version tags, digest use, and any moving aliases.
- [ ] Decide foundation/derived image support window, architectures, and update SLA.
- [ ] Review redistribution rights for every agent/tool artifact.
- [ ] Decide attestation/SBOM requirements and workflow permissions.
- [ ] Create or update an ADR for durable image-release trade-offs.
- [ ] Obtain design approval.

## Implement

- [x] Add base image metadata/lock vocabulary and validation.
- [x] Add canonical-source synchronization/checks for embedded runtime assets.
- [ ] Add PR-only image build and contract checks.
- [ ] Add scheduled dependency/base refresh workflow.
- [x] Add the protected main-push multi-architecture base publication workflow.
- [ ] Add digest manifest, SBOM/provenance, retention, and rollback checks.
- [x] Update release/public/security/architecture documentation for the base
      publication boundary.
- [x] Add public-safe base fixtures and local image contract verification.

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
      `runtimecheck` and local Docker build passed; GHCR push is pending main.
- [ ] Published digest is verified on every supported architecture. Evidence:
- [ ] Provenance/SBOM and license evidence match each digest. Evidence:
- [ ] Generated diff and repository status are understood. Evidence:

## Hand off

- [ ] Acceptance criteria have evidence.
- [ ] Durable release conclusions are promoted out of the work packet.
- [ ] Temporary diagnostics and sensitive artifacts are removed.
- [ ] Any deferred package or vendor decision has an explicit successor.
- [ ] Handoff summary explains outcome, why, checks, and rollback.
