# Work Goal: Make human PTY evidence reproducible and safe

- Status: Complete
- Retention: temporary
- Retention reason: Close the raw pseudo-TTY evidence gap found in the four new-user journeys.
- Governing contract: `docs/00_theses.md`, `docs/02_architecture.md`, `docs/03_security_model.md`, `docs/04_harness.md`, `docs/05_public_repository.md`, `docs/09_agent_readiness_validation.md`
- Review/delete trigger: Delete after the capture/checkpoint procedure remains covered by the harness contract test and the successor evidence packet no longer needs the external artifact reference.
- Successor: [new-user-value-e2e](../new-user-value-e2e/goal.md)
- Owner: Harness and agent-readiness maintainers
- Target: Child-agent PTY capture, redaction, digest, checkpoint, and handoff
- Related ADRs: None

## Outcome

Every future blind human-path E2E run returns enough raw and readable
pseudo-TTY evidence to verify terminal dimensions, typed timing, redraws,
visible state transitions, exit status, and cleanup without committing host
paths, credentials, or shell history.

## Why now

All four recent child runs reported real PTYs and functional outcomes, but none
returned a raw-byte digest or complete timestamped checkpoint artifact. The
parent-owned capture boundary is now implemented and has been exercised around
the validated integration path; the blind child remains intentionally unaware
of the capture route.

## Non-goals

- Do not change Tobari product behavior or add a public CLI command.
- Do not make a piped/non-TTY replay count as human success.
- Do not store raw host paths, credentials, shell history, or private output in
  Git.
- Do not give child agents the scenario route, thesis, source, or harness
  instructions that would invalidate blind discovery.

## Acceptance criteria

- [x] A supported PTY runner captures raw bytes, terminal size, `TERM`, typed
      input timing, output checkpoints, and exit status.
- [x] The runner produces SHA-256 digests and a redacted readable projection;
      raw captures are refused inside the repository and remain outside Git.
- [x] Redaction preserves evidence of ANSI redraw/cursor restoration while
      removing host-specific paths, usernames, opaque IDs, and caller-supplied
      sensitive values.
- [x] One blind synthetic journey completes without receiving a command
      sequence or bypassing the human path. Evidence: the 2026-08-04
      outcome-only Medium-02 rerun completed recovery, reuse, nested cancel,
      and cleanup.
- [x] The parent-owned artifact path and discovery count are included in
      feedback, with no extra parser/join needed to interpret routine success.
      Evidence: the rerun feedback records the child's discovery result and
      the external integration capture records the artifact path, checkpoints,
      exit status, and digests.
- [x] `task check`, `task security`, and `task public:check` pass.

## Governing documents

- Thesis: [Project theses](../../00_theses.md)
- Architecture/security: [Architecture](../../02_architecture.md) and [Security model](../../03_security_model.md)
- Harness/public: [Harness](../../04_harness.md) and [Public Repository](../../05_public_repository.md)
- Readiness: [Agent Readiness Validation](../../09_agent_readiness_validation.md)

## Completion definition

The harness produces safe raw/readable PTY evidence from a parent-owned run,
the procedure is independently replayed by its contract test, required gates
pass, and the evidence format is promoted into the governing harness/readiness
documentation. A blind Tobari rerun independently completes the outcome-only
journey; the child does not need to know the capture route because capture is a
parent-owned boundary.

## Evaluation and handoff

- Agent-readiness scenario: one outcome-only blind replay of the denial/review
  or runtime lifecycle journey, selected by the parent only after the capture
  boundary is ready; the child receives the desired outcome, not the route.
- Discovery budget: preserve the selected source scenario's lookup budget and
  report every help lookup, guessed command, backtrack, and recovery lookup
  alongside the digest/checkpoints.
- Required gates: `task check`, `task security`, `task public:check`, and
  the relevant agent-readiness/integration replay.
- Handoff: the bounded harness/template/docs change is committed in `b8e0d50`
  and the single-delivery contract assertion is committed in `7f1b9ac`. The
  parent-owned capture is external to Git at
  `/private/tmp/tobari-pty-integration-20260804/capture`; its raw digest is
  `db2b7f75ae62ab98e3ef56853267ec1dcc00ffaaaaa89bc07a86223c7ead257b` and its
  redacted digest is
  `0dc2b769ab9092f848b638348d1de6d39f49f596930b2f312e8a4c1c1fbf8d72`.
