# Work Tasks: Make human PTY evidence reproducible and safe

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read the governing thesis, architecture, security, harness, public, and
      readiness documents. Evidence: governing references are listed in the
      packet and the durable harness/readiness sections were reviewed.
- [x] Inspect existing PTY helpers, child feedback intake, and public-boundary
      checks. Evidence: `scripts/test-integration.sh`, the policy PTY test, and
      `tools/repoguard` were inspected.
- [x] Record the raw-evidence gap and the non-negotiable redaction fields.

## Decide

- [x] Choose the parent capture boundary and cross-platform metadata format:
      `scripts/pty-evidence.py` writes a schema-versioned bundle to an external
      task-owned directory.
- [x] Define the readable checkpoint schema and digest procedure: metadata
      records terminal settings, monotonic elapsed time, input timing, output
      offsets, prefix digests, visible tails, and raw/redacted SHA-256 values.
- [x] Confirm the child prompt remains outcome-only; the capture boundary is
      parent-owned and is not part of blind scenario instructions.

## Implement

- [x] Add or refine the bounded capture helper and redaction tests. Evidence:
      `scripts/pty_evidence.py` and `scripts/test-pty-evidence.py`.
- [x] Add parent-owned artifact intake without changing scenario usage steps.
      Evidence: the feedback README now names the external bundle contract.
- [x] Update the feedback template and harness/readiness docs if durable.

## Verify

- [x] Replay a delayed-input PTY with ANSI redraw and inspect raw plus redacted
      outputs. Evidence: `python3 scripts/test-pty-evidence.py`.
- [x] Prove no host-specific or sensitive data enters the packet. The helper
      refuses repository-local output and the contract test checks path, opaque
      ID, and literal-value redaction while requiring ANSI preservation.
- [x] Run one blind Tobari E2E at the parent-owned capture boundary without
      handing the child the artifact route. Evidence: the 2026-08-04 blind
      Medium-02 lifecycle rerun completed functionally; the external parent
      capture and its metadata are recorded in `context.md`.
- [x] Run `task check`, `task security`, and `task public:check`.

## Hand off

- [x] Commit the scoped harness/evidence changes and report the SHA. The
      parent will make a narrow rescue commit because the assigned agent
      stopped without creating one.
- [x] Update `new-user-value-e2e` to consume the artifact contract; its
      feedback README now distinguishes pre-helper reports from future bundles.
- [x] Mark complete after the blind outcome-only run and the parent-owned
      digest/checkpoint capture are both recorded. The child-returned artifact
      distinction remains explicit in feedback and is not presented as a raw
      child transcript.
