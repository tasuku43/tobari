# Work Goal: Distribute the trusted Gateway as an official image

- Status: Accepted
- Retention: evidence
- Retention reason: Preserve the verified trusted Gateway image contract and the external GHCR visibility handoff until the package owner completes or explicitly declines publication.
- Governing contract: docs/00_theses.md, docs/01_product_contract.md, docs/02_architecture.md, docs/03_security_model.md, docs/04_harness.md, docs/05_public_repository.md, docs/06_release.md
- Review/delete trigger: Delete after the GHCR visibility handoff is completed or explicitly deferred and the Gateway release decision remains promoted in the ADR and release contract.
- Successor: `docs/06_release.md` public OCI review; a new packet is required for any changed publication policy.
- Owner: Tobari maintainers
- Target: Trusted Gateway source layout, verified image, and release boundary
- Execution state: Source/image implementation and supported-runtime E2E are
  complete. Anonymous GHCR visibility could not be verified because the
  current GitHub token lacks package scopes and the device-login refresh timed
  out; this is an explicit external owner action, not a product claim.
- Related ADRs: docs/decisions/0017-gateway-source-and-image-boundary.md

## Outcome

Tobari can obtain a verified, architecture-compatible Gateway image without
building the trusted proxy implementation on every user's host. The image is
published through a dedicated GHCR lifecycle, selected by an immutable digest,
and checked against the Gateway/CLI compatibility contract before cluster
mutation. Gateway source, Python tests, Docker packaging, and the Go CLI's
embedded asset boundary have one clear monorepo ownership model.

## Why now

Gateway is currently managed as Python addon files embedded under the Go
`runtimeassets` package. `cluster up` materializes those files and runs
`docker compose up --build`, producing a host-UID/GID-specific image. This keeps
the CLI and Gateway source aligned but makes first startup depend on a Docker
build and leaves the image distribution/release boundary implicit.

## Non-goals

- Do not make Gateway user-customizable through a Context runtime recipe.
- Do not move policy authoring, credential files, or project principals into the
  Gateway image.
- Do not introduce a second network enforcement boundary or provider-specific
  API adapter.
- Do not silently use a mutable Gateway `latest` tag for routine cluster startup.
- Do not split the Gateway into a separate repository unless monorepo ownership
  is proven insufficient.

## Acceptance criteria

- [x] The canonical Gateway source, embedded snapshot/materialization boundary,
      Dockerfile, tests, and release metadata have one documented source of truth.
- [x] A multi-architecture official Gateway image is built from a reviewed
      source revision and published with an immutable digest and least
      privilege workflow permissions. Provenance/SBOM is included only if the
      release contract explicitly adopts those claims.
- [x] The CLI selects an immutable compatible Gateway image and fails before
      cluster mutation when it is missing, incompatible, or for the wrong
      architecture.
- [x] The non-root host UID/GID and owner-only credential/CA access contract is
      preserved or deliberately replaced with an equivalent verified design;
      this includes the private/public named-CA-volume distinction.
- [x] Source development remains possible without publishing an image for every
      local edit; the fallback is explicit rather than silently changing the
      trust path.
- [x] Gateway unit, image, integration, security, public-boundary, and release
      gates pass.

## Completion definition

The source ownership, image contract, UID/GID design, release workflow,
rollback, and user-facing startup behavior are documented and tested. Durable
decisions are promoted to an ADR and governing documents, required gates pass,
and the remaining package-visibility action is recorded with an exact owner
action and resumption condition rather than being inferred from CI output.
