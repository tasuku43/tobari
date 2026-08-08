# ADR 0016: Manage custom runtime recipes through a Context

- Status: Accepted
- Date: 2026-08-02
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, and harness
- Supersedes: None
- Superseded by: ADR 0018 supersedes active-Context authority; the Context-owned recipe remains accepted

## Context

Tobari's Context now composes the agent profile, runtime image, policy, and
credential references, but custom runtime creation still requires users to
name and build an image outside Tobari. Project-local Dev Container metadata
was deliberately removed because it would become a second runtime authority.
The remaining user task is to make a user-specific runtime easy to create
without making Docker image identity or Context selection a routine concern.

## Decision drivers

- Runtime customization must be as natural as the current-Context workflow.
- The user should perform one explicit host-side build for arbitrary Dockerfile
  instructions.
- A failed or incomplete build must never replace the last known-good runtime.
- The Docker build context must not contain policy or credential stores.
- Existing Context manifests and explicit local OCI image selectors must remain
  readable during pre-v1 evolution.
- Future runtime import formats must attach to Context rather than bypass it.

## Considered options

### Option A: Context-owned recipe with current-Context commands

`runtime init` creates a fixed recipe below the current Context and
`runtime build` builds, validates, and selects a generated local image. Chosen.

### Option B: Project-local Dockerfile precedence

This makes recipes easy to share but allows project data to change a trusted
host execution setup implicitly. It also recreates a partial Dev Container
authority. Rejected.

### Option C: User-named image plus a separate selection command

This preserves the current Docker workflow but makes users manage a second
identity and repeat the Context composition themselves. Retained only as the
compatibility path for existing explicit image selectors.

## Decision

The current Context owns a conventional `runtime/Dockerfile` recipe directory.
`tobari runtime init` creates the template without overwriting an existing
recipe. `tobari runtime build` is the explicit host-side Docker build boundary.
It uses only that directory as build context, validates the resulting image
against the existing Tobari runtime contract, obtains its local image digest,
and atomically promotes the generated image reference into the Context.

When the first `FROM` is the exact official
`ghcr.io/tasuku43/tobari/runtime:latest` base, the explicit build requests a
base refresh with Docker's `--pull`. Explicit local or custom bases do not
receive that request, so the local-base development path remains valid.

The generated image name is an implementation detail derived from the Context
name and recipe source digest. The image digest and recipe source digest are
the durable identities. No `runtime use` or user image-name input is part of
the routine workflow. `context use` remains the operation that changes the
current/default Context only.

The selected image remains unchanged while a recipe is being edited or a build
is failing. Existing Context manifests continue to use their selected `image`
field; the recipe and build record are additive metadata during this pre-v1
schema evolution.
Existing Workspaces bound to that Context observe a successfully promoted image
on their next matching-Context `tobari` root entry. Runtime reconciliation
validates their bound Context image, recreates only the work container when the
spec changed, preserves the Workspace home, and updates the stored project
image only after success. Workspaces in other Contexts are unchanged.

## Consequences

### Positive

- The routine custom-runtime path is two commands plus Dockerfile editing.
- A Tobari's permanently bound Context is its only runtime selection authority.
- Build failure is safe and recoverable because promotion is atomic.
- Policy and credentials remain outside the Docker build context.
- The ordinary recipe workflow gives the moving official base an explicit
  refresh point without making local bases depend on a registry.
- Existing Workspaces can adopt a custom runtime without deleting tool-owned
  authentication state in the Workspace home.
- A future Dev Container or other recipe importer can target the same Context
  runtime boundary instead of becoming a second source of truth.

### Negative

- The first build executes user-authored Dockerfile instructions through the
  trusted host Docker daemon and is intentionally explicit.
- Context recipes are host-local until a future import/export capability is
  designed.
- The existing generic image selector remains available while the new workflow
  becomes the preferred path.

## Security and public-boundary impact

The build context is a dedicated owner-only Context child. Credential values,
policy source, host home, Docker sockets, and Workspace mounts are not part of
the build context. The built image is still untrusted at runtime and must pass
the fixed non-root, entrypoint, lifetime, network, mount, and resource checks.
No secrets are accepted in arguments or written to the manifest.

## Mechanical enforcement

- Domain tests validate recipe kinds, fixed relative recipe paths, digests, and
  runtime report states.
- Infrastructure tests validate owner-only recipe files, build-context argv,
  image inspection, atomic promotion, unchanged selection on failure, and
  existing-Workspace image reconciliation after promotion.
- Catalog tests validate fixed targets, complete output, stable faults, and
  scoped help for both commands.
- The capability ledger, product contract, architecture, security model, and
  harness claim table record the boundary and its checks.

## Validation

- `task check`
- `task security`
- `task public:check`
