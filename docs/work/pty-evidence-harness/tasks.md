# Work Tasks: Make human PTY evidence reproducible and safe

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [ ] Read the governing thesis, architecture, security, harness, public, and
      readiness documents.
- [ ] Inspect existing PTY helpers, child feedback intake, and public-boundary
      checks.
- [ ] Record the raw-evidence gap and the non-negotiable redaction fields.

## Decide

- [ ] Choose the parent capture boundary and cross-platform metadata format.
- [ ] Define the readable checkpoint schema and digest procedure.
- [ ] Confirm the child prompt remains outcome-only.

## Implement

- [ ] Add or refine the bounded capture helper and redaction tests.
- [ ] Add parent-owned artifact intake without changing scenario usage steps.
- [ ] Update the feedback template and harness/readiness docs if durable.

## Verify

- [ ] Replay a delayed-input/cancel PTY and inspect raw plus redacted outputs.
- [ ] Prove no host-specific or sensitive data enters the packet.
- [ ] Run one blind E2E using the artifact path.
- [ ] Run `task check`, `task security`, and `task public:check`.

## Hand off

- [ ] Commit the scoped harness/evidence changes and report the SHA.
- [ ] Update `new-user-value-e2e` to consume the artifact.
- [ ] Mark complete only after a real blind run returns the digest/checkpoints.
