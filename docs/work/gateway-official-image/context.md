# Work Context: Distribute the trusted Gateway as an official image

## Current behavior

- Gateway's canonical source is now under `gateway/`: `Dockerfile`,
  `entrypoint.sh`, Python addons, config example, and Python unit tests.
- `internal/infra/runtimeassets/assets/gateway/` is the checked snapshot used
  by the Go embed boundary. `scripts/check-gateway-source.sh` compares the
  trees and `scripts/sync-gateway-source.sh` updates the snapshot.
- `internal/infra/runtimeassets` embeds the complete `assets/` tree into the Go
  binary. The CLI materializes it into versioned state before Docker operations.
- `compose.yaml` consumes `TOBARI_GATEWAY_IMAGE` and explicitly uses
  `--no-build`; the normal path does not build Gateway on the host.
- `cluster up` preflights the immutable Gateway digest before policy tests or
  cluster mutation. `cluster up --gateway-source` explicitly builds the
  materialized snapshot as a development/recovery path.
- The Gateway Dockerfile no longer receives host UID/GID build arguments. Image
  code is root-owned/read-only, the default image user is numeric non-root, and
  Compose supplies the invoking host UID/GID at runtime. The CA initialization
  directories are writable by the selected service identity.
- Gateway CA state is held in two Docker named volumes, `tobari-gateway-ca`
  (private CA, Gateway-only) and `tobari-public-ca` (public certificate,
  read-only to the runtime). Normal cluster down retains them; purge removes
  them.
- Gateway tests run inside the pinned mitmproxy image through
  `scripts/check.sh gateway`; they are not a Go package test.
- `.github/workflows/gateway-image.yml` validates pull requests with a
  cache-only multi-architecture build and publishes main-push `latest`, `main`,
  and `sha-<commit>` tags. The CLI records the reviewed immutable Gateway
  digest in `versions.env`:
  `ghcr.io/tasuku43/tobari/gateway@sha256:9f2b714d9a61dafc451fb015535d5f60265c13a6754d76df21b87800fdc65078`.
  Main workflow `30754778241` published revision
  `34e7caa93359f9fa5cbbbe71dd3504885c44dd29` as an OCI index for
  `linux/amd64` and `linux/arm64`; the local digest inspection succeeded
  without credentials before the package visibility check.

## Relevant structure

- Go composition root: `internal/infra/runtimeassets/assets.go`
- Embedded Gateway source: `internal/infra/runtimeassets/assets/gateway/`
- Compose and shared service contract:
  `internal/infra/runtimeassets/assets/compose.yaml`
- Docker orchestration: `internal/infra/dockerruntime/runtime.go`
- Gateway adapter tests: `internal/infra/runtimeassets/assets/gateway/test_tobari_gateway.py`
- Gateway gate: `scripts/check.sh` and `task gateway:test`
- Existing official runtime image workflow: `.github/workflows/runtime-base.yml`

## Constraints

- Gateway is the trusted HTTP/HTTPS enforcement point. It must remain separate
  from the untrusted Context runtime image.
- The image must not contain policy data, project roots, credential values, or
  project-principal state; these remain host-owned mounts.
- Gateway has no host-published port, no Docker socket, no added capability, and
  runs non-root with fixed CPU, memory, PID, and log bounds.
- The image reference used by routine startup must be immutable. Moving tags may
  exist only for development publication, not as the enforcement identity.
- The supported CLI still needs a source-development path and must preserve the
  embedded-asset contract during the transition.
- Native Linux and the supported macOS Colima engine must preserve access to
  owner-only host credential files and CA state. Docker Desktop-specific
  behavior is not currently part of the release contract.

## Unknowns

- [x] The host-UID-independent official image preserves the private/public CA
      volume contract on fresh and reused volumes in the supported Colima
      integration: the first official-image startup creates both named volumes,
      a repeated `cluster up` reuses them, and the HTTPS proxy path succeeds.
- [x] The Gateway source moves to a top-level `gateway/` directory while
      retaining one checked snapshot for embedding; Compose and OPA remain
      CLI-owned orchestration inputs.
- [x] Which Gateway API/source revision labels and digest checks must the CLI
      enforce before compose startup? The CLI verifies the immutable digest,
      `io.tobari.gateway-api=1`, `io.tobari.gateway-role=enforcement`, the
      non-root user, entrypoint, and Docker Engine platform.
- [x] Source builds are selected explicitly with `cluster up --gateway-source`;
      rollback restores the reviewed digest in `versions.env`.
- [x] The first GHCR package was published successfully, but its visibility is
      still pending the package-owner visibility change. On 2026-08-04 an
      anonymous manifest request returned `401`, and the active GitHub token
      exposed no `packages` scope. `gh auth refresh -h github.com
      -s read:packages -s write:packages` was started with device code
      `49F4-FCAF` but ended with `context deadline exceeded` without changing
      credentials. Provenance and SBOM are not current release claims and
      require an explicit release-contract decision first.

## Thesis evidence

- Repeated decision: the safe path should not require users to be Docker build
  operators just to start the enforcement boundary.
- User outcome: cluster startup should obtain a known Gateway artifact while the
  agent remains free inside its separate runtime boundary.
- Current workaround: every CLI embeds Gateway source and asks Docker Compose to
  build it, coupling source freshness to host build capability.
- Thesis implication: Gateway distribution may be centralized, but authority,
  policy, credentials, and project identity remain host-controlled.

## Reproduction or observation

```sh
sed -n '48,96p' internal/infra/runtimeassets/assets/compose.yaml
sed -n '1,45p' internal/infra/runtimeassets/assets/gateway/Dockerfile
sed -n '447,460p' internal/infra/dockerruntime/runtime.go
task gateway:test
```

Official-image evidence on 2026-08-03:

```sh
DOCKER_CONFIG=/private/tmp/tobari-docker-config.zfoQ3U \
DOCKER_CONTEXT=colima task runtime:test
```

The runtime profile passed OPA 27/27, Gateway 25/25, and the complete
integration scenario with the pinned official Gateway digest. The temporary
Docker config was required only because the local default config denied
BuildKit activity-file writes; it is not a product dependency.

## Current external handoff

- Main workflow `30754778241` produced the immutable multi-architecture OCI
  index and the CLI preflight/integration checks pass.
- Anonymous verification of the pinned Gateway manifest is not complete
  because the package is currently not readable without authentication.
- Owner action to resume: authenticate a GitHub account with package read/write
  scope, verify that public visibility is intentionally approved, make the
  one-way visibility change if approved, then rerun the anonymous manifest
  check and record the result here. Do not treat `latest`, workflow success, or
  an authenticated pull as proof of public visibility.
- This packet is therefore `Accepted` with an explicit external handoff; it is
  not marked `Complete` and does not claim a public trusted image.

## Security and public-boundary notes

- The image is part of the trusted enforcement boundary; registry provenance
  and digest identity must be explicit rather than inferred from a tag.
- No secret-bearing file may enter the build context or image layer.
- Public package publication requires license/source review, least-privilege
  workflow permissions, synthetic fixtures, and reproducible multi-architecture
  evidence.
- External image references, tags, digests, and attestations are release data,
  not user-provided routine inputs.

## Glossary

- **Gateway core image:** the reviewed multi-architecture image containing
  mitmproxy and the Tobari addon code.
- **UID adaptation layer:** an optional small local image layer that assigns the
  invoking host UID/GID without rebuilding Gateway source.
- **Embedded snapshot:** the CLI-bundled copy used for source development or a
  deliberate fallback, checked against the canonical Gateway source.
