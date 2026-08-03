# Work Tasks: Make runtime preparation and reuse deterministic

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read the governing thesis, product, architecture, security, harness, and
      readiness documents. Evidence: the packet context and implementation
      review cite the selected governing documents.
- [x] Inspect the current Context/runtime catalog, use cases, adapters, image
      assets, Bash runtime packet, and relevant tests. Evidence: the focused
      preflight test and runtime integration replay cover those boundaries.
- [x] Record fresh reproductions for runtime-not-ready registration and
      build-after-Workspace registration.
- [x] Attempt to reproduce and classify the `I have no name!` prompt in a
      clean PTY; the external wrapper timed out and the identity question is
      kept separate from the preflight fix.

## Decide

- [x] Choose prevention (read-only runtime-image preflight) and record why
      alternatives are rejected.
- [x] Decide whether shell identity requires a runtime fix, a contract note, or
      a separate deferred packet. Evidence: the clean supported image reports
      `tobari` with `/bin/bash`; the older `I have no name!` observation did
      not reproduce and remains classified as unsupported-image evidence.
- [x] Confirm and implement the public catalog recovery impact:
      `image_not_found` points to `runtime build`.

## Implement

- [x] Add the smallest application/infrastructure/CLI change required by the
      decision: preflight before logical Workspace creation plus its tests.
- [x] Add negative-path, cancellation, failed-build, and opaque/lifecycle
      ownership coverage in the owning layer, or record existing contract
      coverage where no new behavior is required. Evidence: the preflight
      no-registration test, runtime image tests, and lifecycle integration
      checks pass.
- [x] Update durable product/architecture/security/harness docs if the decision
      changes a contract. Evidence: no governing contract changed; the
      implementation preserves the existing explicit build/reuse rule.
- [x] Update the public Quick Start only through its dependent handoff packet.
      Evidence: `new-user-quickstart-handoff` consumes commit `6094f08`.

## Verify

- [x] Replay the complete runtime journey through a real 120x40 PTY.
- [x] Verify existing Workspace behavior and cleanup ownership; CLI cleanup
      removed the task-owned records and cluster.
- [x] Run `task runtime:test`, `task check`, `task security`, and
      `task public:check`. The repository gates pass; the runtime profile exits
      130 at its existing interactive policy-review handoff and is recorded as
      an external blocker in `context.md`.
- [x] Record integration outcome and the remaining external policy-review PTY
      blocker.

## Hand off

- [x] Commit all scoped implementation/tests/docs changes with an intentional
      message and report the SHA. Evidence: parent rescue commit `6094f08`.
- [x] Update dependent `new-user-quickstart-handoff` context before it starts.
      Evidence: its context records `6094f08` and the new-Workspace-only rule.
- [x] Mark this packet complete after the real PTY E2E, passing repository
      gates, and exact specialized-profile blocker were recorded.
