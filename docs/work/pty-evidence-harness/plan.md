# Work Plan: Make human PTY evidence reproducible and safe

- Status: Complete
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Keep blind-discovery prompts outcome-only, but give the parent orchestration a
separate capture boundary. The boundary records raw bytes and metadata outside
Git, computes a digest, and writes only a reviewed redacted projection into
the parent feedback packet.

## Alternatives considered

### Capture only normalized stdout

Rejected because it hides ANSI redraw, cursor restoration, terminal sizing, and
the distinction between a visible screen and a piped transcript.

### Give the child the repository harness

Rejected because it teaches the route and invalidates first-time discovery.

### Commit raw captures

Rejected by the public-boundary and security constraints.

## Verification

- Real-PTY smoke run with delayed input and ANSI redraw passed, and a blind
  Tobari lifecycle replay completed without a supplied route. The parent-owned
  integration capture produced the external raw/readable bundle used for
  handoff.
- Digest and redaction negative tests for paths, credentials, control
  sequences, literal values, and opaque IDs pass in
  `scripts/test-pty-evidence.py`.
- The four official child reports predate this artifact format; the new
  parent-owned bundle closes the capture-boundary requirement while the child
  feedback records the absence of a child-returned bundle honestly.
- `task check`, `task security`, and `task public:check` pass.
