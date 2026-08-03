# Work Plan: Restore a visible interactive policy review

- Status: Active
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Diagnose the behavior in three bounded layers before changing production code:

1. Verify the semantic report and existing selector state machine with focused tests.
2. Run a synthetic candidate through a real supported PTY wrapper, preserving the raw byte transcript and the human-visible final state.
3. Run the Docker policy-learning scenario once the independent cluster-start blocker is resolved or reproduced in a clean supported runtime.

Only after those layers agree should the smallest fix be chosen. The preferred
fix stays in the CLI/terminal presentation boundary, keeps the candidate ID
opaque, preserves the JSON/redirected read-only path, and adds a regression
test at the layer where the failure was observed.

## Alternatives considered

### Alternative A: Change cancellation output immediately

Not chosen yet. The current final `Permission review canceled` may be correct
for an actual user cancellation but misleading when the initial screen never
appears. Changing it before reproducing could hide the real input or PTY bug.

### Alternative B: Remove raw mode and use only line input

Not chosen. This would alter the intended human interaction and terminal
contract to work around an unverified environment issue. Line fallback remains
valuable, but raw mode should be repaired or deliberately retired with an
explicit compatibility decision and retirement evidence.

### Alternative C: Add a second command that acts from JSON output

Rejected. It would duplicate the catalog workflow and weaken the discovery/
action separation rather than repairing the Permission Inbox.

## Design

### Public contract

The command remains `policy review [--tail N] [--format text|json]` with the
existing discover role. Text on an interactive TTY may confirm one exact
candidate; redirected text and JSON remain read-only. The child packet may
refine human wording or final-state visibility, but it must not change the
schema, bounded queue scope, action commands, or opaque-reference kind without
promoting the decision to the governing contracts.

### Layer changes

- Domain: no change unless the observed behavior exposes a missing semantic state such as empty versus canceled.
- Application: preserve the typed `PolicyCandidateReport` and terminal gating; add only task-owned behavior if the report/interaction boundary is incomplete.
- Infrastructure: use the existing terminal capability and platform PTY wrapper; do not add a general terminal dependency without an ADR and dependency review.
- CLI: own the selector rendering, input state machine, terminal cleanup, and final human presentation.
- Harness: add a regression test that fails under the observed PTY condition and keeps JSON/redirected behavior edge-equivalent.

### Data and control flow

```text
Gateway denial evidence
  -> bounded validated PolicyCandidateReport
  -> TTY gate
  -> raw or line selector
  -> one unchanged opaque candidate ID
  -> preflighted policy allow/deny action
  -> refreshed read-only candidate report
```

No display label, list index, command string, or provider-specific interpretation
may substitute for the opaque candidate ID.

### Error and cancellation behavior

Invalid reports, unsupported terminal mode, stale selection, EOF, and explicit
cancel must not mutate policy. Raw terminal state must be restored on every
return path. A mutation result is emitted only after the existing action has
confirmed its outcome; late cancellation cannot turn a confirmed action into a
replay permission. The independent cluster-start failure remains a separate
runtime fault and recovery path.

### Resolution of the observed race

Unix raw mode polls with `VMIN=0/VTIME=1`. On the affected PTY path, Go surfaced
the zero-byte poll as `io.EOF`, and the selector interpreted it as cancellation.
The shared raw-byte reader now treats EOF from a character device as a polling
timeout; ordinary redirected readers retain EOF cancellation semantics. The
integration helper uses a standard-library Python PTY bridge so stdin remains
attached to the child PTY until the process exits.

### Security and public boundary

Use synthetic hosts, paths, IDs, timestamps, and reasons in tests and docs.
Preserve secret/body redaction and visible external-text projection. Do not
copy user shell history or local absolute paths into committed fixtures.

## Implementation slices

1. Capture the supported-PTY transcript and classify the failure layer.
2. Add or refine the smallest failing selector/CLI regression test.
3. Implement the minimal terminal/presentation fix, if needed.
4. Re-run focused tests and the full policy-learning integration scenario.
5. Update agent-readiness evidence and public docs only if the visible contract changes.

## Verification

- Unit and contract tests: selector raw/line state machine, CLI orchestration, catalog interactive metadata, and application terminal gating.
- Negative side-effect tests: cancellation, EOF, invalid index, stale ID, wrong-kind ID, and output-write failure before policy mutation.
- Opaque-reference tests: unchanged candidate ID into allow/deny and refreshed queue after one action.
- Hostile-output tests: controls, Unicode separators, backslashes, and printable prompt-like fields remain projected safely.
- PTY observation: macOS `script` wrapper and the supported Linux equivalent, with raw transcript and final visible output recorded separately.
- Integration: `task integration:test` or the focused policy-learning scenario after cluster startup is healthy.
- Required profiles: `task check`, `task public:check`, `task policy:test`, `task gateway:test`, and `task integration:test` as applicable.
- Agent readiness: update the denial-to-retry transcript only after the human path is verified.

## Rollout and rollback

No state migration is expected. A CLI presentation fix rolls back with the
previous binary. Policy data and opaque candidate semantics must remain
compatible; a failed policy action keeps the prior policy authoritative.

## Documentation promotion

Promote only a durable change in cancellation/empty-state semantics, terminal
support, or agent-readiness expectations into the product, architecture,
harness, or readiness documents. Keep raw PTY diagnostics in this temporary
packet until handoff, then remove them with the packet.

## Current-main verification snapshot

- `main` is `ed37f805a4e2876f93c6ad86fb70beb40b6fc073`, containing the first-wave
  merge `966dd08841a7ccd88212dd9c8683562c99e17aa9` and the scoped policy-review
  implementation commit `7d096bb5749e3ad8afd6d85c88af301f5dda113f`.
- The focused real-PTY/read-only E2E passes all five subcases. `task check` and
  `task public:check` pass on the current documentation edit.
- `task integration:test` remains unresolved: its preflight finds an already
  running `tobari-gateway` and exits 1; the active cluster is intentionally not
  stopped. Current-worktree `task security` is separately blocked by an
  out-of-scope architecture-publication link, while the isolated HEAD-plus-
  allowed-packet snapshot passes security.
