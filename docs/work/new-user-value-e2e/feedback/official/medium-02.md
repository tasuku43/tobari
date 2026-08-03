# Medium-02: Workspace recovery, reuse, cancellation, and cleanup

- Status: functional E2E complete; raw transcript digest and per-key timing
  remain packet-level evidence follow-ups.
- Subject: fresh disposable `cc-bash-guard` snapshot with an isolated Tobari
  state and a real OS pseudo-TTY.
- Delegation: the child received only the desired outcome and sandbox
  boundary. It did not read repository documentation, source, work packets, or
  harness files, and it made no repository changes or commit.

## Value journey

1. The child tried the primary entry before shared preparation. The visible
   failure was `cluster_not_configured`, with the next action
   `tobari cluster up`.
2. The child followed that visible recovery, reached `Cluster ready` with
   healthy Gateway/OPA services, and entered a Workspace from the same
   project. `pwd` showed the mirrored project path.
3. After `exit`, the child re-entered through the same project and observed the
   same Workspace being reused.
4. The child created a temporary nested location and invoked the same entry
   there. The selector clearly showed the existing Workspace and
   `Create a new Workspace here`; the child pressed `q` to cancel without
   creating another Workspace.
5. Reopening the state showed one ready nearest-ancestor Workspace with no
   change from the cancellation.
6. The child deleted the Workspace, found and stopped the shared environment,
   and verified `No Tobari`, `No Workspaces`, and `Cluster not configured`.

## Discovery and evidence

The child discovered the recovery command from the first visible failure and
found the shared-environment cleanup command through the product/help surface.
It did not use JSON, a non-interactive replay, or direct Docker/OPA commands.
The child reported one unrelated shell-startup warning about a missing
kubectl configuration file; it did not affect the Tobari journey and is
classified as environment noise.

The run used a real pseudo-TTY at 120x40 with `TERM=xterm-256color`. The child
did not return a raw-byte digest or timestamped key transcript, so the
functional result is complete but the packet-wide raw-PTY predicate remains
open.

## Readable checkpoints

| Checkpoint | Visible result | Interpretation |
|---|---|---|
| Premature entry | `cluster_not_configured`; `Next: tobari cluster up` | Recovery ownership is visible. |
| Bootstrap | `Cluster ready`; Gateway/OPA healthy | Shared prerequisites are reachable. |
| Workspace entry | Mirrored project path from `pwd` | The isolation value is concrete. |
| Re-entry | Same Workspace reused | Exit preserves deliberate reusable state. |
| Nested selector | Existing Workspace plus `Create a new Workspace here` | Scope and creation choice are visible. |
| Cancel | `q`; one ready ancestor remains | Cancel is safe and non-mutating. |
| Cleanup | `No Tobari`; `No Workspaces`; `Cluster not configured` | No project or shared state remains. |

## Command-surface candidates

- **Keep:** the visible `cluster up` recovery cue, the nested selector's
  existing-versus-create distinction, and the separate `delete`/cluster
  cleanup boundaries. This run found them discoverable enough for the target
  outcome.
- **Docs-only:** explain that a shell-startup warning unrelated to Tobari is
  not a product failure when it appears in a disposable runtime, and state the
  ownership difference between Workspace deletion and shared-environment
  shutdown.
- **Narrow candidate:** none evidenced by the successful blind journey. A
  combined cleanup command is not justified merely because both cleanup steps
  occur in one scenario.
- **Deprecate candidate:** none.

## Acceptance boundary

The child independently recovered from a premature entry, reached an isolated
Workspace, reused it, canceled a nested selection without mutation, and
completed bounded cleanup through a real TTY. The scenario is functionally
complete. Raw PTY digest/timing evidence remains a packet-wide verification
task, not a failure of this journey.
