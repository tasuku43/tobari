# Work Context: Make runtime preparation and reuse deterministic

This file records verified facts and unknowns. It does not treat a proposed
runtime lifecycle as current behavior.

## Current behavior

- The public runtime path is host-owned `runtime init`, editing the active
  Context Dockerfile, and explicit `runtime build`.
- Parent E2E observed that a Workspace entered before the runtime was ready
  could become an instance with `runtime: missing`; later initialization/build
  did not repair that instance without deletion and re-registration.
- Parent and blind Long-01 E2E observed that `runtime build` selects a new
  image, while an already-existing Workspace continues to use its old image on
  re-entry. Deleting and recreating the Workspace consumed the new image.
- Blind Long-01 and the parent shell transcript displayed `I have no name!`
  in the runtime prompt. The cause and whether it is product-visible defect or
  an intentional dynamic UID model are not yet established.
- The current fixed Workspace lifetime boundary is host-controlled; runtime
  image selection must not become an implicit project-file or agent-side side
  effect.

## Verified reproduction and decision

- On 2026-08-03, a fresh task-owned project with an active Context selecting
  `localhost/missing-runtime:latest` reached the real `tobari` PTY. The first
  entry attempt returned `undeclared_fault_contract` and left one logical
  Workspace with `runtime: missing` in `list --format json`. No unrelated
  containers were touched; the Gateway and OPA containers were task-owned and
  healthy.
- The failure was two defects: entry created logical state before runtime
  image validation, and the image fault was not declared by the `tobari`
  catalog. This made a normal missing-image prerequisite look like a contract
  failure.
- Decision: a new Workspace now performs a read-only runtime-image preflight
  before `CreateProject`. A missing or incompatible image is surfaced as the
  declared `image_not_found`/runtime prerequisite and the logical Workspace
  remains absent. Existing Workspaces still use their stored image and retain
  the no-silent-refresh boundary.
- The shell identity probe was attempted with a real 120x40 PTY, but the
- A subsequent clean 120x40 PTY replay on the supported built image produced
  `tobari@...`, `id -un=tobari`, `HOME=/var/lib/tobari`, and `BASH=/bin/bash`,
  followed by a successful re-entry. The earlier `I have no name!` output is
  classified as a runtime image whose passwd entry did not match the host UID;
  no product change is justified without reproducing it on a supported build.
- The complete sanitized replay is in
  [the runtime lifecycle transcript](e2e/runtime-lifecycle-transcript.md).

## Relevant structure

- Runtime use cases: `internal/app/contextcmd/`.
- Runtime/domain vocabulary: `internal/domain/tobari/`.
- Runtime adapters and Context store: `internal/infra/dockerruntime/`.
- Embedded/runtime assets: `internal/infra/runtimeassets/` and runtime Docker
  sources.
- CLI catalog and presentation: `internal/cli/runtime_catalog.go` and related
  Context/runtime commands.
- E2E and gates: `scripts/test-integration.sh`, `task runtime:test`, and the
  existing `runtime-bash-shell` evidence packet.
- Source evidence: [new-user E2E comparison](../new-user-value-e2e/comparison.md).

## Constraints

- Context runtime authority remains host-owned and bounded.
- A build must not silently mutate an existing live Workspace or broaden
  credentials/network access.
- Runtime failures must remain explicit, typed, and recoverable without direct
  Docker/OPA operations in the supported user journey.
- Use synthetic projects, paths, timestamps, and markers in committed evidence.
- Public docs are English and must not contain host paths, usernames, secrets,
  or shell history.

## Unknowns

- [x] Runtime preparation is a hard prerequisite before new Workspace
      registration; no safe reconcile mutation is needed for this finding.
- [x] `runtime build` is new-Workspace-only for image selection. Existing
      Workspaces retain their stored image; refresh/recreate remains a future
      explicit lifecycle capability, not an implicit side effect.
- [x] The `I have no name!` prompt did not reproduce on the clean supported
      image replay; the visible identity is `tobari` and the shell is Bash.
- [ ] Which existing runtime/integration profile is the authoritative E2E gate
      once the policy-review PTY blocker is fixed?

## Evidence intake

- Parent baseline: `feedback/official/parent-baseline.md` in the
  `new-user-value-e2e` packet.
- Blind Long-01: `feedback/official/long-01.md` in the same packet.
- Existing Bash runtime evidence: `docs/work/runtime-bash-shell/`.

## Verification outcome

- Focused Go tests and `go test ./...` pass with the preflight change.
- The fresh PTY replay and CLI-owned cleanup pass; the task-owned state ended
  with no Workspace records and no `tobari-*` containers.
- `task runtime:test` passes its policy, Gateway, custom-image, and ordinary
  lifecycle portions, then exits 130 at the checked-in interactive policy
  review handoff. The process tree shows the test's PTY wrapper waiting at
  `policy review --tail 1000`; the same external wrapper/input handoff is
  already recorded in `policy-review-tty`. This remains an integration
  harness blocker, not a runtime-preflight failure.

## Security and public-boundary notes

- Do not solve runtime readiness by weakening Docker VM sharing, pulling
  arbitrary images, or adding an unrestricted runtime command.
- Treat image contents and shell output as untrusted external text; preserve
  terminal-safe projection and redaction.
