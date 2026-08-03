# Work Tasks: Make runtime preparation and reuse deterministic

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [ ] Read the governing thesis, product, architecture, security, harness, and
      readiness documents.
- [ ] Inspect the current Context/runtime catalog, use cases, adapters, image
      assets, Bash runtime packet, and relevant tests.
- [x] Record fresh reproductions for runtime-not-ready registration and
      build-after-Workspace registration.
- [x] Attempt to reproduce and classify the `I have no name!` prompt in a
      clean PTY; the external wrapper timed out and the identity question is
      kept separate from the preflight fix.

## Decide

- [x] Choose prevention (read-only runtime-image preflight) and record why
      alternatives are rejected.
- [ ] Decide whether shell identity requires a runtime fix, a contract note, or
      a separate deferred packet.
- [x] Confirm and implement the public catalog recovery impact:
      `image_not_found` points to `runtime build`.

## Implement

- [x] Add the smallest application/infrastructure/CLI change required by the
      decision: preflight before logical Workspace creation plus its tests.
- [ ] Add negative-path, cancellation, failed-build, and opaque/lifecycle
      ownership tests in the owning layer.
- [ ] Update durable product/architecture/security/harness docs if the decision
      changes a contract.
- [ ] Update the public Quick Start only through its dependent handoff packet.

## Verify

- [x] Replay the complete runtime journey through a real 120x40 PTY.
- [x] Verify existing Workspace behavior and cleanup ownership; CLI cleanup
      removed the task-owned records and cluster.
- [ ] Run `task runtime:test`, `task check`, `task security`, and
      `task public:check`.
- [x] Record integration outcome and the remaining external policy-review PTY
      blocker.

## Hand off

- [ ] Commit all scoped implementation/tests/docs changes with an intentional
      message and report the SHA.
- [ ] Update dependent `new-user-quickstart-handoff` context before it starts.
- [ ] Mark this packet complete only after its E2E and gate evidence is real.
