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
      implemented and verified before policy/shared-resource mutation.
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
      25/25, explicit embedded-snapshot integration `OK`, and
      official-digest integration `OK` on 2026-08-03.
- [x] Multi-architecture publication evidence is recorded from main workflow
      `30754778241`: immutable OCI index digest
      `sha256:9f2b714d9a61dafc451fb015535d5f60265c13a6754d76df21b87800fdc65078`
      with `linux/amd64` and `linux/arm64`, plus matching `latest`/`main`
      tag checks.
- [x] GHCR package visibility is either changed to public and verified with an
      anonymous manifest request, or explicitly handed to the package owner.
      Current evidence: the anonymous request returned `401`; the token refresh
      for package scope timed out, so no visibility mutation was attempted.

## Hand off

- [x] Acceptance criteria have evidence or an explicit external owner action;
      package visibility is not falsely claimed.
- [x] Durable decisions are promoted to ADR 0017 and the architecture,
      security, harness, public, and release documents.
- [x] Temporary diagnostics and sensitive artifacts are removed; the external
      Docker config and raw runtime artifacts remain outside Git.
