# Work Tasks: Make runtime preparation and reuse deterministic

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [ ] Read the governing thesis, product, architecture, security, harness, and
      readiness documents.
- [ ] Inspect the current Context/runtime catalog, use cases, adapters, image
      assets, Bash runtime packet, and relevant tests.
- [ ] Record fresh reproductions for runtime-not-ready registration and
      build-after-Workspace registration.
- [ ] Reproduce and classify the `I have no name!` prompt in a clean PTY.

## Decide

- [ ] Choose prevention, reconcile, or explicit new-Workspace-only semantics
      and record why alternatives are rejected.
- [ ] Decide whether shell identity requires a runtime fix, a contract note, or
      a separate deferred packet.
- [ ] Confirm the public command/catalog/recovery impact before implementation.

## Implement

- [ ] Add the smallest domain/application/infrastructure/CLI change required by
      the decision, or record why no code change is needed.
- [ ] Add negative-path, cancellation, failed-build, and opaque/lifecycle
      ownership tests in the owning layer.
- [ ] Update durable product/architecture/security/harness docs if the decision
      changes a contract.
- [ ] Update the public Quick Start only through its dependent handoff packet.

## Verify

- [ ] Replay the complete runtime journey through a real 120x40 PTY.
- [ ] Verify existing Workspace behavior and cleanup ownership.
- [ ] Run `task runtime:test`, `task check`, `task security`, and
      `task public:check`.
- [ ] Record integration outcome and any remaining external blocker.

## Hand off

- [ ] Commit all scoped implementation/tests/docs changes with an intentional
      message and report the SHA.
- [ ] Update dependent `new-user-quickstart-handoff` context before it starts.
- [ ] Mark this packet complete only after its E2E and gate evidence is real.
