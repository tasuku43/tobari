# Work Context: Publish and pin Gateway API 3

## Current behavior

- `internal/infra/dockerruntime/gateway_image.go` requires Gateway API label `3`.
- `internal/infra/runtimeassets/assets/versions.env` pins manifest `sha256:9b4dbfaf587f22a1a036dec85df8637cc323d4377142b0463781b25e3ef15049`.
- Docker inspection on 2026-08-09 showed that pinned manifest declares Gateway API `2`.
- The local `tobari-gateway:dev` image declares API `3`; `bin/tobari-dev cluster up` completed successfully.

## Relevant structure

- Canonical image source: `gateway/`
- Embedded snapshot: `internal/infra/runtimeassets/assets/gateway/`
- Production pin: `internal/infra/runtimeassets/assets/versions.env`
- Publication workflow: `.github/workflows/gateway-image.yml`
- Image preflight: `internal/infra/dockerruntime/gateway_image.go`

## Constraints

- Only a push to `main` may publish Gateway images to GHCR.
- Pull requests perform cache-only multi-architecture builds.
- Routine startup must use a reviewed immutable manifest digest.
- The image must support Linux amd64 and arm64, API role `enforcement`, non-root `1000:1000`, and `/opt/tobari/entrypoint.sh`.
- Public source, image layers, workflow output, and evidence must be secret-free and license-complete.

## External facts

- GitHub/GHCR state will be read through authenticated `gh` and anonymous Docker registry inspection after publication.

## Unknowns

- [ ] The immutable digest produced by the main publication workflow.
- [ ] Whether all required checks pass for the current complete GraphQL enforcement change.

## Thesis evidence

- The fail-closed API label rejected a real CLI/image skew before shared-resource mutation.
- The explicit `tobari_dev` resolver provided the intended source-validation path.

## Reproduction or observation

```sh
tobari cluster up
# gateway_image_incompatible because the published pin is API 2

bin/tobari-dev cluster up
# Cluster ready with the local API-3 Gateway image
```

## Security and public-boundary notes

- Publication uses the repository-owned GitHub Actions token and never a local registry credential.
- The new GraphQL dependency is hash-pinned and carries an MIT notice in the image source.
- No live credential, handle, request body, account data, or authenticated transcript is publication evidence.

## Glossary

- **Gateway API 3:** The image contract that adds declared GraphQL operation/root-field enforcement.
- **Manifest digest:** The immutable OCI index digest covering the reviewed amd64 and arm64 image members.
