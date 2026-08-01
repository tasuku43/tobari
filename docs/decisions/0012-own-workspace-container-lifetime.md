# ADR 0012: Tobari owns the Workspace container lifetime

- Status: Accepted
- Date: 2026-08-01
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, and harness
- Supersedes: None
- Superseded by: None

## Context

The Workspace is a reusable CWD-owned environment. The runtime already has a
single work container with a bootstrap entrypoint and enters it through
`docker exec`, but Docker create previously inherited the selected image's
`CMD`. An image containing `CMD ["claude"]` could therefore make a normal
agent exit look like Workspace runtime loss.

The design also considered a Kubernetes-style pause container as a separate
holder for the network namespace and lifetime. Tobari's MVP already has a
named per-Workspace network, a Gateway connection, one health-checked work
container, and explicit host-side reconciliation. A second holder would add
resource, label, recovery, deletion, and network-coordination state without
removing the need for a compatible work image.

## Decision drivers

- A shell or agent command must be a child session, not the Workspace lifetime.
- A child nonzero exit must preserve its status without stopping the Workspace.
- The selected image must remain a useful environment/tool bundle while its
  lifecycle authority stays with Tobari.
- Runtime failure and image incompatibility must remain distinguishable and
  fail closed before project Docker resources are mutated.
- The MVP should keep one work container per Workspace and no second
  orchestrator or holder container.

## Considered options

### Option A: Let the selected image `CMD` own the work container

This keeps Docker create short, but turns an image or user-command exit into a
Workspace lifecycle event. Restarting the command could duplicate coding-agent
side effects and does not provide a stable reusable environment.

### Option B: Add a Kubernetes-style pause or holder container

This would keep a namespace or holder process alive separately, but would add a
second resource and more coordination to every Workspace. It would not make an
arbitrary image compatible with Tobari's bootstrap, user, mounts, health, or
security contract, and it does not simplify the current single-container
reconciliation path.

### Option C: Tobari-owned lifetime command in the existing work container

This keeps one labeled work container and the existing network, health, and
`docker exec` boundaries. Docker create passes a fixed `sleep infinity`
command after the selected image, so Docker ignores that image's default `CMD`
for Workspace lifetime. User shells and exact commands remain child execs.

## Decision

Choose Option C. Tobari owns the work container's lifetime with an explicit
infrastructure-owned `sleep infinity` command. The selected image is an
environment image: it must preserve runtime API `1`, the `tobari` user, the
bootstrap entrypoint, and assert the
`io.tobari.runtime-lifetime-command=sleep infinity` capability, but its `CMD`
is not a lifecycle contract. A missing or incompatible
image is rejected before project home, network, or container mutation. The
minimal Tobari runtime is the compatibility foundation; future Claude, Codex,
and toolbox images may be published as reviewed derived convenience bases, but
publication is a separate release and public-boundary decision.

## Consequences

### Positive

- Workspace existence is independent of agent and shell command exit.
- One container remains the unit of mounts, network attachment, health,
  resource bounds, logs, recovery, and deletion.
- Custom images can add tools without taking ownership of lifecycle or Docker
  isolation settings.
- The minimal-plus-derived image family has a clear compatibility base without
  making registry artifacts a new trust boundary.

### Negative

- Compatible custom images must retain the Tobari bootstrap and provide the
  lifetime command capability; arbitrary OCI images are not accepted.
- Tobari owns one fixed lifetime command and must include it in runtime drift
  detection and integration coverage.
- A failed user command remains a user-visible nonzero result; Tobari does not
  automatically restart it.

### Risks and mitigations

- A custom image can assert compatibility metadata without actually containing
  every required executable. The reviewed minimal base, derived-image build
  checks, and terminating-`CMD` integration fixture keep the supported path
  explicit; compatibility metadata is not a provenance signature.
- A future lifetime-command change could silently drift existing containers.
  The desired command is part of the project spec hash and fixed create-argv
  tests.

## Mechanical enforcement

- `EnsureProjectRuntime` validates the selected image before preparing the
  project home or creating project network/container resources.
- `validateCompatibleImage` rejects missing images and wrong runtime API,
  lifetime capability, image user, or bootstrap entrypoint with typed faults.
- `ensureProjectContainer` and the legacy attach path append
  `sleep infinity` after the image in Docker create argv.
- The project runtime spec hash records the lifetime command, so drift causes
  deterministic work-container recreation.
- Unit tests assert exact create argv, command-sensitive spec hashing, and
  pre-mutation incompatible-image failure. Docker integration uses a compatible
  image whose `CMD` exits immediately and proves the created work container
  still runs and remains ready after a nonzero child exec.
- The harness keeps the no-pause decision visible through the one-container
  lifecycle model and the absence of a second holder resource.

## Compatibility and migration

The public lifecycle model does not add a command or state. Existing logical
Workspace records and homes remain reusable; a changed runtime spec recreates
only the work container. Existing custom images must be rebuilt from the
compatible minimal runtime or otherwise satisfy runtime API `1`. No GHCR image
name, tag, credential, or publication workflow is introduced by this decision.

## Security and public-boundary impact

The explicit lifetime command adds no mount, capability, network, credential,
or resource authority. Image contents remain untrusted and are run with the
same fixed UID/GID, read-only root, capabilities, mounts, proxy, internal
network, healthcheck, and resource bounds. Future image publication requires
separate source, license, provenance, permission, and release review.

## Validation

- `go test ./internal/infra/dockerruntime ./internal/app/tobaricmd ./internal/cli`
- `task check`
- `task runtime:test` on a clean Docker environment

## Reconsideration signals

Reconsider this ADR if the single work-container model cannot reliably host
long-lived background processes, if a measured recovery or cleanup problem
requires a separate holder, or if another isolation primitive provides the
same mount, network, health, resource, and child-exec semantics with lower
operational cost. A reconsideration must add a successor ADR rather than
silently restoring image-command ownership.
