# Work Plan: Make human PTY evidence reproducible and safe

- Status: Active
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

- Real-PTY smoke run with delayed human input and ANSI redraw; cancellation and
  blind Tobari replay remain separate scenario acceptance work.
- Digest and redaction negative tests for paths, credentials, control
  sequences, literal values, and opaque IDs pass in
  `scripts/test-pty-evidence.py`.
- The four official blind scenario reports predate this artifact format; the
  next parent run must consume one bundle before this packet can be removed.
- `task check`, `task security`, and `task public:check` pass.
