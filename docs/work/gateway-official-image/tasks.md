# Work Tasks: Distribute the trusted Gateway as an official image

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read governing theses, product, architecture, security, harness, release,
      and public-repository sections.
- [x] Inspect current Gateway source, embedding, Compose, Dockerfile, tests, and
      image inputs.
- [x] Record verified facts and unknowns in `context.md`.
- [x] Confirm the user outcome and non-goals in `goal.md`.

## Decide

- [x] Compare host build, direct official image, and thin UID adaptation.
- [x] Decide the canonical monorepo Gateway source location.
- [x] Decide the UID/GID and CA-volume permission contract for the image
      transition; direct startup still requires fresh/reused volume evidence.
- [x] Decide the image API labels, moving/immutable tag shape, package-write
      split, and current no-attestation release claim. CLI digest preflight is
      an explicit follow-up.
- [x] Create an ADR for the durable Gateway image/source decision.

## Implement

- [x] Establish canonical Gateway source and checked embedding/snapshot flow.
- [x] Add the official multi-architecture image workflow and metadata.
- [x] Add preflight compatibility and architecture validation. Evidence:
      immutable digest, API/role labels, non-root user, entrypoint, and
      Docker Engine platform are checked before policy or cluster mutation.
- [x] Preserve or replace UID/GID and non-root security behavior with tests.
- [x] Keep the embedded source-development path and document rollback.

## Verify

- [x] Focused Gateway source/image tests pass. Evidence: canonical-source
      Python tests 25/25, snapshot check, local image build, stable labels,
      default non-root user, and volume-directory mode inspection passed.
- [x] `task check` passes. Evidence: full gate passed on 2026-08-02.
- [x] `task security` passes. Evidence: isolated-cache security gate reported
      no vulnerabilities on 2026-08-02.
- [x] `task public:check` passes. Evidence: public guard and contract lint
      passed on 2026-08-02.
- [x] `task release:check` passes. Evidence: release lint and actionlint
      passed on 2026-08-02.
- [x] Runtime/Gateway integration evidence is recorded: OPA 27/27, Gateway
      25/25, and explicit `--gateway-source` embedded-snapshot integration
      `OK` on 2026-08-03.
- [ ] Multi-architecture publication evidence is pending the main-push GHCR
      workflow; the workflow is present and cache-only PR behavior is defined.

## Hand off

- [ ] Acceptance criteria have evidence.
- [ ] Durable decisions are promoted.
- [ ] Temporary diagnostics and packet are removed after completion.
