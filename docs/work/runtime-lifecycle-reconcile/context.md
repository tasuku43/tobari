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

- [ ] Should runtime preparation be a hard prerequisite before Workspace
      registration, or should the CLI expose a safe reconcile transition?
- [ ] Is a safe refresh/recreate operation necessary, or is new-Workspace-only
      behavior the intended contract?
- [ ] Does the `I have no name!` prompt reproduce on the supported base image
      and current main, and what identity contract should be visible?
- [ ] Which existing runtime/integration profile is the authoritative E2E gate
      once the policy-review PTY blocker is fixed?

## Evidence intake

- Parent baseline: `feedback/official/parent-baseline.md` in the
  `new-user-value-e2e` packet.
- Blind Long-01: `feedback/official/long-01.md` in the same packet.
- Existing Bash runtime evidence: `docs/work/runtime-bash-shell/`.

## Security and public-boundary notes

- Do not solve runtime readiness by weakening Docker VM sharing, pulling
  arbitrary images, or adding an unrestricted runtime command.
- Treat image contents and shell output as untrusted external text; preserve
  terminal-safe projection and redaction.
