# Work Plan: Distribute the trusted Gateway as an official image

- Status: Active
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Start with a top-level canonical Gateway source tree and a separate official
Gateway image release boundary. Keep the Go embed snapshot and check it for
byte drift. Publish a multi-architecture GHCR image with moving development
tags and an immutable commit tag, then add a checked digest reference to the
CLI only after the UID/GID, architecture, API, and CA-volume contracts are
proven.

The likely migration path is direct official image consumption: the image is
host-UID-independent and Compose supplies the invoking numeric identity. The
embedded source build remains the explicit development/recovery path during
the migration window; it is not silently selected as a runtime fallback after
the official digest becomes the routine authority.

## Alternatives considered

### Alternative A: Keep host builds as the normal path

This preserves source/CLI coherence and dynamic UID/GID handling with the fewest
release artifacts. It keeps Docker build cost and host BuildKit failures on the
first-use path, which conflicts with the adoption thesis.

### Alternative B: Use a direct immutable official image

This gives the simplest startup and reproducible multi-architecture artifact,
but requires a host-UID-independent volume/permission design and creates a
stronger supply-chain dependency for the trusted boundary.

### Alternative C: Official core plus thin UID adaptation layer

This avoids rebuilding mitmproxy and Gateway source on each host while retaining
host-specific ownership of mounted files. It still performs a small local build,
so it remains a recovery fallback rather than the target routine experience.

## Design

### Public contract

No new routine Gateway command is required. `cluster up` continues to own shared
Gateway/OPA reconciliation. The selected Gateway image is an internal
installation artifact, not a Context or Workspace input. Missing or incompatible
images fail before network/container mutation with an exact diagnostic and an
explicit source-build/retry path for development where supported.

### Layer changes

- Domain: add only the Gateway image/API identity needed for validated state.
- Application: preserve cluster lifecycle ownership and failure ordering.
- Infrastructure: resolve the immutable image, verify architecture/labels/digest,
  and build only the explicit source fallback.
- CLI/catalog: likely no new public command; improve diagnostics and doctor/help
  only if the image source becomes user-visible.
- Repository: establish one canonical Gateway source and a checked embedding
  boundary, with Python tests and image workflow owned beside that source.

### Data and control flow

```text
reviewed Gateway source
  -> multi-arch CI image build
  -> immutable GHCR digest
  -> CLI/release metadata
  -> preflight image/API/architecture validation
  -> compose startup with host-owned policy/credential/principal mounts
```

### Error and cancellation behavior

Image resolution, digest, architecture, and compatibility failures occur before
Gateway/OPA or project network mutation. A source/adaptation build failure does
not silently switch authority paths. Existing cluster recovery and ownership
checks remain unchanged.

### Security and public boundary

The Gateway image must not contain policy or secrets. The workflow uses only
least-privilege package permissions. Official tags are moving development
aliases only; routine startup uses a digest. The image contract
must preserve non-root operation, fixed resources, no host port, no Docker
socket, and the project-principal boundary.

## Verification

- Python unit tests inside the pinned mitmproxy base.
- Docker image contract and multi-architecture inspection.
- UID/GID, owner-only credential mount, private/public CA volume, and non-root
  tests on the supported Linux and Colima environments.
- Gateway/OPA/project-principal integration and outage tests.
- Source snapshot/embedding drift check.
- `task check`, `task security`, `task public:check`, `task release:check`, and
  the relevant image/release gate.

## Rollout and rollback

Publish the image before changing the CLI's default reference. Keep the
embedded source/adaptation path for one migration window. Roll back by restoring
the prior immutable image digest and CLI metadata; never retag an existing
release identity to different bytes.

## Documentation promotion

Promote the source-of-truth location, Gateway image contract, UID/GID decision,
release workflow, and rollback policy into an ADR plus architecture, security,
harness, and release documentation. Remove this temporary packet after handoff.
