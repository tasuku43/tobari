# ADR 0015: Defer Dev Container integration until Context runtime operations stabilize

- Status: Accepted
- Date: 2026-08-02
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, and harness
- Supersedes: None
- Superseded by: None

## Context

Tobari added a narrow Dev Container compatibility path before the Context
composition model and custom-runtime workflow were settled. The path reads a
project-local `.devcontainer/devcontainer.json` and lets a literal image
override the Tobari's bound Context image. It also exposes a legacy
`attach --devcontainer` input.

The standard Dev Container format describes a broader environment model than
this subset. Keeping only an image parser provides little help for creating a
custom runtime, while expanding it would introduce a second runtime
orchestrator with overlapping build, mount, privilege, lifecycle, and feature
semantics.

## Decision drivers

- Context should remain the one understandable runtime/policy/credential setup.
- Project files are untrusted data and must not silently change the selected
  host execution setup.
- Tobari owns the fixed network, mount, capability, and lifecycle boundary.
- A future custom-runtime workflow should make Dockerfile/image construction
  explicit rather than hiding it behind incidental project metadata.
- Pre-v1 removal must preserve existing Workspace state and ordinary project
  files.

## Considered options

### Option A: Keep the image-only Dev Container subset

This preserves a familiar filename but leaves an implicit project-file-over-
Context precedence rule and does not make custom runtime creation easier.
Rejected for now.

### Option B: Delegate the full Dev Container specification

This would require accepting or translating build, Compose, Features, mounts,
users, privileges, lifecycle commands, and related supply-chain behavior. It
would make Dev Container a second authority over the runtime boundary.
Rejected for the current product.

### Option C: Defer and remove the integration

Runtime selection remains owned by each Tobari's bound Context. Generic compatible OCI
images remain supported, while future Dev Container support can be designed as
an explicit Context/runtime import or build operation after the runtime
workflow is clear. Chosen.

## Decision

Tobari does not interpret `.devcontainer/devcontainer.json` for CWD-owned
Workspace creation and does not expose `--devcontainer` on the legacy
`attach` command. The selected Context is the only runtime image source for new
project records. Existing persisted generic image selectors remain readable;
existing project files and Workspace homes are not deleted or rewritten.

Future reconsideration must choose an explicit operation such as a Context
runtime import/build flow. It must not restore automatic project-file
precedence as a hidden fallback.

## Consequences

### Positive

- Runtime selection has one source of truth.
- A repository cannot silently change the host-selected Context runtime.
- The code and product vocabulary no longer promise a partial Dev Container
  implementation.
- The future custom-runtime surface can be designed around the actual Context
  workflow rather than an upstream format subset.

### Negative

- Projects that previously relied on the automatic image override must select
  the same image through a Context or generic explicit image path.
- `.devcontainer/devcontainer.json` is not a custom-runtime mechanism for now.
- Reintroducing compatibility later requires a deliberate migration/design
  decision rather than restoring the old parser.

## Security and public-boundary impact

The project-data read set shrinks. No additional host-side execution is added,
and no project-local file can change runtime image selection implicitly.
The fixed Tobari Docker specification, local image compatibility validation,
policy boundary, and tool-owned credential home are unchanged.

## Mechanical enforcement

- Domain and infrastructure tests prove project creation uses only the active
  Context image and ignores a `.devcontainer` file.
- CLI/catalog tests prove `--devcontainer` is absent and unknown input follows
  the normal parser failure path.
- Repository scans and documentation checks prevent stale public references.
- Capability-retirement evidence records the removed faults, ports, parser,
  fixture, and persisted-state disposition.

## Compatibility and rollback

This is a pre-v1 public-contract narrowing. Existing Workspace state and homes
remain intact. A source-level rollback is possible before release. Any future
reintroduction must be an explicit Context/runtime import or build decision.

## Validation

`task check`, `task security`, and `task public:check` are required.
