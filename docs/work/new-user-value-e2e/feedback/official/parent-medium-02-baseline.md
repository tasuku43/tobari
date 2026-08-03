# Parent baseline: bootstrap, reuse, cancellation, and cleanup

- Status: parent baseline completed; this is an entry criterion and not one of
  the four blind child-agent acceptance records.
- Subject: a fresh disposable `cc-bash-guard` snapshot with isolated Tobari
  state and synthetic-only test data.
- Interaction: every product interaction used a real pseudo-TTY. The raw bytes
  remain in the parent execution log rather than Git; this is a redacted
  readable projection. No repository source or harness was changed.

## Result

The lifecycle journey is functionally available:

1. Running the primary `tobari` entry before setup failed closed with
   `cluster_not_configured` and the visible next action `tobari cluster up`.
2. `doctor --root .` identified the Docker/Compose environment and a valid
   bind-sharing path. `cluster up` brought Gateway and OPA to healthy state and
   displayed the next entry action.
3. Running the primary entry from the project root opened a Workspace shell.
   `pwd` showed the mirrored project path and a harmless marker command produced
   the expected output. `exit` returned a clear summary: `Workspace remains
   available`, `Resume: tobari`, and `Remove: tobari delete`.
4. Running the primary entry again from the same root automatically reused the
   existing Workspace and returned to the same mirrored path without another
   creation decision.
5. Running the entry from a nested synthetic directory opened a selector with
   the existing project marked `ready · nearest ancestor` and a separate
   `Create a new Workspace here` option. Pressing `q` returned
   `operation_canceled` with a retry cue and did not create another Workspace;
   supported `list` still showed exactly one.
6. `tobari delete` removed the project Workspace without an additional
   confirmation prompt. `cluster down --purge` was a safe no-op because the
   shared cluster was already unconfigured. `status` showed `No Tobari for this
   directory`, and no owned containers remained.

## Readable checkpoints

| Checkpoint | Visible result | Interpretation |
|---|---|---|
| Premature entry | `cluster_not_configured`; `Next: tobari cluster up` | The first recovery boundary is explicit. |
| Bootstrap | `Cluster ready`; Gateway/OPA healthy | Shared prerequisites are understandable. |
| First entry | Workspace shell; mirrored `pwd` | The isolated value is visible. |
| Exit | `Workspace remains available`; `Resume`; `Remove` | Reuse versus removal is explained at the boundary. |
| Re-entry | Same shell/path without creation prompt | Current-root reuse is simple. |
| Nested selection | Existing ancestor marked ready; `Create ... here`; `q/Esc` | The user can choose or cancel safely. |
| Cancel | `operation_canceled`; retry cue | Cancellation does not create state, though it is styled as a failure. |
| Cleanup | `Tobari deleted`; `No Tobari`; `Cluster not configured` | Project and shared state are gone. |

## Friction and follow-up candidates

### Cancellation is semantically safe but presented as failure

The nested selector visibly explained `q/Esc to cancel`, and cancellation left
the existing Workspace unchanged. The result nevertheless used the red
`Command failed` presentation and a retryable fault code
`operation_canceled`.

Candidate disposition: **narrow or clarify the presentation contract**. Keep
the cancellation semantics and retry cue, but distinguish an intentional user
cancel from an operational failure in the human renderer if the product
contract permits it. Do not treat this as a missing lifecycle capability.

### Root reuse is simple; nested scope is the discovery surface

The same-root entry has no selector because the current directory resolves to a
single CWD-owned Workspace. The nested selector exposes both the nearest
ancestor and a new local Workspace, which makes the ownership decision visible
without JSON or source inspection.

Candidate disposition: **keep** the fixed current-root entry and the explicit
nested selector. A convenience command that merges these states is not
evidenced as necessary.

### Cleanup boundaries are separate and safe

The exit summary differentiates reusable Workspace state from destructive
delete, while `cluster down --purge` owns shared state. After deletion, the
cluster command is idempotent and reports `Cluster not configured`.

Candidate disposition: **keep** the separate project and cluster boundaries;
use docs to state that `cluster down --purge` may be an explicit no-op after the
last project is deleted. A combined cleanup command remains only a candidate,
not an approved catalog change. No deprecation is proven.

## Parent conclusion

The lifecycle path is sufficiently proven for a blind child run. The child
should receive only the desired outcome of recovery, reuse, nested cancellation,
and cleanup, and should discover the selector and command ownership from the
visible product surface.
